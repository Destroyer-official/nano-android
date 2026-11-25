package ast_input

import (
	"fmt"
	"strings"
)

// NodeDiff captures changes between two states of a single semantic node.
type NodeDiff struct {
	ID          string `json:"id"`
	Class       string `json:"class"`
	OldText     string `json:"old_text,omitempty"`
	NewText     string `json:"new_text,omitempty"`
	StateChange string `json:"state_change,omitempty"`
}

// ASTDiff represents the structural and state differences between two SemanticTrees.
type ASTDiff struct {
	Added       []*SemanticNode `json:"added,omitempty"`
	Removed     []*SemanticNode `json:"removed,omitempty"`
	Modified    []*NodeDiff     `json:"modified,omitempty"`
	Unchanged   int             `json:"unchanged"`
	Summary     string          `json:"summary"`
}

// DiffAST compares an old SemanticTree with a new SemanticTree and produces a concise diff.
func DiffAST(oldTree, newTree *SemanticTree) *ASTDiff {
	diff := &ASTDiff{
		Added:    make([]*SemanticNode, 0),
		Removed:  make([]*SemanticNode, 0),
		Modified: make([]*NodeDiff, 0),
	}

	if oldTree == nil && newTree == nil {
		diff.Summary = "No trees provided"
		return diff
	}
	if oldTree == nil {
		diff.Added = append(diff.Added, newTree.Nodes...)
		diff.Summary = fmt.Sprintf("Screen initialized with %d nodes", len(newTree.Nodes))
		return diff
	}
	if newTree == nil {
		diff.Removed = append(diff.Removed, oldTree.Nodes...)
		diff.Summary = fmt.Sprintf("All %d nodes removed", len(oldTree.Nodes))
		return diff
	}

	oldMap := make(map[string]*SemanticNode, len(oldTree.Nodes))
	for _, n := range oldTree.Nodes {
		oldMap[n.ID] = n
	}

	newMap := make(map[string]*SemanticNode, len(newTree.Nodes))
	for _, n := range newTree.Nodes {
		newMap[n.ID] = n
	}

	// Identify added and modified nodes
	for id, newNode := range newMap {
		oldNode, exists := oldMap[id]
		if !exists {
			diff.Added = append(diff.Added, newNode)
		} else {
			// Check for modifications
			changes := make([]string, 0)
			if oldNode.Text != newNode.Text {
				changes = append(changes, fmt.Sprintf("text: %q -> %q", oldNode.Text, newNode.Text))
			}
			if oldNode.Selected != newNode.Selected {
				changes = append(changes, fmt.Sprintf("selected: %v -> %v", oldNode.Selected, newNode.Selected))
			}
			if oldNode.Clickable != newNode.Clickable {
				changes = append(changes, fmt.Sprintf("clickable: %v -> %v", oldNode.Clickable, newNode.Clickable))
			}

			if len(changes) > 0 {
				diff.Modified = append(diff.Modified, &NodeDiff{
					ID:          id,
					Class:       newNode.Class,
					OldText:     oldNode.Text,
					NewText:     newNode.Text,
					StateChange: strings.Join(changes, ", "),
				})
			} else {
				diff.Unchanged++
			}
		}
	}

	// Identify removed nodes
	for id, oldNode := range oldMap {
		if _, exists := newMap[id]; !exists {
			diff.Removed = append(diff.Removed, oldNode)
		}
	}

	// Generate summary
	var parts []string
	if len(diff.Added) > 0 {
		parts = append(parts, fmt.Sprintf("+%d added", len(diff.Added)))
	}
	if len(diff.Modified) > 0 {
		parts = append(parts, fmt.Sprintf("~%d modified", len(diff.Modified)))
	}
	if len(diff.Removed) > 0 {
		parts = append(parts, fmt.Sprintf("-%d removed", len(diff.Removed)))
	}
	if len(parts) == 0 {
		diff.Summary = fmt.Sprintf("No change (%d identical nodes)", diff.Unchanged)
	} else {
		diff.Summary = strings.Join(parts, ", ") + fmt.Sprintf(" (%d unchanged)", diff.Unchanged)
	}

	return diff
}
