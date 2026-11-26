package mcp

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// Server implements the MCP stdio/streamable transport, tool registry,
// policy guard enforcement, and forensic audit logging pipeline.
type Server struct {
	mu        sync.RWMutex
	handlers  map[string]types.ToolHandler
	guard     types.PolicyGuard
	logger    types.AuditLogger
	config    *types.DroidForgeConfig
	policy    *types.PolicyConfig
	sessionID string
	clientIDE string

	inFlightMu sync.Mutex
	inFlight   map[string]context.CancelFunc

	closersMu sync.Mutex
	closers   []io.Closer

	writerMu sync.Mutex
}

// NewServer constructs an MCP Server initialized with config, policy,
// security guard, and audit logger.
func NewServer(
	cfg *types.DroidForgeConfig,
	pol *types.PolicyConfig,
	guard types.PolicyGuard,
	logger types.AuditLogger,
) *Server {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if pol == nil {
		pol = DefaultPolicy()
	}

	sessionID := generateSessionID()

	return &Server{
		handlers:  make(map[string]types.ToolHandler),
		guard:     guard,
		logger:    logger,
		config:    cfg,
		policy:    pol,
		sessionID: sessionID,
		clientIDE: "unknown",
		inFlight:  make(map[string]context.CancelFunc),
	}
}

func (s *Server) registerInFlight(id any, cancel context.CancelFunc) {
	if id == nil {
		return
	}
	s.inFlightMu.Lock()
	defer s.inFlightMu.Unlock()
	key := fmt.Sprintf("%v", id)
	s.inFlight[key] = cancel
}

func (s *Server) unregisterInFlight(id any) {
	if id == nil {
		return
	}
	s.inFlightMu.Lock()
	defer s.inFlightMu.Unlock()
	key := fmt.Sprintf("%v", id)
	delete(s.inFlight, key)
}

func (s *Server) cancelInFlight(id any) bool {
	if id == nil {
		return false
	}
	s.inFlightMu.Lock()
	defer s.inFlightMu.Unlock()
	key := fmt.Sprintf("%v", id)
	if cancel, ok := s.inFlight[key]; ok {
		cancel()
		delete(s.inFlight, key)
		return true
	}
	return false
}

// SessionID returns the unique session ID of the server.
func (s *Server) SessionID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionID
}

// SetClientIDE updates the client identification label.
func (s *Server) SetClientIDE(ide string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clientIDE = ide
}

// Register registers a tool handler. If a handler with the same name
// was previously registered, it is overwritten.
func (s *Server) Register(h types.ToolHandler) {
	if h == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	def := h.Definition()
	s.handlers[def.Name] = h
}

// GetHandler returns the handler for a given tool name, if present.
func (s *Server) GetHandler(name string) (types.ToolHandler, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.handlers[name]
	return h, ok
}

// ListHandlers returns definitions of all currently registered tools.
func (s *Server) ListHandlers() []types.ToolDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	defs := make([]types.ToolDefinition, 0, len(s.handlers))
	for _, h := range s.handlers {
		defs = append(defs, h.Definition())
	}
	return defs
}

// RegisterCloser registers a resource closer to be invoked when Server.Close runs.
func (s *Server) RegisterCloser(c io.Closer) {
	if c == nil {
		return
	}
	s.closersMu.Lock()
	defer s.closersMu.Unlock()
	s.closers = append(s.closers, c)
}

// Close tears down registered subsystem resources (such as vision streams and sidecars).
func (s *Server) Close() error {
	s.closersMu.Lock()
	closers := make([]io.Closer, len(s.closers))
	copy(closers, s.closers)
	s.closers = nil
	s.closersMu.Unlock()

	var firstErr error
	for i := len(closers) - 1; i >= 0; i-- {
		if err := closers[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Serve starts the MCP server over standard input and output.
func (s *Server) Serve(ctx context.Context) error {
	return s.ServeIO(ctx, os.Stdin, os.Stdout)
}

// ServeIO runs the MCP JSON-RPC 2.0 loop reading from r and writing to w.
func (s *Server) ServeIO(ctx context.Context, r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	// Allow large messages (up to 16MB) for base64 screenshots and logs
	const maxMessageSize = 16 * 1024 * 1024
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, maxMessageSize)

	var wg sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		default:
		}

		if !scanner.Scan() {
			wg.Wait()
			if err := scanner.Err(); err != nil && err != io.EOF {
				return fmt.Errorf("read error: %w", err)
			}
			return nil // Clean EOF
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeResponse(w, &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error: &JSONRPCError{
					Code:    CodeParseError,
					Message: fmt.Sprintf("invalid JSON-RPC: %v", err),
				},
			})
			continue
		}

		// Handle notifications synchronously so cancellations are processed immediately
		if req.IsNotification() {
			s.handleNotification(ctx, &req)
			continue
		}

		// Dispatch tools/call in a goroutine so notifications (such as cancellations)
		// and ping requests are never blocked by long-running tool executions.
		if req.Method == "tools/call" {
			wg.Add(1)
			go func(r JSONRPCRequest) {
				defer wg.Done()
				resp := s.dispatch(ctx, &r)
				if resp != nil {
					s.writeResponse(w, resp)
				}
			}(req)
			continue
		}

		// Fast control requests (initialize, ping, tools/list) are handled synchronously
		// to guarantee strict protocol ordering.
		resp := s.dispatch(ctx, &req)
		if resp != nil {
			s.writeResponse(w, resp)
		}
	}
}

// writeResponse writes a thread-safe JSON-RPC response followed by newline.
func (s *Server) writeResponse(w io.Writer, resp *JSONRPCResponse) {
	s.writerMu.Lock()
	defer s.writerMu.Unlock()

	data, err := json.Marshal(resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[droidforge-mcp] marshal error: %v\n", err)
		return
	}
	data = append(data, '\n')
	_, _ = w.Write(data)
}

func generateSessionID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("sess_%d", time.Now().UnixNano()%1000000)
	}
	return "sess_" + hex.EncodeToString(b)
}
