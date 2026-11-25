package profiler

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// PerfTraceRequest holds options for capturing a Perfetto performance trace.
type PerfTraceRequest struct {
	Serial      string   `json:"serial"`
	PackageID   string   `json:"package_id,omitempty"`
	DurationSec int      `json:"duration_sec,omitempty"` // default 10
	Categories  []string `json:"categories,omitempty"`   // sched, freq, mem, gfx, am, binder
	OutputPath  string   `json:"output_path,omitempty"`  // local destination for .perfetto-trace
}

// GeneratePerfettoConfig builds a textproto Perfetto configuration string.
func GeneratePerfettoConfig(categories []string, pkg string, durationSec int) string {
	if durationSec <= 0 {
		durationSec = 10
	}
	durationMS := durationSec * 1000

	if len(categories) == 0 {
		categories = []string{"sched", "freq", "mem", "gfx", "am", "binder"}
	}

	ftraceEvents := make([]string, 0)
	atraceCategories := make([]string, 0)

	for _, c := range categories {
		switch strings.ToLower(strings.TrimSpace(c)) {
		case "sched":
			ftraceEvents = append(ftraceEvents, `"sched/sched_switch"`, `"sched/sched_wakeup"`)
		case "freq":
			ftraceEvents = append(ftraceEvents, `"power/cpu_frequency"`, `"power/cpu_idle"`)
		case "mem":
			ftraceEvents = append(ftraceEvents, `"kmem/kmem_cache_alloc"`, `"kmem/kmem_cache_free"`, `"rss_stat"`)
		case "gfx":
			atraceCategories = append(atraceCategories, `"gfx"`, `"view"`, `"render_thread"`)
		case "am":
			atraceCategories = append(atraceCategories, `"am"`)
		case "binder":
			ftraceEvents = append(ftraceEvents, `"binder/binder_transaction"`, `"binder/binder_transaction_received"`)
			atraceCategories = append(atraceCategories, `"binder_driver"`)
		}
	}

	var sb strings.Builder
	sb.WriteString("buffers: {\n")
	sb.WriteString("  size_kb: 65536\n")
	sb.WriteString("  fill_policy: RING_BUFFER\n")
	sb.WriteString("}\n")
	sb.WriteString("data_sources: {\n")
	sb.WriteString("  config {\n")
	sb.WriteString("    name: \"linux.ftrace\"\n")
	sb.WriteString("    ftrace_config {\n")
	for _, ev := range ftraceEvents {
		sb.WriteString(fmt.Sprintf("      ftrace_events: %s\n", ev))
	}
	for _, cat := range atraceCategories {
		sb.WriteString(fmt.Sprintf("      atrace_categories: %s\n", cat))
	}
	if pkg != "" {
		sb.WriteString(fmt.Sprintf("      atrace_apps: \"%s\"\n", pkg))
	}
	sb.WriteString("    }\n")
	sb.WriteString("  }\n")
	sb.WriteString("}\n")
	sb.WriteString(fmt.Sprintf("duration_ms: %d\n", durationMS))

	return sb.String()
}

// CapturePerfettoTrace records a Perfetto system trace on Android and analyzes it with the Python sidecar.
func CapturePerfettoTrace(
	ctx context.Context,
	dm types.DeviceManager,
	runner SidecarRunner,
	req PerfTraceRequest,
) (*types.PerfTraceResult, error) {
	if dm == nil {
		return nil, fmt.Errorf("device manager is required")
	}
	if req.Serial == "" {
		return nil, fmt.Errorf("device serial is required")
	}
	if req.DurationSec <= 0 {
		req.DurationSec = 10
	}

	ts := time.Now().Unix()
	deviceTracePath := fmt.Sprintf("/data/local/tmp/df_%d.perfetto-trace", ts)
	deviceCfgPath := fmt.Sprintf("/data/local/tmp/df_cfg_%d.txt", ts)

	// 1. Generate and push config file
	cfgContent := GeneratePerfettoConfig(req.Categories, req.PackageID, req.DurationSec)
	tmpCfg, err := os.CreateTemp("", "df_perfetto_cfg_*.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp config: %w", err)
	}
	tmpCfgPath := tmpCfg.Name()
	if _, err := tmpCfg.WriteString(cfgContent); err != nil {
		tmpCfg.Close()
		os.Remove(tmpCfgPath)
		return nil, fmt.Errorf("failed to write temp config: %w", err)
	}
	tmpCfg.Close()
	defer os.Remove(tmpCfgPath)

	if _, err := dm.ExecADB(req.Serial, "push", tmpCfgPath, deviceCfgPath); err != nil {
		return nil, fmt.Errorf("failed to push perfetto config: %w", err)
	}
	defer func() {
		_, _ = dm.ExecADB(req.Serial, "shell", "rm", "-f", deviceCfgPath)
	}()

	// 2. Start trace capture on device
	_, err = dm.ExecADB(req.Serial, "shell", "perfetto", "--txt", "-c", deviceCfgPath, "-o", deviceTracePath)
	if err != nil {
		return nil, fmt.Errorf("failed to execute perfetto trace capture: %w", err)
	}
	defer func() {
		_, _ = dm.ExecADB(req.Serial, "shell", "rm", "-f", deviceTracePath)
	}()

	// 3. Pull trace to host
	hostTracePath := req.OutputPath
	if hostTracePath == "" {
		tmpTrace, err := os.CreateTemp("", fmt.Sprintf("df_%d_*.perfetto-trace", ts))
		if err != nil {
			return nil, fmt.Errorf("failed to create temp trace file: %w", err)
		}
		hostTracePath = tmpTrace.Name()
		tmpTrace.Close()
	}

	if _, err := dm.ExecADB(req.Serial, "pull", deviceTracePath, hostTracePath); err != nil {
		return nil, fmt.Errorf("failed to pull trace from device: %w", err)
	}

	// 4. Analyze trace via Python sidecar
	if runner == nil {
		runner = NewPythonSidecarRunner("")
	}

	res, err := runner.AnalyzeTrace(ctx, hostTracePath, req.PackageID, "")
	if err != nil {
		return nil, fmt.Errorf("perfetto trace analysis failed: %w", err)
	}
	res.TracePath = hostTracePath

	return res, nil
}
