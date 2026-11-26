package vision

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/bmp"
)

// ScrcpySession represents an active scrcpy streaming session for one device.
type ScrcpySession struct {
	serial       string
	state        SessionState
	mu           sync.RWMutex
	scid         string
	localPort    int
	serverCmd    *exec.Cmd
	videoConn    net.Conn
	ctrlConn     net.Conn
	ffCmd        *exec.Cmd
	ffStdin      io.WriteCloser
	ffStdout     io.ReadCloser
	stopCh       chan struct{}
	retryCount   int
	lastActive   time.Time
	frameWidth   int
	frameHeight  int
	latestBMP    []byte
	frameMu      sync.RWMutex
	firstFrameCh chan struct{}

	// Metrics
	framesGrabbed int64
	totalLatency  time.Duration
	lastLatency   time.Duration
	lastFrameTime time.Time
}

// ScrcpyBackend manages the lifecycle of transient scrcpy-server instances.
type ScrcpyBackend struct {
	jarPath    string
	version    string
	ffmpegPath string
	adbRunner  ADBRunner
	binRunner  ADBBinaryRunner
	sessions   map[string]*ScrcpySession
	mu         sync.RWMutex
	fallback   *ScreencapBackend
}

// ScrcpyConfig configures the ScrcpyBackend.
type ScrcpyConfig struct {
	JarPath    string
	Version    string
	FFmpegPath string
	ADBRunner  ADBRunner
	BinRunner  ADBBinaryRunner
}

// NewScrcpyBackend creates a new ScrcpyBackend.
func NewScrcpyBackend(cfg ScrcpyConfig) *ScrcpyBackend {
	if cfg.JarPath == "" {
		cfg.JarPath = findScrcpyJar()
	}
	if cfg.Version == "" {
		cfg.Version = "5.0.1"
	}
	if cfg.FFmpegPath == "" {
		cfg.FFmpegPath = findFFmpeg()
	}
	if cfg.ADBRunner == nil {
		cfg.ADBRunner = DefaultADBRunner
	}
	if cfg.BinRunner == nil {
		cfg.BinRunner = DefaultADBBinaryRunner
	}

	return &ScrcpyBackend{
		jarPath:    cfg.JarPath,
		version:    cfg.Version,
		ffmpegPath: cfg.FFmpegPath,
		adbRunner:  cfg.ADBRunner,
		binRunner:  cfg.BinRunner,
		sessions:   make(map[string]*ScrcpySession),
		fallback:   NewScreencapBackend(cfg.BinRunner),
	}
}

// findScrcpyJar attempts to locate scrcpy-server.jar across common paths.
func findScrcpyJar() string {
	candidates := []string{
		"../../bin/scrcpy-server.jar",
		"../../bin/scrcpy-server-v5.0.1",
		"../bin/scrcpy-server.jar",
		"../bin/scrcpy-server-v5.0.1",
		"bin/scrcpy-server.jar",
		"bin/scrcpy-server-v5.0.1",
		"droidforge-mcp/bin/scrcpy-server.jar",
		"droidforge-mcp/bin/scrcpy-server-v5.0.1",
		os.Getenv("SCRCPY_SERVER_PATH"),
	}
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		candidates = append([]string{
			filepath.Join(execDir, "bin", "scrcpy-server.jar"),
			filepath.Join(execDir, "bin", "scrcpy-server-v5.0.1"),
			filepath.Join(execDir, "scrcpy-server.jar"),
		}, candidates...)
	}
	for _, c := range candidates {
		if c != "" {
			if _, err := os.Stat(c); err == nil {
				abs, err := filepath.Abs(c)
				if err == nil {
					return abs
				}
				return c
			}
		}
	}
	return "bin/scrcpy-server.jar"
}

// findFFmpeg locates ffmpeg binary.
func findFFmpeg() string {
	if env := strings.TrimSpace(os.Getenv("FFMPEG_PATH")); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
	}
	candidates := []string{
		"../../bin/ffmpeg.exe",
		"../bin/ffmpeg.exe",
		"bin/ffmpeg.exe",
		"droidforge-mcp/bin/ffmpeg.exe",
		"../../bin/ffmpeg",
		"../bin/ffmpeg",
		"bin/ffmpeg",
		"droidforge-mcp/bin/ffmpeg",
		"ffmpeg",
	}
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		candidates = append([]string{
			filepath.Join(execDir, "bin", "ffmpeg.exe"),
			filepath.Join(execDir, "ffmpeg.exe"),
			filepath.Join(execDir, "bin", "ffmpeg"),
			filepath.Join(execDir, "ffmpeg"),
		}, candidates...)
	}
	for _, c := range candidates {
		if path, err := exec.LookPath(c); err == nil {
			return path
		}
		if _, err := os.Stat(c); err == nil {
			if abs, err := filepath.Abs(c); err == nil {
				return abs
			}
			return c
		}
	}
	return "ffmpeg"
}

// GetSession returns the current session for serial.
func (v *ScrcpyBackend) GetSession(serial string) *ScrcpySession {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.sessions[serial]
}

// IsStreaming returns true if the device session is currently in StateStreaming.
func (v *ScrcpyBackend) IsStreaming(serial string) bool {
	v.mu.RLock()
	s, ok := v.sessions[serial]
	v.mu.RUnlock()
	if !ok || s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state == StateStreaming
}

// GetStats returns telemetry metrics for a device session.
func (v *ScrcpyBackend) GetStats(serial string) *SessionStats {
	v.mu.RLock()
	s, ok := v.sessions[serial]
	v.mu.RUnlock()
	if !ok || s == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	s.frameMu.RLock()
	w := s.frameWidth
	h := s.frameHeight
	grabbed := s.framesGrabbed
	lastLat := s.lastLatency
	totalLat := s.totalLatency
	lastTime := s.lastFrameTime
	s.frameMu.RUnlock()

	var avgLat time.Duration
	if grabbed > 0 {
		avgLat = totalLat / time.Duration(grabbed)
	}

	backendType := "scrcpy"
	if s.state == StateFallbackScreencap {
		backendType = "screencap"
	}

	return &SessionStats{
		Serial:        serial,
		State:         s.state,
		BackendType:   backendType,
		Width:         w,
		Height:        h,
		FramesGrabbed: grabbed,
		LastLatency:   lastLat,
		AvgLatency:    avgLat,
		LastFrameTime: lastTime,
	}
}

// Deploy sets up and connects a transient scrcpy-server session for serial.
func (v *ScrcpyBackend) Deploy(serial string) error {
	v.mu.Lock()
	session, exists := v.sessions[serial]
	if !exists {
		session = &ScrcpySession{
			serial:       serial,
			state:        StateIdle,
			stopCh:       make(chan struct{}),
			firstFrameCh: make(chan struct{}, 1),
			lastActive:   time.Now(),
		}
		v.sessions[serial] = session
	}
	v.mu.Unlock()

	session.mu.Lock()
	defer session.mu.Unlock()

	session.state = StateDeploying
	session.lastActive = time.Now()

	// Clean up any stale handles before re-deploying
	if session.serverCmd != nil && session.serverCmd.Process != nil {
		_ = session.serverCmd.Process.Kill()
	}
	if session.videoConn != nil {
		_ = session.videoConn.Close()
	}
	if session.ctrlConn != nil {
		_ = session.ctrlConn.Close()
	}
	if session.localPort > 0 {
		_, _ = v.adbRunner(serial, "forward", "--remove", fmt.Sprintf("tcp:%d", session.localPort))
	}
	// Terminate any stale scrcpy server process on device to free hardware video encoder
	_, _ = v.adbRunner(serial, "shell", "pkill -f com.genymobile.scrcpy.Server 2>/dev/null || true")

	// 1. Verify scrcpy jar exists on host
	if _, err := os.Stat(v.jarPath); err != nil {
		session.state = StateFallbackScreencap
		return fmt.Errorf("scrcpy-server jar not found at %s: %w", v.jarPath, err)
	}

	// 2. Verify ffmpeg exists; if not available, gracefully fallback
	if _, err := exec.LookPath(v.ffmpegPath); err != nil {
		if _, statErr := os.Stat(v.ffmpegPath); statErr != nil {
			session.state = StateFallbackScreencap
			return fmt.Errorf("ffmpeg not found, falling back to screencap: %w", err)
		}
	}

	// 3. Push server jar to device
	_, err := v.adbRunner(serial, "push", v.jarPath, "/data/local/tmp/droidforge-scrcpy.jar")
	if err != nil {
		session.state = StateFallbackScreencap
		return fmt.Errorf("failed to push scrcpy jar: %w", err)
	}

	// 4. Forward ephemeral port with scid <= 0x7fffffff for Java Integer.parseInt compatibility
	scid := randomScid()
	session.scid = scid
	portStr, err := v.adbRunner(serial, "forward", "tcp:0", "localabstract:scrcpy_"+scid)
	if err != nil {
		session.state = StateFallbackScreencap
		return fmt.Errorf("failed to forward port: %w", err)
	}
	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil {
		session.state = StateFallbackScreencap
		return fmt.Errorf("invalid port returned from adb forward %q: %w", portStr, err)
	}
	session.localPort = port

	// 5. Launch server process
	serverCmd := exec.Command(findADB(), "-s", serial, "shell",
		"CLASSPATH=/data/local/tmp/droidforge-scrcpy.jar",
		"app_process", "/", "com.genymobile.scrcpy.Server", v.version,
		"scid="+scid, "tunnel_forward=true", "video=true", "audio=false", "control=true",
		"max_size=720", "max_fps=15", "video_bit_rate=2000000", "cleanup=false")
	var stderrBuf bytes.Buffer
	serverCmd.Stderr = &stderrBuf
	if err := serverCmd.Start(); err != nil {
		session.state = StateFallbackScreencap
		return fmt.Errorf("failed to launch scrcpy server process: %w", err)
	}
	session.serverCmd = serverCmd

	// 6. Connect to localhost:port with retry loop (up to 15 seconds)
	var vConn net.Conn
	time.Sleep(300 * time.Millisecond) // Give app_process time to initialize JVM
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		vConn, err = net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 1*time.Second)
		if err == nil {
			var dummy [1]byte
			// Once TCP socket connects, wait up to 10s for MediaCodec initialization & dummy byte
			vConn.SetReadDeadline(time.Now().Add(10 * time.Second))
			_, err = io.ReadFull(vConn, dummy[:])
			if err == nil {
				vConn.SetReadDeadline(time.Time{})
				break
			}
			vConn.Close()
			vConn = nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	if vConn == nil {
		session.state = StateFallbackScreencap
		_ = serverCmd.Process.Kill()
		errMsg := "failed to connect to scrcpy video socket"
		if err != nil {
			errMsg += fmt.Sprintf(": %v", err)
		}
		if s := strings.TrimSpace(stderrBuf.String()); s != "" {
			errMsg += fmt.Sprintf(" (server stderr: %s)", s)
		}
		return errors.New(errMsg)
	}
	session.videoConn = vConn

	// Also connect control socket (control=true expects 2nd connection)
	cConn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 5*time.Second)
	if err != nil {
		session.state = StateFallbackScreencap
		_ = vConn.Close()
		_ = serverCmd.Process.Kill()
		return fmt.Errorf("failed to connect to scrcpy control socket: %w", err)
	}
	session.ctrlConn = cConn

	// Set 10s read deadline for handshake metadata to avoid indefinite block if encoder stalls
	vConn.SetReadDeadline(time.Now().Add(10 * time.Second))

	// Read 64 bytes device name
	var devName [64]byte
	if _, err := io.ReadFull(vConn, devName[:]); err != nil {
		session.state = StateFallbackScreencap
		_ = vConn.Close()
		_ = cConn.Close()
		_ = serverCmd.Process.Kill()
		errMsg := fmt.Sprintf("failed to read device metadata: %v", err)
		if s := strings.TrimSpace(stderrBuf.String()); s != "" {
			errMsg += fmt.Sprintf(" (server stderr: %s)", s)
		}
		return errors.New(errMsg)
	}

	// Read 4 bytes codec
	var codec [4]byte
	if _, err := io.ReadFull(vConn, codec[:]); err != nil {
		session.state = StateFallbackScreencap
		_ = vConn.Close()
		_ = cConn.Close()
		_ = serverCmd.Process.Kill()
		errMsg := fmt.Sprintf("failed to read codec: %v", err)
		if s := strings.TrimSpace(stderrBuf.String()); s != "" {
			errMsg += fmt.Sprintf(" (server stderr: %s)", s)
		}
		return errors.New(errMsg)
	}

	// Read 12 bytes session metadata
	var sessionMeta [12]byte
	if _, err := io.ReadFull(vConn, sessionMeta[:]); err != nil {
		session.state = StateFallbackScreencap
		_ = vConn.Close()
		_ = cConn.Close()
		_ = serverCmd.Process.Kill()
		errMsg := fmt.Sprintf("failed to read session meta: %v", err)
		if s := strings.TrimSpace(stderrBuf.String()); s != "" {
			errMsg += fmt.Sprintf(" (server stderr: %s)", s)
		}
		return errors.New(errMsg)
	}
	vConn.SetReadDeadline(time.Time{})
	session.frameWidth = int(binary.BigEndian.Uint32(sessionMeta[4:8]))
	session.frameHeight = int(binary.BigEndian.Uint32(sessionMeta[8:12]))

	// 7. Start decoder pipeline (FFmpeg)
	if err := v.startDecoder(session); err != nil {
		session.state = StateFallbackScreencap
		_ = vConn.Close()
		_ = cConn.Close()
		_ = serverCmd.Process.Kill()
		return fmt.Errorf("failed to start decoder: %w", err)
	}

	session.state = StateStreaming
	return nil
}

// startDecoder launches FFmpeg and starts the background packet reader and frame decoder goroutines.
func (v *ScrcpyBackend) startDecoder(s *ScrcpySession) error {
	ffCmd := exec.Command(v.ffmpegPath,
		"-loglevel", "error",
		"-threads", "1",
		"-flags", "low_delay",
		"-fflags", "nobuffer",
		"-probesize", "32768",
		"-analyzeduration", "0",
		"-f", "h264",
		"-i", "pipe:0",
		"-f", "image2pipe",
		"-vcodec", "bmp",
		"pipe:1")
	stdin, err := ffCmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stdin pipe: %w", err)
	}
	stdout, err := ffCmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return fmt.Errorf("ffmpeg stdout pipe: %w", err)
	}
	if err := ffCmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return fmt.Errorf("ffmpeg start: %w", err)
	}

	s.ffCmd = ffCmd
	s.ffStdin = stdin
	s.ffStdout = stdout

	// Goroutine 1: Demux packets from videoConn and write media payloads to FFmpeg stdin.
	// Automatically handles rotation/resize session packets (bit 63 set).
	go func() {
		defer stdin.Close()
		for {
			select {
			case <-s.stopCh:
				return
			default:
			}

			var hdr [12]byte
			if _, err := io.ReadFull(s.videoConn, hdr[:]); err != nil {
				v.handleStreamError(s, err)
				return
			}

			// Check if packet is a Session packet (bit 63 / 0x80000000 set)
			flags := binary.BigEndian.Uint32(hdr[0:4])
			if (flags & 0x80000000) != 0 {
				// Rotation or screen resize: updates dimensions without payload
				newW := int(binary.BigEndian.Uint32(hdr[4:8]))
				newH := int(binary.BigEndian.Uint32(hdr[8:12]))
				s.frameMu.Lock()
				s.frameWidth = newW
				s.frameHeight = newH
				s.frameMu.Unlock()
				continue
			}

			// Media packet: payload follows
			size := binary.BigEndian.Uint32(hdr[8:12])
			if size > 16*1024*1024 { // 16MB sanity cap
				v.handleStreamError(s, fmt.Errorf("packet size exceeds limit: %d", size))
				return
			}
			packet := make([]byte, size)
			if _, err := io.ReadFull(s.videoConn, packet); err != nil {
				v.handleStreamError(s, err)
				return
			}
			if _, err := stdin.Write(packet); err != nil {
				return
			}
		}
	}()

	// Goroutine 2: Read decoded BMP frames from FFmpeg stdout and update latestBMP
	go func() {
		var hdr [54]byte
		for {
			select {
			case <-s.stopCh:
				return
			default:
			}

			// 1. Read 54-byte BMP header
			if _, err := io.ReadFull(stdout, hdr[:]); err != nil {
				return
			}
			if hdr[0] != 'B' || hdr[1] != 'M' {
				continue
			}

			fileSize := binary.LittleEndian.Uint32(hdr[2:6])
			if fileSize < 54 || fileSize > 32*1024*1024 { // 32MB sanity cap
				continue
			}

			bmpBuf := make([]byte, fileSize)
			copy(bmpBuf[:54], hdr[:])

			// 2. Read remaining image payload
			if _, err := io.ReadFull(stdout, bmpBuf[54:]); err != nil {
				return
			}

			// 3. Extract dimensions from BMP header (little-endian int32)
			w := int(int32(binary.LittleEndian.Uint32(hdr[18:22])))
			h := int(int32(binary.LittleEndian.Uint32(hdr[22:26])))
			if h < 0 {
				h = -h
			}

			s.frameMu.Lock()
			s.frameWidth = w
			s.frameHeight = h
			s.latestBMP = bmpBuf
			s.lastFrameTime = time.Now()
			s.frameMu.Unlock()

			// Signal first frame arrival non-blockingly
			select {
			case s.firstFrameCh <- struct{}{}:
			default:
			}
		}
	}()

	return nil
}

// handleStreamError manages reconnect logic: max 2 retries, then FallbackScreencap.
func (v *ScrcpyBackend) handleStreamError(s *ScrcpySession, streamErr error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateTornDown {
		return
	}

	if s.retryCount < 2 {
		s.state = StateReconnecting
		s.retryCount++
		go func() {
			time.Sleep(500 * time.Millisecond)
			_ = v.Deploy(s.serial)
		}()
	} else {
		s.state = StateFallbackScreencap
	}
}

// GrabFrame grabs the latest frame from the live stream. Falls back to screencap if needed.
func (v *ScrcpyBackend) GrabFrame(serial string, maxHeight int, format string) ([]byte, error) {
	start := time.Now()

	v.mu.RLock()
	session, exists := v.sessions[serial]
	v.mu.RUnlock()

	if !exists || session == nil {
		if err := v.Deploy(serial); err != nil {
			return v.fallback.GrabFrame(serial, maxHeight, format)
		}
		session = v.GetSession(serial)
	}

	session.mu.RLock()
	state := session.state
	session.mu.RUnlock()

	if state == StateFallbackScreencap {
		return v.fallback.GrabFrame(serial, maxHeight, format)
	}

	session.lastActive = time.Now()

	// Wait up to 3 seconds for first frame if not yet arrived
	session.frameMu.RLock()
	hasFrame := len(session.latestBMP) > 0
	session.frameMu.RUnlock()

	if !hasFrame {
		select {
		case <-session.firstFrameCh:
		case <-time.After(3 * time.Second):
			// Timeout waiting for stream frame, fall back to screencap
			return v.fallback.GrabFrame(serial, maxHeight, format)
		}
	}

	session.frameMu.RLock()
	bmpData := make([]byte, len(session.latestBMP))
	copy(bmpData, session.latestBMP)
	session.frameMu.RUnlock()

	if len(bmpData) < 54 {
		return v.fallback.GrabFrame(serial, maxHeight, format)
	}

	img, err := bmp.Decode(bytes.NewReader(bmpData))
	if err != nil {
		return v.fallback.GrabFrame(serial, maxHeight, format)
	}

	if maxHeight <= 0 {
		maxHeight = 720
	}
	resized := ResizeImage(img, maxHeight)

	if format == "" {
		format = "webp"
	}
	out, err := EncodeImage(resized, format, 75)
	if err == nil {
		elapsed := time.Since(start)
		session.frameMu.Lock()
		session.framesGrabbed++
		session.lastLatency = elapsed
		session.totalLatency += elapsed
		session.frameMu.Unlock()
	}
	return out, err
}

// Teardown cleans up a device session, processes, and port forwards.
func (v *ScrcpyBackend) Teardown(serial string) error {
	v.mu.Lock()
	session, exists := v.sessions[serial]
	if exists {
		delete(v.sessions, serial)
	}
	v.mu.Unlock()

	if !exists || session == nil {
		return nil
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	session.state = StateTornDown

	// Close stop channel
	select {
	case <-session.stopCh:
	default:
		close(session.stopCh)
	}

	// Close network connections
	if session.videoConn != nil {
		_ = session.videoConn.Close()
	}
	if session.ctrlConn != nil {
		_ = session.ctrlConn.Close()
	}

	// Terminate decoder
	if session.ffCmd != nil && session.ffCmd.Process != nil {
		_ = session.ffCmd.Process.Kill()
	}

	// Terminate scrcpy server
	if session.serverCmd != nil && session.serverCmd.Process != nil {
		_ = session.serverCmd.Process.Kill()
	}

	// Remove port forward
	if session.localPort > 0 {
		_, _ = v.adbRunner(serial, "forward", "--remove", fmt.Sprintf("tcp:%d", session.localPort))
	}

	// Kill any lingering server process on device to release hardware video encoder
	_, _ = v.adbRunner(serial, "shell", "pkill -f com.genymobile.scrcpy.Server 2>/dev/null || true")

	// Clean up temporary server jar from device
	_, _ = v.adbRunner(serial, "shell", "rm", "-f", "/data/local/tmp/droidforge-scrcpy.jar")

	return nil
}
