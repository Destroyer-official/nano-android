package vision

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// findADB resolves the adb executable path across PATH and standard SDK locations.
func findADB() string {
	if p, err := exec.LookPath("adb"); err == nil {
		return p
	}
	candidates := []string{
		"D:\\program_software\\Android_toolkit\\platform-tools\\adb.exe",
		os.Getenv("ANDROID_HOME") + "/platform-tools/adb.exe",
		os.Getenv("ANDROID_HOME") + "/platform-tools/adb",
		os.Getenv("ANDROID_SDK_ROOT") + "/platform-tools/adb.exe",
		os.Getenv("ANDROID_SDK_ROOT") + "/platform-tools/adb",
		os.ExpandEnv("$LOCALAPPDATA/Android/Sdk/platform-tools/adb.exe"),
	}
	for _, c := range candidates {
		if c != "" {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
	}
	return "adb"
}

// SessionState represents the lifecycle state of a device's vision backend.
type SessionState string

const (
	StateIdle              SessionState = "Idle"
	StateDeploying         SessionState = "Deploying"
	StateStreaming         SessionState = "Streaming"
	StateReconnecting      SessionState = "Reconnecting"
	StateFallbackScreencap SessionState = "FallbackScreencap"
	StateTornDown          SessionState = "TornDown"
)

// SessionStats holds operational telemetry and metrics for an active device vision session.
type SessionStats struct {
	Serial        string        `json:"serial"`
	State         SessionState  `json:"state"`
	BackendType   string        `json:"backend_type"` // "scrcpy" or "screencap"
	Width         int           `json:"width"`
	Height        int           `json:"height"`
	FramesGrabbed int64         `json:"frames_grabbed"`
	LastLatency   time.Duration `json:"last_latency"`
	AvgLatency    time.Duration `json:"avg_latency"`
	LastFrameTime time.Time     `json:"last_frame_time"`
}

// ADBRunner runs an ADB command returning string output.
type ADBRunner func(serial string, args ...string) (string, error)

// ADBBinaryRunner runs an ADB command returning raw bytes output.
type ADBBinaryRunner func(serial string, args ...string) ([]byte, error)

// DefaultADBBinaryRunner executes an ADB command safely using exec.CommandContext (no shell interpolation).
func DefaultADBBinaryRunner(serial string, args ...string) ([]byte, error) {
	cmdArgs := make([]string, 0, len(args)+2)
	if serial != "" {
		cmdArgs = append(cmdArgs, "-s", serial)
	}
	cmdArgs = append(cmdArgs, args...)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, findADB(), cmdArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("adb %v failed: %w (stderr: %s)", args, err, stderr.String())
	}
	return out, nil
}

// DefaultADBRunner wraps DefaultADBBinaryRunner returning string output.
func DefaultADBRunner(serial string, args ...string) (string, error) {
	out, err := DefaultADBBinaryRunner(serial, args...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// randomHex generates n random hex characters.
func randomHex(n int) string {
	b := make([]byte, (n+1)/2)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%08x", time.Now().UnixNano())[:n]
	}
	s := hex.EncodeToString(b)
	if len(s) > n {
		return s[:n]
	}
	return s
}

// randomScid generates an 8-character hex string representing a positive 31-bit
// integer (0x00000000..0x7fffffff) compatible with scrcpy's Integer.parseInt(scid, 16).
func randomScid() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%08x", time.Now().UnixNano()&0x7fffffff)
	}
	b[0] &= 0x7f // Clear MSB so Integer.parseInt(s, 16) in Java never overflows signed 32-bit int
	return hex.EncodeToString(b)
}
