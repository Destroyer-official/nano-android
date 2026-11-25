package uitree

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// Diff compares current pruned nodes against previous step's nodes and computes
// added, modified, and removed nodes, summarizing unchanged nodes with a counter.
func Diff(prev, curr []types.PrunedNode) DiffResult {
	if len(prev) == 0 {
		return DiffResult{
			Added:          curr,
			UnchangedCount: 0,
			Summary:        fmt.Sprintf("Initial screen with %d nodes", len(curr)),
		}
	}

	// Index previous nodes by stable semantic keys
	type prevEntry struct {
		index int
		node  types.PrunedNode
	}
	prevKeyMap := make(map[string]prevEntry)
	resCounts := make(map[string]int)
	clsBoundsCounts := make(map[string]int)

	for i, p := range prev {
		key := buildSemanticKey(p, resCounts, clsBoundsCounts)
		prevKeyMap[key] = prevEntry{index: i, node: p}
	}

	matchedPrevIndices := make(map[int]bool)
	currResCounts := make(map[string]int)
	currClsBoundsCounts := make(map[string]int)

	var added []types.PrunedNode
	var modified []types.PrunedNode
	unchangedCount := 0

	for _, c := range curr {
		key := buildSemanticKey(c, currResCounts, currClsBoundsCounts)
		if entry, ok := prevKeyMap[key]; ok {
			matchedPrevIndices[entry.index] = true
			p := entry.node

			if isNodeModified(p, c) {
				modified = append(modified, c)
			} else {
				unchangedCount++
			}
		} else {
			added = append(added, c)
		}
	}

	// Any unmatched previous nodes are removed
	var removedIDs []string
	for i, p := range prev {
		if !matchedPrevIndices[i] {
			removedIDs = append(removedIDs, p.ID)
		}
	}

	summary := formatDiffSummary(len(added), len(modified), len(removedIDs), unchangedCount)

	return DiffResult{
		Added:          added,
		Modified:       modified,
		RemovedIDs:     removedIDs,
		UnchangedCount: unchangedCount,
		Summary:        summary,
	}
}

// buildSemanticKey generates a key based on ResID or Class+Bounds with instance counters.
func buildSemanticKey(n types.PrunedNode, resCounts, clsBoundsCounts map[string]int) string {
	if n.ResID != "" {
		cnt := resCounts[n.ResID]
		resCounts[n.ResID] = cnt + 1
		return "res:" + n.ResID + "#" + strconv.Itoa(cnt)
	}

	boundsStr := fmt.Sprintf("[%d,%d,%d,%d]", n.Bounds[0], n.Bounds[1], n.Bounds[2], n.Bounds[3])
	baseKey := n.Class + "|" + boundsStr
	cnt := clsBoundsCounts[baseKey]
	clsBoundsCounts[baseKey] = cnt + 1
	return "cls_b:" + baseKey + "#" + strconv.Itoa(cnt)
}

// isNodeModified checks if any semantic attribute has changed between previous and current node.
func isNodeModified(p, c types.PrunedNode) bool {
	// Compare masked text and description to avoid flagging clock/battery tick differences
	if MaskNoise(p.Text) != MaskNoise(c.Text) {
		return true
	}
	if MaskNoise(p.ContentDesc) != MaskNoise(c.ContentDesc) {
		return true
	}
	if p.Bounds != c.Bounds {
		return true
	}
	if p.Class != c.Class {
		return true
	}
	if !slicesEqual(p.Flags, c.Flags) {
		return true
	}
	return false
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aCopy := slices.Clone(a)
	bCopy := slices.Clone(b)
	slices.Sort(aCopy)
	slices.Sort(bCopy)
	for i := range aCopy {
		if aCopy[i] != bCopy[i] {
			return false
		}
	}
	return true
}

// formatDiffSummary generates human-readable diff summaries like:
// "2 nodes added, 1 changed (14 unchanged omitted)"
func formatDiffSummary(addedCount, modifiedCount, removedCount, unchangedCount int) string {
	if addedCount == 0 && modifiedCount == 0 && removedCount == 0 {
		return fmt.Sprintf("All %d nodes unchanged", unchangedCount)
	}

	var parts []string
	if addedCount > 0 {
		if addedCount == 1 {
			parts = append(parts, "1 node added")
		} else {
			parts = append(parts, fmt.Sprintf("%d nodes added", addedCount))
		}
	}
	if modifiedCount > 0 {
		if modifiedCount == 1 {
			parts = append(parts, "1 changed")
		} else {
			parts = append(parts, fmt.Sprintf("%d changed", modifiedCount))
		}
	}
	if removedCount > 0 {
		if removedCount == 1 {
			parts = append(parts, "1 removed")
		} else {
			parts = append(parts, fmt.Sprintf("%d removed", removedCount))
		}
	}

	summary := strings.Join(parts, ", ")
	if unchangedCount > 0 {
		summary += fmt.Sprintf(" (%d unchanged omitted)", unchangedCount)
	}
	return summary
}
