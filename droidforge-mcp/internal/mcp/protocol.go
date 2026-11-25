package mcp

import (
	"encoding/json"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// MCP Protocol Version supported.
const ProtocolVersion = "2024-11-05"

// JSON-RPC 2.0 error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
	CodePolicyDenied   = -32003
)

// JSONRPCRequest represents an incoming JSON-RPC 2.0 request or notification.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// IsNotification returns true if the request has no ID.
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

// ClientInfo describes the connecting IDE or client agent.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// InitializeParams holds parameters for the initialize method.
type InitializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
	ClientInfo      ClientInfo     `json:"clientInfo,omitempty"`
}

// InitializeResult is the server's response to initialize.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
}

// ServerCapabilities advertises supported MCP features.
type ServerCapabilities struct {
	Tools ToolsCapability `json:"tools"`
}

// ToolsCapability indicates tool support.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged"`
}

// ServerInfo identifies the DroidForge MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ToolSchema represents a tool definition as advertised to MCP clients.
type ToolSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	RiskTier    string         `json:"risk_tier,omitempty"`
}

// ToolsListResult holds the tools list returned by tools/list.
type ToolsListResult struct {
	Tools []ToolSchema `json:"tools"`
}

// ToolCallParams represents parameters for tools/call.
type ToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// TextContent represents text content in a tool call response.
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ToolCallResult represents the MCP return structure for tools/call.
type ToolCallResult struct {
	Content []TextContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// CancelledNotificationParams represents parameters for notifications/cancelled.
type CancelledNotificationParams struct {
	RequestID any    `json:"requestId"`
	Reason    string `json:"reason,omitempty"`
}

// ToToolSchema converts an internal ToolDefinition to an MCP ToolSchema.
func ToToolSchema(def types.ToolDefinition) ToolSchema {
	schema := def.InputSchema
	if schema == nil {
		schema = map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	return ToolSchema{
		Name:        def.Name,
		Description: def.Description,
		InputSchema: schema,
		RiskTier:    def.RiskTier.String(),
	}
}
