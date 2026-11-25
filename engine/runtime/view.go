package runtime

import (
	"sync"
)

// Standard View visibility constants
const (
	ViewVisible   = 0
	ViewInvisible = 4
	ViewGone      = 8
)

// View represents the fundamental Android UI building block (and container)
type View struct {
	mu          sync.RWMutex
	id          int
	tag         any
	visibility  int
	enabled     bool
	clickable   bool
	parent      *View
	children    []*View
	context     *Context
	clickAction func(v *View)
	text        string
}

// NewView creates an initialized View
func NewView(ctx *Context, id ...int) *View {
	viewId := 0
	if len(id) > 0 {
		viewId = id[0]
	}
	return &View{
		id:         viewId,
		visibility: ViewVisible,
		enabled:    true,
		clickable:  true,
		context:    ctx,
		children:   make([]*View, 0),
	}
}

// GetId returns view id
func (v *View) GetId() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.id
}

// SetId updates view id
func (v *View) SetId(id int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.id = id
}

// GetVisibility returns visibility flag
func (v *View) GetVisibility() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.visibility
}

// SetVisibility updates visibility
func (v *View) SetVisibility(visibility int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.visibility = visibility
}

// IsEnabled returns enabled flag
func (v *View) IsEnabled() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.enabled
}

// SetEnabled updates enabled flag
func (v *View) SetEnabled(enabled bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.enabled = enabled
}

// SetOnClickListener installs an onClick listener
func (v *View) SetOnClickListener(l func(v *View)) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.clickAction = l
	v.clickable = true
}

// PerformClick simulates a touch/click event on the view
func (v *View) PerformClick() bool {
	v.mu.RLock()
	l := v.clickAction
	enabled := v.enabled
	vis := v.visibility
	v.mu.RUnlock()

	if !enabled || vis != ViewVisible || l == nil {
		return false
	}
	l(v)
	return true
}

// SetTag stores an arbitrary tag
func (v *View) SetTag(tag any) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.tag = tag
}

// GetTag returns the view tag
func (v *View) GetTag() any {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.tag
}

// AddView adds a child view
func (v *View) AddView(child *View) {
	if child == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	child.parent = v
	v.children = append(v.children, child)
}

// RemoveView removes a child view
func (v *View) RemoveView(child *View) {
	if child == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	newChildren := make([]*View, 0, len(v.children))
	for _, c := range v.children {
		if c != child {
			newChildren = append(newChildren, c)
		} else {
			c.parent = nil
		}
	}
	v.children = newChildren
}

// GetChildCount returns number of children
func (v *View) GetChildCount() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return len(v.children)
}

// GetChildAt returns child at index
func (v *View) GetChildAt(index int) *View {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if index >= 0 && index < len(v.children) {
		return v.children[index]
	}
	return nil
}

// SetText sets text on the view
func (v *View) SetText(text string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.text = text
}

// GetText returns text on the view
func (v *View) GetText() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.text
}

// FindViewById recursively searches this view and descendants for matching ID
func (v *View) FindViewById(id int) *View {
	if v.GetId() == id {
		return v
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	for _, child := range v.children {
		if found := child.FindViewById(id); found != nil {
			return found
		}
	}
	return nil
}

// ViewGroup represents a container view
type ViewGroup struct {
	*View
}

// NewViewGroup creates an initialized ViewGroup
func NewViewGroup(ctx *Context, id ...int) *ViewGroup {
	return &ViewGroup{
		View: NewView(ctx, id...),
	}
}

// TextView represents android.widget.TextView
type TextView struct {
	*View
}

// NewTextView creates an initialized TextView
func NewTextView(ctx *Context, id ...int) *TextView {
	return &TextView{
		View: NewView(ctx, id...),
	}
}

// Button represents android.widget.Button
type Button struct {
	*View
}

// NewButton creates an initialized Button
func NewButton(ctx *Context, id ...int) *Button {
	btn := &Button{
		View: NewView(ctx, id...),
	}
	btn.clickable = true
	return btn
}

// SetContentView binds a root view to the Activity
func (a *Activity) SetContentView(view *View) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.contentView = view
}

// GetContentView retrieves the root content view
func (a *Activity) GetContentView() *View {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.contentView
}

// FindViewById searches the activity's content view hierarchy
func (a *Activity) FindViewById(id int) *View {
	a.mu.RLock()
	root := a.contentView
	a.mu.RUnlock()
	if root == nil {
		return nil
	}
	return root.FindViewById(id)
}
