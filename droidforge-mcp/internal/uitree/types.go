package uitree

import (
	"encoding/xml"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// RawNode represents an unpruned UI node directly unmarshaled from uiautomator dump XML.
type RawNode struct {
	XMLName       xml.Name   `xml:"node"`
	Index         int        `xml:"index,attr"`
	Text          string     `xml:"text,attr"`
	ResourceID    string     `xml:"resource-id,attr"`
	Class         string     `xml:"class,attr"`
	Package       string     `xml:"package,attr"`
	ContentDesc   string     `xml:"content-desc,attr"`
	Checkable     bool       `xml:"checkable,attr"`
	Checked       bool       `xml:"checked,attr"`
	Clickable     bool       `xml:"clickable,attr"`
	Enabled       bool       `xml:"enabled,attr"`
	Focusable     bool       `xml:"focusable,attr"`
	Focused       bool       `xml:"focused,attr"`
	Scrollable    bool       `xml:"scrollable,attr"`
	LongClickable bool       `xml:"long-clickable,attr"`
	Password      bool       `xml:"password,attr"`
	Selected      bool       `xml:"selected,attr"`
	Editable      bool       `xml:"editable,attr"`
	Bounds        string     `xml:"bounds,attr"`
	Children      []*RawNode `xml:"node"`

	// Parsed bounds [left, top, right, bottom]
	ParsedBounds [4]int `xml:"-"`
}

// RawHierarchy represents the root element of uiautomator dump XML.
type RawHierarchy struct {
	XMLName  xml.Name   `xml:"hierarchy"`
	Rotation int        `xml:"rotation,attr"`
	Nodes    []*RawNode `xml:"node"`
}

// DiffResult holds the structural diff between two consecutive UI snapshots.
type DiffResult struct {
	Added          []types.PrunedNode `json:"added,omitempty"`
	Modified       []types.PrunedNode `json:"modified,omitempty"`
	RemovedIDs     []string           `json:"removed_ids,omitempty"`
	UnchangedCount int                `json:"unchanged_count"`
	Summary        string             `json:"summary"`
}

// ActionSpec defines the parameters for an action to be performed in act_and_observe.
type ActionSpec struct {
	Action            string `json:"action"` // tap_node, tap_xy, type, swipe, key, intent
	Target            string `json:"target"` // Node ID (n1), coords (X,Y or X1,Y1,X2,Y2), keycode, or intent URI
	Text              string `json:"text,omitempty"`
	SettleMS          int    `json:"settle_ms,omitempty"`
	IncludeScreenshot bool   `json:"include_screenshot,omitempty"`
}

// SettleResult holds the outcome of running the smart UI settle detector.
type SettleResult struct {
	Snapshot   *types.UITreeSnapshot `json:"snapshot"`
	DurationMS int64                 `json:"duration_ms"`
	Settled    bool                  `json:"settled"`
}

// Viewport defines the active display boundaries for visibility checks.
type Viewport struct {
	Width  int
	Height int
}

// DefaultViewport provides standard 1080x2400 bounds when screen size cannot be inferred.
var DefaultViewport = Viewport{Width: 1080, Height: 2400}
