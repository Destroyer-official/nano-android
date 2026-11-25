package uitree

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// cleanXMLBytes removes invalid XML control characters (ASCII 0x00-0x08, 0x0B, 0x0C, 0x0E-0x1F)
// which Android occasionally includes in malformed accessibility dumps.
func cleanXMLBytes(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for _, b := range data {
		// Valid XML characters: tab (0x09), newline (0x0A), carriage return (0x0D), and >= 0x20
		if b == 0x09 || b == 0x0A || b == 0x0D || b >= 0x20 {
			out = append(out, b)
		} else {
			out = append(out, ' ')
		}
	}
	return out
}

// ParseXML parses raw XML from `adb exec-out uiautomator dump /dev/tty` into a RawHierarchy.
// It handles Android banners, whitespace, dirty control characters, and XML encoding declarations cleanly.
func ParseXML(data []byte) (*RawHierarchy, error) {
	// Clean illegal XML control bytes
	data = cleanXMLBytes(data)

	// Strip any leading non-XML output such as "UI hierachy dumped to: /dev/tty"
	startHierarchy := bytes.Index(data, []byte("<hierarchy"))
	startNode := bytes.Index(data, []byte("<node"))

	start := -1
	if startHierarchy != -1 {
		start = startHierarchy
	} else if startNode != -1 {
		start = startNode
	}

	if start == -1 {
		// Fallback: look for <?xml
		if xmlDecl := bytes.Index(data, []byte("<?xml")); xmlDecl != -1 {
			start = xmlDecl
		} else {
			return nil, fmt.Errorf("no valid XML root found in dump (length: %d bytes)", len(data))
		}
	}

	cleanData := data[start:]

	// Try unmarshaling as RawHierarchy
	var hierarchy RawHierarchy
	err := xml.Unmarshal(cleanData, &hierarchy)
	if err == nil && len(hierarchy.Nodes) > 0 {
		for _, n := range hierarchy.Nodes {
			initNodeRecursive(n)
		}
		return &hierarchy, nil
	}

	// If root was a direct <node> element rather than <hierarchy>
	var singleNode RawNode
	if errNode := xml.Unmarshal(cleanData, &singleNode); errNode == nil && singleNode.Class != "" {
		initNodeRecursive(&singleNode)
		return &RawHierarchy{
			Rotation: 0,
			Nodes:    []*RawNode{&singleNode},
		}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal uiautomator XML: %w", err)
	}

	return &hierarchy, nil
}

// ParseBounds converts bounds strings like "[48,1620][1032,1764]" into [4]int{48, 1620, 1032, 1764}.
func ParseBounds(boundsStr string) ([4]int, error) {
	var bounds [4]int
	if boundsStr == "" {
		return bounds, nil
	}

	// Format: [x1,y1][x2,y2]
	boundsStr = strings.TrimSpace(boundsStr)
	parts := strings.Split(boundsStr, "][")
	if len(parts) != 2 {
		return bounds, fmt.Errorf("invalid bounds format %q", boundsStr)
	}

	p1 := strings.TrimPrefix(parts[0], "[")
	p2 := strings.TrimSuffix(parts[1], "]")

	c1 := strings.Split(p1, ",")
	c2 := strings.Split(p2, ",")
	if len(c1) != 2 || len(c2) != 2 {
		return bounds, fmt.Errorf("invalid coordinate pairs in bounds %q", boundsStr)
	}

	x1, err := strconv.Atoi(strings.TrimSpace(c1[0]))
	if err != nil {
		return bounds, fmt.Errorf("invalid x1 in bounds %q: %w", boundsStr, err)
	}
	y1, err := strconv.Atoi(strings.TrimSpace(c1[1]))
	if err != nil {
		return bounds, fmt.Errorf("invalid y1 in bounds %q: %w", boundsStr, err)
	}
	x2, err := strconv.Atoi(strings.TrimSpace(c2[0]))
	if err != nil {
		return bounds, fmt.Errorf("invalid x2 in bounds %q: %w", boundsStr, err)
	}
	y2, err := strconv.Atoi(strings.TrimSpace(c2[1]))
	if err != nil {
		return bounds, fmt.Errorf("invalid y2 in bounds %q: %w", boundsStr, err)
	}

	// Normalize reversed coordinates if any OEM dump inverted them
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	if y1 > y2 {
		y1, y2 = y2, y1
	}

	bounds[0] = x1
	bounds[1] = y1
	bounds[2] = x2
	bounds[3] = y2
	return bounds, nil
}

// initNodeRecursive parses bounds and normalizes flags throughout the subtree.
func initNodeRecursive(node *RawNode) {
	if node == nil {
		return
	}

	if node.Bounds != "" {
		if b, err := ParseBounds(node.Bounds); err == nil {
			node.ParsedBounds = b
		}
	}

	// Class-based inference for editable:
	// Classes like EditText, AutoCompleteTextView, or Compose TextField
	clsLower := strings.ToLower(node.Class)
	if !node.Editable {
		if strings.Contains(clsLower, "edittext") ||
			strings.Contains(clsLower, "autocompletetextview") ||
			strings.Contains(clsLower, "searchautocomplete") ||
			(strings.Contains(clsLower, "textfield") && node.Focusable) {
			node.Editable = true
		}
	}

	for _, child := range node.Children {
		initNodeRecursive(child)
	}
}

// FlattenAllNodes returns a flat slice of all raw nodes in pre-order traversal.
func FlattenAllNodes(hierarchy *RawHierarchy) []*RawNode {
	var result []*RawNode
	if hierarchy == nil {
		return result
	}
	for _, n := range hierarchy.Nodes {
		result = append(result, collectFlat(n)...)
	}
	return result
}

func collectFlat(n *RawNode) []*RawNode {
	if n == nil {
		return nil
	}
	nodes := []*RawNode{n}
	for _, child := range n.Children {
		nodes = append(nodes, collectFlat(child)...)
	}
	return nodes
}
