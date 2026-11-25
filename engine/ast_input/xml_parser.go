package ast_input

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// RawXMLNode represents an unmarshaled node from Android uiautomator dump XML.
type RawXMLNode struct {
	XMLName       xml.Name      `xml:"node"`
	Index         int           `xml:"index,attr"`
	Text          string        `xml:"text,attr"`
	ResourceID    string        `xml:"resource-id,attr"`
	Class         string        `xml:"class,attr"`
	Package       string        `xml:"package,attr"`
	ContentDesc   string        `xml:"content-desc,attr"`
	Checkable     bool          `xml:"checkable,attr"`
	Checked       bool          `xml:"checked,attr"`
	Clickable     bool          `xml:"clickable,attr"`
	Enabled       bool          `xml:"enabled,attr"`
	Focusable     bool          `xml:"focusable,attr"`
	Focused       bool          `xml:"focused,attr"`
	Scrollable    bool          `xml:"scrollable,attr"`
	LongClickable bool          `xml:"long-clickable,attr"`
	Password      bool          `xml:"password,attr"`
	Selected      bool          `xml:"selected,attr"`
	Bounds        string        `xml:"bounds,attr"`
	Children      []*RawXMLNode `xml:"node"`
}

// RawXMLHierarchy represents the root element of uiautomator dump XML.
type RawXMLHierarchy struct {
	XMLName  xml.Name      `xml:"hierarchy"`
	Rotation int           `xml:"rotation,attr"`
	Nodes    []*RawXMLNode `xml:"node"`
}

// ParseXMLHierarchy parses raw XML bytes (e.g. from uiautomator dump) into a *View hierarchy.
func ParseXMLHierarchy(data []byte) (*View, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty XML data")
	}

	// Trim potential ADB header prefix like "UI hierachy dumped to: /dev/tty\n"
	startIdx := bytes.Index(data, []byte("<hierarchy"))
	if startIdx == -1 {
		startIdx = bytes.Index(data, []byte("<node"))
	}
	if startIdx != -1 {
		data = data[startIdx:]
	}

	// Try unmarshaling as <hierarchy>
	var hierarchy RawXMLHierarchy
	err := xml.Unmarshal(data, &hierarchy)
	if err == nil && len(hierarchy.Nodes) > 0 {
		if len(hierarchy.Nodes) == 1 {
			return convertRawXMLNode(hierarchy.Nodes[0]), nil
		}
		// Virtual root if multiple top-level nodes
		root := NewView("hierarchy_root", "android.widget.FrameLayout", [4]int{0, 0, 1080, 2400})
		for _, n := range hierarchy.Nodes {
			root.AddChild(convertRawXMLNode(n))
		}
		return root, nil
	}

	// Try unmarshaling as direct root <node>
	var singleNode RawXMLNode
	if err2 := xml.Unmarshal(data, &singleNode); err2 == nil && singleNode.Class != "" {
		return convertRawXMLNode(&singleNode), nil
	}

	return nil, fmt.Errorf("failed to parse XML hierarchy: %w", err)
}

// ParseBounds converts "[x1,y1][x2,y2]" into [4]int{x1, y1, x2, y2}.
func ParseBounds(boundsStr string) [4]int {
	var bounds [4]int
	if boundsStr == "" {
		return bounds
	}

	// Expected format: "[x1,y1][x2,y2]"
	boundsStr = strings.TrimSpace(boundsStr)
	parts := strings.Split(boundsStr, "][")
	if len(parts) != 2 {
		return bounds
	}

	leftTop := strings.TrimPrefix(parts[0], "[")
	rightBottom := strings.TrimSuffix(parts[1], "]")

	ltCoords := strings.Split(leftTop, ",")
	rbCoords := strings.Split(rightBottom, ",")

	if len(ltCoords) == 2 && len(rbCoords) == 2 {
		x1, err1 := strconv.Atoi(strings.TrimSpace(ltCoords[0]))
		y1, err2 := strconv.Atoi(strings.TrimSpace(ltCoords[1]))
		x2, err3 := strconv.Atoi(strings.TrimSpace(rbCoords[0]))
		y2, err4 := strconv.Atoi(strings.TrimSpace(rbCoords[1]))
		if err1 == nil && err2 == nil && err3 == nil && err4 == nil {
			bounds[0] = x1
			bounds[1] = y1
			bounds[2] = x2
			bounds[3] = y2
		}
	}

	return bounds
}

// convertRawXMLNode converts a RawXMLNode into a *View.
func convertRawXMLNode(raw *RawXMLNode) *View {
	if raw == nil {
		return nil
	}

	id := raw.ResourceID
	if id == "" {
		id = NextNodeID()
	}

	bounds := ParseBounds(raw.Bounds)

	v := &View{
		ID:                 id,
		ClassName:          raw.Class,
		Text:               raw.Text,
		ContentDescription: raw.ContentDesc,
		Bounds:             bounds,
		Clickable:          raw.Clickable,
		Focusable:          raw.Focusable,
		Scrollable:         raw.Scrollable,
		Selected:           raw.Selected,
		Enabled:            raw.Enabled,
		Checkable:          raw.Checkable,
		Checked:            raw.Checked,
		Focused:            raw.Focused,
		Visibility:         VisibilityVisible,
		Children:           make([]*View, 0, len(raw.Children)),
	}

	for _, child := range raw.Children {
		childView := convertRawXMLNode(child)
		if childView != nil {
			v.AddChild(childView)
		}
	}

	return v
}
