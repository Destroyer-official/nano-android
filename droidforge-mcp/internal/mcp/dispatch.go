package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// dispatch processes an incoming JSON-RPC request and returns the appropriate response,
// or nil if the request was a notification.
func (s *Server) dispatch(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	// Notifications (no ID) never produce a JSON-RPC response
	if req.IsNotification() {
		s.handleNotification(ctx, req)
		return nil
	}

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "ping":
		return s.handlePing(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	default:
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    CodeMethodNotFound,
				Message: fmt.Sprintf("method not found: %s", req.Method),
			},
		}
	}
}

func (s *Server) handleNotification(ctx context.Context, req *JSONRPCRequest) {
	switch req.Method {
	case "notifications/initialized":
		// MCP client finished handshaking
	case "notifications/cancelled":
		var params CancelledNotificationParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &params); err == nil && params.RequestID != nil {
				s.cancelInFlight(params.RequestID)
			}
		}
	default:
		// Other notifications ignored
	}
}

func (s *Server) handleInitialize(req *JSONRPCRequest) *JSONRPCResponse {
	var params InitializeParams
	if len(req.Params) > 0 {
		_ = json.Unmarshal(req.Params, &params)
	}

	if params.ClientInfo.Name != "" {
		s.SetClientIDE(params.ClientInfo.Name)
	}

	result := InitializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities: ServerCapabilities{
			Tools: ToolsCapability{
				ListChanged: false,
			},
		},
		ServerInfo: ServerInfo{
			Name:    "droidforge-mcp",
			Version: "0.1.0",
		},
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

func (s *Server) handlePing(req *JSONRPCRequest) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]any{},
	}
}

func (s *Server) handleToolsList(req *JSONRPCRequest) *JSONRPCResponse {
	defs := s.ListHandlers()

	// Sort alphabetically for deterministic ordering
	sort.Slice(defs, func(i, j int) bool {
		return defs[i].Name < defs[j].Name
	})

	schemas := make([]ToolSchema, 0, len(defs))
	for _, def := range defs {
		schemas = append(schemas, ToToolSchema(def))
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: ToolsListResult{
			Tools: schemas,
		},
	}
}

func (s *Server) handleToolsCall(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("invalid tool call params: %v", err),
			},
		}
	}

	handler, exists := s.GetHandler(params.Name)
	if !exists {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    CodeMethodNotFound,
				Message: fmt.Sprintf("tool not found: %s", params.Name),
			},
		}
	}

	def := handler.Definition()
	args := params.Arguments
	if args == nil {
		args = make(map[string]any)
	}

	startTime := time.Now()

	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.registerInFlight(req.ID, cancel)
	defer s.unregisterInFlight(req.ID)

	// 1. Policy Guard evaluation before execution
	if s.guard != nil {
		eval, err := s.guard.Evaluate(reqCtx, def, args)
		if err != nil {
			s.recordAudit(startTime, def, args, "DENY", 1, nil, err)
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &JSONRPCError{
					Code:    CodePolicyDenied,
					Message: fmt.Sprintf("policy evaluation error: %v", err),
				},
			}
		}

		if eval != nil && eval.Decision != types.PolicyALLOW {
			decisionStr := eval.Decision.String()
			reason := eval.Reason
			if reason == "" {
				reason = fmt.Sprintf("tool execution rejected with policy decision %s", decisionStr)
			}
			s.recordAudit(startTime, def, args, decisionStr, 1, nil, fmt.Errorf("%s", reason))
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &JSONRPCError{
					Code:    CodePolicyDenied,
					Message: fmt.Sprintf("policy %s: %s", decisionStr, reason),
				},
			}
		}
	}

	// 2. Execute handler wrapped in recovery middleware
	var res *types.ToolResult
	var execErr error

	func() {
		defer func() {
			if r := recover(); r != nil {
				execErr = fmt.Errorf("tool %s panicked: %v", def.Name, r)
			}
		}()
		res, execErr = handler.Execute(reqCtx, args)
	}()

	// If cancelled by client notification
	if reqCtx.Err() == context.Canceled {
		s.recordAudit(startTime, def, args, "CANCELLED", 1, nil, reqCtx.Err())
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: ToolCallResult{
				Content: []TextContent{
					{
						Type: "text",
						Text: fmt.Sprintf("tool %s execution cancelled by client", def.Name),
					},
				},
				IsError: true,
			},
		}
	}

	// 3. Audit logging
	exitCode := 0
	if execErr != nil || (res != nil && !res.Success) {
		exitCode = 1
	}
	s.recordAudit(startTime, def, args, "ALLOW", exitCode, res, execErr)

	// 4. Return result envelope
	if execErr != nil {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: ToolCallResult{
				Content: []TextContent{
					{
						Type: "text",
						Text: fmt.Sprintf("error executing tool %s: %v", def.Name, execErr),
					},
				},
				IsError: true,
			},
		}
	}

	if res == nil {
		res = &types.ToolResult{Success: true}
	}

	resBytes, err := json.Marshal(res)
	text := string(resBytes)
	if err != nil {
		text = fmt.Sprintf(`{"success":%t,"error":%q}`, res.Success, res.Error)
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: ToolCallResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: text,
				},
			},
			IsError: !res.Success,
		},
	}
}

func (s *Server) recordAudit(
	startTime time.Time,
	def types.ToolDefinition,
	args map[string]any,
	decision string,
	exitCode int,
	res *types.ToolResult,
	err error,
) {
	if s.logger == nil {
		return
	}

	s.mu.RLock()
	sessionID := s.sessionID
	clientIDE := s.clientIDE
	s.mu.RUnlock()

	rec := types.AuditRecord{
		Timestamp:    startTime.UTC(),
		SessionID:    sessionID,
		ClientIDE:    clientIDE,
		Tool:         def.Name,
		RiskTier:     def.RiskTier.String(),
		Decision:     decision,
		ExitCode:     exitCode,
		DurationMS:   time.Since(startTime).Milliseconds(),
		ArgsRedacted: redactArgs(args),
	}

	if serial, ok := args["serial"].(string); ok {
		rec.DeviceSerial = serial
	}
	if pkg, ok := args["package_id"].(string); ok {
		rec.PackageLock = pkg
	}

	if res != nil && res.Data != nil {
		if hash, ok := res.Data["ui_hash"].(string); ok {
			rec.UIHashAfter = hash
		}
		if arts, ok := res.Data["artifacts"].([]string); ok {
			rec.Artifacts = arts
		}
	}

	_ = s.logger.Log(rec)
}

func redactArgs(args map[string]any) map[string]any {
	if args == nil {
		return nil
	}
	redacted := make(map[string]any, len(args))
	for k, v := range args {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "password") ||
			strings.Contains(lk, "token") ||
			strings.Contains(lk, "secret") ||
			strings.Contains(lk, "credential") ||
			strings.Contains(lk, "private_key") {
			redacted[k] = "***REDACTED***"
		} else {
			redacted[k] = v
		}
	}
	return redacted
}
