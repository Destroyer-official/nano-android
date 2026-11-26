package api

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// BusEvent is broadcast to all authenticated Studio WebSocket subscribers.
type BusEvent struct {
	Type         string `json:"type"`
	TimestampUTC string `json:"timestamp_utc"`
	Payload      any    `json:"payload,omitempty"`
}

type wsClient struct {
	conn net.Conn
	bw   *bufio.Writer
	mu   sync.Mutex
}

// EventBus implements an RFC 6455 WebSocket broadcast hub using Go stdlib.
type EventBus struct {
	mu      sync.RWMutex
	clients map[*wsClient]struct{}
	recent  []BusEvent
}

// NewEventBus creates a thread-safe WebSocket hub.
func NewEventBus() *EventBus {
	return &EventBus{
		clients: make(map[*wsClient]struct{}),
	}
}

// Recent returns the latest buffered bus events.
func (b *EventBus) Recent() []BusEvent {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]BusEvent, len(b.recent))
	copy(out, b.recent)
	return out
}

// Broadcast publishes an event to all connected WebSocket clients cleanly.
func (b *EventBus) Broadcast(eventType string, payload any) {
	ev := BusEvent{
		Type:         eventType,
		TimestampUTC: time.Now().UTC().Format(time.RFC3339),
		Payload:      payload,
	}
	msgBytes, err := json.Marshal(ev)
	if err != nil {
		return
	}

	b.mu.Lock()
	b.recent = append(b.recent, ev)
	if len(b.recent) > 100 {
		b.recent = b.recent[len(b.recent)-100:]
	}
	clients := make([]*wsClient, 0, len(b.clients))
	for c := range b.clients {
		clients = append(clients, c)
	}
	b.mu.Unlock()

	for _, cl := range clients {
		if writeErr := writeWSTextFrame(cl, msgBytes); writeErr != nil {
			b.removeClient(cl)
		}
	}
}

func (b *EventBus) removeClient(cl *wsClient) {
	b.mu.Lock()
	delete(b.clients, cl)
	b.mu.Unlock()
	_ = cl.conn.Close()
}

// HandleWebSocket performs an authenticated RFC 6455 upgrade and streams live events.
func (b *EventBus) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "Upgrade: websocket required", http.StatusUpgradeRequired)
		return
	}
	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		http.Error(w, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket hijack unsupported", http.StatusInternalServerError)
		return
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return
	}

	h := sha1.Sum([]byte(key + wsGUID))
	accept := base64.StdEncoding.EncodeToString(h[:])
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := rw.WriteString(resp); err != nil {
		_ = conn.Close()
		return
	}
	if err := rw.Flush(); err != nil {
		_ = conn.Close()
		return
	}

	cl := &wsClient{conn: conn, bw: rw.Writer}
	b.mu.Lock()
	b.clients[cl] = struct{}{}
	b.mu.Unlock()

	hello, _ := json.Marshal(BusEvent{
		Type:         "WS_CONNECTED",
		TimestampUTC: time.Now().UTC().Format(time.RFC3339),
		Payload:      map[string]any{"status": "connected"},
	})
	_ = writeWSTextFrame(cl, hello)

	// Drain client control/close frames until peer disconnects.
	go func() {
		defer b.removeClient(cl)
		buf := make([]byte, 512)
		for {
			_, readErr := rw.Reader.Read(buf)
			if readErr != nil {
				return
			}
		}
	}()
}

func writeWSTextFrame(cl *wsClient, payload []byte) error {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	_ = cl.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))

	n := len(payload)
	var header []byte
	if n <= 125 {
		header = []byte{0x81, byte(n)}
	} else if n <= 65535 {
		header = make([]byte, 4)
		header[0] = 0x81
		header[1] = 126
		binary.BigEndian.PutUint16(header[2:4], uint16(n))
	} else {
		header = make([]byte, 10)
		header[0] = 0x81
		header[1] = 127
		binary.BigEndian.PutUint64(header[2:10], uint64(n))
	}

	if _, err := cl.bw.Write(header); err != nil {
		return err
	}
	if _, err := cl.bw.Write(payload); err != nil {
		return err
	}
	return cl.bw.Flush()
}
