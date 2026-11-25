package device

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultConsoleTimeout = 10 * time.Second
)

var (
	serialPortRegex = regexp.MustCompile(`^emulator-(\d+)$`)
)

// EmulatorConsoleClient interacts with the Android emulator's built-in telnet console.
// This interface allows injecting hardware events such as SMS, incoming GSM calls,
// GPS geo-coordinates, network conditions, and battery status.
type EmulatorConsoleClient struct {
	host      string
	port      int
	timeout   time.Duration
	tokenPath string
	authToken string
}

// NewEmulatorConsoleClient creates a console client for the given emulator serial.
// The serial must be in the form "emulator-<port>" (e.g., "emulator-5554").
func NewEmulatorConsoleClient(serial string) (*EmulatorConsoleClient, error) {
	port, err := ExtractConsolePort(serial)
	if err != nil {
		return nil, err
	}

	return &EmulatorConsoleClient{
		host:      "127.0.0.1",
		port:      port,
		timeout:   defaultConsoleTimeout,
		tokenPath: defaultAuthTokenPath(),
	}, nil
}

// ExtractConsolePort extracts the numeric console port from an emulator serial.
func ExtractConsolePort(serial string) (int, error) {
	serial = strings.TrimSpace(serial)
	matches := serialPortRegex.FindStringSubmatch(serial)
	if len(matches) == 2 {
		port, err := strconv.Atoi(matches[1])
		if err == nil && port > 0 && port < 65536 {
			return port, nil
		}
	}

	// Also handle bare port or :port
	trimmed := strings.TrimPrefix(serial, ":")
	if port, err := strconv.Atoi(trimmed); err == nil && port > 0 && port < 65536 {
		return port, nil
	}

	return 0, fmt.Errorf("invalid emulator serial format %q: expected 'emulator-<port>' (e.g. emulator-5554)", serial)
}

// SetAuthToken explicitly sets the authentication token, bypassing file lookup.
func (c *EmulatorConsoleClient) SetAuthToken(token string) {
	c.authToken = strings.TrimSpace(token)
}

// SetTimeout updates the connection and read/write timeout.
func (c *EmulatorConsoleClient) SetTimeout(d time.Duration) {
	if d > 0 {
		c.timeout = d
	}
}

// Exec executes a raw command against the emulator console with authentication handling.
func (c *EmulatorConsoleClient) Exec(ctx context.Context, command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", errors.New("command cannot be empty")
	}

	addr := fmt.Sprintf("%s:%d", c.host, c.port)
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return "", fmt.Errorf("failed to connect to emulator console at %s: %w", addr, err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(c.timeout))
	}

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// Step 1: Read welcome banner and detect if auth is required
	authRequired, err := readWelcomeBanner(reader)
	if err != nil {
		return "", fmt.Errorf("failed reading console banner: %w", err)
	}

	// Step 2: Authenticate if needed
	if authRequired {
		token := c.authToken
		if token == "" {
			token, err = readAuthTokenFile(c.tokenPath)
			if err != nil {
				return "", fmt.Errorf("emulator console requires authentication: %w", err)
			}
		}

		if err := sendCommandAndExpectOK(writer, reader, fmt.Sprintf("auth %s", token)); err != nil {
			return "", fmt.Errorf("emulator console authentication failed: %w", err)
		}
	}

	// Step 3: Send command and read response
	result, err := sendCommand(writer, reader, command)
	if err != nil {
		return "", err
	}

	// Step 4: Gracefully disconnect
	_, _ = writer.WriteString("quit\r\n")
	_ = writer.Flush()

	return result, nil
}

// SendSMS injects an incoming SMS text message into the emulator.
// Format: sms send <senderPhoneNumber> <messageText>
func (c *EmulatorConsoleClient) SendSMS(ctx context.Context, sender, message string) error {
	sender = strings.TrimSpace(sender)
	if sender == "" {
		return errors.New("sender phone number cannot be empty")
	}
	// Sanitize newlines in message text
	message = strings.ReplaceAll(message, "\r", " ")
	message = strings.ReplaceAll(message, "\n", " ")

	cmd := fmt.Sprintf("sms send %s %s", sender, message)
	_, err := c.Exec(ctx, cmd)
	return err
}

// SimulateCall initiates an incoming voice call on the emulator.
// Format: gsm call <callerPhoneNumber>
func (c *EmulatorConsoleClient) SimulateCall(ctx context.Context, caller string) error {
	caller = strings.TrimSpace(caller)
	if caller == "" {
		return errors.New("caller phone number cannot be empty")
	}
	cmd := fmt.Sprintf("gsm call %s", caller)
	_, err := c.Exec(ctx, cmd)
	return err
}

// CancelCall terminates an ongoing or incoming voice call.
// Format: gsm cancel <callerPhoneNumber>
func (c *EmulatorConsoleClient) CancelCall(ctx context.Context, caller string) error {
	caller = strings.TrimSpace(caller)
	if caller == "" {
		return errors.New("caller phone number cannot be empty")
	}
	cmd := fmt.Sprintf("gsm cancel %s", caller)
	_, err := c.Exec(ctx, cmd)
	return err
}

// SetGeoLocation sets the mock GPS coordinates of the emulator.
// Note: Android emulator console accepts: geo fix <longitude> <latitude> [altitude]
func (c *EmulatorConsoleClient) SetGeoLocation(ctx context.Context, lon, lat, alt float64) error {
	cmd := fmt.Sprintf("geo fix %f %f %f", lon, lat, alt)
	_, err := c.Exec(ctx, cmd)
	return err
}

// SetNetworkDelay simulates network latency conditions.
// delay: "none", "gprs" (150-550ms), "edge" (80-400ms), "umts" (35-200ms), "lte"
func (c *EmulatorConsoleClient) SetNetworkDelay(ctx context.Context, delay string) error {
	delay = strings.ToLower(strings.TrimSpace(delay))
	if delay == "" {
		delay = "none"
	}
	cmd := fmt.Sprintf("network delay %s", delay)
	_, err := c.Exec(ctx, cmd)
	return err
}

// SetNetworkSpeed simulates network download/upload throughput limits.
// speed: "full", "lte", "hsdpa", "umts", "edge", "gprs", "gsm"
func (c *EmulatorConsoleClient) SetNetworkSpeed(ctx context.Context, speed string) error {
	speed = strings.ToLower(strings.TrimSpace(speed))
	if speed == "" {
		speed = "full"
	}
	cmd := fmt.Sprintf("network speed %s", speed)
	_, err := c.Exec(ctx, cmd)
	return err
}

// SetPowerCapacity sets the battery charge percentage (0-100).
func (c *EmulatorConsoleClient) SetPowerCapacity(ctx context.Context, pct int) error {
	if pct < 0 || pct > 100 {
		return fmt.Errorf("battery capacity must be between 0 and 100, got %d", pct)
	}
	cmd := fmt.Sprintf("power capacity %d", pct)
	_, err := c.Exec(ctx, cmd)
	return err
}

// SetPowerAC controls the simulated AC power adapter state ("on" or "off").
func (c *EmulatorConsoleClient) SetPowerAC(ctx context.Context, state string) error {
	state = strings.ToLower(strings.TrimSpace(state))
	if state != "on" && state != "off" {
		return fmt.Errorf("power ac state must be 'on' or 'off', got %q", state)
	}
	cmd := fmt.Sprintf("power ac %s", state)
	_, err := c.Exec(ctx, cmd)
	return err
}

// SetPowerStatus sets battery charging status: "unknown", "charging", "discharging", "not-charging", "full".
func (c *EmulatorConsoleClient) SetPowerStatus(ctx context.Context, status string) error {
	status = strings.ToLower(strings.TrimSpace(status))
	validStatuses := map[string]bool{
		"unknown":      true,
		"charging":     true,
		"discharging":  true,
		"not-charging": true,
		"full":         true,
	}
	if !validStatuses[status] {
		return fmt.Errorf("invalid power status %q: must be unknown|charging|discharging|not-charging|full", status)
	}
	cmd := fmt.Sprintf("power status %s", status)
	_, err := c.Exec(ctx, cmd)
	return err
}

// Helper: readWelcomeBanner reads lines until "OK" or detects auth requirement.
func readWelcomeBanner(r *bufio.Reader) (authRequired bool, err error) {
	for {
		line, readErr := r.ReadString('\n')
		if readErr != nil {
			return false, readErr
		}
		lineTrim := strings.TrimSpace(line)
		if strings.Contains(lineTrim, "Authentication required") {
			authRequired = true
		}
		if lineTrim == "OK" {
			return authRequired, nil
		}
	}
}

// Helper: sendCommand writes command and reads multiline output until OK or KO.
func sendCommand(w *bufio.Writer, r *bufio.Reader, command string) (string, error) {
	if _, err := w.WriteString(command + "\r\n"); err != nil {
		return "", fmt.Errorf("failed sending command: %w", err)
	}
	if err := w.Flush(); err != nil {
		return "", fmt.Errorf("failed flushing command: %w", err)
	}

	var outputLines []string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("reading response: %w", err)
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "OK" {
			return strings.Join(outputLines, "\n"), nil
		}
		if strings.HasPrefix(trimmed, "KO:") {
			return strings.Join(outputLines, "\n"), fmt.Errorf("emulator console rejected command: %s", trimmed)
		}
		outputLines = append(outputLines, line)
	}
}

func sendCommandAndExpectOK(w *bufio.Writer, r *bufio.Reader, command string) error {
	_, err := sendCommand(w, r, command)
	return err
}

func defaultAuthTokenPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("USERPROFILE")
	}
	return filepath.Join(home, ".emulator_console_auth_token")
}

func readAuthTokenFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("could not read emulator auth token from %s: %w", path, err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("emulator auth token file %s is empty", path)
	}
	return token, nil
}
