//go:build windows

package worker_isolation

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"
)

const (
	jobObjectExtendedLimitInformationClass = 9
	jobObjectCpuRateControlInformation     = 15

	jobObjectLimitActiveProcess          = 0x00000008
	jobObjectLimitDieOnUnhandledException = 0x00000400
	jobObjectLimitKillOnJobClose         = 0x00002000
	jobObjectLimitJobMemory              = 0x00000200

	jobObjectCpuRateControlEnable  = 0x00000001
	jobObjectCpuRateControlHardCap = 0x00000004

	createSuspended = 0x00000004
	createNoWindow  = 0x08000000

	processAllAccess = 0x1F0FFF
	th32csSnapThread = 0x00000004
	threadSuspendResume = 0x0002
)

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type jobObjectExtendedLimitInformation struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

type jobObjectBasicAccountingInformation struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

type jobObjectBasicAndIoAccountingInformation struct {
	BasicInfo jobObjectBasicAccountingInformation
	IoInfo    ioCounters
}

type jobObjectCpuRateControlInfo struct {
	ControlFlags uint32
	CpuRate      uint32 // in 1/100th of 1% (e.g., 80% = 8000)
}

type threadEntry32 struct {
	Size           uint32
	Usage          uint32
	ThreadID       uint32
	OwnerProcessID uint32
	BasePri        int32
	DeltaPri       int32
	Flags          uint32
}

var (
	modKernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW         = modKernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject  = modKernel32.NewProc("SetInformationJobObject")
	procQueryInformationJobObj   = modKernel32.NewProc("QueryInformationJobObject")
	procAssignProcessToJobObject = modKernel32.NewProc("AssignProcessToJobObject")
	procTerminateJobObject       = modKernel32.NewProc("TerminateJobObject")
	procOpenProcess              = modKernel32.NewProc("OpenProcess")
	procCreateToolhelp32Snapshot = modKernel32.NewProc("CreateToolhelp32Snapshot")
	procThread32First            = modKernel32.NewProc("Thread32First")
	procThread32Next             = modKernel32.NewProc("Thread32Next")
	procOpenThread               = modKernel32.NewProc("OpenThread")
	procResumeThread             = modKernel32.NewProc("ResumeThread")
)

func startPlatformIsolated(cmd *exec.Cmd, workerID string, ws *WorkerWorkspace, limits Limits) (*IsolatedHandle, error) {
	if limits.MaxMemoryMB == 0 {
		limits.MaxMemoryMB = 1024
	}
	if limits.MaxActiveProcesses == 0 {
		limits.MaxActiveProcesses = 2
	}

	// Create anonymous Job Object before starting process.
	jobHandleRaw, _, err := procCreateJobObjectW.Call(0, 0)
	if jobHandleRaw == 0 {
		return nil, fmt.Errorf("CreateJobObjectW failed: %w", err)
	}
	jobHandle := syscall.Handle(jobHandleRaw)

	var ext jobObjectExtendedLimitInformation
	ext.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose |
		jobObjectLimitActiveProcess |
		jobObjectLimitJobMemory |
		jobObjectLimitDieOnUnhandledException
	ext.BasicLimitInformation.ActiveProcessLimit = limits.MaxActiveProcesses
	ext.JobMemoryLimit = uintptr(limits.MaxMemoryMB * 1024 * 1024)

	r1, _, err := procSetInformationJobObject.Call(
		uintptr(jobHandle),
		uintptr(jobObjectExtendedLimitInformationClass),
		uintptr(unsafe.Pointer(&ext)),
		unsafe.Sizeof(ext),
	)
	if r1 == 0 {
		_ = syscall.CloseHandle(jobHandle)
		return nil, fmt.Errorf("SetInformationJobObject(ExtendedLimit) failed: %w", err)
	}

	if limits.CPURatePercent > 0 && limits.CPURatePercent <= 100 {
		cpuInfo := jobObjectCpuRateControlInfo{
			ControlFlags: jobObjectCpuRateControlEnable | jobObjectCpuRateControlHardCap,
			CpuRate:      limits.CPURatePercent * 100,
		}
		rCpu, _, errCpu := procSetInformationJobObject.Call(
			uintptr(jobHandle),
			uintptr(jobObjectCpuRateControlInformation),
			uintptr(unsafe.Pointer(&cpuInfo)),
			unsafe.Sizeof(cpuInfo),
		)
		if rCpu == 0 {
			_ = syscall.CloseHandle(jobHandle)
			return nil, fmt.Errorf("SetInformationJobObject(CpuRate) failed: %w", errCpu)
		}
	}

	// Start child suspended so it enters the Job Object before executing a single instruction.
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createSuspended | createNoWindow
	cmd.SysProcAttr.HideWindow = true

	if err := cmd.Start(); err != nil {
		_ = syscall.CloseHandle(jobHandle)
		return nil, fmt.Errorf("start suspended worker: %w", err)
	}

	pid := uint32(cmd.Process.Pid)
	procHandleRaw, _, openErr := procOpenProcess.Call(uintptr(processAllAccess), 0, uintptr(pid))
	if procHandleRaw == 0 {
		_ = cmd.Process.Kill()
		_ = syscall.CloseHandle(jobHandle)
		return nil, fmt.Errorf("OpenProcess(%d) failed: %w", pid, openErr)
	}
	procHandle := syscall.Handle(procHandleRaw)

	rAssign, _, assignErr := procAssignProcessToJobObject.Call(uintptr(jobHandle), uintptr(procHandle))
	_ = syscall.CloseHandle(procHandle)
	if rAssign == 0 {
		_ = cmd.Process.Kill()
		_ = syscall.CloseHandle(jobHandle)
		return nil, fmt.Errorf("AssignProcessToJobObject(%d) failed: %w", pid, assignErr)
	}

	// Resume main thread now that OS job containment is active.
	if err := resumeProcessThreads(pid); err != nil {
		_, _, _ = procTerminateJobObject.Call(uintptr(jobHandle), 1)
		_ = syscall.CloseHandle(jobHandle)
		return nil, fmt.Errorf("resume worker threads: %w", err)
	}

	handle := &IsolatedHandle{
		WorkerID: workerID,
		Backend:  "windows_job_object",
		PID:      int(pid),
		closeFn: func() error {
			_, _, _ = procTerminateJobObject.Call(uintptr(jobHandle), 1)
			return syscall.CloseHandle(jobHandle)
		},
		statsFn: func() (WorkerStats, error) {
			var acct jobObjectBasicAndIoAccountingInformation
			rQ, _, qErr := procQueryInformationJobObj.Call(
				uintptr(jobHandle),
				8, // JobObjectBasicAndIoAccountingInformation
				uintptr(unsafe.Pointer(&acct)),
				unsafe.Sizeof(acct),
				0,
			)
			if rQ == 0 {
				return WorkerStats{}, fmt.Errorf("QueryInformationJobObject(acct) failed: %w", qErr)
			}
			var extOut jobObjectExtendedLimitInformation
			_, _, _ = procQueryInformationJobObj.Call(
				uintptr(jobHandle),
				uintptr(jobObjectExtendedLimitInformationClass),
				uintptr(unsafe.Pointer(&extOut)),
				unsafe.Sizeof(extOut),
				0,
			)
			return WorkerStats{
				WorkerID:        workerID,
				Backend:         "windows_job_object",
				PID:             int(pid),
				ActiveProcesses: acct.BasicInfo.ActiveProcesses,
				PeakMemoryBytes: uint64(extOut.PeakJobMemoryUsed),
			}, nil
		},
	}

	return handle, nil
}

func resumeProcessThreads(pid uint32) error {
	snapRaw, _, err := procCreateToolhelp32Snapshot.Call(uintptr(th32csSnapThread), 0)
	if snapRaw == uintptr(^uintptr(0)) || snapRaw == 0 {
		return fmt.Errorf("CreateToolhelp32Snapshot failed: %w", err)
	}
	snap := syscall.Handle(snapRaw)
	defer syscall.CloseHandle(snap)

	var te threadEntry32
	te.Size = uint32(unsafe.Sizeof(te))
	r, _, _ := procThread32First.Call(uintptr(snap), uintptr(unsafe.Pointer(&te)))
	resumed := 0
	for r != 0 {
		if te.OwnerProcessID == pid {
			thRaw, _, _ := procOpenThread.Call(uintptr(threadSuspendResume), 0, uintptr(te.ThreadID))
			if thRaw != 0 {
				_, _, _ = procResumeThread.Call(thRaw)
				_ = syscall.CloseHandle(syscall.Handle(thRaw))
				resumed++
			}
		}
		te.Size = uint32(unsafe.Sizeof(te))
		r, _, _ = procThread32Next.Call(uintptr(snap), uintptr(unsafe.Pointer(&te)))
	}
	if resumed == 0 {
		return fmt.Errorf("no threads resumed for pid %d", pid)
	}
	return nil
}
