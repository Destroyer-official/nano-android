package ast_input

import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

var nodeIDCounter uint64

// NextNodeID returns an auto-generated unique node identifier like "node_1".
func NextNodeID() string {
	val := atomic.AddUint64(&nodeIDCounter, 1)
	return "node_" + strconv.FormatUint(val, 10)
}

// Common Android Visibility Constants
const (
	VisibilityVisible   = 0
	VisibilityInvisible = 4
	VisibilityGone      = 8
)

// View represents a native or simulated Android View / ViewGroup.
type View struct {
	mu sync.RWMutex

	ID                 string
	ClassName          string
	Text               string
	ContentDescription string
	Bounds             [4]int // [x1, y1, x2, y2]
	Clickable          bool
	Focusable          bool
	Scrollable         bool
	Selected           bool
	Enabled            bool
	Checkable          bool
	Checked            bool
	Focused            bool
	Visibility         int // 0: VISIBLE, 4: INVISIBLE, 8: GONE
	Tag                any

	Parent   *View
	Children []*View

	// Listeners
	OnClickListener       func(v *View)
	OnLongClickListener   func(v *View) bool
	OnTouchListener       func(v *View, event *MotionEvent) bool
	OnKeyListener         func(v *View, event *KeyEvent) bool
	OnFocusChangeListener func(v *View, hasFocus bool)
	OnTextChangedListener func(v *View, oldText, newText string)

	// Custom action handlers
	OnPerformClick func(v *View) bool

	// Internal state
	isPressed bool
}

// NewView creates a new View with sensible defaults.
func NewView(id, className string, bounds [4]int) *View {
	return &View{
		ID:         id,
		ClassName:  className,
		Bounds:     bounds,
		Enabled:    true,
		Visibility: VisibilityVisible,
		Children:   make([]*View, 0),
	}
}

// AddChild adds a child view and sets its parent pointer.
func (v *View) AddChild(child *View) {
	v.mu.Lock()
	defer v.mu.Unlock()
	child.Parent = v
	v.Children = append(v.Children, child)
}

// RemoveChild removes a child view.
func (v *View) RemoveChild(child *View) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	for i, c := range v.Children {
		if c == child {
			child.Parent = nil
			v.Children = append(v.Children[:i], v.Children[i+1:]...)
			return true
		}
	}
	return false
}

// GetChildren returns a slice copy of children.
func (v *View) GetChildren() []*View {
	v.mu.RLock()
	defer v.mu.RUnlock()
	res := make([]*View, len(v.Children))
	copy(res, v.Children)
	return res
}

// FindViewById recursively searches for a view by ID.
func (v *View) FindViewById(id string) *View {
	v.mu.RLock()
	defer v.mu.RUnlock()

	cleanTarget := CleanID(id)
	if CleanID(v.ID) == cleanTarget || v.ID == id {
		return v
	}

	for _, child := range v.Children {
		if found := child.FindViewById(id); found != nil {
			return found
		}
	}
	return nil
}

// FindViewAt finds the deepest visible, interactive or matching child view containing (x, y).
func (v *View) FindViewAt(x, y int) *View {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if !v.IsVisible() || !v.Contains(x, y) {
		return nil
	}

	// Traverse children in reverse order (topmost/highest z-order first)
	for i := len(v.Children) - 1; i >= 0; i-- {
		child := v.Children[i]
		if target := child.FindViewAt(x, y); target != nil {
			return target
		}
	}

	return v
}

// Contains checks if point (x, y) is within bounds [x1, y1, x2, y2].
func (v *View) Contains(x, y int) bool {
	return x >= v.Bounds[0] && x <= v.Bounds[2] && y >= v.Bounds[1] && y <= v.Bounds[3]
}

// IsVisible returns true if visibility is VISIBLE.
func (v *View) IsVisible() bool {
	return v.Visibility == VisibilityVisible
}

// RequestFocus requests focus for this view.
func (v *View) RequestFocus() bool {
	v.mu.Lock()
	defer v.mu.Unlock()

	if !v.Focusable || !v.Enabled || !v.IsVisible() {
		return false
	}
	if !v.Focused {
		v.Focused = true
		if v.OnFocusChangeListener != nil {
			v.OnFocusChangeListener(v, true)
		}
	}
	return true
}

// ClearFocus clears focus from this view.
func (v *View) ClearFocus() {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.Focused {
		v.Focused = false
		if v.OnFocusChangeListener != nil {
			v.OnFocusChangeListener(v, false)
		}
	}
}

// SetText sets text and triggers OnTextChangedListener.
func (v *View) SetText(text string) {
	v.mu.Lock()
	oldText := v.Text
	v.Text = text
	listener := v.OnTextChangedListener
	v.mu.Unlock()

	if listener != nil && oldText != text {
		listener(v, oldText, text)
	}
}

// GetText returns the current text.
func (v *View) GetText() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.Text
}

// PerformClick executes click actions if enabled and clickable.
func (v *View) PerformClick() bool {
	v.mu.Lock()
	if !v.Enabled {
		v.mu.Unlock()
		return false
	}

	if v.Checkable {
		v.Checked = !v.Checked
	}

	clickListener := v.OnClickListener
	customAction := v.OnPerformClick
	v.mu.Unlock()

	handled := false
	if customAction != nil {
		handled = customAction(v)
	}
	if clickListener != nil {
		clickListener(v)
		handled = true
	}
	return handled
}

// PerformLongClick executes long-click actions.
func (v *View) PerformLongClick() bool {
	v.mu.Lock()
	if !v.Enabled {
		v.mu.Unlock()
		return false
	}
	listener := v.OnLongClickListener
	v.mu.Unlock()

	if listener != nil {
		return listener(v)
	}
	return false
}

// DispatchTouchEvent distributes a MotionEvent down the view hierarchy.
func (v *View) DispatchTouchEvent(event *MotionEvent) bool {
	if event == nil {
		return false
	}

	v.mu.RLock()
	if !v.IsVisible() {
		v.mu.RUnlock()
		return false
	}
	enabled := v.Enabled
	touchListener := v.OnTouchListener
	childCount := len(v.Children)
	v.mu.RUnlock()

	if !enabled {
		return false
	}

	// 1. First check if any child handles the touch event (topmost child first)
	x := int(event.X)
	y := int(event.Y)

	if childCount > 0 {
		v.mu.RLock()
		children := make([]*View, len(v.Children))
		copy(children, v.Children)
		v.mu.RUnlock()

		for i := len(children) - 1; i >= 0; i-- {
			child := children[i]
			if child.IsVisible() && child.Contains(x, y) {
				if child.DispatchTouchEvent(event) {
					return true
				}
			}
		}
	}

	// 2. Check OnTouchListener
	if touchListener != nil {
		if touchListener(v, event) {
			return true
		}
	}

	// 3. Fallback to OnTouchEvent
	return v.OnTouchEvent(event)
}

// OnTouchEvent handles standard view touch behavior (DOWN/UP -> click).
func (v *View) OnTouchEvent(event *MotionEvent) bool {
	v.mu.Lock()
	clickable := v.Clickable || v.OnClickListener != nil
	enabled := v.Enabled
	v.mu.Unlock()

	if !enabled || !clickable {
		return false
	}

	switch event.Action {
	case ActionDown:
		v.mu.Lock()
		v.isPressed = true
		v.mu.Unlock()
		return true

	case ActionMove:
		x := int(event.X)
		y := int(event.Y)
		v.mu.Lock()
		v.isPressed = v.Contains(x, y)
		v.mu.Unlock()
		return true

	case ActionUp:
		v.mu.Lock()
		wasPressed := v.isPressed
		v.isPressed = false
		v.mu.Unlock()

		if wasPressed && v.Contains(int(event.X), int(event.Y)) {
			return v.PerformClick()
		}
		return true

	case ActionCancel:
		v.mu.Lock()
		v.isPressed = false
		v.mu.Unlock()
		return true
	}

	return false
}

// DispatchKeyEvent dispatches a KeyEvent down to focused views or listeners.
func (v *View) DispatchKeyEvent(event *KeyEvent) bool {
	if event == nil {
		return false
	}

	v.mu.RLock()
	keyListener := v.OnKeyListener
	focused := v.Focused
	v.mu.RUnlock()

	// If child is focused, route to focused child
	for _, child := range v.GetChildren() {
		if child.Focused {
			if child.DispatchKeyEvent(event) {
				return true
			}
		}
	}

	// Check this view's key listener
	if keyListener != nil {
		if keyListener(v, event) {
			return true
		}
	}

	// Handle default key event behavior if focused
	if focused {
		return v.OnKeyEvent(event)
	}

	return false
}

// OnKeyEvent handles key events on focused view.
func (v *View) OnKeyEvent(event *KeyEvent) bool {
	if event.Action == KeyActionUp {
		if event.KeyCode == KeyCodeEnter && v.Clickable {
			return v.PerformClick()
		}
		if event.Characters != "" {
			v.SetText(v.GetText() + event.Characters)
			return true
		}
	}
	return false
}

// ----------------------------------------------------------------------------
// Jetpack Compose LayoutNode
// ----------------------------------------------------------------------------

// LayoutNode represents a Jetpack Compose layout or semantics node.
type LayoutNode struct {
	mu sync.RWMutex

	ID                 string
	Role               string // "Button", "Checkbox", "Switch", "Tab", "TextField", "Text", etc.
	Modifier           string
	Text               string
	ContentDescription string
	Bounds             [4]int // [x1, y1, x2, y2]
	Clickable          bool
	Focusable          bool
	Scrollable         bool
	Selected           bool
	Enabled            bool
	Focused            bool

	Children []*LayoutNode
	Parent   *LayoutNode

	// Compose Semantics Actions
	OnClick       func()
	OnLongClick   func() bool
	OnSetText     func(text string)
	OnTouchAction func(event *MotionEvent) bool
	OnKeyAction   func(event *KeyEvent) bool
}

// NewLayoutNode creates a new Compose LayoutNode.
func NewLayoutNode(id, role string, bounds [4]int) *LayoutNode {
	return &LayoutNode{
		ID:       id,
		Role:     role,
		Bounds:   bounds,
		Enabled:  true,
		Children: make([]*LayoutNode, 0),
	}
}

// AddChild adds a child Compose node.
func (n *LayoutNode) AddChild(child *LayoutNode) {
	n.mu.Lock()
	defer n.mu.Unlock()
	child.Parent = n
	n.Children = append(n.Children, child)
}

// GetChildren returns a slice copy of children.
func (n *LayoutNode) GetChildren() []*LayoutNode {
	n.mu.RLock()
	defer n.mu.RUnlock()
	res := make([]*LayoutNode, len(n.Children))
	copy(res, n.Children)
	return res
}

// FindNodeById recursively searches for a Compose node by ID.
func (n *LayoutNode) FindNodeById(id string) *LayoutNode {
	n.mu.RLock()
	defer n.mu.RUnlock()

	cleanTarget := CleanID(id)
	if CleanID(n.ID) == cleanTarget || n.ID == id {
		return n
	}

	for _, child := range n.Children {
		if found := child.FindNodeById(id); found != nil {
			return found
		}
	}
	return nil
}

// FindNodeAt finds the deepest Compose node containing (x, y).
func (n *LayoutNode) FindNodeAt(x, y int) *LayoutNode {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if !n.Contains(x, y) {
		return nil
	}

	for i := len(n.Children) - 1; i >= 0; i-- {
		child := n.Children[i]
		if target := child.FindNodeAt(x, y); target != nil {
			return target
		}
	}
	return n
}

// Contains checks if point is within bounds.
func (n *LayoutNode) Contains(x, y int) bool {
	return x >= n.Bounds[0] && x <= n.Bounds[2] && y >= n.Bounds[1] && y <= n.Bounds[3]
}

// PerformClick executes Compose OnClick callback.
func (n *LayoutNode) PerformClick() bool {
	n.mu.RLock()
	onClick := n.OnClick
	enabled := n.Enabled
	n.mu.RUnlock()

	if !enabled {
		return false
	}
	if onClick != nil {
		onClick()
		return true
	}
	return false
}

// SetText sets text via Compose OnSetText callback or directly.
func (n *LayoutNode) SetText(text string) {
	n.mu.Lock()
	n.Text = text
	onSetText := n.OnSetText
	n.mu.Unlock()

	if onSetText != nil {
		onSetText(text)
	}
}

// DispatchTouchEvent distributes touch events down the Compose node hierarchy.
func (n *LayoutNode) DispatchTouchEvent(event *MotionEvent) bool {
	if event == nil {
		return false
	}

	n.mu.RLock()
	enabled := n.Enabled
	onTouch := n.OnTouchAction
	children := make([]*LayoutNode, len(n.Children))
	copy(children, n.Children)
	n.mu.RUnlock()

	if !enabled {
		return false
	}

	x := int(event.X)
	y := int(event.Y)

	// Hit test children topmost first
	for i := len(children) - 1; i >= 0; i-- {
		child := children[i]
		if child.Contains(x, y) {
			if child.DispatchTouchEvent(event) {
				return true
			}
		}
	}

	if onTouch != nil {
		if onTouch(event) {
			return true
		}
	}

	if (n.Clickable || n.OnClick != nil) && event.Action == ActionUp && n.Contains(x, y) {
		return n.PerformClick()
	}

	return false
}

// DispatchKeyEvent distributes key events to Compose nodes.
func (n *LayoutNode) DispatchKeyEvent(event *KeyEvent) bool {
	if event == nil {
		return false
	}

	n.mu.RLock()
	onKey := n.OnKeyAction
	focused := n.Focused
	n.mu.RUnlock()

	for _, child := range n.GetChildren() {
		if child.Focused {
			if child.DispatchKeyEvent(event) {
				return true
			}
		}
	}

	if onKey != nil {
		if onKey(event) {
			return true
		}
	}

	if focused && event.Action == KeyActionUp {
		if event.KeyCode == KeyCodeEnter && n.Clickable {
			return n.PerformClick()
		}
		if event.Characters != "" {
			n.SetText(n.Text + event.Characters)
			return true
		}
	}

	return false
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

// CleanID normalizes IDs like "@+id/btn_save" or "com.app:id/btn_save" to "btn_save".
func CleanID(id string) string {
	if idx := strings.LastIndex(id, "/"); idx != -1 {
		return id[idx+1:]
	}
	return id
}

// ShortClassName simplifies "android.widget.Button" to "Button", or role if Compose.
func ShortClassName(fullClass string) string {
	if fullClass == "" {
		return "View"
	}
	if idx := strings.LastIndex(fullClass, "."); idx != -1 {
		return fullClass[idx+1:]
	}
	return fullClass
}
