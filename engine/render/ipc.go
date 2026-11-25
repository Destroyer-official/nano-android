package render

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// IPCRequest represents a command request from runtime/MCP callers.
type IPCRequest struct {
	Command  string         `json:"command"` // "render", "diff", "hit_test", "semantics"
	Width    int            `json:"width"`
	Height   int            `json:"height"`
	X        int            `json:"x,omitempty"` // For hit_test
	Y        int            `json:"y,omitempty"` // For hit_test
	Root     any            `json:"root"`
	PrevRoot any            `json:"prevRoot,omitempty"` // For diff
	Quality  int            `json:"quality,omitempty"`
	Lossless bool           `json:"lossless,omitempty"`
}

// IPCResponse represents the structured response returned by the rendering engine.
type IPCResponse struct {
	Status        string          `json:"status"` // "ok" or "error"
	Error         string          `json:"error,omitempty"`
	WebPBase64    string          `json:"webpBase64,omitempty"`
	FrameHash     uint64          `json:"frameHash,omitempty"`
	RenderTimeMs  float64         `json:"renderTimeMs,omitempty"`
	Semantics     []SemanticNode  `json:"semantics,omitempty"`
	HitNode       *ViewNode       `json:"hitNode,omitempty"`
	Diff          *DiffResult     `json:"diff,omitempty"`
}

// ProcessIPCCommand processes a single IPC command using the provided engine.
func ProcessIPCCommand(engine RenderEngine, req *IPCRequest) *IPCResponse {
	if req == nil {
		return &IPCResponse{Status: "error", Error: "nil request"}
	}

	w := req.Width
	if w <= 0 {
		w = 540
	}
	h := req.Height
	if h <= 0 {
		h = 1200
	}

	switch req.Command {
	case "render", "":
		res, err := engine.RenderView(req.Root, w, h)
		if err != nil {
			return &IPCResponse{Status: "error", Error: err.Error()}
		}
		return &IPCResponse{
			Status:       "ok",
			WebPBase64:   res.Base64URI,
			FrameHash:    res.FrameHash,
			RenderTimeMs: float64(res.RenderTime.Microseconds()) / 1000.0,
			Semantics:    res.Semantics,
		}

	case "hit_test":
		if he, ok := engine.(*HeadlessRenderEngine); ok {
			hit, err := he.HitTest(req.Root, w, h, req.X, req.Y)
			if err != nil {
				return &IPCResponse{Status: "error", Error: err.Error()}
			}
			return &IPCResponse{
				Status:  "ok",
				HitNode: hit,
			}
		}
		return &IPCResponse{Status: "error", Error: "engine does not support hit_test"}

	case "diff":
		if he, ok := engine.(*HeadlessRenderEngine); ok {
			diff, err := he.RenderDiff(req.PrevRoot, req.Root, w, h)
			if err != nil {
				return &IPCResponse{Status: "error", Error: err.Error()}
			}
			return &IPCResponse{
				Status:       "ok",
				Diff:         diff,
				WebPBase64:   diff.DiffBase64URI,
				FrameHash:    diff.CurrentHash,
				RenderTimeMs: float64(diff.RenderTime.Microseconds()) / 1000.0,
			}
		}
		return &IPCResponse{Status: "error", Error: "engine does not support diff"}

	default:
		return &IPCResponse{Status: "error", Error: fmt.Sprintf("unknown command: %s", req.Command)}
	}
}

// ServeStdio reads newline-delimited JSON requests from r and writes responses to w.
func ServeStdio(engine RenderEngine, r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	encoder := json.NewEncoder(w)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req IPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			if encErr := encoder.Encode(&IPCResponse{Status: "error", Error: fmt.Sprintf("invalid json: %v", err)}); encErr != nil {
				return encErr
			}
			continue
		}

		resp := ProcessIPCCommand(engine, &req)
		if err := encoder.Encode(resp); err != nil {
			return err
		}
	}
	return scanner.Err()
}
