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
	reThermalStatusNum = regexp.MustCompile(`(?i)mThermalStatus[=:\s]+(\d+)`)
	reThermalStatusStr = regexp.MustCompile(`(?i)mThermalStatus[=:\s]+([A-Z_]+)`)
	reSkinTemp         = regexp.MustCompile(`(?i)(?:skin|Skin)[^\n\r]*?[=:\s]+([0-9.]+)`)
	reBatteryTemp      = regexp.MustCompile(`(?i)(?:battery|Battery)[^\n\r]*?[=:\s]+([0-9.]+)`)
	reTempObject       = regexp.MustCompile(`Temperature\{mValue=([0-9.]+)[^}]*?mName=([a-zA-Z0-9_]+)`)

	reTotalPSS      = regexp.MustCompile(`(?i)(?:TOTAL PSS|TOTAL)[=:\s]+([0-9,]+)`)
	reTotalRSS      = regexp.MustCompile(`(?i)(?:TOTAL RSS|RSS)[=:\s]+([0-9,]+)`)
	reTotalAnonSwap = regexp.MustCompile(`(?i)(?:TOTAL SWAP PSS|TOTAL SWAP|SwapPss|AnonSwap)[=:\s]+([0-9,]+)`)

	reBatteryLevel   = regexp.MustCompile(`(?i)level[=:\s]+(\d+)`)
	reBatteryTempDbg = regexp.MustCompile(`(?i)temperature[=:\s]+(\d+)`)
	reChargeCounter  = regexp.MustCompile(`(?i)Charge counter[=:\s]+(-?\d+)`)

	reGfxTotalFrames = regexp.MustCompile(`(?i)Total frames rendered[=:\s]+(\d+)`)
	reGfxJankyFrames = regexp.MustCompile(`(?i)Janky frames[=:\s]+(\d+)`)
	reGfxP95FrameMS  = regexp.MustCompile(`(?i)95th percentile[=:\s]+([0-9.]+)ms?`)
)

// ParseThermalStatus maps raw status numbers or strings to canonical status strings.
func ParseThermalStatus(raw string) string {
	raw = strings.ToUpper(strings.TrimSpace(raw))
	switch raw {
	case "0", "NONE":
		return "NONE"
	case "1", "LIGHT":
		return "LIGHT"
	case "2", "MODERATE":
		return "MODERATE"
	case "3", "SEVERE":
		return "SEVERE"
	case "4", "CRITICAL":
		return "CRITICAL"
	case "5", "EMERGENCY":
		return "EMERGENCY"
	case "6", "SHUTDOWN":
		return "SHUTDOWN"
	default:
		if raw == "" {
			return "NONE"
		}
		return raw
	}
}

// ParseThermalService extracts thermal status and temperature sensor readings.
func ParseThermalService(output string) (status string, skinTemp, batteryTemp float64) {
	status = "NONE"

	// 1. Status
	if match := reThermalStatusStr.FindStringSubmatch(output); len(match) > 1 {
		status = ParseThermalStatus(match[1])
	} else if match := reThermalStatusNum.FindStringSubmatch(output); len(match) > 1 {
		status = ParseThermalStatus(match[1])
	}

	// 2. Structured Temperature objects (Android 10+)
	matches := reTempObject.FindAllStringSubmatch(output, -1)
	for _, m := range matches {
		if len(m) > 2 {
			val, _ := strconv.ParseFloat(m[1], 64)
			name := strings.ToLower(m[2])
			if strings.Contains(name, "skin") && skinTemp == 0 {
				skinTemp = val
			} else if strings.Contains(name, "battery") && batteryTemp == 0 {
				batteryTemp = val
			}
		}
	}

	// 3. Fallback regexes
	if skinTemp == 0 {
		if match := reSkinTemp.FindStringSubmatch(output); len(match) > 1 {
			skinTemp, _ = strconv.ParseFloat(match[1], 64)
		}
	}
	if batteryTemp == 0 {
		if match := reBatteryTemp.FindStringSubmatch(output); len(match) > 1 {
			batteryTemp, _ = strconv.ParseFloat(match[1], 64)
		}
	}

	return status, skinTemp, batteryTemp
}

// ParseMeminfo extracts Total PSS, Total RSS, and AnonSwap in MB from dumpsys meminfo output.
func ParseMeminfo(output string) (totalPSS, totalRSS, anonSwap float64) {
	cleanNum := func(s string) float64 {
		s = strings.ReplaceAll(s, ",", "")
		val, _ := strconv.ParseFloat(s, 64)
		return val / 1024.0 // KB to MB
	}

	if match := reTotalPSS.FindStringSubmatch(output); len(match) > 1 {
		totalPSS = cleanNum(match[1])
	}
	if match := reTotalRSS.FindStringSubmatch(output); len(match) > 1 {
		totalRSS = cleanNum(match[1])
	}
	if match := reTotalAnonSwap.FindStringSubmatch(output); len(match) > 1 {
		anonSwap = cleanNum(match[1])
	}

	return totalPSS, totalRSS, anonSwap
}

// ParseBatteryStats extracts charge level, temperature in Celsius, and charge counter in mAh.
func ParseBatteryStats(output string) (chargeMAh float64, batteryPct int, tempC float64) {
	if match := reBatteryLevel.FindStringSubmatch(output); len(match) > 1 {
		batteryPct, _ = strconv.Atoi(match[1])
	}
	if match := reBatteryTempDbg.FindStringSubmatch(output); len(match) > 1 {
		val, _ := strconv.ParseFloat(match[1], 64)
		// Usually reported in tenths of a degree Celsius (e.g. 312 -> 31.2°C)
		if val > 100 {
			tempC = val / 10.0
		} else {
			tempC = val
		}
	}
	if match := reChargeCounter.FindStringSubmatch(output); len(match) > 1 {
		val, _ := strconv.ParseFloat(match[1], 64)
		// Microampere-hours to milliampere-hours
		if val > 100000 || val < -100000 {
			chargeMAh = val / 1000.0
		} else {
			chargeMAh = val
		}
	}
	return chargeMAh, batteryPct, tempC
}

// ParseGfxInfo extracts frame metrics from dumpsys gfxinfo.
func ParseGfxInfo(output string) (totalFrames int, jankyFrames int, p95FrameMS float64) {
	if match := reGfxTotalFrames.FindStringSubmatch(output); len(match) > 1 {
		totalFrames, _ = strconv.Atoi(match[1])
	}
	if match := reGfxJankyFrames.FindStringSubmatch(output); len(match) > 1 {
		jankyFrames, _ = strconv.Atoi(match[1])
	}
	if match := reGfxP95FrameMS.FindStringSubmatch(output); len(match) > 1 {
		p95FrameMS, _ = strconv.ParseFloat(match[1], 64)
	}
	return totalFrames, jankyFrames, p95FrameMS
}

// ParseODPMRails extracts On-Device Power Monitor (ODPM) rail energy readings (e.g. Pixel 6+).
// Converts micro-Watt-seconds (uWs) to milli-Watt-hours (mWh).
func ParseODPMRails(output string) map[string]float64 {
	rails := make(map[string]float64)
	reRailItem := regexp.MustCompile(`([a-zA-Z0-9_]+)=([0-9.]+)(?:uWs|uwh)?`)

	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "Rail energy:") || strings.Contains(line, "ODPM") || strings.Contains(line, "Energy counter") {
			matches := reRailItem.FindAllStringSubmatch(line, -1)
			for _, m := range matches {
				if len(m) > 2 {
					name := m[1]
					val, _ := strconv.ParseFloat(m[2], 64)
					// Convert uWs to mWh: 1 mWh = 3,600,000 uWs
					mwh := val / 3600000.0
					rails[name] = roundFloat(mwh, 4)
				}
			}
		}
	}
	return rails
}

func roundFloat(val float64, precision int) float64 {
	p := 1.0
	for i := 0; i < precision; i++ {
		p *= 10.0
	}
	return float64(int(val*p+0.5)) / p
}

// PowerThermalSnapshot captures system thermal status and battery power metrics.
func PowerThermalSnapshot(
	ctx context.Context,
	dm types.DeviceManager,
	serial string,
	durationS int,
	mode string,
	pkg string,
) (*types.ThermalPowerResult, error) {
	if dm == nil {
		return nil, fmt.Errorf("device manager is required")
	}
	if serial == "" {
		return nil, fmt.Errorf("device serial is required")
	}

	// 1. Thermal reading
	thermalOut, _ := dm.ExecADB(serial, "shell", "dumpsys", "thermalservice")
	status, skinTemp, batTemp := ParseThermalService(thermalOut)

	// 2. Battery initial reading
	batOut1, _ := dm.ExecADB(serial, "shell", "dumpsys", "batterystats")
	charge1, _, batTempFromBat := ParseBatteryStats(batOut1)
	if batTemp == 0 && batTempFromBat > 0 {
		batTemp = batTempFromBat
	}
	odpmRails := ParseODPMRails(batOut1)

	var chargeDeltaMAh float64

	// If delta measurement requested with duration
	if durationS > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(durationS) * time.Second):
		}

		batOut2, _ := dm.ExecADB(serial, "shell", "dumpsys", "batterystats")
		charge2, _, _ := ParseBatteryStats(batOut2)
		if charge1 != 0 && charge2 != 0 {
			chargeDeltaMAh = charge2 - charge1
		}
		// Update ODPM delta if available
		odpmRails2 := ParseODPMRails(batOut2)
		if len(odpmRails2) > 0 {
			deltaRails := make(map[string]float64)
			for k, v2 := range odpmRails2 {
				if v1, ok := odpmRails[k]; ok {
					deltaRails[k] = roundFloat(v2-v1, 4)
				} else {
					deltaRails[k] = v2
				}
			}
			odpmRails = deltaRails
		}
	}

	return &types.ThermalPowerResult{
		ThermalStatus:  status,
		SkinTempC:      skinTemp,
		BatteryTempC:   batTemp,
		ChargeDeltaMAh: chargeDeltaMAh,
		ODPMRailsMWh:   odpmRails,
	}, nil
}
