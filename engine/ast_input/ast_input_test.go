package ast_input_test

import (
	"sync"
	"testing"

	"nanoandroid/engine/ast_input"
)

// TestComplexViewHierarchyPruningUnder200Tokens verifies that a complex view hierarchy
// prunes non-interactive layout containers and stays strictly under 200 tokens.
func TestComplexViewHierarchyPruningUnder200Tokens(t *testing.T) {
	// Build a complex, realistic Android View hierarchy with 20+ nodes including
	// deeply nested FrameLayouts, LinearLayouts, empty spacers, and decorators.
	root := ast_input.NewView("decor_root", "android.widget.FrameLayout", [4]int{0, 0, 1080, 2400})

	coordinator := ast_input.NewView("coordinator", "androidx.coordinatorlayout.widget.CoordinatorLayout", [4]int{0, 0, 1080, 2400})
	root.AddChild(coordinator)

	// Decorative app bar and empty spacers
	appBar := ast_input.NewView("appbar", "android.widget.FrameLayout", [4]int{0, 0, 1080, 150})
	spacer1 := ast_input.NewView("statusBarSpacer", "android.view.View", [4]int{0, 0, 1080, 50})
	emptyDecor := ast_input.NewView("emptyDecor", "android.widget.FrameLayout", [4]int{0, 50, 1080, 150})
	appBar.AddChild(spacer1)
	appBar.AddChild(emptyDecor)
	coordinator.AddChild(appBar)

	// Content wrapper
	contentWrapper := ast_input.NewView("content_wrapper", "android.widget.LinearLayout", [4]int{0, 150, 1080, 2400})
	coordinator.AddChild(contentWrapper)

	emptyFrame1 := ast_input.NewView("emptyMarginFrame", "android.widget.FrameLayout", [4]int{0, 150, 1080, 170})
	contentWrapper.AddChild(emptyFrame1)

	// Header section
	headerLayout := ast_input.NewView("header_layout", "android.widget.RelativeLayout", [4]int{0, 170, 1080, 270})
	titleText := ast_input.NewView("txt_title", "android.widget.TextView", [4]int{40, 180, 350, 250})
	titleText.Text = "Settings"
	divider1 := ast_input.NewView("divider1", "android.view.View", [4]int{0, 268, 1080, 270})
	headerLayout.AddChild(titleText)
	headerLayout.AddChild(divider1)
	contentWrapper.AddChild(headerLayout)

	// Body section
	bodyLayout := ast_input.NewView("body_layout", "android.widget.LinearLayout", [4]int{0, 270, 1080, 2000})
	contentWrapper.AddChild(bodyLayout)

	emptyFrame2 := ast_input.NewView("emptyFrame2", "android.widget.FrameLayout", [4]int{0, 270, 1080, 290})
	bodyLayout.AddChild(emptyFrame2)

	// Row 1: Username input
	rowUser := ast_input.NewView("row_user", "androidx.constraintlayout.widget.ConstraintLayout", [4]int{0, 290, 1080, 420})
	labelUser := ast_input.NewView("lbl_user", "android.widget.TextView", [4]int{40, 310, 220, 380})
	labelUser.Text = "Username"
	editUser := ast_input.NewView("edit_user", "android.widget.EditText", [4]int{240, 300, 800, 400})
	editUser.Text = "alice"
	editUser.Focusable = true
	rowUser.AddChild(labelUser)
	rowUser.AddChild(editUser)
	bodyLayout.AddChild(rowUser)

	// Row 2: Dark mode switch
	rowSwitch := ast_input.NewView("row_switch", "androidx.constraintlayout.widget.ConstraintLayout", [4]int{0, 420, 1080, 550})
	labelSwitch := ast_input.NewView("lbl_switch", "android.widget.TextView", [4]int{40, 440, 400, 510})
	labelSwitch.Text = "Dark Mode"
	switchDark := ast_input.NewView("switch_dark", "android.widget.Switch", [4]int{900, 440, 1020, 510})
	switchDark.Clickable = true
	switchDark.Checkable = true
	rowSwitch.AddChild(labelSwitch)
	rowSwitch.AddChild(switchDark)
	bodyLayout.AddChild(rowSwitch)

	// Row 3: Action Buttons in nested layout
	actionsLayout := ast_input.NewView("actions_layout", "android.widget.LinearLayout", [4]int{0, 560, 1080, 720})
	btnSpacer := ast_input.NewView("btn_spacer", "android.view.View", [4]int{0, 560, 50, 720})
	btnSave := ast_input.NewView("btn_save", "android.widget.Button", [4]int{50, 580, 450, 680})
	btnSave.Text = "Save"
	btnSave.Clickable = true

	btnCancel := ast_input.NewView("btn_cancel", "android.widget.Button", [4]int{500, 580, 900, 680})
	btnCancel.Text = "Cancel"
	btnCancel.Clickable = true

	emptyShadow := ast_input.NewView("empty_shadow", "android.widget.FrameLayout", [4]int{0, 700, 1080, 720})

	actionsLayout.AddChild(btnSpacer)
	actionsLayout.AddChild(btnSave)
	actionsLayout.AddChild(btnCancel)
	actionsLayout.AddChild(emptyShadow)
	bodyLayout.AddChild(actionsLayout)

	// Service Extraction
	service := ast_input.NewASTService()
	st, err := service.ExtractAST(root)
	if err != nil {
		t.Fatalf("ExtractAST failed: %v", err)
	}

	t.Logf("Total raw nodes traversed: %d", st.TotalRawNodes)
	t.Logf("Total pruned non-interactive containers: %d", st.PrunedCount)
	t.Logf("Semantic nodes extracted: %d", len(st.Nodes))

	if st.PrunedCount == 0 {
		t.Errorf("Expected pruned non-interactive layout containers, but pruned count was 0")
	}

	// Verify required interactive/semantic nodes are present
	expectedIDs := []string{"edit_user", "switch_dark", "btn_save", "btn_cancel"}
	for _, id := range expectedIDs {
		node := st.FindNode(id)
		if node == nil {
			t.Errorf("Expected node with ID %q in semantic tree, but was missing", id)
		}
	}

	// Verify compact JSON token count is strictly < 200 tokens
	compactJSON, err := st.ToCompactNodesJSON()
	if err != nil {
		t.Fatalf("Failed to marshal compact JSON: %v", err)
	}

	tokenCount := st.TokenEstimate
	t.Logf("Compact JSON length: %d bytes", len(compactJSON))
	t.Logf("Estimated token count: %d tokens", tokenCount)
	t.Logf("Compact JSON: %s", compactJSON)

	if tokenCount >= 200 {
		t.Fatalf("Token count %d exceeds the required threshold of 200 tokens", tokenCount)
	}
}

// TestSimulatedTapInvokesOnClickListener verifies that a simulated tap on a target view
// invokes the target view's OnClickListener.
func TestSimulatedTapInvokesOnClickListener(t *testing.T) {
	root := ast_input.NewView("root_frame", "android.widget.FrameLayout", [4]int{0, 0, 1080, 2400})

	container := ast_input.NewView("button_container", "android.widget.LinearLayout", [4]int{0, 100, 1080, 800})
	root.AddChild(container)

	button := ast_input.NewView("btn_submit", "android.widget.Button", [4]int{100, 200, 500, 350})
	button.Text = "Submit Order"
	button.Clickable = true

	clicked := false
	var clickedView *ast_input.View
	button.OnClickListener = func(v *ast_input.View) {
		clicked = true
		clickedView = v
	}

	container.AddChild(button)

	service := ast_input.NewASTService()

	// Dispatch tap to "btn_submit"
	err := service.DispatchTap(root, "btn_submit")
	if err != nil {
		t.Fatalf("DispatchTap failed: %v", err)
	}

	if !clicked {
		t.Fatalf("Expected OnClickListener to be invoked, but clicked was false")
	}
	if clickedView != button {
		t.Fatalf("Expected clicked view to match button pointer")
	}
}

// TestJetpackComposeLayoutNodeExtractionAndTap verifies Compose LayoutNode trees.
func TestJetpackComposeLayoutNodeExtractionAndTap(t *testing.T) {
	root := ast_input.NewLayoutNode("compose_root", "Box", [4]int{0, 0, 1080, 2400})

	column := ast_input.NewLayoutNode("column_main", "Column", [4]int{0, 0, 1080, 2400})
	root.AddChild(column)

	emptyBox := ast_input.NewLayoutNode("spacer_box", "Box", [4]int{0, 0, 1080, 100})
	column.AddChild(emptyBox)

	btnNode := ast_input.NewLayoutNode("compose_btn", "Button", [4]int{100, 150, 400, 250})
	btnNode.Text = "Confirm"
	btnNode.Clickable = true

	clicked := false
	btnNode.OnClick = func() {
		clicked = true
	}
	column.AddChild(btnNode)

	service := ast_input.NewASTService()
	st, err := service.ExtractAST(root)
	if err != nil {
		t.Fatalf("ExtractAST failed on Compose hierarchy: %v", err)
	}

	if st.FindNode("compose_btn") == nil {
		t.Fatalf("Expected compose_btn in extracted semantic tree")
	}

	err = service.DispatchTap(root, "compose_btn")
	if err != nil {
		t.Fatalf("DispatchTap on compose_btn failed: %v", err)
	}

	if !clicked {
		t.Fatalf("Expected Compose OnClick to be invoked")
	}
}

// TestSwipeDispatch verifies synthesized MotionEvents during a swipe gesture.
func TestSwipeDispatch(t *testing.T) {
	root := ast_input.NewView("scroll_container", "android.widget.ScrollView", [4]int{0, 0, 1080, 2400})
	root.Scrollable = true

	var recordedEvents []*ast_input.MotionEvent
	root.OnTouchListener = func(v *ast_input.View, event *ast_input.MotionEvent) bool {
		// Clone event for testing
		recordedEvents = append(recordedEvents, &ast_input.MotionEvent{
			Action: event.Action,
			X:      event.X,
			Y:      event.Y,
		})
		return true
	}

	service := ast_input.NewASTService()
	err := service.DispatchSwipe(root, 500, 1500, 500, 500, 200)
	if err != nil {
		t.Fatalf("DispatchSwipe failed: %v", err)
	}

	if len(recordedEvents) < 3 {
		t.Fatalf("Expected at least 3 events (DOWN, MOVE, UP), got %d", len(recordedEvents))
	}

	first := recordedEvents[0]
	if first.Action != ast_input.ActionDown {
		t.Errorf("Expected first event to be ActionDown, got %d", first.Action)
	}
	if first.X != 500 || first.Y != 1500 {
		t.Errorf("Expected start coords (500, 1500), got (%v, %v)", first.X, first.Y)
	}

	last := recordedEvents[len(recordedEvents)-1]
	if last.Action != ast_input.ActionUp {
		t.Errorf("Expected last event to be ActionUp, got %d", last.Action)
	}
	if last.X != 500 || last.Y != 500 {
		t.Errorf("Expected end coords (500, 500), got (%v, %v)", last.X, last.Y)
	}
}

// TestTextInjectionAndTyping verifies typing and focus management.
func TestTextInjectionAndTyping(t *testing.T) {
	root := ast_input.NewView("root_view", "android.widget.FrameLayout", [4]int{0, 0, 1080, 2400})

	editText := ast_input.NewView("input_field", "android.widget.EditText", [4]int{50, 100, 600, 180})
	editText.Focusable = true
	root.AddChild(editText)

	service := ast_input.NewASTService()
	err := service.DispatchText(root, "input_field", "Hello Nano")
	if err != nil {
		t.Fatalf("DispatchText failed: %v", err)
	}

	if editText.GetText() != "Hello Nano" {
		t.Fatalf("Expected text 'Hello Nano', got %q", editText.GetText())
	}
	if !editText.Focused {
		t.Fatalf("Expected editText to be focused after typing")
	}
}

// TestBackNavigationAndKeyInjection verifies KeyEvents including Back button navigation.
func TestBackNavigationAndKeyInjection(t *testing.T) {
	keyInjector := ast_input.NewKeyInjector()
	backHandled := false
	keyInjector.OnBackPressedHandler = func() bool {
		backHandled = true
		return true
	}

	service := ast_input.NewASTService(ast_input.ServiceConfig{
		KeyInjector: keyInjector,
	})

	root := ast_input.NewView("root_view", "android.widget.FrameLayout", [4]int{0, 0, 1080, 2400})
	err := service.(interface{ DispatchKey(any, int) error }).DispatchKey(root, ast_input.KeyCodeBack)
	if err != nil {
		t.Fatalf("DispatchKey failed: %v", err)
	}

	if !backHandled {
		t.Fatalf("Expected OnBackPressedHandler to be invoked on KeyCodeBack")
	}
}

// TestConcurrentAccessRace verifies concurrent access passes the race detector.
func TestConcurrentAccessRace(t *testing.T) {
	root := ast_input.NewView("root", "android.widget.FrameLayout", [4]int{0, 0, 1080, 2400})
	btn := ast_input.NewView("btn_sync", "android.widget.Button", [4]int{100, 100, 400, 200})
	btn.Clickable = true
	btn.OnClickListener = func(v *ast_input.View) {}
	root.AddChild(btn)

	service := ast_input.NewASTService()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = service.ExtractAST(root)
		}()
		go func() {
			defer wg.Done()
			_ = service.DispatchTap(root, "btn_sync")
		}()
	}
	wg.Wait()
}

// TestDirectCoordinateTap verifies tapping using coordinate string "x,y".
func TestDirectCoordinateTap(t *testing.T) {
	root := ast_input.NewView("root", "android.widget.FrameLayout", [4]int{0, 0, 1080, 2400})
	btn := ast_input.NewView("btn_coord", "android.widget.Button", [4]int{200, 300, 600, 500})
	btn.Clickable = true
	clicked := false
	btn.OnClickListener = func(v *ast_input.View) {
		clicked = true
	}
	root.AddChild(btn)

	service := ast_input.NewASTService()
	// Tap center of button: x=400, y=400
	err := service.DispatchTap(root, "400,400")
	if err != nil {
		t.Fatalf("DispatchTap with coords failed: %v", err)
	}
	if !clicked {
		t.Fatalf("Expected coordinate tap to invoke OnClickListener")
	}
}

// TestLongPressDispatch verifies synthesizing a long press invokes OnLongClickListener.
func TestLongPressDispatch(t *testing.T) {
	btn := ast_input.NewView("btn_long", "android.widget.Button", [4]int{100, 100, 400, 300})
	longClicked := false
	btn.OnLongClickListener = func(v *ast_input.View) bool {
		longClicked = true
		return true
	}

	ts := ast_input.NewTouchSynthesizer()
	events := ts.SynthesizeLongPress(250, 200, 600)
	if len(events) != 2 {
		t.Fatalf("Expected 2 events for long press, got %d", len(events))
	}
	if events[1].EventTime-events[0].EventTime < 500 {
		t.Fatalf("Expected long press duration >= 500ms")
	}

	btn.PerformLongClick()
	if !longClicked {
		t.Fatalf("Expected PerformLongClick to trigger OnLongClickListener")
	}
}

// TestScrollDispatch verifies synthesizing a scroll action.
func TestScrollDispatch(t *testing.T) {
	ts := ast_input.NewTouchSynthesizer()
	events := ts.SynthesizeScroll(500, 1000, 0, 300, 200)
	if len(events) < 4 {
		t.Fatalf("Expected at least 4 events for scroll gesture, got %d", len(events))
	}
	if events[0].Action != ast_input.ActionScroll {
		t.Errorf("Expected first event to be ActionScroll, got %d", events[0].Action)
	}
	if events[0].ScrollY != 300 {
		t.Errorf("Expected ScrollY = 300, got %v", events[0].ScrollY)
	}
}

// TestErrorHandling verifies proper error returns for nil views or invalid targets.
func TestErrorHandling(t *testing.T) {
	service := ast_input.NewASTService()

	if _, err := service.ExtractAST(nil); err == nil {
		t.Errorf("Expected error when ExtractAST(nil)")
	}

	if err := service.DispatchTap(nil, "btn"); err == nil {
		t.Errorf("Expected error when DispatchTap(nil, ...)")
	}

	root := ast_input.NewView("root", "android.widget.FrameLayout", [4]int{0, 0, 1080, 2400})
	if err := service.DispatchTap(root, "non_existent_node"); err == nil {
		t.Errorf("Expected error when targeting non-existent node ID")
	}

	if err := service.DispatchText(nil, "input", "abc"); err == nil {
		t.Errorf("Expected error when DispatchText(nil, ...)")
	}
}

// TestInteractiveNodesTokenCount verifies strictly interactive nodes are even more compact (< 100 tokens).
func TestInteractiveNodesTokenCount(t *testing.T) {
	root := ast_input.NewView("root", "android.widget.FrameLayout", [4]int{0, 0, 1080, 2400})
	btn1 := ast_input.NewView("btn_login", "android.widget.Button", [4]int{100, 200, 400, 300})
	btn1.Text = "Login"
	btn1.Clickable = true

	edit := ast_input.NewView("input_pass", "android.widget.EditText", [4]int{100, 100, 600, 180})
	edit.Focusable = true

	root.AddChild(btn1)
	root.AddChild(edit)

	service := ast_input.NewASTService()
	st, err := service.ExtractAST(root)
	if err != nil {
		t.Fatalf("ExtractAST failed: %v", err)
	}

	interactive := st.InteractiveNodes()
	if len(interactive) != 2 {
		t.Fatalf("Expected 2 interactive nodes, got %d", len(interactive))
	}

	compactInteractive, err := st.ToCompactInteractiveJSON()
	if err != nil {
		t.Fatalf("ToCompactInteractiveJSON failed: %v", err)
	}

	tokens := ast_input.EstimateTokens(compactInteractive)
	t.Logf("Interactive JSON: %s", compactInteractive)
	t.Logf("Interactive tokens: %d", tokens)

	if tokens >= 100 {
		t.Fatalf("Expected interactive tokens < 100, got %d", tokens)
	}
}

// TestParseXMLHierarchyAndExtractAST verifies XML dump parsing and tap dispatch.
func TestParseXMLHierarchyAndExtractAST(t *testing.T) {
	xmlData := `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
  <node index="0" class="android.widget.FrameLayout" bounds="[0,0][1080,2400]">
    <node index="0" class="android.widget.LinearLayout" bounds="[0,0][1080,2400]">
      <node index="0" resource-id="com.example:id/title" class="android.widget.TextView" text="Media Studio" bounds="[50,100][500,200]" />
      <node index="1" resource-id="com.example:id/btn_record" class="android.widget.Button" text="Record" clickable="true" bounds="[50,220][500,340]" />
      <node index="2" resource-id="com.example:id/switch_hd" class="android.widget.Switch" text="HD Mode" checkable="true" checked="true" clickable="true" bounds="[50,360][500,480]" />
    </node>
  </node>
</hierarchy>`

	service := ast_input.NewASTService()
	st, err := service.ExtractAST([]byte(xmlData))
	if err != nil {
		t.Fatalf("ExtractAST from XML failed: %v", err)
	}

	if len(st.Nodes) < 3 {
		t.Fatalf("Expected at least 3 nodes, got %d", len(st.Nodes))
	}

	btn := st.FindNode("btn_record")
	if btn == nil {
		t.Fatalf("Expected btn_record in extracted tree")
	}
	if btn.Text != "Record" || !btn.Clickable {
		t.Fatalf("btn_record text or clickable state mismatch")
	}

	// Dispatch tap to XML byte input
	err = service.DispatchTap([]byte(xmlData), "btn_record")
	if err != nil {
		t.Fatalf("DispatchTap on XML failed: %v", err)
	}
}

// TestASTDiffing verifies screen differential generation between UI states.
func TestASTDiffing(t *testing.T) {
	node1 := &ast_input.SemanticNode{
		ID:        "btn_submit",
		Class:     "Button",
		Text:      "Submit",
		Clickable: true,
	}
	node2 := &ast_input.SemanticNode{
		ID:        "lbl_status",
		Class:     "TextView",
		Text:      "Ready",
	}

	tree1 := &ast_input.SemanticTree{
		Nodes: []*ast_input.SemanticNode{node1, node2},
	}

	// State 2: btn_submit modified, lbl_status text changed, new banner added
	node1Mod := &ast_input.SemanticNode{
		ID:        "btn_submit",
		Class:     "Button",
		Text:      "Submitting...",
		Clickable: false,
	}
	node2Mod := &ast_input.SemanticNode{
		ID:        "lbl_status",
		Class:     "TextView",
		Text:      "Processing",
	}
	node3New := &ast_input.SemanticNode{
		ID:        "banner_alert",
		Class:     "TextView",
		Text:      "Uploading video",
	}

	tree2 := &ast_input.SemanticTree{
		Nodes: []*ast_input.SemanticNode{node1Mod, node2Mod, node3New},
	}

	diff := ast_input.DiffAST(tree1, tree2)
	if len(diff.Added) != 1 || diff.Added[0].ID != "banner_alert" {
		t.Fatalf("Expected 1 added node (banner_alert), got %v", diff.Added)
	}
	if len(diff.Modified) != 2 {
		t.Fatalf("Expected 2 modified nodes, got %d", len(diff.Modified))
	}
	t.Logf("AST Diff Summary: %s", diff.Summary)
}

// TestToPromptTextFormat verifies ultra-compact text prompt serialization.
func TestToPromptTextFormat(t *testing.T) {
	st := &ast_input.SemanticTree{
		Nodes: []*ast_input.SemanticNode{
			{
				ID:        "btn_save",
				Class:     "Button",
				Text:      "Save",
				Bounds:    [4]int{50, 580, 450, 680},
				Clickable: true,
			},
			{
				ID:        "edit_user",
				Class:     "EditText",
				Text:      "alice",
				Bounds:    [4]int{240, 300, 800, 400},
				Focusable: true,
			},
		},
	}

	promptText := st.ToPromptText()
	t.Logf("Prompt Text Format:\n%s", promptText)

	tokens := ast_input.EstimateTokens(promptText)
	t.Logf("Prompt Text tokens: %d", tokens)
	if tokens >= 50 {
		t.Fatalf("Expected < 50 tokens for prompt text, got %d", tokens)
	}
}

// TestSemanticLabelFusion verifies adjacent labels are fused into unlabeled interactive elements.
func TestSemanticLabelFusion(t *testing.T) {
	root := ast_input.NewView("root", "android.widget.LinearLayout", [4]int{0, 0, 1080, 500})

	lbl := ast_input.NewView("lbl_wifi", "android.widget.TextView", [4]int{50, 100, 300, 160})
	lbl.Text = "Wi-Fi Calling"

	sw := ast_input.NewView("switch_wifi", "android.widget.Switch", [4]int{800, 100, 950, 160})
	sw.Clickable = true
	// No text or content description on the switch itself

	root.AddChild(lbl)
	root.AddChild(sw)

	service := ast_input.NewASTService()
	st, err := service.ExtractAST(root)
	if err != nil {
		t.Fatalf("ExtractAST failed: %v", err)
	}

	swNode := st.FindNode("switch_wifi")
	if swNode == nil {
		t.Fatalf("Expected switch_wifi in tree")
	}
	if swNode.ContentDesc != "Wi-Fi Calling" {
		t.Fatalf("Expected switch_wifi ContentDesc to be fused with label 'Wi-Fi Calling', got %q", swNode.ContentDesc)
	}
}

// TestGesturesSmoothSwipePinchFlingDrag verifies advanced physical touch gestures.
func TestGesturesSmoothSwipePinchFlingDrag(t *testing.T) {
	ts := ast_input.NewTouchSynthesizer()

	// 1. Smooth swipe
	smoothEvents := ts.SynthesizeSmoothSwipe(100, 1000, 100, 200, 300)
	if len(smoothEvents) < 10 {
		t.Fatalf("Expected >= 10 smooth swipe events, got %d", len(smoothEvents))
	}

	// 2. Fling gesture
	flingEvents := ts.SynthesizeFling(100, 1000, 100, 200, 100)
	if len(flingEvents) < 3 {
		t.Fatalf("Expected >= 3 fling events")
	}

	// 3. Pinch gesture
	pinchEvents := ts.SynthesizePinch(500, 1000, 100, 600, 200)
	hasPointerDown := false
	hasPointerUp := false
	for _, e := range pinchEvents {
		if e.Action == ast_input.ActionPointerDown {
			hasPointerDown = true
		}
		if e.Action == ast_input.ActionPointerUp {
			hasPointerUp = true
		}
	}
	if !hasPointerDown || !hasPointerUp {
		t.Fatalf("Expected ActionPointerDown and ActionPointerUp in pinch sequence")
	}

	// 4. Drag and Drop
	dragEvents := ts.SynthesizeDragAndDrop(200, 500, 800, 500, 300, 200)
	if len(dragEvents) < 5 {
		t.Fatalf("Expected drag and drop events, got %d", len(dragEvents))
	}
}

// TestIMEInputConnection verifies IME InputConnection text commit, backspace, and action codes.
func TestIMEInputConnection(t *testing.T) {
	root := ast_input.NewView("root", "android.widget.FrameLayout", [4]int{0, 0, 1080, 2400})
	editText := ast_input.NewView("input_box", "android.widget.EditText", [4]int{50, 100, 600, 200})
	editText.Focusable = true
	root.AddChild(editText)

	ki := ast_input.NewKeyInjector()

	// 1. Commit text with Unicode and emojis
	err := ki.CommitText(root, "input_box", "Nano 🚀 世界")
	if err != nil {
		t.Fatalf("CommitText failed: %v", err)
	}
	if editText.GetText() != "Nano 🚀 世界" {
		t.Fatalf("Expected text 'Nano 🚀 世界', got %q", editText.GetText())
	}

	// 2. Delete surrounding text (delete last 2 runes)
	err = ki.DeleteSurroundingText(root, "input_box", 2, 0)
	if err != nil {
		t.Fatalf("DeleteSurroundingText failed: %v", err)
	}
	if editText.GetText() != "Nano 🚀 " {
		t.Fatalf("Expected text 'Nano 🚀 ', got %q", editText.GetText())
	}

	// 3. Perform editor action
	err = ki.PerformEditorAction(root, "input_box", ast_input.EditorActionDone)
	if err != nil {
		t.Fatalf("PerformEditorAction failed: %v", err)
	}
}

// TestScreenClippingAndZeroArea verifies zero-area dividers and offscreen elements are pruned.
func TestScreenClippingAndZeroArea(t *testing.T) {
	root := ast_input.NewView("root", "android.widget.FrameLayout", [4]int{0, 0, 1080, 2400})

	// Valid button on screen
	btn := ast_input.NewView("btn_valid", "android.widget.Button", [4]int{50, 100, 400, 200})
	btn.Clickable = true

	// Zero-width line divider: [50, 300, 50, 500] (width = 0)
	zeroWidth := ast_input.NewView("divider_zero", "android.view.View", [4]int{50, 300, 50, 500})

	// Off-screen button beyond display height (y=3000)
	offscreen := ast_input.NewView("btn_offscreen", "android.widget.Button", [4]int{50, 3000, 400, 3100})
	offscreen.Clickable = true

	root.AddChild(btn)
	root.AddChild(zeroWidth)
	root.AddChild(offscreen)

	service := ast_input.NewASTService()
	st, err := service.ExtractAST(root)
	if err != nil {
		t.Fatalf("ExtractAST failed: %v", err)
	}

	if st.FindNode("btn_valid") == nil {
		t.Fatalf("Expected btn_valid to be kept")
	}
	if st.FindNode("divider_zero") != nil {
		t.Fatalf("Expected zero-width divider to be pruned")
	}
	if st.FindNode("btn_offscreen") != nil {
		t.Fatalf("Expected off-screen button to be pruned")
	}
}


