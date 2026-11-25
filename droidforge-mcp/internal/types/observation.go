package types

// PrunedNode is a compact, token-efficient representation of one interactive
// or labeled UI element, pruned from the full Android accessibility tree.
type PrunedNode struct {
	ID          string   `json:"id"`              // Sequential: "n1", "n2", ...
	ResID       string   `json:"res,omitempty"`   // resource-id or Compose testTag
	Class       string   `json:"cls"`             // Short class name: "Button", "EditText", etc.
	Text        string   `json:"text,omitempty"`  // Nonce-fenced visible text
	ContentDesc string   `json:"desc,omitempty"`  // Nonce-fenced content description
	Bounds      [4]int   `json:"bounds"`          // [left, top, right, bottom]
	Flags       []string `json:"flags,omitempty"` // "click", "longclick", "check", "scroll", "edit", "enabled", "checked", "selected"
}

// ObservationResult is the compact structured response from observe_screen
// and act_and_observe, designed to minimize LLM token consumption.
type ObservationResult struct {
	StepID             int          `json:"step_id"`
	DeviceID           string       `json:"device_id"`
	PackageLock        string       `json:"package_lock"`
	Activity           string       `json:"activity"`
	SettleMS           int64        `json:"settle_ms,omitempty"`
	UIHash             string       `json:"ui_hash"`
	UIDiffSummary      string       `json:"ui_diff_summary,omitempty"`
	Nonce              string       `json:"untrusted_device_data_nonce"`
	Nodes              []PrunedNode `json:"nodes"`
	LogcatDelta        []string     `json:"logcat_errors_delta,omitempty"`
	ScreenshotB64      string       `json:"screenshot_b64,omitempty"`
	ScreenshotAttached bool         `json:"screenshot_attached"`
	ScreenshotReason   string       `json:"screenshot_reason,omitempty"`
	ActionApplied      string       `json:"action_applied,omitempty"`
	StuckWarning       bool         `json:"stuck_warning,omitempty"`
}

// UITreeSnapshot holds a full pruned UI tree with its structural hash.
type UITreeSnapshot struct {
	Nodes  []PrunedNode `json:"nodes"`
	Hash   string       `json:"ui_hash"`
	RawLen int          `json:"raw_xml_bytes"` // Original XML size for compression metrics
}

// VisionBackend abstracts frame capture so scrcpy can be swapped for screencap.
type VisionBackend interface {
	// GrabFrame captures the current screen and returns compressed image bytes.
	GrabFrame(serial string, maxHeight int, format string) ([]byte, error)
	// IsStreaming returns true if a live frame stream is active for this device.
	IsStreaming(serial string) bool
	// Teardown stops the frame stream and cleans up for a device.
	Teardown(serial string) error
}

// UITreeProvider abstracts UI tree acquisition (uiautomator vs UiAutomation harness).
type UITreeProvider interface {
	// Dump acquires and prunes the current UI tree.
	Dump(serial string) (*UITreeSnapshot, error)
}

// SessionStep records one action+observation for the explore-to-freeze compiler.
type SessionStep struct {
	StepIndex    int          `json:"step_index"`
	Action       string       `json:"action"`         // "tap_node", "type", "swipe", "key", "intent", "observe"
	Target       string       `json:"target"`         // Node ID, coordinates, or intent URI
	Text         string       `json:"text,omitempty"` // Typed text content
	UIHashBefore string       `json:"ui_hash_before"`
	UIHashAfter  string       `json:"ui_hash_after"`
	NodesAfter   []PrunedNode `json:"nodes_after,omitempty"`
	LogcatDelta  []string     `json:"logcat_delta,omitempty"`
	Success      bool         `json:"success"`
	TimestampMS  int64        `json:"timestamp_ms"`
}
