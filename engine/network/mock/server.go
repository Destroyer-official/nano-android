package mock

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// StubRoute configures one deterministic HTTP response fixture.
type StubRoute struct {
	ID              string            `json:"id"`
	Method          string            `json:"method,omitempty"` // "" matches all methods
	PathPrefix      string            `json:"path_prefix"`
	ExactPath       string            `json:"exact_path,omitempty"`
	StatusCode      int               `json:"status_code"`
	Headers         map[string]string `json:"headers,omitempty"`
	Body            string            `json:"body"`
	LatencyMs       int64             `json:"latency_ms,omitempty"`
	DropConnection  bool              `json:"drop_connection,omitempty"`
}

// CapturedExchange records one HTTP/WebSocket transaction observed by the mock server.
type CapturedExchange struct {
	Sequence     int64             `json:"sequence"`
	TimestampUTC string            `json:"timestamp_utc"`
	Method       string            `json:"method"`
	Path         string            `json:"path"`
	Query        string            `json:"query,omitempty"`
	RequestBody  string            `json:"request_body,omitempty"`
	StatusCode   int               `json:"status_code"`
	LatencyMs    int64             `json:"latency_ms"`
	MatchedRule  string            `json:"matched_rule,omitempty"`
	Error        string            `json:"error,omitempty"`
}

// Server is a loopback-only HTTP + WebSocket test fixture server with fault injection and traffic capture.
type Server struct {
	mu             sync.RWMutex
	ln             net.Listener
	httpSrv        *http.Server
	baseURL        string
	routes         []StubRoute
	offline        bool
	packetLossRate float64
	rng            *rand.Rand
	captured       []CapturedExchange
	seq            int64
}

// Start launches the mock HTTP/WS server bound strictly to 127.0.0.1.
func Start(seed int64) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{
		ln:      ln,
		baseURL: "http://" + ln.Addr().String(),
		rng:     rand.New(rand.NewSource(seed)),
	}
	s.httpSrv = &http.Server{
		Handler:           s,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() { _ = s.httpSrv.Serve(ln) }()
	return s, nil
}

// URL returns the loopback base URL (suitable for adb reverse or HTTP client fixtures).
func (s *Server) URL() string {
	return s.baseURL
}

// Close cleanly shuts down the mock server.
func (s *Server) Close() error {
	if s.httpSrv != nil {
		return s.httpSrv.Close()
	}
	return nil
}

// AddRoute appends or updates a route fixture rule (first-match-wins order).
func (s *Server) AddRoute(r StubRoute) {
	if r.StatusCode == 0 {
		r.StatusCode = http.StatusOK
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes = append(s.routes, r)
}

// SetOffline toggles airplane / disconnected network simulation.
func (s *Server) SetOffline(offline bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.offline = offline
}

// SetPacketLossRate sets deterministic packet loss ratio (0.0 .. 1.0).
func (s *Server) SetPacketLossRate(rate float64) {
	if rate < 0 {
		rate = 0
	} else if rate > 1 {
		rate = 1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.packetLossRate = rate
}

// Captured returns a snapshot of all observed HTTP/WebSocket requests.
func (s *Server) Captured() []CapturedExchange {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]CapturedExchange, len(s.captured))
	copy(out, s.captured)
	return out
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bodyBytes, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))

	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		s.handleWebSocket(w, r)
		return
	}

	s.mu.Lock()
	s.seq++
	seq := s.seq
	offline := s.offline
	lossRate := s.packetLossRate
	dropped := lossRate > 0 && s.rng.Float64() < lossRate

	var matched *StubRoute
	for i := range s.routes {
		rt := &s.routes[i]
		if rt.Method != "" && !strings.EqualFold(rt.Method, r.Method) {
			continue
		}
		if rt.ExactPath != "" {
			if r.URL.Path == rt.ExactPath {
				cp := *rt
				matched = &cp
				break
			}
			continue
		}
		if strings.HasPrefix(r.URL.Path, rt.PathPrefix) {
			cp := *rt
			matched = &cp
			break
		}
	}
	s.mu.Unlock()

	ex := CapturedExchange{
		Sequence:     seq,
		TimestampUTC: time.Now().UTC().Format(time.RFC3339Nano),
		Method:       r.Method,
		Path:         r.URL.Path,
		Query:        r.URL.RawQuery,
		RequestBody:  string(bodyBytes),
	}

	if offline || dropped || (matched != nil && matched.DropConnection) {
		reason := "network_offline"
		if dropped {
			reason = "simulated_packet_loss"
		} else if matched != nil && matched.DropConnection {
			reason = "simulated_connection_drop"
		}
		ex.Error = reason
		ex.StatusCode = http.StatusServiceUnavailable
		s.recordExchange(ex)
		if hj, ok := w.(http.Hijacker); ok && (dropped || (matched != nil && matched.DropConnection)) {
			if conn, _, err := hj.Hijack(); err == nil {
				_ = conn.Close()
				return
			}
		}
		http.Error(w, reason, http.StatusServiceUnavailable)
		return
	}

	if matched != nil && matched.LatencyMs > 0 {
		time.Sleep(time.Duration(matched.LatencyMs) * time.Millisecond)
		ex.LatencyMs = matched.LatencyMs
	}

	if matched == nil {
		ex.StatusCode = http.StatusNotFound
		ex.Error = "no matching stub route"
		s.recordExchange(ex)
		http.Error(w, "no matching fixture route", http.StatusNotFound)
		return
	}

	ex.StatusCode = matched.StatusCode
	ex.MatchedRule = matched.ID
	s.recordExchange(ex)

	for k, v := range matched.Headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(matched.StatusCode)
	_, _ = w.Write([]byte(matched.Body))
}

func (s *Server) recordExchange(ex CapturedExchange) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.captured = append(s.captured, ex)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
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

	const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	sum := sha1.Sum([]byte(key + wsGUID))
	accept := base64.StdEncoding.EncodeToString(sum[:])

	resp := fmt.Sprintf("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept)
	_, _ = rw.WriteString(resp)
	_ = rw.Flush()

	s.mu.Lock()
	s.seq++
	s.captured = append(s.captured, CapturedExchange{
		Sequence:     s.seq,
		TimestampUTC: time.Now().UTC().Format(time.RFC3339Nano),
		Method:       "WS_UPGRADE",
		Path:         r.URL.Path,
		StatusCode:   http.StatusSwitchingProtocols,
		MatchedRule:  "ws_echo",
	})
	s.mu.Unlock()

	go func(c net.Conn, br *bufio.Reader) {
		defer c.Close()
		var h [2]byte
		if _, err := io.ReadFull(br, h[:]); err != nil {
			return
		}
		opcode := h[0] & 0x0F
		masked := (h[1] & 0x80) != 0
		pLen := int(h[1] & 0x7F)
		if pLen > 125 || opcode == 0x8 {
			return
		}
		var mask [4]byte
		if masked {
			if _, err := io.ReadFull(br, mask[:]); err != nil {
				return
			}
		}
		payload := make([]byte, pLen)
		if _, err := io.ReadFull(br, payload); err != nil {
			return
		}
		if masked {
			for i := range payload {
				payload[i] ^= mask[i%4]
			}
		}
		// Echo back unmasked text frame
		reply := append([]byte{0x81, byte(len(payload))}, payload...)
		_, _ = c.Write(reply)
	}(conn, rw.Reader)
}
