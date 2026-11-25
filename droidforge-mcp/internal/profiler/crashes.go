package profiler

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

var (
	reFatalException = regexp.MustCompile(`FATAL EXCEPTION:\s*(.*)`)
	reExceptionClass = regexp.MustCompile(`Process:\s*([^\s,]+).*?PID:\s*(\d+)`)
	reStackTraceLine = regexp.MustCompile(`^\s+at\s+.*`)
	reCausedBy       = regexp.MustCompile(`^Caused by:\s*(.*)`)

	reANRLine   = regexp.MustCompile(`ANR in\s+([^\s(]+)`)
	reANRReason = regexp.MustCompile(`Reason:\s*(.*)`)

	reExitInfoBlock = regexp.MustCompile(`ApplicationExitInfo\(.*?\)`)
	reExitReason    = regexp.MustCompile(`reason=(\d+)\s*\(([^)]+)\)`)
	reExitSubreason = regexp.MustCompile(`subreason=(\d+)\s*\(([^)]+)\)`)
	reExitDesc      = regexp.MustCompile(`description=(.*?)(?:,\s*[a-zA-Z0-9_]+=|\))`)
	reExitTimestamp = regexp.MustCompile(`timestamp=(\d+)`)
)

// CrashCollectRequest holds parameters for collecting crash and ANR reports.
type CrashCollectRequest struct {
	Serial                  string `json:"serial"`
	PackageID               string `json:"package_id,omitempty"`
	IncludeTombstoneSummary bool   `json:"include_tombstone_summary,omitempty"`
}

// ParseLogcatCrashes extracts FATAL EXCEPTION and ANR entries from raw logcat text.
func ParseLogcatCrashes(logcatOutput, pkgFilter string) ([]types.CrashEntry, []types.CrashEntry) {
	crashes := make([]types.CrashEntry, 0)
	anrs := make([]types.CrashEntry, 0)

	lines := strings.Split(logcatOutput, "\n")
	var currentCrash *types.CrashEntry
	var currentANR *types.CrashEntry
	var crashLines []string
	var anrLines []string

	flushCrash := func() {
		if currentCrash != nil {
			currentCrash.StackTrace = strings.Join(crashLines, "\n")
			crashes = append(crashes, *currentCrash)
			currentCrash = nil
			crashLines = nil
		}
	}

	flushANR := func() {
		if currentANR != nil {
			currentANR.StackTrace = strings.Join(anrLines, "\n")
			anrs = append(anrs, *currentANR)
			currentANR = nil
			anrLines = nil
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")

		// 1. FATAL EXCEPTION detection
		if match := reFatalException.FindStringSubmatch(trimmed); len(match) > 1 {
			flushCrash()
			currentCrash = &types.CrashEntry{
				Exception:   "FATAL EXCEPTION",
				Message:     match[1],
				TimestampMS: time.Now().UnixMilli(),
			}
			crashLines = append(crashLines, trimmed)
			continue
		}

		if currentCrash != nil {
			isCrashTraceLine := strings.Contains(trimmed, "AndroidRuntime") ||
				strings.Contains(trimmed, "	at ") ||
				strings.Contains(trimmed, " at ") ||
				strings.Contains(trimmed, "Caused by:") ||
				strings.Contains(trimmed, "Exception:") ||
				strings.Contains(trimmed, "Error:") ||
				strings.Contains(trimmed, "Process:")

			if isCrashTraceLine {
				crashLines = append(crashLines, trimmed)
				if match := reCausedBy.FindStringSubmatch(trimmed); len(match) > 1 {
					currentCrash.Exception = match[1]
				} else if strings.Contains(trimmed, "Exception:") || strings.Contains(trimmed, "Error:") {
					if idx := strings.LastIndex(trimmed, ": "); idx != -1 {
						currentCrash.Message = strings.TrimSpace(trimmed[idx+2:])
					}
				}
				continue
			} else {
				flushCrash()
			}
		}

		// 2. ANR detection
		if match := reANRLine.FindStringSubmatch(trimmed); len(match) > 1 {
			targetPkg := match[1]
			if pkgFilter == "" || strings.Contains(targetPkg, pkgFilter) {
				flushANR()
				currentANR = &types.CrashEntry{
					Exception:   "ANR",
					Message:     fmt.Sprintf("ANR in %s", targetPkg),
					TimestampMS: time.Now().UnixMilli(),
				}
				anrLines = append(anrLines, trimmed)
				continue
			}
		}

		if currentANR != nil {
			isANRLine := strings.Contains(trimmed, "ActivityManager") ||
				strings.Contains(trimmed, "PID:") ||
				strings.Contains(trimmed, "Reason:") ||
				strings.Contains(trimmed, "CPU usage") ||
				strings.Contains(trimmed, "Load:") ||
				strings.Contains(trimmed, "	at ") ||
				strings.Contains(trimmed, " at ")

			if isANRLine {
				anrLines = append(anrLines, trimmed)
				if strings.Contains(trimmed, "Reason:") {
					parts := strings.SplitN(trimmed, "Reason:", 2)
					if len(parts) > 1 {
						currentANR.Message = strings.TrimSpace(parts[1])
					}
				}
				continue
			} else {
				flushANR()
			}
		}
	}

	flushCrash()
	flushANR()

	return crashes, anrs
}

// ParseExitInfo extracts structured ApplicationExitInfo from dumpsys activity exit-info output.
func ParseExitInfo(exitInfoOutput string) []types.ExitReason {
	exitReasons := make([]types.ExitReason, 0)
	blocks := strings.Split(exitInfoOutput, "ApplicationExitInfo")

	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}

		reasonStr := "UNKNOWN"
		if match := reExitReason.FindStringSubmatch(block); len(match) > 2 {
			reasonStr = match[2]
		}

		subreasonStr := ""
		if match := reExitSubreason.FindStringSubmatch(block); len(match) > 2 {
			subreasonStr = match[2]
		}

		description := ""
		if match := reExitDesc.FindStringSubmatch(block); len(match) > 1 {
			description = strings.TrimSpace(match[1])
		}

		var tsMS int64
		if match := reExitTimestamp.FindStringSubmatch(block); len(match) > 1 {
			tsVal, _ := strconv.ParseInt(match[1], 10, 64)
			tsMS = tsVal
		}
		if tsMS == 0 {
			tsMS = time.Now().UnixMilli()
		}

		// Specifically flag MemoryLimiter:AnonSwap if present in description or subreason
		if strings.Contains(description, "MemoryLimiter") || strings.Contains(description, "AnonSwap") || strings.Contains(subreasonStr, "AnonSwap") {
			reasonStr = "MemoryLimiter:AnonSwap"
		}

		fullDesc := description
		if subreasonStr != "" {
			if fullDesc != "" {
				fullDesc = fmt.Sprintf("[%s] %s", subreasonStr, fullDesc)
			} else {
				fullDesc = subreasonStr
			}
		}

		exitReasons = append(exitReasons, types.ExitReason{
			Reason:      reasonStr,
			Description: fullDesc,
			TimestampMS: tsMS,
		})
	}

	return exitReasons
}

// CollectCrashReport gathers recent crashes, ANRs, tombstones, and exit reasons into a structured report.
func CollectCrashReport(ctx context.Context, dm types.DeviceManager, req CrashCollectRequest) (*types.CrashReport, error) {
	if dm == nil {
		return nil, fmt.Errorf("device manager is required")
	}
	if req.Serial == "" {
		return nil, fmt.Errorf("device serial is required")
	}
	if req.PackageID != "" && !ValidatePackageID(req.PackageID) {
		return nil, fmt.Errorf("invalid package_id %q: must match ^[a-zA-Z0-9._]+$", req.PackageID)
	}

	report := &types.CrashReport{
		Nonce:       GenerateCryptoNonce(),
		Crashes:     make([]types.CrashEntry, 0),
		ANRs:        make([]types.CrashEntry, 0),
		ExitReasons: make([]types.ExitReason, 0),
	}

	// 1. Scan logcat for FATAL EXCEPTION and ANRs
	logcatArgs := []string{"logcat", "-d", "-v", "threadtime", "*:E"}
	logcatOut, err := dm.ExecADB(req.Serial, logcatArgs...)
	if err == nil && logcatOut != "" {
		crashes, anrs := ParseLogcatCrashes(logcatOut, req.PackageID)
		report.Crashes = append(report.Crashes, crashes...)
		report.ANRs = append(report.ANRs, anrs...)

		// Also check for MemoryLimiter in logcat
		if strings.Contains(logcatOut, "MemoryLimiter:AnonSwap") {
			report.ExitReasons = append(report.ExitReasons, types.ExitReason{
				Reason:      "MemoryLimiter:AnonSwap",
				Description: "Android 17 anonymous swap limiter event intercepted in logcat",
				TimestampMS: time.Now().UnixMilli(),
			})
		}
	}

	// 2. Scan dumpsys activity exit-info
	exitInfoArgs := []string{"shell", "dumpsys", "activity", "exit-info"}
	if req.PackageID != "" {
		exitInfoArgs = append(exitInfoArgs, req.PackageID)
	}
	exitOut, err := dm.ExecADB(req.Serial, exitInfoArgs...)
	if err == nil && exitOut != "" {
		exitReasons := ParseExitInfo(exitOut)
		report.ExitReasons = append(report.ExitReasons, exitReasons...)
	}

	// 3. Tombstones check if requested
	if req.IncludeTombstoneSummary {
		tombstonesOut, _ := dm.ExecADB(req.Serial, "shell", "ls", "-t", "/data/tombstones")
		tombstones := strings.Fields(tombstonesOut)
		if len(tombstones) > 0 {
			report.BundlePath = fmt.Sprintf("/data/tombstones/%s", tombstones[0])
		}
	}

	return report, nil
}
