package mcp_compiler

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// MCP Protocol Version
const MCPProtocolVersion = "2024-11-05"

// Standard JSON-RPC 2.0 error codes
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// JSONRPCRequest represents an incoming JSON-RPC 2.0 request or notification.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// IsNotification returns true if the message is a notification (no ID).
func (r *JSONRPCRequest) IsNotification() bool {
	return r.ID == nil
}

// JSONRPCResponse represents an outgoing JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      any           `json:"id"`
	Result  any           `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// ClientInfo describes the connecting AI IDE.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// InitializeParams holds parameters for the initialize handshake.
type InitializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
	ClientInfo      ClientInfo     `json:"clientInfo,omitempty"`
}

// InitializeResult is the server response to initialize.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
}

// ServerCapabilities lists supported features.
type ServerCapabilities struct {
	Tools   ToolsCapability   `json:"tools"`
	Logging LoggingCapability `json:"logging,omitempty"`
}

// ToolsCapability indicates tool list capabilities.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged"`
}

// LoggingCapability indicates logging support.
type LoggingCapability struct{}

// ServerInfo identifies this MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ToolsListResult returns the list of tools.
type ToolsListResult struct {
	Tools []ToolSchema `json:"tools"`
}

// ToolSchema represents a tool as advertised to MCP clients.
type ToolSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// ToolCallParams represents parameters for tools/call.
type ToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// CancelledParams represents parameters for notifications/cancelled.
type CancelledParams struct {
	RequestID any    `json:"requestId"`
	Reason    string `json:"reason,omitempty"`
}

// SetLevelParams represents parameters for logging/setLevel.
type SetLevelParams struct {
	Level string `json:"level"`
}

// Server provides the production-ready stdio JSON-RPC 2.0 server for AI IDEs.
type Server struct {
	registry  *ToolRegistry
	config    *Config
	sessionID string
	clientIDE string

	mu        sync.RWMutex
	inFlight  map[string]context.CancelFunc
	logLevel  LogLevel

	writerMu  sync.Mutex
	logMu     sync.Mutex
	logWriter io.Writer

	running   bool
	stopChan  chan struct{}
	wg        sync.WaitGroup
}

// NewServer creates a new MCP Server instance.
func NewServer(registry *ToolRegistry, logWriter io.Writer) *Server {
	return NewServerWithConfig(registry, DefaultConfig(), logWriter)
}

// NewServerWithConfig creates a new MCP Server instance with custom configuration.
func NewServerWithConfig(registry *ToolRegistry, cfg *Config, logWriter io.Writer) *Server {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if logWriter == nil {
		logWriter = os.Stderr
	}

	sessionBytes := make([]byte, 4)
	_, _ = rand.Read(sessionBytes)
	sessID := "nano_" + hex.EncodeToString(sessionBytes)

	return &Server{
		registry:  registry,
		config:    cfg,
		sessionID: sessID,
		clientIDE: "unknown",
		inFlight:  make(map[string]context.CancelFunc),
		logLevel:  cfg.LogLevel,
		logWriter: logWriter,
		stopChan:  make(chan struct{}),
	}
}

// SessionID returns the unique session ID.
func (s *Server) SessionID() string {
	return s.sessionID
}

// SetLogLevel updates the active logging threshold.
func (s *Server) SetLogLevel(lvl LogLevel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logLevel = lvl
}

// Log writes a formatted forensic message to logWriter (stderr) thread-safely.
// It NEVER writes to stdout, keeping JSON-RPC transport pristine.
func (s *Server) Log(level string, format string, args ...any) {
	msgLevel := ParseLogLevel(level)
	s.mu.RLock()
	currentLevel := s.logLevel
	s.mu.RUnlock()

	if msgLevel < currentLevel {
		return
	}

	s.logMu.Lock()
	defer s.logMu.Unlock()
	timestamp := time.Now().UTC().Format("2006-01-02 15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(s.logWriter, "[NANO-MCP] [%s] [%s] %s\n", timestamp, level, msg)
}

// Serve runs the server loop on standard os.Stdin and os.Stdout.
func (s *Server) Serve(ctx context.Context) error {
	return s.ServeIO(ctx, os.Stdin, os.Stdout)
}

// ServeIO runs the server loop over arbitrary reader/writer streams.
func (s *Server) ServeIO(ctx context.Context, r io.Reader, w io.Writer) error {
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	s.Log("INFO", "Server starting session %s (protocol %s)", s.sessionID, MCPProtocolVersion)

	scanner := bufio.NewScanner(r)
	maxSize := s.config.MaxMessageSizeBytes
	if maxSize <= 0 {
		maxSize = 16 * 1024 * 1024
	}
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, maxSize)

	for {
		select {
		case <-ctx.Done():
			s.Log("INFO", "Context cancelled, draining in-flight requests...")
			s.wg.Wait()
			return ctx.Err()
		case <-s.stopChan:
			s.Log("INFO", "Stop requested, draining in-flight requests...")
			s.wg.Wait()
			return nil
		default:
		}

		if !scanner.Scan() {
			s.wg.Wait()
			if err := scanner.Err(); err != nil && err != io.EOF {
				s.Log("ERROR", "Reader scan error: %v", err)
				return fmt.Errorf("read error: %w", err)
			}
			s.Log("INFO", "Reached EOF, server shutting down cleanly.")
			return nil
		}

		line := scanner.Bytes()
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}

		// Handle JSON-RPC 2.0 Batch Requests
		if trimmed[0] == '[' {
			s.handleBatchRequest(ctx, trimmed, w)
			continue
		}

		// Handle Single JSON-RPC 2.0 Request
		var req JSONRPCRequest
		if err := json.Unmarshal(trimmed, &req); err != nil {
			s.Log("ERROR", "Parse error: %v", err)
			s.writeResponse(w, &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error: &JSONRPCError{
					Code:    CodeParseError,
					Message: fmt.Sprintf("Parse error: %v", err),
				},
			})
			continue
		}

		// Handle notifications synchronously
		if req.IsNotification() {
			s.handleNotification(ctx, &req)
			continue
		}

		// Asynchronous execution for tools/call to prevent blocking the read loop
		if req.Method == "tools/call" {
			s.wg.Add(1)
			go func(r JSONRPCRequest) {
				defer s.wg.Done()
				timeout := s.config.RequestTimeout
				if timeout <= 0 {
					timeout = 60 * time.Second
				}
				callCtx, cancel := context.WithTimeout(ctx, timeout)
				s.registerInFlight(r.ID, cancel)
				defer func() {
					s.unregisterInFlight(r.ID)
					cancel()
				}()

				resp := s.handleRequest(callCtx, &r)
				if resp != nil {
					s.writeResponse(w, resp)
				}
			}(req)
			continue
		}

		// Fast control requests (initialize, ping, tools/list, logging/setLevel)
		resp := s.handleRequest(ctx, &req)
		if resp != nil {
			s.writeResponse(w, resp)
		}
	}
}

// handleBatchRequest implements JSON-RPC 2.0 specification Section 6.
func (s *Server) handleBatchRequest(ctx context.Context, data []byte, w io.Writer) {
	var batch []JSONRPCRequest
	if err := json.Unmarshal(data, &batch); err != nil {
		s.writeResponse(w, &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      nil,
			Error: &JSONRPCError{
				Code:    CodeParseError,
				Message: fmt.Sprintf("Invalid batch JSON: %v", err),
			},
		})
		return
	}

	if len(batch) == 0 {
		s.writeResponse(w, &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      nil,
			Error: &JSONRPCError{
				Code:    CodeInvalidRequest,
				Message: "Batch request array cannot be empty",
			},
		})
		return
	}

	responses := make([]*JSONRPCResponse, 0, len(batch))
	var batchMu sync.Mutex
	var batchWg sync.WaitGroup

	for _, req := range batch {
		if req.IsNotification() {
			s.handleNotification(ctx, &req)
			continue
		}

		batchWg.Add(1)
		go func(r JSONRPCRequest) {
			defer batchWg.Done()
			resp := s.handleRequest(ctx, &r)
			if resp != nil {
				batchMu.Lock()
				responses = append(responses, resp)
				batchMu.Unlock()
			}
		}(req)
	}

	batchWg.Wait()

	if len(responses) > 0 {
		s.writerMu.Lock()
		defer s.writerMu.Unlock()
		resData, err := json.Marshal(responses)
		if err == nil {
			resData = append(resData, '\n')
			_, _ = w.Write(resData)
		}
	}
}

func (s *Server) handleRequest(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	switch req.Method {
	case "initialize":
		var params InitializeParams
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &params)
		}
		if params.ClientInfo.Name != "" {
			s.clientIDE = params.ClientInfo.Name
			s.Log("INFO", "Client connected: %s (v%s)", params.ClientInfo.Name, params.ClientInfo.Version)
		}
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: InitializeResult{
				ProtocolVersion: MCPProtocolVersion,
				Capabilities: ServerCapabilities{
					Tools:   ToolsCapability{ListChanged: false},
					Logging: LoggingCapability{},
				},
				ServerInfo: ServerInfo{
					Name:    "nano-android-mcp",
					Version: "1.0.0",
				},
			},
		}

	case "ping":
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{},
		}

	case "logging/setLevel":
		var params SetLevelParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &JSONRPCError{
					Code:    CodeInvalidParams,
					Message: "Invalid logging/setLevel params",
				},
			}
		}
		newLevel := ParseLogLevel(params.Level)
		s.SetLogLevel(newLevel)
		s.Log("INFO", "Log level updated to %s", newLevel.String())
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{},
		}

	case "tools/list":
		tools := s.registry.List()
		schemas := make([]ToolSchema, 0, len(tools))
		for _, t := range tools {
			schemas = append(schemas, ToolSchema{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.InputSchema,
			})
		}
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: ToolsListResult{
				Tools: schemas,
			},
		}

	case "tools/call":
		var params ToolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.Log("WARN", "Invalid tools/call params: %v", err)
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &JSONRPCError{
					Code:    CodeInvalidParams,
					Message: fmt.Sprintf("Invalid parameters: %v", err),
				},
			}
		}

		s.Log("INFO", "Executing tool %s with %d args", params.Name, len(params.Arguments))
		toolResult, err := s.registry.Call(ctx, params.Name, params.Arguments)
		if err != nil {
			s.Log("WARN", "Tool %s execution error: %v", params.Name, err)
			if toolResult != nil {
				return &JSONRPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result:  toolResult,
				}
			}
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &JSONRPCError{
					Code:    CodeInternalError,
					Message: err.Error(),
				},
			}
		}

		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  toolResult,
		}

	default:
		s.Log("WARN", "Unknown method requested: %s", req.Method)
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    CodeMethodNotFound,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
		}
	}
}

func (s *Server) handleNotification(ctx context.Context, req *JSONRPCRequest) {
	switch req.Method {
	case "notifications/initialized":
		s.Log("INFO", "Client initialization acknowledged.")

	case "notifications/cancelled":
		var params CancelledParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &params); err == nil {
				s.cancelInFlight(params.RequestID)
				s.Log("INFO", "Cancelled in-flight request %v", params.RequestID)
			}
		}

	default:
		s.Log("DEBUG", "Received unhandled notification: %s", req.Method)
	}
}

func (s *Server) registerInFlight(id any, cancel context.CancelFunc) {
	if id == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("%v", id)
	s.inFlight[key] = cancel
}

func (s *Server) unregisterInFlight(id any) {
	if id == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("%v", id)
	delete(s.inFlight, key)
}

func (s *Server) cancelInFlight(id any) {
	if id == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("%v", id)
	if cancel, ok := s.inFlight[key]; ok {
		cancel()
		delete(s.inFlight, key)
	}
}

// writeResponse writes a thread-safe JSON-RPC response with newline delimiter.
func (s *Server) writeResponse(w io.Writer, resp *JSONRPCResponse) {
	s.writerMu.Lock()
	defer s.writerMu.Unlock()

	data, err := json.Marshal(resp)
	if err != nil {
		s.Log("ERROR", "Failed to marshal response: %v", err)
		return
	}
	data = append(data, '\n')
	_, _ = w.Write(data)
}
