package uitree

import (
	"strconv"
	"strings"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// PruneOptions allows customizing the pruning behavior.
type PruneOptions struct {
	Viewport   Viewport
	MaxTextLen int // maximum characters per text field before truncation (default 256)
}

// Prune compresses a raw uiautomator tree into a compact slice of PrunedNodes.
// It applies the keep-criteria, viewport intersection, and single-child container collapsing rules.
func Prune(hierarchy *RawHierarchy, opts PruneOptions) []types.PrunedNode {
	if hierarchy == nil || len(hierarchy.Nodes) == 0 {
		return nil
	}

	vp := opts.Viewport
	if vp.Width <= 0 || vp.Height <= 0 {
		vp = inferViewport(hierarchy)
	}

	// Step 1: Pre-process nodes - collapse single-child wrapper containers & hoist text to interactive ancestors
	absorbed := make(map[*RawNode]bool)
	hoistedText := make(map[*RawNode]string)
	hoistedDesc := make(map[*RawNode]string)

	for _, root := range hierarchy.Nodes {
		collapseAndHoist(root, nil, absorbed, hoistedText, hoistedDesc, vp)
	}

	// Step 2: Traverse and collect nodes matching keep-criteria
	var keptNodes []*RawNode
	for _, root := range hierarchy.Nodes {
		collectKeptNodes(root, absorbed, &keptNodes, vp)
	}

	// Step 3: Convert to compact types.PrunedNode with sequential IDs
	result := make([]types.PrunedNode, 0, len(keptNodes))
	nodeCounter := 1

	for _, node := range keptNodes {
		text := strings.TrimSpace(node.Text)
		if ht, ok := hoistedText[node]; ok && ht != "" {
			if text == "" {
				text = ht
			} else if !strings.Contains(text, ht) {
				text = text + " | " + ht
			}
		}

		desc := strings.TrimSpace(node.ContentDesc)
		if hd, ok := hoistedDesc[node]; ok && hd != "" {
			if desc == "" {
				desc = hd
			} else if !strings.Contains(desc, hd) {
				desc = desc + " | " + hd
			}
		}

		if opts.MaxTextLen > 0 {
			if len(text) > opts.MaxTextLen {
				text = text[:opts.MaxTextLen]
			}
			if len(desc) > opts.MaxTextLen {
				desc = desc[:opts.MaxTextLen]
			}
		}

		pNode := types.PrunedNode{
			ID:          formatNodeID(nodeCounter),
			ResID:       shortenResourceID(node.ResourceID),
			Class:       shortenClassName(node.Class),
			Text:        text,
			ContentDesc: desc,
			Bounds:      node.ParsedBounds,
			Flags:       extractFlags(node),
		}

		result = append(result, pNode)
		nodeCounter++
	}

	return result
}

// inferViewport attempts to extract display size from the root container node bounds.
func inferViewport(h *RawHierarchy) Viewport {
	if len(h.Nodes) > 0 {
		rootBounds := h.Nodes[0].ParsedBounds
		w := rootBounds[2] - rootBounds[0]
		h := rootBounds[3] - rootBounds[1]
		if w > 0 && h > 0 {
			return Viewport{Width: w, Height: h}
		}
	}
	return DefaultViewport
}

// isInteractive returns true if the node supports user interaction.
func isInteractive(n *RawNode) bool {
	if n == nil {
		return false
	}
	return n.Clickable || n.LongClickable || n.Checkable || n.Scrollable || (n.Editable && n.Focusable)
}

// isVisible returns true if the node bounds have positive area and intersect the viewport.
func isVisible(b [4]int, vp Viewport) bool {
	w := b[2] - b[0]
	h := b[3] - b[1]
	if w <= 0 || h <= 0 {
		return false
	}
	// Intersects screen viewport [0, 0, vp.Width, vp.Height]
	if b[2] <= 0 || b[0] >= vp.Width || b[3] <= 0 || b[1] >= vp.Height {
		return false
	}
	return true
}

// isTagged returns true if the node has a non-empty resource-id or test tag.
func isTagged(n *RawNode) bool {
	if n == nil {
		return false
	}
	return strings.TrimSpace(n.ResourceID) != ""
}

// isLabeled returns true if the node has visible text or content-desc.
func isLabeled(n *RawNode) bool {
	if n == nil {
		return false
	}
	return strings.TrimSpace(n.Text) != "" || strings.TrimSpace(n.ContentDesc) != ""
}

// shouldKeepNode evaluates Core Invention #2 keep-criteria.
func shouldKeepNode(n *RawNode, vp Viewport) bool {
	if n == nil {
		return false
	}
	// Criterion 1: Interactive
	if isInteractive(n) {
		return true
	}
	// Criterion 2: Labeled AND visible bounds intersect screen viewport
	if isLabeled(n) && isVisible(n.ParsedBounds, vp) {
		return true
	}
	// Criterion 3: Tagged AND visible
	if isTagged(n) && isVisible(n.ParsedBounds, vp) {
		return true
	}
	return false
}

// collapseAndHoist implements the collapse rule:
// Single-child wrapper containers (FrameLayout→LinearLayout→TextView) → hoist text up to the nearest interactive ancestor, join with ` | `
func collapseAndHoist(curr *RawNode, nearestInteractive *RawNode, absorbed map[*RawNode]bool, hoistedText, hoistedDesc map[*RawNode]string, vp Viewport) {
	if curr == nil {
		return
	}

	interactiveAncestor := nearestInteractive
	if isInteractive(curr) {
		interactiveAncestor = curr
	}

	// If this node is inside an interactive ancestor, is not itself interactive,
	// and is either a single-child wrapper or a leaf label within the interactive widget:
	if interactiveAncestor != nil && interactiveAncestor != curr {
		// If curr is NOT interactive
		if !isInteractive(curr) {
			hasText := strings.TrimSpace(curr.Text) != ""
			hasDesc := strings.TrimSpace(curr.ContentDesc) != ""

			if hasText || hasDesc {
				// Hoist text and description up to the nearest interactive ancestor
				if hasText {
					t := strings.TrimSpace(curr.Text)
					existing := hoistedText[interactiveAncestor]
					if existing == "" {
						hoistedText[interactiveAncestor] = t
					} else if !strings.Contains(existing, t) {
						hoistedText[interactiveAncestor] = existing + " | " + t
					}
				}
				if hasDesc {
					d := strings.TrimSpace(curr.ContentDesc)
					existing := hoistedDesc[interactiveAncestor]
					if existing == "" {
						hoistedDesc[interactiveAncestor] = d
					} else if !strings.Contains(existing, d) {
						hoistedDesc[interactiveAncestor] = existing + " | " + d
					}
				}
				// Mark as absorbed into interactive parent
				absorbed[curr] = true
			} else if isWrapperContainer(curr) {
				// Empty wrapper container inside interactive widget
				absorbed[curr] = true
			}
		}
	}

	for _, child := range curr.Children {
		collapseAndHoist(child, interactiveAncestor, absorbed, hoistedText, hoistedDesc, vp)
	}
}

// isWrapperContainer returns true if a container node has no semantic data of its own.
func isWrapperContainer(n *RawNode) bool {
	if n == nil {
		return false
	}
	if isInteractive(n) || isTagged(n) || isLabeled(n) {
		return false
	}
	return true
}

// collectKeptNodes recursively traverses the hierarchy, omitting absorbed and non-matching nodes.
func collectKeptNodes(curr *RawNode, absorbed map[*RawNode]bool, result *[]*RawNode, vp Viewport) {
	if curr == nil {
		return
	}

	if !absorbed[curr] && shouldKeepNode(curr, vp) {
		*result = append(*result, curr)
	}

	for _, child := range curr.Children {
		collectKeptNodes(child, absorbed, result, vp)
	}
}

// shortenClassName strips Java/Kotlin package prefixes, leaving only the simple class name.
func shortenClassName(fullClass string) string {
	fullClass = strings.TrimSpace(fullClass)
	if fullClass == "" {
		return "View"
	}
	idx := strings.LastIndex(fullClass, ".")
	if idx != -1 && idx < len(fullClass)-1 {
		fullClass = fullClass[idx+1:]
	}
	// Strip anonymous inner class markers (e.g. MyView$1 -> MyView)
	if dollarIdx := strings.Index(fullClass, "$"); dollarIdx != -1 && dollarIdx > 0 {
		fullClass = fullClass[:dollarIdx]
	}
	return fullClass
}

// shortenResourceID strips the app package and prefix, e.g. "com.example.app:id/btn_block" -> "btn_block",
// and preserves clean Compose testTag identifiers.
func shortenResourceID(resID string) string {
	resID = strings.TrimSpace(resID)
	if resID == "" {
		return ""
	}
	// Compose testTag prefix
	if strings.HasPrefix(resID, "testTag:") {
		return strings.TrimPrefix(resID, "testTag:")
	}
	if idx := strings.Index(resID, ":id/"); idx != -1 {
		return resID[idx+4:]
	}
	if idx := strings.LastIndex(resID, "/"); idx != -1 && idx < len(resID)-1 {
		return resID[idx+1:]
	}
	return resID
}

// extractFlags maps boolean state flags to the compact array format.
func extractFlags(n *RawNode) []string {
	var flags []string
	if n.Clickable {
		flags = append(flags, "click")
	}
	if n.LongClickable {
		flags = append(flags, "longclick")
	}
	if n.Checkable {
		flags = append(flags, "check")
	}
	if n.Scrollable {
		flags = append(flags, "scroll")
	}
	if n.Editable {
		flags = append(flags, "edit")
	}
	if n.Enabled {
		flags = append(flags, "enabled")
	}
	if n.Checked {
		flags = append(flags, "checked")
	}
	if n.Selected {
		flags = append(flags, "selected")
	}
	return flags
}

func formatNodeID(index int) string {
	return "n" + strconv.Itoa(index)
}
