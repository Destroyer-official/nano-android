package ast_input

import (
	"fmt"
	"math"
	"reflect"
	"time"
)

// Standard Android MotionEvent actions
const (
	ActionDown         = 0
	ActionUp           = 1
	ActionMove         = 2
	ActionCancel       = 3
	ActionOutside      = 4
	ActionPointerDown  = 5
	ActionPointerUp    = 6
	ActionHoverMove    = 7
	ActionScroll       = 8
	ActionHoverEnter   = 9
	ActionHoverExit    = 10
)

// PointerCoord represents coordinates and pressure for a single pointer.
type PointerCoord struct {
	X        float32 `json:"x"`
	Y        float32 `json:"y"`
	Pressure float32 `json:"pressure"`
	Size     float32 `json:"size"`
}

// MotionEvent represents an Android MotionEvent structure.
type MotionEvent struct {
	Action       int            `json:"action"`
	DownTime     int64          `json:"down_time"`
	EventTime    int64          `json:"event_time"`
	X            float32        `json:"x"`
	Y            float32        `json:"y"`
	Pressure     float32        `json:"pressure"`
	Size         float32        `json:"size"`
	MetaState    int            `json:"meta_state"`
	ScrollX      float32        `json:"scroll_x,omitempty"`
	ScrollY      float32        `json:"scroll_y,omitempty"`
	PointerCount int            `json:"pointer_count,omitempty"`
	Pointers     []PointerCoord `json:"pointers,omitempty"`
}

// TouchSynthesizer handles converting high-level touch gestures into MotionEvent sequences
// and dispatching them into Android Views, Compose nodes, or root window handlers.
type TouchSynthesizer struct {
	// CurrentTimeFunc allows mocking time in unit tests
	CurrentTimeFunc func() int64
}

// NewTouchSynthesizer creates a new TouchSynthesizer.
func NewTouchSynthesizer() *TouchSynthesizer {
	return &TouchSynthesizer{
		CurrentTimeFunc: func() int64 {
			return time.Now().UnixMilli()
		},
	}
}

// SynthesizeTap produces a DOWN -> UP event sequence.
func (ts *TouchSynthesizer) SynthesizeTap(x, y int, durationMs int) []*MotionEvent {
	if durationMs <= 0 {
		durationMs = 30
	}
	now := ts.CurrentTimeFunc()
	fx := float32(x)
	fy := float32(y)

	return []*MotionEvent{
		{
			Action:    ActionDown,
			DownTime:  now,
			EventTime: now,
			X:         fx,
			Y:         fy,
			Pressure:  1.0,
			Size:      1.0,
		},
		{
			Action:    ActionUp,
			DownTime:  now,
			EventTime: now + int64(durationMs),
			X:         fx,
			Y:         fy,
			Pressure:  0.0,
			Size:      1.0,
		},
	}
}

// SynthesizeLongPress produces a DOWN -> (wait >= 500ms) -> UP sequence.
func (ts *TouchSynthesizer) SynthesizeLongPress(x, y int, durationMs int) []*MotionEvent {
	if durationMs <= 0 {
		durationMs = 600 // Standard Android long press threshold is 500ms
	}
	now := ts.CurrentTimeFunc()
	fx := float32(x)
	fy := float32(y)

	return []*MotionEvent{
		{
			Action:    ActionDown,
			DownTime:  now,
			EventTime: now,
			X:         fx,
			Y:         fy,
			Pressure:  1.0,
			Size:      1.0,
		},
		{
			Action:    ActionUp,
			DownTime:  now,
			EventTime: now + int64(durationMs),
			X:         fx,
			Y:         fy,
			Pressure:  0.0,
			Size:      1.0,
		},
	}
}

// SynthesizeSwipe produces DOWN -> intermediate MOVEs -> UP.
func (ts *TouchSynthesizer) SynthesizeSwipe(x1, y1, x2, y2 int, durationMs int) []*MotionEvent {
	if durationMs <= 0 {
		durationMs = 250
	}

	steps := durationMs / 16
	if steps < 5 {
		steps = 5
	}
	if steps > 50 {
		steps = 50
	}

	now := ts.CurrentTimeFunc()
	events := make([]*MotionEvent, 0, steps+2)

	// ACTION_DOWN
	events = append(events, &MotionEvent{
		Action:    ActionDown,
		DownTime:  now,
		EventTime: now,
		X:         float32(x1),
		Y:         float32(y1),
		Pressure:  1.0,
		Size:      1.0,
	})

	// Interpolated ACTION_MOVE
	stepInterval := int64(durationMs) / int64(steps)
	for i := 1; i <= steps; i++ {
		t := float32(i) / float32(steps)
		cx := float32(x1) + t*float32(x2-x1)
		cy := float32(y1) + t*float32(y2-y1)
		eventTime := now + int64(i)*stepInterval

		events = append(events, &MotionEvent{
			Action:    ActionMove,
			DownTime:  now,
			EventTime: eventTime,
			X:         cx,
			Y:         cy,
			Pressure:  1.0,
			Size:      1.0,
		})
	}

	// ACTION_UP
	events = append(events, &MotionEvent{
		Action:    ActionUp,
		DownTime:  now,
		EventTime: now + int64(durationMs),
		X:         float32(x2),
		Y:         float32(y2),
		Pressure:  0.0,
		Size:      1.0,
	})

	return events
}

// SynthesizeSmoothSwipe uses a cubic ease-in-out curve for natural physical acceleration/deceleration.
func (ts *TouchSynthesizer) SynthesizeSmoothSwipe(x1, y1, x2, y2 int, durationMs int) []*MotionEvent {
	if durationMs <= 0 {
		durationMs = 300
	}

	steps := durationMs / 16
	if steps < 10 {
		steps = 10
	}

	now := ts.CurrentTimeFunc()
	events := make([]*MotionEvent, 0, steps+2)

	events = append(events, &MotionEvent{
		Action:    ActionDown,
		DownTime:  now,
		EventTime: now,
		X:         float32(x1),
		Y:         float32(y1),
		Pressure:  1.0,
		Size:      1.0,
	})

	stepInterval := int64(durationMs) / int64(steps)
	for i := 1; i <= steps; i++ {
		linearT := float64(i) / float64(steps)
		// Smooth cubic ease-in-out: 3t^2 - 2t^3
		t := float32(3*linearT*linearT - 2*linearT*linearT*linearT)

		cx := float32(x1) + t*float32(x2-x1)
		cy := float32(y1) + t*float32(y2-y1)
		eventTime := now + int64(i)*stepInterval

		events = append(events, &MotionEvent{
			Action:    ActionMove,
			DownTime:  now,
			EventTime: eventTime,
			X:         cx,
			Y:         cy,
			Pressure:  1.0,
			Size:      1.0,
		})
	}

	events = append(events, &MotionEvent{
		Action:    ActionUp,
		DownTime:  now,
		EventTime: now + int64(durationMs),
		X:         float32(x2),
		Y:         float32(y2),
		Pressure:  0.0,
		Size:      1.0,
	})

	return events
}

// SynthesizeFling produces a high-velocity flick gesture that triggers momentum scrolling.
func (ts *TouchSynthesizer) SynthesizeFling(x1, y1, x2, y2 int, durationMs int) []*MotionEvent {
	if durationMs <= 0 {
		durationMs = 120 // Short duration imparts high velocity
	}
	return ts.SynthesizeSwipe(x1, y1, x2, y2, durationMs)
}

// SynthesizeDragAndDrop holds at (x1, y1) before smoothly moving to (x2, y2) and releasing.
func (ts *TouchSynthesizer) SynthesizeDragAndDrop(x1, y1, x2, y2 int, holdDurationMs, moveDurationMs int) []*MotionEvent {
	if holdDurationMs <= 0 {
		holdDurationMs = 500
	}
	if moveDurationMs <= 0 {
		moveDurationMs = 300
	}

	now := ts.CurrentTimeFunc()
	events := make([]*MotionEvent, 0)

	// 1. Initial DOWN
	events = append(events, &MotionEvent{
		Action:    ActionDown,
		DownTime:  now,
		EventTime: now,
		X:         float32(x1),
		Y:         float32(y1),
		Pressure:  1.0,
		Size:      1.0,
	})

	// 2. Hold at (x1, y1)
	holdEnd := now + int64(holdDurationMs)
	events = append(events, &MotionEvent{
		Action:    ActionMove,
		DownTime:  now,
		EventTime: holdEnd,
		X:         float32(x1),
		Y:         float32(y1),
		Pressure:  1.0,
		Size:      1.0,
	})

	// 3. Move to (x2, y2)
	steps := moveDurationMs / 16
	if steps < 5 {
		steps = 5
	}
	stepInterval := int64(moveDurationMs) / int64(steps)
	for i := 1; i <= steps; i++ {
		t := float32(i) / float32(steps)
		cx := float32(x1) + t*float32(x2-x1)
		cy := float32(y1) + t*float32(y2-y1)
		eventTime := holdEnd + int64(i)*stepInterval

		events = append(events, &MotionEvent{
			Action:    ActionMove,
			DownTime:  now,
			EventTime: eventTime,
			X:         cx,
			Y:         cy,
			Pressure:  1.0,
			Size:      1.0,
		})
	}

	// 4. Release UP
	totalEnd := holdEnd + int64(moveDurationMs)
	events = append(events, &MotionEvent{
		Action:    ActionUp,
		DownTime:  now,
		EventTime: totalEnd,
		X:         float32(x2),
		Y:         float32(y2),
		Pressure:  0.0,
		Size:      1.0,
	})

	return events
}

// SynthesizePinch produces two-finger pinch-in (zoom out) or pinch-out (zoom in) sequences.
func (ts *TouchSynthesizer) SynthesizePinch(cx, cy int, startSpan, endSpan int, durationMs int) []*MotionEvent {
	if durationMs <= 0 {
		durationMs = 250
	}
	steps := durationMs / 16
	if steps < 5 {
		steps = 5
	}

	now := ts.CurrentTimeFunc()
	events := make([]*MotionEvent, 0, steps+4)

	halfStart := float64(startSpan) / 2.0
	halfEnd := float64(endSpan) / 2.0

	// Pointer 0 initial
	p0X := float32(float64(cx) - halfStart)
	p0Y := float32(cy)
	// Pointer 1 initial
	p1X := float32(float64(cx) + halfStart)
	p1Y := float32(cy)

	// First pointer down
	events = append(events, &MotionEvent{
		Action:       ActionDown,
		DownTime:     now,
		EventTime:    now,
		X:            p0X,
		Y:            p0Y,
		Pressure:     1.0,
		PointerCount: 1,
		Pointers:     []PointerCoord{{X: p0X, Y: p0Y, Pressure: 1.0, Size: 1.0}},
	})

	// Second pointer down
	events = append(events, &MotionEvent{
		Action:       ActionPointerDown,
		DownTime:     now,
		EventTime:    now + 10,
		X:            p1X,
		Y:            p1Y,
		Pressure:     1.0,
		PointerCount: 2,
		Pointers: []PointerCoord{
			{X: p0X, Y: p0Y, Pressure: 1.0, Size: 1.0},
			{X: p1X, Y: p1Y, Pressure: 1.0, Size: 1.0},
		},
	})

	// Intermediate moves
	stepInterval := int64(durationMs) / int64(steps)
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		span := halfStart + t*(halfEnd-halfStart)

		curP0X := float32(float64(cx) - span)
		curP1X := float32(float64(cx) + span)
		eventTime := now + 10 + int64(i)*stepInterval

		events = append(events, &MotionEvent{
			Action:       ActionMove,
			DownTime:     now,
			EventTime:    eventTime,
			X:            curP0X,
			Y:            p0Y,
			Pressure:     1.0,
			PointerCount: 2,
			Pointers: []PointerCoord{
				{X: curP0X, Y: p0Y, Pressure: 1.0, Size: 1.0},
				{X: curP1X, Y: p1Y, Pressure: 1.0, Size: 1.0},
			},
		})
	}

	// Pointer 1 up
	endTime := now + 10 + int64(durationMs)
	finalP0X := float32(float64(cx) - halfEnd)
	finalP1X := float32(float64(cx) + halfEnd)

	events = append(events, &MotionEvent{
		Action:       ActionPointerUp,
		DownTime:     now,
		EventTime:    endTime,
		X:            finalP1X,
		Y:            p1Y,
		Pressure:     0.0,
		PointerCount: 2,
		Pointers: []PointerCoord{
			{X: finalP0X, Y: p0Y, Pressure: 1.0, Size: 1.0},
			{X: finalP1X, Y: p1Y, Pressure: 0.0, Size: 1.0},
		},
	})

	// Pointer 0 up
	events = append(events, &MotionEvent{
		Action:       ActionUp,
		DownTime:     now,
		EventTime:    endTime + 10,
		X:            finalP0X,
		Y:            p0Y,
		Pressure:     0.0,
		PointerCount: 1,
		Pointers:     []PointerCoord{{X: finalP0X, Y: p0Y, Pressure: 0.0, Size: 1.0}},
	})

	return events
}

// SynthesizeScroll produces an ACTION_SCROLL event or swipe-based scroll sequence.
func (ts *TouchSynthesizer) SynthesizeScroll(x, y, deltaX, deltaY int, durationMs int) []*MotionEvent {
	now := ts.CurrentTimeFunc()
	scrollEvent := &MotionEvent{
		Action:    ActionScroll,
		DownTime:  now,
		EventTime: now,
		X:         float32(x),
		Y:         float32(y),
		ScrollX:   float32(deltaX),
		ScrollY:   float32(deltaY),
		Pressure:  0.0,
		Size:      1.0,
	}

	swipeEvents := ts.SynthesizeSwipe(x, y, x-deltaX, y-deltaY, durationMs)
	res := make([]*MotionEvent, 0, len(swipeEvents)+1)
	res = append(res, scrollEvent)
	res = append(res, swipeEvents...)
	return res
}

// DispatchTouchEvent dispatches a single MotionEvent into the target view or hierarchy.
func (ts *TouchSynthesizer) DispatchTouchEvent(rootView any, event *MotionEvent) bool {
	if rootView == nil || event == nil {
		return false
	}

	switch target := rootView.(type) {
	case *View:
		return target.DispatchTouchEvent(event)
	case *LayoutNode:
		return target.DispatchTouchEvent(event)
	case interface{ DispatchTouchEvent(*MotionEvent) bool }:
		return target.DispatchTouchEvent(event)
	case interface{ DispatchTouchEvent(any) bool }:
		return target.DispatchTouchEvent(event)
	default:
		v := reflect.ValueOf(rootView)
		m := v.MethodByName("DispatchTouchEvent")
		if m.IsValid() && m.Type().NumIn() == 1 {
			paramType := m.Type().In(0)
			var arg reflect.Value
			if reflect.TypeOf(event).AssignableTo(paramType) {
				arg = reflect.ValueOf(event)
			} else {
				arg = reflect.ValueOf(event)
			}
			out := m.Call([]reflect.Value{arg})
			if len(out) > 0 && out[0].Kind() == reflect.Bool {
				return out[0].Bool()
			}
			return true
		}
	}

	return false
}

// DispatchEvents dispatches a sequence of MotionEvents into the target hierarchy.
func (ts *TouchSynthesizer) DispatchEvents(rootView any, events []*MotionEvent) error {
	if rootView == nil {
		return fmt.Errorf("rootView is nil")
	}

	for _, event := range events {
		ts.DispatchTouchEvent(rootView, event)
	}
	return nil
}

// Tap performs a complete tap action on rootView at (x, y).
func (ts *TouchSynthesizer) Tap(rootView any, x, y int) error {
	events := ts.SynthesizeTap(x, y, 30)
	return ts.DispatchEvents(rootView, events)
}

// LongPress performs a long press on rootView at (x, y).
func (ts *TouchSynthesizer) LongPress(rootView any, x, y int, durationMs int) error {
	events := ts.SynthesizeLongPress(x, y, durationMs)
	return ts.DispatchEvents(rootView, events)
}

// Swipe performs a swipe gesture from (x1, y1) to (x2, y2).
func (ts *TouchSynthesizer) Swipe(rootView any, x1, y1, x2, y2 int, durationMs int) error {
	events := ts.SynthesizeSwipe(x1, y1, x2, y2, durationMs)
	return ts.DispatchEvents(rootView, events)
}

// SmoothSwipe performs an ease-in-out curved swipe.
func (ts *TouchSynthesizer) SmoothSwipe(rootView any, x1, y1, x2, y2 int, durationMs int) error {
	events := ts.SynthesizeSmoothSwipe(x1, y1, x2, y2, durationMs)
	return ts.DispatchEvents(rootView, events)
}

// Scroll performs a scroll gesture at (x, y) with delta offsets.
func (ts *TouchSynthesizer) Scroll(rootView any, x, y, deltaX, deltaY int, durationMs int) error {
	events := ts.SynthesizeScroll(x, y, deltaX, deltaY, durationMs)
	return ts.DispatchEvents(rootView, events)
}

// Fling performs a high velocity fling gesture.
func (ts *TouchSynthesizer) Fling(rootView any, x1, y1, x2, y2 int, durationMs int) error {
	events := ts.SynthesizeFling(x1, y1, x2, y2, durationMs)
	return ts.DispatchEvents(rootView, events)
}

// Pinch performs a 2-finger pinch gesture.
func (ts *TouchSynthesizer) Pinch(rootView any, cx, cy int, startSpan, endSpan int, durationMs int) error {
	events := ts.SynthesizePinch(cx, cy, startSpan, endSpan, durationMs)
	return ts.DispatchEvents(rootView, events)
}

// DragAndDrop performs long-press hold, moves to target coordinates, and drops.
func (ts *TouchSynthesizer) DragAndDrop(rootView any, x1, y1, x2, y2 int, holdDurationMs, moveDurationMs int) error {
	events := ts.SynthesizeDragAndDrop(x1, y1, x2, y2, holdDurationMs, moveDurationMs)
	return ts.DispatchEvents(rootView, events)
}

// Distance calculates Euclidean distance between (x1, y1) and (x2, y2).
func Distance(x1, y1, x2, y2 float32) float32 {
	dx := x2 - x1
	dy := y2 - y1
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}
