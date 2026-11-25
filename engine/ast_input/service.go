package ast_input

import (
	"fmt"
	"strconv"
	"strings"
)

// ASTService defines the primary interface for AST extraction and synthetic input dispatch.
type ASTService interface {
	ExtractAST(rootView any) (*SemanticTree, error)
	DispatchTap(rootView any, targetNodeID string) error
	DispatchSwipe(rootView any, x1, y1, x2, y2 int, durationMs int) error
	DispatchText(rootView any, targetNodeID string, text string) error
}

// ProductionASTService extends ASTService with gestures, IME operations, and tree diffing.
type ProductionASTService interface {
	ASTService
	DiffAST(oldTree, newTree *SemanticTree) *ASTDiff
	DispatchSmoothSwipe(rootView any, x1, y1, x2, y2 int, durationMs int) error
	DispatchFling(rootView any, x1, y1, x2, y2 int, durationMs int) error
	DispatchPinch(rootView any, cx, cy int, startSpan, endSpan int, durationMs int) error
	DispatchDragAndDrop(rootView any, x1, y1, x2, y2 int, holdDurationMs, moveDurationMs int) error
	DispatchKey(rootView any, keyCode int) error
	CommitText(rootView any, targetNodeID string, text string) error
	PerformEditorAction(rootView any, targetNodeID string, actionCode int) error
}

// ServiceConfig contains optional configuration for ASTService.
type ServiceConfig struct {
	Extractor        *ASTExtractor
	TouchSynthesizer *TouchSynthesizer
	KeyInjector      *KeyInjector
}

// astServiceImpl implements ProductionASTService and ASTService.
type astServiceImpl struct {
	extractor        *ASTExtractor
	touchSynthesizer *TouchSynthesizer
	keyInjector      *KeyInjector
}

// NewASTService creates and initializes a new ASTService implementation.
func NewASTService(configs ...ServiceConfig) ASTService {
	var cfg ServiceConfig
	if len(configs) > 0 {
		cfg = configs[0]
	}

	extractor := cfg.Extractor
	if extractor == nil {
		extractor = NewASTExtractor()
	}

	touchSynthesizer := cfg.TouchSynthesizer
	if touchSynthesizer == nil {
		touchSynthesizer = NewTouchSynthesizer()
	}

	keyInjector := cfg.KeyInjector
	if keyInjector == nil {
		keyInjector = NewKeyInjector()
	}

	return &astServiceImpl{
		extractor:        extractor,
		touchSynthesizer: touchSynthesizer,
		keyInjector:      keyInjector,
	}
}

// ExtractAST traverses the root view / layout node and extracts the pruned semantic tree.
func (s *astServiceImpl) ExtractAST(rootView any) (*SemanticTree, error) {
	if rootView == nil {
		return nil, fmt.Errorf("rootView cannot be nil")
	}
	return s.extractor.Extract(rootView)
}

// DispatchTap maps targetNodeID to its screen coordinates and dispatches standard MotionEvent tap sequence.
func (s *astServiceImpl) DispatchTap(rootView any, targetNodeID string) error {
	if rootView == nil {
		return fmt.Errorf("rootView cannot be nil")
	}

	// 1. Direct coordinate format "x,y"
	if coords := parseCoordinates(targetNodeID); coords != nil {
		return s.touchSynthesizer.Tap(rootView, coords[0], coords[1])
	}

	// 2. Handle XML byte/string inputs by parsing to View first
	if xmlBytes, ok := rootView.([]byte); ok {
		parsed, err := ParseXMLHierarchy(xmlBytes)
		if err != nil {
			return err
		}
		rootView = parsed
	} else if xmlStr, ok := rootView.(string); ok && (strings.Contains(xmlStr, "<node") || strings.Contains(xmlStr, "<hierarchy")) {
		parsed, err := ParseXMLHierarchy([]byte(xmlStr))
		if err != nil {
			return err
		}
		rootView = parsed
	}

	// 3. Locate node in Android *View hierarchy
	if vRoot, ok := rootView.(*View); ok {
		target := vRoot.FindViewById(targetNodeID)
		if target != nil {
			target.mu.RLock()
			b := target.Bounds
			target.mu.RUnlock()

			cx := b[0] + (b[2]-b[0])/2
			cy := b[1] + (b[3]-b[1])/2

			_ = s.touchSynthesizer.Tap(vRoot, cx, cy)
			// Trigger direct click callback if present
			target.PerformClick()
			return nil
		}
	}

	// 4. Locate node in Compose *LayoutNode hierarchy
	if cRoot, ok := rootView.(*LayoutNode); ok {
		target := cRoot.FindNodeById(targetNodeID)
		if target != nil {
			target.mu.RLock()
			b := target.Bounds
			target.mu.RUnlock()

			cx := b[0] + (b[2]-b[0])/2
			cy := b[1] + (b[3]-b[1])/2

			_ = s.touchSynthesizer.Tap(cRoot, cx, cy)
			target.PerformClick()
			return nil
		}
	}

	// 5. Try extracting AST to find node bounds as fallback
	st, err := s.extractor.Extract(rootView)
	if err == nil && st != nil {
		if node := st.FindNode(targetNodeID); node != nil {
			cx := node.Bounds[0] + (node.Bounds[2]-node.Bounds[0])/2
			cy := node.Bounds[1] + (node.Bounds[3]-node.Bounds[1])/2
			return s.touchSynthesizer.Tap(rootView, cx, cy)
		}
	}

	return fmt.Errorf("target node %q not found in UI hierarchy", targetNodeID)
}

// DispatchSwipe synthesizes and dispatches an ACTION_DOWN -> ACTION_MOVE... -> ACTION_UP swipe sequence.
func (s *astServiceImpl) DispatchSwipe(rootView any, x1, y1, x2, y2 int, durationMs int) error {
	if rootView == nil {
		return fmt.Errorf("rootView cannot be nil")
	}
	return s.touchSynthesizer.Swipe(rootView, x1, y1, x2, y2, durationMs)
}

// DispatchSmoothSwipe performs an ease-in-out curved swipe gesture.
func (s *astServiceImpl) DispatchSmoothSwipe(rootView any, x1, y1, x2, y2 int, durationMs int) error {
	if rootView == nil {
		return fmt.Errorf("rootView cannot be nil")
	}
	return s.touchSynthesizer.SmoothSwipe(rootView, x1, y1, x2, y2, durationMs)
}

// DispatchFling performs a high velocity fling gesture.
func (s *astServiceImpl) DispatchFling(rootView any, x1, y1, x2, y2 int, durationMs int) error {
	if rootView == nil {
		return fmt.Errorf("rootView cannot be nil")
	}
	return s.touchSynthesizer.Fling(rootView, x1, y1, x2, y2, durationMs)
}

// DispatchPinch performs a two-finger pinch gesture.
func (s *astServiceImpl) DispatchPinch(rootView any, cx, cy int, startSpan, endSpan int, durationMs int) error {
	if rootView == nil {
		return fmt.Errorf("rootView cannot be nil")
	}
	return s.touchSynthesizer.Pinch(rootView, cx, cy, startSpan, endSpan, durationMs)
}

// DispatchDragAndDrop performs a drag-and-drop gesture.
func (s *astServiceImpl) DispatchDragAndDrop(rootView any, x1, y1, x2, y2 int, holdDurationMs, moveDurationMs int) error {
	if rootView == nil {
		return fmt.Errorf("rootView cannot be nil")
	}
	return s.touchSynthesizer.DragAndDrop(rootView, x1, y1, x2, y2, holdDurationMs, moveDurationMs)
}

// DispatchText requests focus on targetNodeID and injects typed text / KeyEvents.
func (s *astServiceImpl) DispatchText(rootView any, targetNodeID string, text string) error {
	if rootView == nil {
		return fmt.Errorf("rootView cannot be nil")
	}
	return s.keyInjector.Type(rootView, targetNodeID, text)
}

// CommitText injects text directly simulating IME input.
func (s *astServiceImpl) CommitText(rootView any, targetNodeID string, text string) error {
	if rootView == nil {
		return fmt.Errorf("rootView cannot be nil")
	}
	return s.keyInjector.CommitText(rootView, targetNodeID, text)
}

// PerformEditorAction triggers an IME action.
func (s *astServiceImpl) PerformEditorAction(rootView any, targetNodeID string, actionCode int) error {
	if rootView == nil {
		return fmt.Errorf("rootView cannot be nil")
	}
	return s.keyInjector.PerformEditorAction(rootView, targetNodeID, actionCode)
}

// DispatchKey dispatches a single key action into the UI hierarchy.
func (s *astServiceImpl) DispatchKey(rootView any, keyCode int) error {
	if rootView == nil {
		return fmt.Errorf("rootView cannot be nil")
	}
	return s.keyInjector.PressKey(rootView, keyCode)
}

// DiffAST computes structural and state differences between two SemanticTrees.
func (s *astServiceImpl) DiffAST(oldTree, newTree *SemanticTree) *ASTDiff {
	return DiffAST(oldTree, newTree)
}

// parseCoordinates attempts to parse strings like "100,200" or "100 200" into [x, y].
func parseCoordinates(s string) []int {
	s = strings.TrimSpace(s)
	var parts []string
	if strings.Contains(s, ",") {
		parts = strings.Split(s, ",")
	} else if strings.Contains(s, " ") {
		parts = strings.Fields(s)
	} else {
		return nil
	}

	if len(parts) == 2 {
		x, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		y, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 == nil && err2 == nil {
			return []int{x, y}
		}
	}
	return nil
}
