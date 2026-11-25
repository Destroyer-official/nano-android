package ast_input

import (
	"fmt"
	"reflect"
	"time"
	"unicode"
)

// Standard Android KeyEvent Actions
const (
	KeyActionDown     = 0
	KeyActionUp       = 1
	KeyActionMultiple = 2
)

// Standard Android IME Editor Actions
const (
	EditorActionUnspecified = 0
	EditorActionNone        = 1
	EditorActionGo          = 2
	EditorActionSearch      = 3
	EditorActionSend        = 4
	EditorActionNext        = 5
	EditorActionDone        = 6
	EditorActionPrevious    = 7
)

// Standard Android KeyCodes
const (
	KeyCodeUnknown      = 0
	KeyCodeHome         = 3
	KeyCodeBack         = 4
	KeyCodeCall         = 5
	KeyCodeEndCall      = 6
	KeyCode0            = 7
	KeyCode1            = 8
	KeyCode2            = 9
	KeyCode3            = 10
	KeyCode4            = 11
	KeyCode5            = 12
	KeyCode6            = 13
	KeyCode7            = 14
	KeyCode8            = 15
	KeyCode9            = 16
	KeyCodeStar         = 17
	KeyCodePound        = 18
	KeyCodeDpadUp       = 19
	KeyCodeDpadDown     = 20
	KeyCodeDpadLeft     = 21
	KeyCodeDpadRight    = 22
	KeyCodeDpadCenter   = 23
	KeyCodeVolumeUp     = 24
	KeyCodeVolumeDown   = 25
	KeyCodePower        = 26
	KeyCodeCamera       = 27
	KeyCodeClear        = 28
	KeyCodeA            = 29
	KeyCodeB            = 30
	KeyCodeC            = 31
	KeyCodeD            = 32
	KeyCodeE            = 33
	KeyCodeF            = 34
	KeyCodeG            = 35
	KeyCodeH            = 36
	KeyCodeI            = 37
	KeyCodeJ            = 38
	KeyCodeK            = 39
	KeyCodeL            = 40
	KeyCodeM            = 41
	KeyCodeN            = 42
	KeyCodeO            = 43
	KeyCodeP            = 44
	KeyCodeQ            = 45
	KeyCodeR            = 46
	KeyCodeS            = 47
	KeyCodeT            = 48
	KeyCodeU            = 49
	KeyCodeV            = 50
	KeyCodeW            = 51
	KeyCodeX            = 52
	KeyCodeY            = 53
	KeyCodeZ            = 54
	KeyCodeComma        = 55
	KeyCodePeriod       = 56
	KeyCodeAltLeft      = 57
	KeyCodeAltRight     = 58
	KeyCodeShiftLeft    = 59
	KeyCodeShiftRight   = 60
	KeyCodeTab          = 61
	KeyCodeSpace        = 62
	KeyCodeEnter        = 66
	KeyCodeDel          = 67 // Backspace
	KeyCodeGrave        = 68
	KeyCodeMinus        = 69
	KeyCodeEquals       = 70
	KeyCodeLeftBracket  = 71
	KeyCodeRightBracket = 72
	KeyCodeBackslash    = 73
	KeyCodeSemicolon    = 74
	KeyCodeApostrophe   = 75
	KeyCodeSlash        = 76
	KeyCodeAt           = 77
	KeyCodeEscape       = 111
	KeyCodeForwardDel   = 112
	KeyCodeCtrlLeft     = 113
	KeyCodeCtrlRight    = 114
	KeyCodeAppSwitch    = 187
)

// KeyEvent represents an Android KeyEvent structure.
type KeyEvent struct {
	Action      int    `json:"action"`
	KeyCode     int    `json:"key_code"`
	Characters  string `json:"characters,omitempty"`
	RepeatCount int    `json:"repeat_count"`
	DownTime    int64  `json:"down_time"`
	EventTime   int64  `json:"event_time"`
	MetaState   int    `json:"meta_state"`
}

// KeyInjector handles injecting synthesized KeyEvents, typing text into views,
// and simulating Android InputConnection / IME operations.
type KeyInjector struct {
	CurrentTimeFunc func() int64

	// Optional global Back button handler callback
	OnBackPressedHandler func() bool
}

// NewKeyInjector creates a new KeyInjector.
func NewKeyInjector() *KeyInjector {
	return &KeyInjector{
		CurrentTimeFunc: func() int64 {
			return time.Now().UnixMilli()
		},
	}
}

// RuneToKeyCode maps an ASCII rune to standard Android keycode and meta state.
func RuneToKeyCode(r rune) (int, int) {
	meta := 0
	if unicode.IsUpper(r) {
		meta = 1 // Meta Shift
		r = unicode.ToLower(r)
	}

	if r >= 'a' && r <= 'z' {
		return KeyCodeA + int(r-'a'), meta
	}
	if r >= '0' && r <= '9' {
		return KeyCode0 + int(r-'0'), meta
	}

	switch r {
	case ' ':
		return KeyCodeSpace, 0
	case '\n', '\r':
		return KeyCodeEnter, 0
	case '\t':
		return KeyCodeTab, 0
	case '\b':
		return KeyCodeDel, 0
	case ',':
		return KeyCodeComma, 0
	case '.':
		return KeyCodePeriod, 0
	case '-':
		return KeyCodeMinus, 0
	case '=':
		return KeyCodeEquals, 0
	case '/':
		return KeyCodeSlash, 0
	case '@':
		return KeyCodeAt, 0
	case ';':
		return KeyCodeSemicolon, 0
	case '\'':
		return KeyCodeApostrophe, 0
	case '\\':
		return KeyCodeBackslash, 0
	case '[':
		return KeyCodeLeftBracket, 0
	case ']':
		return KeyCodeRightBracket, 0
	case '`':
		return KeyCodeGrave, 0
	// Shifted symbols
	case '!':
		return KeyCode1, 1
	case '#':
		return KeyCode3, 1
	case '$':
		return KeyCode4, 1
	case '%':
		return KeyCode5, 1
	case '^':
		return KeyCode6, 1
	case '&':
		return KeyCode7, 1
	case '*':
		return KeyCode8, 1
	case '(':
		return KeyCode9, 1
	case ')':
		return KeyCode0, 1
	case '_':
		return KeyCodeMinus, 1
	case '+':
		return KeyCodeEquals, 1
	case '{':
		return KeyCodeLeftBracket, 1
	case '}':
		return KeyCodeRightBracket, 1
	case '|':
		return KeyCodeBackslash, 1
	case ':':
		return KeyCodeSemicolon, 1
	case '"':
		return KeyCodeApostrophe, 1
	case '<':
		return KeyCodeComma, 1
	case '>':
		return KeyCodePeriod, 1
	case '?':
		return KeyCodeSlash, 1
	case '~':
		return KeyCodeGrave, 1
	default:
		return KeyCodeUnknown, 0
	}
}

// Type synthesizes key events and injects text into the target node or focused view.
func (ki *KeyInjector) Type(rootView any, nodeID string, text string) error {
	if rootView == nil {
		return fmt.Errorf("rootView is nil")
	}

	now := ki.CurrentTimeFunc()

	// 1. If target is *View hierarchy
	if vRoot, ok := rootView.(*View); ok {
		var target *View
		if nodeID != "" {
			target = vRoot.FindViewById(nodeID)
		}
		if target == nil {
			target = findFocusedView(vRoot)
		}
		if target == nil && nodeID == "" {
			target = vRoot
		}

		if target == nil {
			return fmt.Errorf("target node %q not found in View hierarchy", nodeID)
		}

		target.RequestFocus()

		for _, r := range text {
			keyCode, meta := RuneToKeyCode(r)
			down := &KeyEvent{
				Action:     KeyActionDown,
				KeyCode:    keyCode,
				Characters: string(r),
				DownTime:   now,
				EventTime:  now,
				MetaState:  meta,
			}
			up := &KeyEvent{
				Action:     KeyActionUp,
				KeyCode:    keyCode,
				Characters: string(r),
				DownTime:   now,
				EventTime:  now + 10,
				MetaState:  meta,
			}
			target.DispatchKeyEvent(down)
			target.DispatchKeyEvent(up)
		}

		current := target.GetText()
		if current == "" {
			target.SetText(text)
		} else if current != text && !hasSubstring(current, text) {
			target.SetText(text)
		}
		return nil
	}

	// 2. If target is *LayoutNode hierarchy (Compose)
	if cRoot, ok := rootView.(*LayoutNode); ok {
		var target *LayoutNode
		if nodeID != "" {
			target = cRoot.FindNodeById(nodeID)
		}
		if target == nil {
			target = findFocusedLayoutNode(cRoot)
		}
		if target == nil && nodeID == "" {
			target = cRoot
		}

		if target == nil {
			return fmt.Errorf("target node %q not found in Compose hierarchy", nodeID)
		}

		target.Focused = true

		for _, r := range text {
			keyCode, meta := RuneToKeyCode(r)
			down := &KeyEvent{
				Action:     KeyActionDown,
				KeyCode:    keyCode,
				Characters: string(r),
				DownTime:   now,
				EventTime:  now,
				MetaState:  meta,
			}
			up := &KeyEvent{
				Action:     KeyActionUp,
				KeyCode:    keyCode,
				Characters: string(r),
				DownTime:   now,
				EventTime:  now + 10,
				MetaState:  meta,
			}
			target.DispatchKeyEvent(down)
			target.DispatchKeyEvent(up)
		}

		target.SetText(text)
		return nil
	}

	return ki.typeReflective(rootView, nodeID, text)
}

// CommitText directly injects text (simulating IME InputConnection.commitText), supporting Unicode and emojis.
func (ki *KeyInjector) CommitText(rootView any, nodeID string, text string) error {
	if rootView == nil {
		return fmt.Errorf("rootView is nil")
	}

	if vRoot, ok := rootView.(*View); ok {
		var target *View
		if nodeID != "" {
			target = vRoot.FindViewById(nodeID)
		} else {
			target = findFocusedView(vRoot)
		}
		if target == nil {
			return fmt.Errorf("target node %q not found", nodeID)
		}
		target.RequestFocus()
		target.SetText(target.GetText() + text)
		return nil
	}

	if cRoot, ok := rootView.(*LayoutNode); ok {
		var target *LayoutNode
		if nodeID != "" {
			target = cRoot.FindNodeById(nodeID)
		} else {
			target = findFocusedLayoutNode(cRoot)
		}
		if target == nil {
			return fmt.Errorf("target node %q not found", nodeID)
		}
		target.Focused = true
		target.SetText(target.Text + text)
		return nil
	}

	return ki.typeReflective(rootView, nodeID, text)
}

// DeleteSurroundingText deletes characters before and after cursor (simulating IME InputConnection).
func (ki *KeyInjector) DeleteSurroundingText(rootView any, targetNodeID string, beforeLength, afterLength int) error {
	if rootView == nil {
		return fmt.Errorf("rootView is nil")
	}

	if vRoot, ok := rootView.(*View); ok {
		var target *View
		if targetNodeID != "" {
			target = vRoot.FindViewById(targetNodeID)
		} else {
			target = findFocusedView(vRoot)
		}
		if target != nil {
			cur := []rune(target.GetText())
			if len(cur) >= beforeLength {
				target.SetText(string(cur[:len(cur)-beforeLength]))
			} else {
				target.SetText("")
			}
			return nil
		}
	}

	if cRoot, ok := rootView.(*LayoutNode); ok {
		var target *LayoutNode
		if targetNodeID != "" {
			target = cRoot.FindNodeById(targetNodeID)
		} else {
			target = findFocusedLayoutNode(cRoot)
		}
		if target != nil {
			cur := []rune(target.Text)
			if len(cur) >= beforeLength {
				target.SetText(string(cur[:len(cur)-beforeLength]))
			} else {
				target.SetText("")
			}
			return nil
		}
	}

	return nil
}

// PerformEditorAction triggers an IME action (e.g. Done, Search, Go, Next).
func (ki *KeyInjector) PerformEditorAction(rootView any, targetNodeID string, actionCode int) error {
	// Synthesize Enter key as standard IME commit behavior
	return ki.PressKey(rootView, KeyCodeEnter)
}

// PressKey synthesizes DOWN and UP KeyEvents for keyCode, handling special keys like Back.
func (ki *KeyInjector) PressKey(rootView any, keyCode int) error {
	if rootView == nil {
		return fmt.Errorf("rootView is nil")
	}

	now := ki.CurrentTimeFunc()
	down := &KeyEvent{
		Action:    KeyActionDown,
		KeyCode:   keyCode,
		DownTime:  now,
		EventTime: now,
	}
	up := &KeyEvent{
		Action:    KeyActionUp,
		KeyCode:   keyCode,
		DownTime:  now,
		EventTime: now + 20,
	}

	if keyCode == KeyCodeBack {
		if ki.OnBackPressedHandler != nil {
			if ki.OnBackPressedHandler() {
				return nil
			}
		}

		v := reflect.ValueOf(rootView)
		m := v.MethodByName("OnBackPressed")
		if m.IsValid() {
			out := m.Call(nil)
			if len(out) > 0 && out[0].Kind() == reflect.Bool && out[0].Bool() {
				return nil
			}
		}
	}

	if vRoot, ok := rootView.(*View); ok {
		focused := findFocusedView(vRoot)
		if focused != nil {
			if keyCode == KeyCodeDel {
				cur := []rune(focused.GetText())
				if len(cur) > 0 {
					focused.SetText(string(cur[:len(cur)-1]))
				}
			}
			focused.DispatchKeyEvent(down)
			focused.DispatchKeyEvent(up)
			return nil
		}
		vRoot.DispatchKeyEvent(down)
		vRoot.DispatchKeyEvent(up)
		return nil
	}

	if cRoot, ok := rootView.(*LayoutNode); ok {
		focused := findFocusedLayoutNode(cRoot)
		if focused != nil {
			if keyCode == KeyCodeDel {
				cur := []rune(focused.Text)
				if len(cur) > 0 {
					focused.SetText(string(cur[:len(cur)-1]))
				}
			}
			focused.DispatchKeyEvent(down)
			focused.DispatchKeyEvent(up)
			return nil
		}
		cRoot.DispatchKeyEvent(down)
		cRoot.DispatchKeyEvent(up)
		return nil
	}

	return ki.dispatchKeyReflective(rootView, down, up)
}

// ----------------------------------------------------------------------------
// Internal Helpers
// ----------------------------------------------------------------------------

func findFocusedView(v *View) *View {
	if v == nil {
		return nil
	}
	v.mu.RLock()
	focused := v.Focused
	children := make([]*View, len(v.Children))
	copy(children, v.Children)
	v.mu.RUnlock()

	if focused {
		return v
	}
	for _, c := range children {
		if found := findFocusedView(c); found != nil {
			return found
		}
	}
	return nil
}

func findFocusedLayoutNode(n *LayoutNode) *LayoutNode {
	if n == nil {
		return nil
	}
	n.mu.RLock()
	focused := n.Focused
	children := make([]*LayoutNode, len(n.Children))
	copy(children, n.Children)
	n.mu.RUnlock()

	if focused {
		return n
	}
	for _, c := range children {
		if found := findFocusedLayoutNode(c); found != nil {
			return found
		}
	}
	return nil
}

func (ki *KeyInjector) typeReflective(rootView any, nodeID, text string) error {
	val := reflect.ValueOf(rootView)
	m := val.MethodByName("SetText")
	if m.IsValid() && m.Type().NumIn() == 1 && m.Type().In(0).Kind() == reflect.String {
		m.Call([]reflect.Value{reflect.ValueOf(text)})
		return nil
	}
	return nil
}

func (ki *KeyInjector) dispatchKeyReflective(rootView any, down, up *KeyEvent) error {
	val := reflect.ValueOf(rootView)
	m := val.MethodByName("DispatchKeyEvent")
	if m.IsValid() {
		m.Call([]reflect.Value{reflect.ValueOf(down)})
		m.Call([]reflect.Value{reflect.ValueOf(up)})
		return nil
	}
	return nil
}

func hasSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
