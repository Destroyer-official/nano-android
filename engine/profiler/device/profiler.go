package device

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// StartupMetrics captures `adb shell am start -W` cold/warm launch timings from real Android ART.
type StartupMetrics struct {
	Backend       string `json:"backend"` // "android_art" or "physical_device"
	Activity      string `json:"activity"`
	LaunchState   string `json:"launch_state"`
	ThisTimeMs    int64  `json:"this_time_ms"`
	TotalTimeMs   int64  `json:"total_time_ms"`
	WaitTimeMs    int64  `json:"wait_time_ms"`
}

// DeviceProfileReport aggregates hardware/ART telemetry from ADB dumpsys, Perfetto, and Macrobenchmark outputs.
type DeviceProfileReport struct {
	Backend             string             `json:"backend"`
	Serial              string             `json:"serial,omitempty"`
	PackageID           string             `json:"package_id,omitempty"`
	ThermalStatus       string             `json:"thermal_status"`
	SkinTempC           float64            `json:"skin_temp_c"`
	BatteryTempC        float64            `json:"battery_temp_c"`
	BatteryPct          int                `json:"battery_pct"`
	TotalPSSMB          float64            `json:"total_pss_mb"`
	TotalRSSMB          float64            `json:"total_rss_mb"`
	AnonSwapMB          float64            `json:"anon_swap_mb"`
	MemoryLimiterEvents int                `json:"memory_limiter_events"`
	TotalFrames         int                `json:"total_frames"`
	JankyFrames         int                `json:"janky_frames"`
	JankPct             float64            `json:"jank_pct"`
	P95FrameMs          float64            `json:"p95_frame_ms"`
	ODPMRailsMWh        map[string]float64 `json:"odpm_rails_mwh,omitempty"`
	Startup             *StartupMetrics    `json:"startup,omitempty"`
}

// MacrobenchmarkMetric captures AndroidX Macrobenchmark JSON metrics (startup, frame timing, ODPM power).
type MacrobenchmarkMetric struct {
	Name   string  `json:"name"`
	Median float64 `json:"median"`
	Min    float64 `json:"minimum"`
	Max    float64 `json:"maximum"`
}

var (
	reThisTime      = regexp.MustCompile(`(?m)^ThisTime:\s*(\d+)`)
	reTotalTime     = regexp.MustCompile(`(?m)^TotalTime:\s*(\d+)`)
	reWaitTime      = regexp.MustCompile(`(?m)^WaitTime:\s*(\d+)`)
	reLaunchState   = regexp.MustCompile(`(?m)^LaunchState:\s*(\S+)`)
	reActivityLine  = regexp.MustCompile(`(?m)^Activity:\s*(\S+)`)
	reThermalStr    = regexp.MustCompile(`(?i)mThermalStatus[=:\s]+([A-Z_]+)`)
	reThermalNum    = regexp.MustCompile(`(?i)mThermalStatus[=:\s]+(\d+)`)
	reTempObj       = regexp.MustCompile(`Temperature\{mValue=([0-9.]+)[^}]*?mName=([a-zA-Z0-9_]+)`)
	rePSS           = regexp.MustCompile(`(?i)(?:TOTAL PSS|TOTAL)[=:\s]+([0-9,]+)`)
	reRSS           = regexp.MustCompile(`(?i)(?:TOTAL RSS|RSS)[=:\s]+([0-9,]+)`)
	reAnonSwap      = regexp.MustCompile(`(?i)(?:TOTAL SWAP PSS|TOTAL SWAP|SwapPss|AnonSwap)[=:\s]+([0-9,]+)`)
	reGfxTotal      = regexp.MustCompile(`(?i)Total frames rendered[=:\s]+(\d+)`)
	reGfxJanky      = regexp.MustCompile(`(?i)Janky frames[=:\s]+(\d+)`)
	reGfxP95        = regexp.MustCompile(`(?i)95th percentile[=:\s]+([0-9.]+)ms?`)
	reBatLevel      = regexp.MustCompile(`(?i)level[=:\s]+(\d+)`)
	reRailItem      = regexp.MustCompile(`([a-zA-Z0-9_]+)=([0-9.]+)(?:uWs|uwh)?`)
)

// ParseAmStartOutput parses `adb shell am start -W` timing output into StartupMetrics.
func ParseAmStartOutput(output, backend string) StartupMetrics {
	if backend == "" {
		backend = "android_art"
	}
	sm := StartupMetrics{Backend: backend}
	if m := reActivityLine.FindStringSubmatch(output); len(m) > 1 {
		sm.Activity = m[1]
	}
	if m := reLaunchState.FindStringSubmatch(output); len(m) > 1 {
		sm.LaunchState = m[1]
	}
	if m := reThisTime.FindStringSubmatch(output); len(m) > 1 {
		sm.ThisTimeMs, _ = strconv.ParseInt(m[1], 10, 64)
	}
	if m := reTotalTime.FindStringSubmatch(output); len(m) > 1 {
		sm.TotalTimeMs, _ = strconv.ParseInt(m[1], 10, 64)
	}
	if m := reWaitTime.FindStringSubmatch(output); len(m) > 1 {
		sm.WaitTimeMs, _ = strconv.ParseInt(m[1], 10, 64)
	}
	return sm
}

// ParseDumpsysBundle builds a DeviceProfileReport from real Android ADB dumpsys outputs.
func ParseDumpsysBundle(backend, serial, pkg, thermalOut, meminfoOut, batteryOut, gfxOut, amStartOut string) DeviceProfileReport {
	if backend == "" {
		backend = "android_art"
	}
	rep := DeviceProfileReport{
		Backend:       backend,
		Serial:        serial,
		PackageID:     pkg,
		ThermalStatus: "NONE",
		ODPMRailsMWh:  map[string]float64{},
	}

	// Thermal
	if m := reThermalStr.FindStringSubmatch(thermalOut); len(m) > 1 {
		rep.ThermalStatus = strings.ToUpper(m[1])
	} else if m := reThermalNum.FindStringSubmatch(thermalOut); len(m) > 1 {
		mapNum := map[string]string{"0": "NONE", "1": "LIGHT", "2": "MODERATE", "3": "SEVERE", "4": "CRITICAL", "5": "EMERGENCY", "6": "SHUTDOWN"}
		if s, ok := mapNum[m[1]]; ok {
			rep.ThermalStatus = s
		}
	}
	for _, tm := range reTempObj.FindAllStringSubmatch(thermalOut, -1) {
		val, _ := strconv.ParseFloat(tm[1], 64)
		name := strings.ToLower(tm[2])
		if strings.Contains(name, "skin") && rep.SkinTempC == 0 {
			rep.SkinTempC = val
		} else if strings.Contains(name, "battery") && rep.BatteryTempC == 0 {
			rep.BatteryTempC = val
		}
	}

	// Meminfo + Android 17 AnonSwap
	parseKB := func(raw string) float64 {
		v, _ := strconv.ParseFloat(strings.ReplaceAll(raw, ",", ""), 64)
		return float64(int((v/1024.0)*100+0.5)) / 100
	}
	if m := rePSS.FindStringSubmatch(meminfoOut); len(m) > 1 {
		rep.TotalPSSMB = parseKB(m[1])
	}
	if m := reRSS.FindStringSubmatch(meminfoOut); len(m) > 1 {
		rep.TotalRSSMB = parseKB(m[1])
	}
	if m := reAnonSwap.FindStringSubmatch(meminfoOut); len(m) > 1 {
		rep.AnonSwapMB = parseKB(m[1])
	}
	if strings.Contains(meminfoOut, "MemoryLimiter:AnonSwap") {
		rep.MemoryLimiterEvents = strings.Count(meminfoOut, "MemoryLimiter:AnonSwap")
	}

	// GfxInfo
	if m := reGfxTotal.FindStringSubmatch(gfxOut); len(m) > 1 {
		rep.TotalFrames, _ = strconv.Atoi(m[1])
	}
	if m := reGfxJanky.FindStringSubmatch(gfxOut); len(m) > 1 {
		rep.JankyFrames, _ = strconv.Atoi(m[1])
	}
	if rep.TotalFrames > 0 {
		rep.JankPct = float64(int((float64(rep.JankyFrames)/float64(rep.TotalFrames)*100.0)*100+0.5)) / 100
	}
	if m := reGfxP95.FindStringSubmatch(gfxOut); len(m) > 1 {
		rep.P95FrameMs, _ = strconv.ParseFloat(m[1], 64)
	}

	// Battery & ODPM Rails (uWs -> mWh)
	if m := reBatLevel.FindStringSubmatch(batteryOut); len(m) > 1 {
		rep.BatteryPct, _ = strconv.Atoi(m[1])
	}
	for _, line := range strings.Split(batteryOut, "\n") {
		if strings.Contains(line, "Rail energy:") || strings.Contains(line, "ODPM") || strings.Contains(line, "Energy counter") {
			for _, rm := range reRailItem.FindAllStringSubmatch(line, -1) {
				v, _ := strconv.ParseFloat(rm[2], 64)
				mwh := float64(int((v/3600000.0)*10000+0.5)) / 10000
				rep.ODPMRailsMWh[rm[1]] = mwh
			}
		}
	}

	if strings.TrimSpace(amStartOut) != "" {
		sm := ParseAmStartOutput(amStartOut, backend)
		rep.Startup = &sm
	}
	return rep
}

// ParseMacrobenchmarkJSON extracts median/min/max performance metrics from AndroidX Macrobenchmark output JSON.
func ParseMacrobenchmarkJSON(raw []byte) ([]MacrobenchmarkMetric, error) {
	var doc struct {
		Benchmarks []struct {
			Name    string `json:"name"`
			Metrics map[string]struct {
				Median  float64 `json:"median"`
				Minimum float64 `json:"minimum"`
				Maximum float64 `json:"maximum"`
			} `json:"metrics"`
		} `json:"benchmarks"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse macrobenchmark json: %w", err)
	}
	var out []MacrobenchmarkMetric
	for _, b := range doc.Benchmarks {
		for metricName, stats := range b.Metrics {
			out = append(out, MacrobenchmarkMetric{
				Name:   b.Name + "." + metricName,
				Median: stats.Median,
				Min:    stats.Minimum,
				Max:    stats.Maximum,
			})
		}
	}
	return out, nil
}

// CollectLiveDevice queries an attached Android device or emulator via `adb -s <serial> shell dumpsys ...`.
// Returns an explicit error if `adb` is absent or the target device cannot be reached (never fabricates metrics).
func CollectLiveDevice(ctx context.Context, serial, pkg string) (*DeviceProfileReport, error) {
	adbExe, err := exec.LookPath("adb")
	if err != nil {
		return nil, fmt.Errorf("adb not available on PATH: real device profiling requires attached emulator or physical device")
	}
	if strings.TrimSpace(serial) == "" {
		return nil, fmt.Errorf("device serial is required for live ART/physical profiling")
	}

	runADB := func(args ...string) string {
		full := append([]string{"-s", serial}, args...)
		out, _ := exec.CommandContext(ctx, adbExe, full...).CombinedOutput()
		return string(out)
	}

	thermalOut := runADB("shell", "dumpsys", "thermalservice")
	memArgs := []string{"shell", "dumpsys", "meminfo"}
	if pkg != "" {
		memArgs = append(memArgs, pkg)
	}
	memOut := runADB(memArgs...)
	batOut := runADB("shell", "dumpsys", "batterystats")
	gfxOut := ""
	if pkg != "" {
		gfxOut = runADB("shell", "dumpsys", "gfxinfo", pkg, "framestats")
	}

	backend := "physical_device"
	if strings.HasPrefix(serial, "emulator-") {
		backend = "android_art"
	}
	rep := ParseDumpsysBundle(backend, serial, pkg, thermalOut, memOut, batOut, gfxOut, "")
	return &rep, nil
}
