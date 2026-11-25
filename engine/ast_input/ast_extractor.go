package ast_input

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// Viewport defines screen boundary for clipping off-screen elements.
type Viewport struct {
	Width  int
	Height int
}

// DefaultViewport represents standard modern Android phone resolution (1080x2400).
var DefaultViewport = Viewport{Width: 1080, Height: 2400}

// SemanticNode represents an interactive, pruned node in the semantic AST.
type SemanticNode struct {
	ID          string          `json:"id"`
	Class       string          `json:"class"`
	Text        string          `json:"text,omitempty"`
	ContentDesc string          `json:"content_desc,omitempty"`
	Bounds      [4]int          `json:"bounds"`
	Clickable   bool            `json:"clickable,omitempty"`
	Focusable   bool            `json:"focusable,omitempty"`
	Scrollable  bool            `json:"scrollable,omitempty"`
	Selected    bool            `json:"selected,omitempty"`
	Children    []*SemanticNode `json:"children,omitempty"`
}

// SemanticTree represents the extracted UI accessibility tree with metadata.
type SemanticTree struct {
	Root          *SemanticNode   `json:"root,omitempty"`
	Nodes         []*SemanticNode `json:"nodes"`
	TotalRawNodes int             `json:"total_raw_nodes"`
	PrunedCount   int             `json:"pruned_count"`
	TokenEstimate int             `json:"token_estimate"`
}

// ToJSON serializes the SemanticTree to standard JSON.
func (st *SemanticTree) ToJSON() ([]byte, error) {
	return json.Marshal(st)
}

// ToCompactNodesJSON returns the raw interactive nodes as a minified JSON string for LLMs.
func (st *SemanticTree) ToCompactNodesJSON() (string, error) {
	data, err := json.Marshal(st.Nodes)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ToCompactTreeJSON returns the hierarchical pruned tree as a minified JSON string.
func (st *SemanticTree) ToCompactTreeJSON() (string, error) {
	if st.Root != nil {
		data, err := json.Marshal(st.Root)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	return st.ToCompactNodesJSON()
}

// InteractiveNodes returns only nodes that are interactive (clickable, focusable, scrollable, selected, or interactive widget classes).
func (st *SemanticTree) InteractiveNodes() []*SemanticNode {
	var interactive []*SemanticNode
	for _, n := range st.Nodes {
		if n.Clickable || n.Focusable || n.Scrollable || n.Selected || isInteractiveClass(n.Class) {
			interactive = append(interactive, n)
		}
	}
	return interactive
}

// ToCompactInteractiveJSON returns strictly the interactive nodes as minified JSON.
func (st *SemanticTree) ToCompactInteractiveJSON() (string, error) {
	data, err := json.Marshal(st.InteractiveNodes())
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ToPromptText generates an ultra-compact, token-minimal line-based format for LLM prompts.
// Example:
// [@btn_save] Button "Save" [50,580,450,680] clickable
// [@edit_user] EditText "alice" [240,300,800,400] focusable
func (st *SemanticTree) ToPromptText() string {
	var sb strings.Builder
	for _, n := range st.Nodes {
		sb.WriteString(fmt.Sprintf("[@%s] %s", n.ID, n.Class))
		if n.Text != "" {
			sb.WriteString(fmt.Sprintf(" %q", n.Text))
		} else if n.ContentDesc != "" {
			sb.WriteString(fmt.Sprintf(" desc=%q", n.ContentDesc))
		}
		sb.WriteString(fmt.Sprintf(" [%d,%d,%d,%d]", n.Bounds[0], n.Bounds[1], n.Bounds[2], n.Bounds[3]))
		if n.Clickable {
			sb.WriteString(" clickable")
		}
		if n.Focusable {
			sb.WriteString(" focusable")
		}
		if n.Scrollable {
			sb.WriteString(" scrollable")
		}
		if n.Selected {
			sb.WriteString(" selected")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// FindNode finds a SemanticNode by ID across the extracted tree.
func (st *SemanticTree) FindNode(id string) *SemanticNode {
	cleanTarget := CleanID(id)
	for _, n := range st.Nodes {
		if CleanID(n.ID) == cleanTarget || n.ID == id {
			return n
		}
	}
	return nil
}

// ASTExtractor handles traversing UI trees, pruning layout containers, and generating compact ASTs.
type ASTExtractor struct {
	Viewport      Viewport
	ClipOffscreen bool
	FuseLabels    bool
}

// NewASTExtractor creates a new ASTExtractor with default settings.
func NewASTExtractor() *ASTExtractor {
	return &ASTExtractor{
		Viewport:      DefaultViewport,
		ClipOffscreen: true,
		FuseLabels:    true,
	}
}

// Extract traverses the provided root view / layout node and extracts the SemanticTree.
func (ae *ASTExtractor) Extract(rootView any) (*SemanticTree, error) {
	if rootView == nil {
		return nil, fmt.Errorf("rootView is nil")
	}

	st := &SemanticTree{
		Nodes: make([]*SemanticNode, 0),
	}

	switch v := rootView.(type) {
	case *View:
		st.Root = ae.extractView(v, st)
	case *LayoutNode:
		st.Root = ae.extractLayoutNode(v, st)
	case []byte:
		view, err := ParseXMLHierarchy(v)
		if err != nil {
			return nil, err
		}
		st.Root = ae.extractView(view, st)
	case string:
		if strings.Contains(v, "<node") || strings.Contains(v, "<hierarchy") {
			view, err := ParseXMLHierarchy([]byte(v))
			if err != nil {
				return nil, err
			}
			st.Root = ae.extractView(view, st)
		} else {
			return nil, fmt.Errorf("unrecognized string input for rootView")
		}
	default:
		// Attempt extraction via reflection / generic adapter
		st.Root = ae.extractReflective(rootView, st)
	}

	// Semantic fusion: fuse labels into adjacent unlabeled interactive controls
	if ae.FuseLabels {
		ae.fuseAdjacentLabels(st)
	}

	// Compute token estimate on compact nodes JSON
	compactJSON, err := st.ToCompactNodesJSON()
	if err == nil {
		st.TokenEstimate = EstimateTokens(compactJSON)
	}

	return st, nil
}

// extractView extracts and prunes a standard Android View hierarchy.
func (ae *ASTExtractor) extractView(v *View, st *SemanticTree) *SemanticNode {
	if v == nil || !v.IsVisible() {
		return nil
	}

	st.TotalRawNodes++

	v.mu.RLock()
	id := v.ID
	className := v.ClassName
	text := v.Text
	contentDesc := v.ContentDescription
	bounds := v.Bounds
	clickable := v.Clickable || v.OnClickListener != nil
	focusable := v.Focusable
	scrollable := v.Scrollable
	selected := v.Selected
	hasTouchListener := v.OnTouchListener != nil
	children := make([]*View, len(v.Children))
	copy(children, v.Children)
	v.mu.RUnlock()

	// Screen clipping: check zero-area and off-screen bounds
	if isZeroArea(bounds) {
		st.PrunedCount++
		return nil
	}
	if ae.ClipOffscreen && ae.isOffscreen(bounds) {
		st.PrunedCount++
		return nil
	}

	// Recursively process children
	var prunedChildren []*SemanticNode
	for _, child := range children {
		childSemantic := ae.extractView(child, st)
		if childSemantic != nil {
			prunedChildren = append(prunedChildren, childSemantic)
		}
	}

	shortClass := ShortClassName(className)
	isInteractive := clickable || focusable || scrollable || selected || hasTouchListener ||
		isInteractiveClass(shortClass) || (text != "" && isActionableText(shortClass)) || contentDesc != ""

	isLayoutContainer := isGenericLayoutContainer(shortClass)

	// If this is a non-interactive layout container without text/contentDesc:
	if isLayoutContainer && !isInteractive {
		st.PrunedCount++

		if len(prunedChildren) == 0 {
			// Container is completely empty/non-interactive -> prune entirely
			return nil
		}
		if len(prunedChildren) == 1 {
			// Transparent pass-through wrapper -> bypass container
			return prunedChildren[0]
		}
		// Container with multiple children -> preserve grouping
		node := &SemanticNode{
			ID:       id,
			Class:    shortClass,
			Bounds:   bounds,
			Children: prunedChildren,
		}
		return node
	}

	if id == "" {
		id = NextNodeID()
	}

	node := &SemanticNode{
		ID:          CleanID(id),
		Class:       shortClass,
		Text:        text,
		ContentDesc: contentDesc,
		Bounds:      bounds,
		Clickable:   clickable,
		Focusable:   focusable,
		Scrollable:  scrollable,
		Selected:    selected,
		Children:    prunedChildren,
	}

	st.Nodes = append(st.Nodes, node)
	return node
}

// extractLayoutNode extracts and prunes a Jetpack Compose LayoutNode hierarchy.
func (ae *ASTExtractor) extractLayoutNode(n *LayoutNode, st *SemanticTree) *SemanticNode {
	if n == nil {
		return nil
	}

	st.TotalRawNodes++

	n.mu.RLock()
	id := n.ID
	role := n.Role
	text := n.Text
	contentDesc := n.ContentDescription
	bounds := n.Bounds
	clickable := n.Clickable || n.OnClick != nil
	focusable := n.Focusable
	scrollable := n.Scrollable
	selected := n.Selected
	children := make([]*LayoutNode, len(n.Children))
	copy(children, n.Children)
	n.mu.RUnlock()

	if isZeroArea(bounds) {
		st.PrunedCount++
		return nil
	}
	if ae.ClipOffscreen && ae.isOffscreen(bounds) {
		st.PrunedCount++
		return nil
	}

	var prunedChildren []*SemanticNode
	for _, child := range children {
		childSemantic := ae.extractLayoutNode(child, st)
		if childSemantic != nil {
			prunedChildren = append(prunedChildren, childSemantic)
		}
	}

	shortRole := ShortClassName(role)
	if shortRole == "" || shortRole == "LayoutNode" {
		if clickable {
			shortRole = "Button"
		} else if text != "" {
			shortRole = "Text"
		} else {
			shortRole = "Box"
		}
	}

	isInteractive := clickable || focusable || scrollable || selected ||
		isInteractiveClass(shortRole) || (text != "" && isActionableText(shortRole)) || contentDesc != ""

	isLayoutContainer := isGenericLayoutContainer(shortRole)

	if isLayoutContainer && !isInteractive {
		st.PrunedCount++

		if len(prunedChildren) == 0 {
			return nil
		}
		if len(prunedChildren) == 1 {
			return prunedChildren[0]
		}
		node := &SemanticNode{
			ID:       id,
			Class:    shortRole,
			Bounds:   bounds,
			Children: prunedChildren,
		}
		return node
	}

	if id == "" {
		id = NextNodeID()
	}

	node := &SemanticNode{
		ID:          CleanID(id),
		Class:       shortRole,
		Text:        text,
		ContentDesc: contentDesc,
		Bounds:      bounds,
		Clickable:   clickable,
		Focusable:   focusable,
		Scrollable:  scrollable,
		Selected:    selected,
		Children:    prunedChildren,
	}

	st.Nodes = append(st.Nodes, node)
	return node
}

// extractReflective extracts trees using reflection for generic or foreign structs.
func (ae *ASTExtractor) extractReflective(obj any, st *SemanticTree) *SemanticNode {
	val := reflect.ValueOf(obj)
	if !val.IsValid() {
		return nil
	}
	if val.Kind() == reflect.Pointer || val.Kind() == reflect.Interface {
		if val.IsNil() {
			return nil
		}
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil
	}

	st.TotalRawNodes++

	id := getStringField(val, "ID", "Id", "ResourceID", "Name")
	class := getStringField(val, "Class", "ClassName", "Role", "Type")
	text := getStringField(val, "Text", "Title", "Label", "Value")
	contentDesc := getStringField(val, "ContentDescription", "ContentDesc", "Description")
	bounds := getBoundsField(val)

	clickable := getBoolField(val, "Clickable", "IsClickable")
	focusable := getBoolField(val, "Focusable", "IsFocusable")
	scrollable := getBoolField(val, "Scrollable", "IsScrollable")
	selected := getBoolField(val, "Selected", "IsSelected")

	if isZeroArea(bounds) {
		st.PrunedCount++
		return nil
	}
	if ae.ClipOffscreen && ae.isOffscreen(bounds) {
		st.PrunedCount++
		return nil
	}

	// Children
	var prunedChildren []*SemanticNode
	childrenVal := getSliceField(val, "Children", "ChildNodes", "Nodes")
	if childrenVal.IsValid() {
		for i := 0; i < childrenVal.Len(); i++ {
			c := childrenVal.Index(i).Interface()
			childSemantic := ae.extractReflective(c, st)
			if childSemantic != nil {
				prunedChildren = append(prunedChildren, childSemantic)
			}
		}
	}

	shortClass := ShortClassName(class)
	isInteractive := clickable || focusable || scrollable || selected ||
		isInteractiveClass(shortClass) || text != "" || contentDesc != ""

	isLayoutContainer := isGenericLayoutContainer(shortClass)

	if isLayoutContainer && !isInteractive {
		st.PrunedCount++
		if len(prunedChildren) == 0 {
			return nil
		}
		if len(prunedChildren) == 1 {
			return prunedChildren[0]
		}
		return &SemanticNode{
			ID:       id,
			Class:    shortClass,
			Bounds:   bounds,
			Children: prunedChildren,
		}
	}

	if id == "" {
		id = NextNodeID()
	}

	node := &SemanticNode{
		ID:          CleanID(id),
		Class:       shortClass,
		Text:        text,
		ContentDesc: contentDesc,
		Bounds:      bounds,
		Clickable:   clickable,
		Focusable:   focusable,
		Scrollable:  scrollable,
		Selected:    selected,
		Children:    prunedChildren,
	}

	st.Nodes = append(st.Nodes, node)
	return node
}

// isZeroArea checks if width or height is <= 0.
func isZeroArea(b [4]int) bool {
	return b[2] <= b[0] || b[3] <= b[1]
}

// isOffscreen checks if element is completely outside the viewport.
func (ae *ASTExtractor) isOffscreen(b [4]int) bool {
	if ae.Viewport.Width <= 0 || ae.Viewport.Height <= 0 {
		return false
	}
	return b[2] <= 0 || b[3] <= 0 || b[0] >= ae.Viewport.Width || b[1] >= ae.Viewport.Height
}

// fuseAdjacentLabels searches for unlabeled interactive inputs preceded by informative labels and enriches them.
func (ae *ASTExtractor) fuseAdjacentLabels(st *SemanticTree) {
	for i := 0; i < len(st.Nodes); i++ {
		cur := st.Nodes[i]
		// If current is an interactive control without descriptive text
		if (cur.Class == "Switch" || cur.Class == "CheckBox" || cur.Class == "RadioButton" || cur.Class == "EditText") && cur.Text == "" && cur.ContentDesc == "" {
			// Look backwards for the closest preceding TextView
			for j := i - 1; j >= 0; j-- {
				prev := st.Nodes[j]
				if (prev.Class == "TextView" || prev.Class == "Text") && prev.Text != "" {
					// Check vertical proximity (within 150 pixels)
					vDist := cur.Bounds[1] - prev.Bounds[3]
					if vDist >= -100 && vDist <= 150 {
						cur.ContentDesc = prev.Text
						break
					}
				}
			}
		}
	}
}

// ----------------------------------------------------------------------------
// Pruning and Classification Rules
// ----------------------------------------------------------------------------

var genericContainers = map[string]bool{
	"FrameLayout":       true,
	"LinearLayout":      true,
	"RelativeLayout":    true,
	"CoordinatorLayout": true,
	"ConstraintLayout":  true,
	"ViewGroup":         true,
	"ViewStub":          true,
	"Space":             true,
	"Spacer":            true,
	"View":              true,
	"Box":               true,
	"Row":               true,
	"Column":            true,
	"Surface":           true,
	"LayoutNode":        true,
}

func isGenericLayoutContainer(className string) bool {
	return genericContainers[className]
}

var interactiveClasses = map[string]bool{
	"Button":               true,
	"ImageButton":          true,
	"EditText":             true,
	"Switch":               true,
	"CheckBox":             true,
	"RadioButton":          true,
	"ToggleButton":         true,
	"SeekBar":              true,
	"Slider":               true,
	"Spinner":              true,
	"Tab":                  true,
	"TabBar":               true,
	"TextField":            true,
	"FloatingActionButton": true,
	"MenuItem":             true,
	"Chip":                 true,
}

func isInteractiveClass(className string) bool {
	return interactiveClasses[className]
}

func isActionableText(className string) bool {
	return className == "TextView" || className == "Text"
}

// ----------------------------------------------------------------------------
// Token Estimation
// ----------------------------------------------------------------------------

// EstimateTokens calculates an accurate token count for compact JSON payloads.
// Standard LLM BPE tokenizers (OpenAI cl100k, Claude, Gemini SentencePiece) average ~3.8-4.0 characters per token for JSON.
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	tokenCount := (len(text) + 3) / 4
	if tokenCount == 0 {
		return 1
	}
	return tokenCount
}

// ----------------------------------------------------------------------------
// Reflection Helpers
// ----------------------------------------------------------------------------

func getStringField(val reflect.Value, fieldNames ...string) string {
	for _, name := range fieldNames {
		f := val.FieldByName(name)
		if f.IsValid() && f.Kind() == reflect.String {
			return f.String()
		}
	}
	return ""
}

func getBoolField(val reflect.Value, fieldNames ...string) bool {
	for _, name := range fieldNames {
		f := val.FieldByName(name)
		if f.IsValid() && f.Kind() == reflect.Bool {
			return f.Bool()
		}
	}
	return false
}

func getBoundsField(val reflect.Value) [4]int {
	f := val.FieldByName("Bounds")
	if f.IsValid() && (f.Kind() == reflect.Array || f.Kind() == reflect.Slice) && f.Len() >= 4 {
		return [4]int{
			int(f.Index(0).Int()),
			int(f.Index(1).Int()),
			int(f.Index(2).Int()),
			int(f.Index(3).Int()),
		}
	}
	return [4]int{0, 0, 0, 0}
}

func getSliceField(val reflect.Value, fieldNames ...string) reflect.Value {
	for _, name := range fieldNames {
		f := val.FieldByName(name)
		if f.IsValid() && (f.Kind() == reflect.Slice || f.Kind() == reflect.Array) {
			return f
		}
	}
	return reflect.Value{}
}
