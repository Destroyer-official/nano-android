package network_test

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"nano_android/engine/network/mock"
)

func TestMockServerHTTPFaultsAndWebSocketCapture(t *testing.T) {
	srv, err := mock.Start(42)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	srv.AddRoute(mock.StubRoute{
		ID:         "get_profile",
		Method:     "GET",
		ExactPath:  "/v1/profile",
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       `{"name":"nano-user"}`,
	})

	resp, err := http.Get(srv.URL() + "/v1/profile")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "nano-user") {
		t.Fatalf("unexpected HTTP fixture response (%d): %s", resp.StatusCode, string(body))
	}

	// Simulate offline mode
	srv.SetOffline(true)
	offResp, err := http.Get(srv.URL() + "/v1/profile")
	if err != nil {
		t.Fatal(err)
	}
	offResp.Body.Close()
	if offResp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 during offline mode, got %d", offResp.StatusCode)
	}
	srv.SetOffline(false)

	// RFC 6455 WebSocket handshake + frame echo
	host := strings.TrimPrefix(srv.URL(), "http://")
	conn, err := net.Dial("tcp", host)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = fmt.Fprintf(conn, "GET /ws HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n", host)
	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil || !strings.Contains(statusLine, "101") {
		t.Fatalf("expected 101 Switching Protocols, got %q (%v)", statusLine, err)
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil || line == "\r\n" {
			break
		}
	}
	// Send masked "hi" frame
	frame := []byte{0x81, 0x82, 0x11, 0x22, 0x33, 0x44, 'h' ^ 0x11, 'i' ^ 0x22}
	_, _ = conn.Write(frame)
	var reply [4]byte
	if _, err := io.ReadFull(br, reply[:]); err != nil {
		t.Fatalf("failed reading ws echo: %v", err)
	}
	if string(reply[2:4]) != "hi" {
		t.Fatalf("expected ws echo 'hi', got %q", string(reply[2:4]))
	}

	capLog := srv.Captured()
	if len(capLog) != 3 {
		t.Fatalf("expected 3 captured exchanges, got %d", len(capLog))
	}
}
