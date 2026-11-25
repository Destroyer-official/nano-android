package vision

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// Ensure Manager implements types.VisionBackend
var _ types.VisionBackend = (*Manager)(nil)

// ADBExecutor is an interface satisfied by types.DeviceManager.
type ADBExecutor interface {
	ExecADB(serial string, args ...string) (string, error)
}

// ManagerConfig holds configuration parameters for the VisionBackend Coordinator.
type ManagerConfig struct {
	ScrcpyJarPath     string
	ScrcpyVersion     string
	FFmpegPath        string
	IdleTimeout       time.Duration
	CheckInterval     time.Duration
	DefaultMaxHeight  int
	DefaultQuality    int
	RedactStatusBar   bool
	StatusBarHeightPx int
	ADBRunner         ADBRunner
	ADBBinaryRunner   ADBBinaryRunner
}

// Manager coordinates frame acquisition between high-speed scrcpy-server and fallback screencap.
type Manager struct {
	scrcpy            *ScrcpyBackend
	screencap         *ScreencapBackend
	idleTimeout       time.Duration
	checkInterval     time.Duration
	defaultMaxHeight  int
	defaultQuality    int
	redactStatusBar   bool
	statusBarHeightPx int

	mu          sync.RWMutex
	lastActive  map[string]time.Time
	backendMode map[string]string // "scrcpy" or "screencap"

	stopSweeper chan struct{}
	sweeperWg   sync.WaitGroup
}

// NewManager creates a new VisionBackend Coordinator.
// Accepts either:
//   - NewManager(cfg ManagerConfig)
//   - NewManager(deviceMgr ADBExecutor, jarPath string, idleTimeout time.Duration)
func NewManager(args ...any) *Manager {
	var cfg ManagerConfig
	if len(args) == 1 {
		if c, ok := args[0].(ManagerConfig); ok {
			cfg = c
		}
	} else if len(args) >= 3 {
		if execer, ok := args[0].(interface {
			ExecADB(serial string, args ...string) (string, error)
		}); ok && execer != nil {
			cfg.ADBRunner = execer.ExecADB
		}
		if jar, ok := args[1].(string); ok {
			cfg.ScrcpyJarPath = jar
		}
		if timeout, ok := args[2].(time.Duration); ok {
			cfg.IdleTimeout = timeout
		}
	}

	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 10 * time.Minute
	}
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 30 * time.Second
	}
	if cfg.DefaultMaxHeight <= 0 {
		cfg.DefaultMaxHeight = 720
	}
	if cfg.DefaultQuality <= 0 {
		cfg.DefaultQuality = 75
	}
	if cfg.ADBRunner == nil {
		cfg.ADBRunner = DefaultADBRunner
	}
	if cfg.ADBBinaryRunner == nil {
		cfg.ADBBinaryRunner = DefaultADBBinaryRunner
	}

	scrcpyCfg := ScrcpyConfig{
		JarPath:    cfg.ScrcpyJarPath,
		Version:    cfg.ScrcpyVersion,
		FFmpegPath: cfg.FFmpegPath,
		ADBRunner:  cfg.ADBRunner,
		BinRunner:  cfg.ADBBinaryRunner,
	}

	m := &Manager{
		scrcpy:            NewScrcpyBackend(scrcpyCfg),
		screencap:         NewScreencapBackend(cfg.ADBBinaryRunner),
		idleTimeout:       cfg.IdleTimeout,
		checkInterval:     cfg.CheckInterval,
		defaultMaxHeight:  cfg.DefaultMaxHeight,
		defaultQuality:    cfg.DefaultQuality,
		redactStatusBar:   cfg.RedactStatusBar,
		statusBarHeightPx: cfg.StatusBarHeightPx,
		lastActive:        make(map[string]time.Time),
		backendMode:       make(map[string]string),
		stopSweeper:       make(chan struct{}),
	}

	m.startSweeper()
	return m
}

// startSweeper runs a background goroutine to clean up idle sessions.
func (m *Manager) startSweeper() {
	m.sweeperWg.Add(1)
	go func() {
		defer m.sweeperWg.Done()
		ticker := time.NewTicker(m.checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-m.stopSweeper:
				return
			case now := <-ticker.C:
				m.sweepIdleSessions(now)
			}
		}
	}()
}

// sweepIdleSessions tears down any device sessions that have been idle longer than idleTimeout.
func (m *Manager) sweepIdleSessions(now time.Time) {
	m.mu.Lock()
	var toTeardown []string
	for serial, last := range m.lastActive {
		if now.Sub(last) > m.idleTimeout {
			toTeardown = append(toTeardown, serial)
		}
	}
	m.mu.Unlock()

	for _, serial := range toTeardown {
		_ = m.Teardown(serial)
	}
}

// GrabFrame attempts capture via scrcpy first; on failure, falls back to screencap.
func (m *Manager) GrabFrame(serial string, maxHeight int, format string) ([]byte, error) {
	if maxHeight <= 0 {
		maxHeight = m.defaultMaxHeight
	}
	if format == "" {
		format = "webp"
	}

	m.mu.Lock()
	m.lastActive[serial] = time.Now()
	mode := m.backendMode[serial]
	m.mu.Unlock()

	var frameBytes []byte
	var err error

	// If device is already marked as fallback screencap, use screencap directly
	if mode == "screencap" {
		frameBytes, err = m.screencap.GrabFrame(serial, maxHeight, format)
		if err != nil {
			return nil, fmt.Errorf("screencap fallback failed for %s: %w", serial, err)
		}
	} else {
		// Try scrcpy first
		frameBytes, err = m.scrcpy.GrabFrame(serial, maxHeight, format)
		if err != nil {
			// On failure, record fallback state and use screencap
			m.mu.Lock()
			m.backendMode[serial] = "screencap"
			m.mu.Unlock()

			frameBytes, err = m.screencap.GrabFrame(serial, maxHeight, format)
			if err != nil {
				return nil, fmt.Errorf("both scrcpy and screencap failed for %s: %w", serial, err)
			}
		} else {
			m.mu.Lock()
			m.backendMode[serial] = "scrcpy"
			m.mu.Unlock()
		}
	}

	// Apply status bar redaction if configured
	if m.redactStatusBar && m.statusBarHeightPx > 0 {
		img, _, decErr := DecodeImage(frameBytes)
		if decErr == nil {
			rgba := EnsureRGBA(img)
			RedactStatusBar(rgba, m.statusBarHeightPx)
			encBytes, encErr := EncodeImage(rgba, format, m.defaultQuality)
			if encErr == nil {
				frameBytes = encBytes
			}
		}
	}

	m.mu.Lock()
	m.lastActive[serial] = time.Now()
	m.mu.Unlock()

	return frameBytes, nil
}

// ObserveScreenFrame is a compound method that grabs a frame and optionally renders Set-of-Marks boxes.
func (m *Manager) ObserveScreenFrame(ctx context.Context, serial string, maxHeight int, format string, drawSoM bool, nodes []types.PrunedNode) ([]byte, error) {
	frameBytes, err := m.GrabFrame(serial, maxHeight, format)
	if err != nil {
		return nil, err
	}

	if drawSoM && len(nodes) > 0 {
		somCfg := DefaultSoMConfig()
		if format != "" {
			somCfg.Format = format
		}
		somCfg.Quality = m.defaultQuality

		// Pass native session dimensions for scaling if available
		if sess := m.scrcpy.GetSession(serial); sess != nil {
			sess.frameMu.RLock()
			somCfg.OrigWidth = sess.frameWidth
			somCfg.OrigHeight = sess.frameHeight
			sess.frameMu.RUnlock()
		}

		annotated, err := DrawSoMBoxesWithConfig(frameBytes, nodes, somCfg)
		if err != nil {
			return frameBytes, nil
		}
		return annotated, nil
	}

	return frameBytes, nil
}

// IsStreaming returns true if a live frame stream is active for this device.
func (m *Manager) IsStreaming(serial string) bool {
	return m.scrcpy.IsStreaming(serial)
}

// GetStats returns telemetry metrics for a device session.
func (m *Manager) GetStats(serial string) *SessionStats {
	m.mu.RLock()
	mode := m.backendMode[serial]
	m.mu.RUnlock()

	stats := m.scrcpy.GetStats(serial)
	if stats != nil {
		return stats
	}

	return &SessionStats{
		Serial:      serial,
		State:       StateIdle,
		BackendType: mode,
	}
}

// Teardown stops the frame stream, kills processes, and cleans up resources for a device.
func (m *Manager) Teardown(serial string) error {
	m.mu.Lock()
	delete(m.lastActive, serial)
	delete(m.backendMode, serial)
	m.mu.Unlock()

	return m.scrcpy.Teardown(serial)
}

// Close stops the manager and tears down all active sessions.
func (m *Manager) Close() error {
	select {
	case <-m.stopSweeper:
	default:
		close(m.stopSweeper)
	}
	m.sweeperWg.Wait()

	m.mu.Lock()
	var allSerials []string
	for s := range m.lastActive {
		allSerials = append(allSerials, s)
	}
	m.mu.Unlock()

	var firstErr error
	for _, s := range allSerials {
		if err := m.Teardown(s); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
