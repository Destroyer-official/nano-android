package profiler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// LogcatReadRequest holds parameters for filtered logcat reads.
type LogcatReadRequest struct {
	Serial        string   `json:"serial"`
	PackageID     string   `json:"package_id,omitempty"`
	PID           int      `json:"pid,omitempty"`
	PIDOnly       bool     `json:"pid_only,omitempty"`
	MinLevel      string   `json:"min_level,omitempty"` // V, D, I, W, E, F
	Tags          []string `json:"tags,omitempty"`
	SinceLastCall bool     `json:"since_last_call,omitempty"`
	MaxLines      int      `json:"max_lines,omitempty"` // default 100
}

var (
	// tracks the last read timestamp per device serial
	deviceCursorsMu sync.Mutex
	deviceCursors   = make(map[string]time.Time)
)

// GenerateCryptoNonce generates an 8-byte (16 hex character) cryptographic random string.
func GenerateCryptoNonce() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// LogLevelPriority returns integer priority for Android log levels.
func LogLevelPriority(lvl string) int {
	lvl = strings.ToUpper(strings.TrimSpace(lvl))
	switch lvl {
	case "V", "VERBOSE":
		return 2
	case "D", "DEBUG":
		return 3
	case "I", "INFO":
		return 4
	case "W", "WARN", "WARNING":
		return 5
	case "E", "ERROR":
		return 6
	case "F", "FATAL":
		return 7
	default:
		return 2
	}
}

// ParseThreadtimeLine checks if a log line matches level and extracts timestamp if present.
func ParseThreadtimeLine(line string) (lvl string, tag string, pid int, ok bool) {
	// Standard threadtime format:
	// 11-20 20:00:00.123  1234  5678 D TagName : Message
	fields := strings.Fields(line)
	if len(fields) < 6 {
		return "", "", 0, false
	}

	// Field 2 is PID
	pidVal, _ := strconv.Atoi(fields[2])

	// Field 4 is level (V, D, I, W, E, F)
	lvlCandidate := fields[4]
	if len(lvlCandidate) == 1 && strings.Contains("VDIWEF", lvlCandidate) {
		return lvlCandidate, fields[5], pidVal, true
	}

	return "", "", pidVal, false
}

// ReadLogcat reads and filters logcat entries from the device, applying nonce fencing.
func ReadLogcat(ctx context.Context, dm types.DeviceManager, req LogcatReadRequest) (*types.LogcatReadResult, error) {
	if dm == nil {
		return nil, fmt.Errorf("device manager is required")
	}
	if req.Serial == "" {
		return nil, fmt.Errorf("device serial is required")
	}

	maxLines := req.MaxLines
	if maxLines <= 0 {
		maxLines = 100
	}
	if maxLines > 1000 {
		maxLines = 1000
	}

	// 1. Resolve PID if pid_only is set and PID is not provided
	targetPID := req.PID
	if req.PIDOnly && targetPID <= 0 && req.PackageID != "" {
		pidOut, err := dm.ExecADB(req.Serial, "shell", "pidof", "-s", req.PackageID)
		if err == nil {
			targetPID, _ = strconv.Atoi(strings.TrimSpace(pidOut))
		}
	}

	// 2. Build adb logcat command args
	adbArgs := []string{"logcat", "-d", "-v", "threadtime"}
	if req.PIDOnly && targetPID > 0 {
		adbArgs = append(adbArgs, fmt.Sprintf("--pid=%d", targetPID))
	}

	minLvl := strings.ToUpper(strings.TrimSpace(req.MinLevel))
	if minLvl != "" {
		adbArgs = append(adbArgs, fmt.Sprintf("*:%s", minLvl))
	}

	if len(req.Tags) > 0 {
		adbArgs = append(adbArgs, "-s")
		for _, tag := range req.Tags {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				if minLvl != "" {
					adbArgs = append(adbArgs, fmt.Sprintf("%s:%s", tag, minLvl))
				} else {
					adbArgs = append(adbArgs, fmt.Sprintf("%s:*", tag))
				}
			}
		}
	}

	output, err := dm.ExecADB(req.Serial, adbArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to read logcat: %w", err)
	}

	rawLines := strings.Split(output, "\n")
	filtered := make([]string, 0, len(rawLines))

	deviceCursorsMu.Lock()
	lastTimestamp := deviceCursors[req.Serial]
	deviceCursors[req.Serial] = time.Now()
	deviceCursorsMu.Unlock()

	minPriority := LogLevelPriority(req.MinLevel)

	for _, line := range rawLines {
		trimmed := strings.TrimRight(line, "\r")
		if strings.TrimSpace(trimmed) == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "--------- beginning of") {
			continue
		}

		lvl, tag, pid, ok := ParseThreadtimeLine(trimmed)
		if ok {
			// Level check
			if minPriority > 2 && LogLevelPriority(lvl) < minPriority {
				continue
			}
			// PID check
			if req.PIDOnly && targetPID > 0 && pid != targetPID {
				continue
			}
			// Tags check
			if len(req.Tags) > 0 {
				tagMatched := false
				for _, t := range req.Tags {
					if strings.EqualFold(tag, t) || strings.HasPrefix(strings.ToLower(tag), strings.ToLower(t)) {
						tagMatched = true
						break
					}
				}
				if !tagMatched {
					continue
				}
			}
		}

		cleanedLine := StripZeroWidthAndBidi(trimmed)
		if len(cleanedLine) > 4096 {
			cleanedLine = cleanedLine[:4096] + "...[TRUNCATED]"
		}
		filtered = append(filtered, cleanedLine)
	}

	// Truncate to maxLines if exceeded
	truncated := false
	if len(filtered) > maxLines {
		truncated = true
		filtered = filtered[len(filtered)-maxLines:]
	}

	// Nonce-fencing
	nonce := GenerateCryptoNonce()
	fencedLines := make([]string, 0, len(filtered)+2)
	fencedLines = append(fencedLines, fmt.Sprintf("<<<UNTRUSTED_DEVICE_DATA_NONCE:%s>>>", nonce))
	fencedLines = append(fencedLines, filtered...)
	fencedLines = append(fencedLines, fmt.Sprintf("<<<END_UNTRUSTED_DEVICE_DATA_NONCE:%s>>>", nonce))

	_ = lastTimestamp

	return &types.LogcatReadResult{
		Nonce:     nonce,
		Lines:     fencedLines,
		Truncated: truncated,
	}, nil
}
