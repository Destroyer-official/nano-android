package device

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// startMockEmulatorConsole spins up an in-memory TCP server mimicking the Android emulator console.
func startMockEmulatorConsole(t *testing.T, expectedToken string) (port int, stop func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind mock console: %v", err)
	}

	port = listener.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					return
				}
			}

			go handleMockConnection(t, conn, expectedToken)
		}
	}()

	return port, func() {
		cancel()
		_ = listener.Close()
	}
}

func handleMockConnection(t *testing.T, conn net.Conn, expectedToken string) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// Send initial banner requiring authentication
	_, _ = writer.WriteString("Android Console: Authentication required\r\n")
	_, _ = writer.WriteString("Android Console: type 'auth <auth_token>' to authenticate\r\n")
	_, _ = writer.WriteString("OK\r\n")
	_ = writer.Flush()

	authenticated := false

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		cmd := strings.TrimSpace(line)
		if cmd == "quit" {
			return
		}

		if strings.HasPrefix(cmd, "auth ") {
			token := strings.TrimPrefix(cmd, "auth ")
			if token == expectedToken {
				authenticated = true
				_, _ = writer.WriteString("OK\r\n")
			} else {
				_, _ = writer.WriteString("KO: authentication failed\r\n")
			}
			_ = writer.Flush()
			continue
		}

		if !authenticated {
			_, _ = writer.WriteString("KO: not authenticated\r\n")
			_ = writer.Flush()
			continue
		}

		// Handle authenticated console commands
		switch {
		case strings.HasPrefix(cmd, "sms send "):
			_, _ = writer.WriteString("OK\r\n")
		case strings.HasPrefix(cmd, "gsm call "):
			_, _ = writer.WriteString("OK\r\n")
		case strings.HasPrefix(cmd, "gsm cancel "):
			_, _ = writer.WriteString("OK\r\n")
		case strings.HasPrefix(cmd, "geo fix "):
			_, _ = writer.WriteString("OK\r\n")
		case strings.HasPrefix(cmd, "network delay "):
			_, _ = writer.WriteString("OK\r\n")
		case strings.HasPrefix(cmd, "network speed "):
			_, _ = writer.WriteString("OK\r\n")
		case strings.HasPrefix(cmd, "power capacity "):
			_, _ = writer.WriteString("OK\r\n")
		case strings.HasPrefix(cmd, "power status "):
			_, _ = writer.WriteString("OK\r\n")
		case strings.HasPrefix(cmd, "power ac "):
			_, _ = writer.WriteString("OK\r\n")
		case cmd == "fail_me":
			_, _ = writer.WriteString("KO: intentional test failure\r\n")
		default:
			_, _ = writer.WriteString("OK\r\n")
		}
		_ = writer.Flush()
	}
}

func TestExtractConsolePort(t *testing.T) {
	tests := []struct {
		serial   string
		expected int
		hasErr   bool
	}{
		{"emulator-5554", 5554, false},
		{"emulator-5556", 5556, false},
		{":5554", 5554, false},
		{"5554", 5554, false},
		{"192.168.1.100:5555", 0, true},
		{"invalid-serial", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		port, err := ExtractConsolePort(tt.serial)
		if tt.hasErr && err == nil {
			t.Errorf("ExtractConsolePort(%q): expected error, got nil", tt.serial)
		}
		if !tt.hasErr && err != nil {
			t.Errorf("ExtractConsolePort(%q): unexpected error: %v", tt.serial, err)
		}
		if !tt.hasErr && port != tt.expected {
			t.Errorf("ExtractConsolePort(%q) = %d, expected %d", tt.serial, port, tt.expected)
		}
	}
}

func TestEmulatorConsoleEndToEnd(t *testing.T) {
	token := "secret_console_token_123"
	port, stop := startMockEmulatorConsole(t, token)
	defer stop()

	serial := fmt.Sprintf("emulator-%d", port)
	client, err := NewEmulatorConsoleClient(serial)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	client.SetAuthToken(token)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Test SMS send
	if err := client.SendSMS(ctx, "+1234567890", "Test OTP: 984210"); err != nil {
		t.Fatalf("SendSMS failed: %v", err)
	}

	// 2. Test GSM Call & Cancel
	if err := client.SimulateCall(ctx, "+1987654321"); err != nil {
		t.Fatalf("SimulateCall failed: %v", err)
	}
	if err := client.CancelCall(ctx, "+1987654321"); err != nil {
		t.Fatalf("CancelCall failed: %v", err)
	}

	// 3. Test Geo Fix
	if err := client.SetGeoLocation(ctx, -122.084, 37.422, 10.0); err != nil {
		t.Fatalf("SetGeoLocation failed: %v", err)
	}

	// 4. Test Network Conditions
	if err := client.SetNetworkDelay(ctx, "lte"); err != nil {
		t.Fatalf("SetNetworkDelay failed: %v", err)
	}
	if err := client.SetNetworkSpeed(ctx, "full"); err != nil {
		t.Fatalf("SetNetworkSpeed failed: %v", err)
	}

	// 5. Test Power Capacity & Status
	if err := client.SetPowerCapacity(ctx, 85); err != nil {
		t.Fatalf("SetPowerCapacity failed: %v", err)
	}
	if err := client.SetPowerStatus(ctx, "charging"); err != nil {
		t.Fatalf("SetPowerStatus failed: %v", err)
	}
	if err := client.SetPowerAC(ctx, "on"); err != nil {
		t.Fatalf("SetPowerAC failed: %v", err)
	}

	// 6. Test Error Handling on KO response
	_, err = client.Exec(ctx, "fail_me")
	if err == nil {
		t.Fatal("expected error on 'fail_me', got nil")
	}
	if !strings.Contains(err.Error(), "intentional test failure") {
		t.Fatalf("expected 'intentional test failure' in error, got: %v", err)
	}

	// 7. Test Parameter Validation Errors
	if err := client.SendSMS(ctx, "", "msg"); err == nil {
		t.Error("expected error on empty sender SMS, got nil")
	}
	if err := client.SimulateCall(ctx, ""); err == nil {
		t.Error("expected error on empty caller SimulateCall, got nil")
	}
	if err := client.CancelCall(ctx, ""); err == nil {
		t.Error("expected error on empty caller CancelCall, got nil")
	}
	if err := client.SetPowerCapacity(ctx, -5); err == nil {
		t.Error("expected error on negative battery capacity, got nil")
	}
	if err := client.SetPowerCapacity(ctx, 105); err == nil {
		t.Error("expected error on battery capacity > 100, got nil")
	}
	if err := client.SetPowerAC(ctx, "maybe"); err == nil {
		t.Error("expected error on invalid AC state, got nil")
	}
	if err := client.SetPowerStatus(ctx, "exploding"); err == nil {
		t.Error("expected error on invalid power status, got nil")
	}
}
