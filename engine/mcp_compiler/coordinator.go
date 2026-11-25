package mcp_compiler

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ASTNode represents a single UI element node in the pruned Android accessibility / Compose tree.
type ASTNode struct {
	ID                 string     `json:"id"`
	Type               string     `json:"type,omitempty"`
	Text               string     `json:"text,omitempty"`
	ResourceID         string     `json:"resource_id,omitempty"`
	ContentDescription string     `json:"content_description,omitempty"`
	Class              string     `json:"class,omitempty"`
	Package            string     `json:"package,omitempty"`
	Bounds             [4]int     `json:"bounds,omitempty"` // [left, top, right, bottom]
	Clickable          bool       `json:"clickable,omitempty"`
	Checkable          bool       `json:"checkable,omitempty"`
	Checked            bool       `json:"checked,omitempty"`
	Focused            bool       `json:"focused,omitempty"`
	Scrollable         bool       `json:"scrollable,omitempty"`
	Visible            bool       `json:"visible"`
	Children           []*ASTNode `json:"children,omitempty"`
}

// FindByID recursively searches the AST subtree for a node with the given ID.
func (n *ASTNode) FindByID(id string) *ASTNode {
	if n == nil {
		return nil
	}
	if n.ID == id || n.ResourceID == id {
		return n
	}
	for _, child := range n.Children {
		if found := child.FindByID(id); found != nil {
			return found
		}
	}
	return nil
}

// FindByText recursively searches for a node whose Text or ContentDescription matches query.
func (n *ASTNode) FindByText(query string) *ASTNode {
	if n == nil {
		return nil
	}
	if strings.EqualFold(n.Text, query) || strings.EqualFold(n.ContentDescription, query) {
		return n
	}
	for _, child := range n.Children {
		if found := child.FindByText(query); found != nil {
			return found
		}
	}
	return nil
}

// CollectVisibleTexts collects all visible text strings in this subtree.
func (n *ASTNode) CollectVisibleTexts() []string {
	var texts []string
	if n == nil {
		return texts
	}
	if n.Visible && n.Text != "" {
		texts = append(texts, n.Text)
	}
	if n.Visible && n.ContentDescription != "" && n.ContentDescription != n.Text {
		texts = append(texts, n.ContentDescription)
	}
	for _, child := range n.Children {
		texts = append(texts, child.CollectVisibleTexts()...)
	}
	return texts
}

// Clone creates a deep copy of an ASTNode.
func (n *ASTNode) Clone() *ASTNode {
	if n == nil {
		return nil
	}
	clone := *n
	if len(n.Children) > 0 {
		clone.Children = make([]*ASTNode, len(n.Children))
		for i, child := range n.Children {
			clone.Children[i] = child.Clone()
		}
	}
	return &clone
}

// LaunchRequest specifies parameters to boot an APK into the virtual sandbox.
type LaunchRequest struct {
	APKPath    string `json:"apk_path"`
	Activity   string `json:"activity,omitempty"`
	Package    string `json:"package,omitempty"`
	ClearState bool   `json:"clear_state,omitempty"`
}

// LaunchResult represents the outcome of an app launch.
type LaunchResult struct {
	AppID            string   `json:"app_id"`
	Activity         string   `json:"activity"`
	PID              int      `json:"pid"`
	Status           string   `json:"status"` // "ok", "partial", "error", "running", "booted"
	SandboxID        string   `json:"sandbox_id"`
	VirtualTimeMs    int64    `json:"virtual_time_ms"`
	LaunchDurationMs int64    `json:"launch_duration_ms"`
	ApkPath          string   `json:"apk_path,omitempty"`
	AppLabel         string   `json:"app_label,omitempty"`
	MinSDK           int      `json:"min_sdk,omitempty"`
	TargetSDK        int      `json:"target_sdk,omitempty"`
	Activities       []string `json:"activities,omitempty"`
	NativeLibs       int      `json:"native_libs,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
	ExceptionTrace   string   `json:"exception_trace,omitempty"`
	Error            string   `json:"error,omitempty"`
	ExecutionMode    string   `json:"execution_mode,omitempty"`
}

// RenderRequest specifies screen capture options.
type RenderRequest struct {
	Format  string `json:"format"` // "webp" or "png"
	Quality int    `json:"quality,omitempty"`
}

// RenderResult represents a rendered frame.
type RenderResult struct {
	Format       string `json:"format"`
	Base64Data   string `json:"base64_data"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	SizeBytes    int    `json:"size_bytes"`
	TimestampMs  int64  `json:"timestamp_ms"`
}

// ObserveRequest specifies observation parameters.
type ObserveRequest struct {
	IncludeScreenshot bool   `json:"include_screenshot,omitempty"`
	Format            string `json:"format,omitempty"`
	PruneDepth        int    `json:"prune_depth,omitempty"`
}

// ObserveResult contains AST and visual state returned to MCP clients.
type ObserveResult struct {
	AppID          string        `json:"app_id"`
	CurrentScreen  string        `json:"current_screen"`
	ActiveActivity string        `json:"active_activity,omitempty"`
	ActivityStack  []string      `json:"activity_stack,omitempty"`
	ActiveDialog   string        `json:"active_dialog,omitempty"`
	Toast          string        `json:"toast,omitempty"`
	AST            *ASTNode      `json:"ast"`
	Screenshot     *RenderResult `json:"screenshot,omitempty"`
	VisibleTexts   []string      `json:"visible_texts"`
	NodeCount      int           `json:"node_count"`
	VirtualTimeMs  int64         `json:"virtual_time_ms"`
}

// StagedFileInfo represents a test input media or document staged into the Android runtime sandbox.
type StagedFileInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	URI       string `json:"uri,omitempty"`
	SizeBytes int64  `json:"size_bytes"`
}

// Action specifies an input event to dispatch.
type Action struct {
	Type          string  `json:"type"` // "tap", "type", "swipe", "back"
	NodeID        string  `json:"node_id,omitempty"`
	Text          string  `json:"text,omitempty"`
	Direction     string  `json:"direction,omitempty"` // "UP", "DOWN", "LEFT", "RIGHT"
	X             float64 `json:"x,omitempty"`
	Y             float64 `json:"y,omitempty"`
	StartX        float64 `json:"start_x,omitempty"`
	StartY        float64 `json:"start_y,omitempty"`
	EndX          float64 `json:"end_x,omitempty"`
	EndY          float64 `json:"end_y,omitempty"`
	DurationMs    int64   `json:"duration_ms,omitempty"`
	AssertVisible string  `json:"assert_visible,omitempty"`
}

// ActionResult represents the result of dispatching an action.
type ActionResult struct {
	Status        string `json:"status"` // "success", "failed"
	Action        string `json:"action"`
	TargetNodeID  string `json:"target_node_id,omitempty"`
	ResolvedText  string `json:"resolved_text,omitempty"`
	AssertionMet  bool   `json:"assertion_met,omitempty"`
	Message       string `json:"message,omitempty"`
	VirtualTimeMs int64  `json:"virtual_time_ms"`
}

// StepResult represents the virtual looper clock advance.
type StepResult struct {
	VirtualTimeMs int64 `json:"virtual_time_ms"`
	SteppedMs     int64 `json:"stepped_ms"`
	TasksExecuted int   `json:"tasks_executed"`
	IsIdle        bool  `json:"is_idle"`
}

// HeapInfo details memory allocations.
type HeapInfo struct {
	AllocatedMB float64 `json:"allocated_mb"`
	FreeMB      float64 `json:"free_mb"`
	MaxMB       float64 `json:"max_mb"`
	ObjectsLive int64   `json:"objects_live"`
	GCCycles    int64   `json:"gc_cycles"`
}

// InspectResult details runtime diagnostics.
type InspectResult struct {
	Target        string          `json:"target"`
	AppID         string          `json:"app_id"`
	VirtualTimeMs int64           `json:"virtual_time_ms"`
	Heap          *HeapInfo       `json:"heap,omitempty"`
	MemoryMB      map[string]any  `json:"memory_mb,omitempty"`
	ViewTreeStats map[string]any  `json:"view_tree_stats,omitempty"`
	LooperState   map[string]any  `json:"looper_state,omitempty"`
}

// --- MODULE INTERFACES ---

// RuntimeModule (Module 1: Virtual Android Runtime & Sandbox Looper)
type RuntimeModule interface {
	Launch(ctx context.Context, req LaunchRequest) (*LaunchResult, error)
	StepLooper(ctx context.Context, deltaMs int64) (*StepResult, error)
	GetVirtualTime() int64
	IsAppRunning(appID string) bool
}

// RenderModule (Module 2: Headless Surface & Frame Buffer Renderer)
type RenderModule interface {
	Capture(ctx context.Context, req RenderRequest) (*RenderResult, error)
}

// ASTInputModule (Module 3: UI Hierarchy Parser & Event Synthesizer)
type ASTInputModule interface {
	GetAST(ctx context.Context, pruneDepth int) (*ASTNode, error)
	Dispatch(ctx context.Context, act Action) (*ActionResult, error)
	ResolveNode(nodeID string) (*ASTNode, error)
	SetRootAST(root *ASTNode)
}

// ProfilerMediaModule (Module 4: Heap, Thermal, Memory & Video Codec Profiler)
type ProfilerMediaModule interface {
	InspectState(ctx context.Context, target string) (*InspectResult, error)
	GetHeapMetrics() *HeapInfo
}

// HealthStatus reports diagnostics across all virtual modules.
type HealthStatus struct {
	OverallHealthy  bool           `json:"overall_healthy"`
	ActiveAppID     string         `json:"active_app_id"`
	VirtualTimeMs   int64          `json:"virtual_time_ms"`
	RuntimeHealthy  bool           `json:"runtime_healthy"`
	RenderHealthy   bool           `json:"render_healthy"`
	ASTHealthy      bool           `json:"ast_healthy"`
	ProfilerHealthy bool           `json:"profiler_healthy"`
	SubsystemInfo   map[string]any `json:"subsystem_info"`
}

// Coordinator orchestrates Modules 1, 2, 3, and 4 into a unified interface for MCP handlers.
type Coordinator interface {
	Runtime() RuntimeModule
	Render() RenderModule
	ASTInput() ASTInputModule
	Profiler() ProfilerMediaModule

	LaunchApp(ctx context.Context, apkPath string, activity string, clearState bool) (*LaunchResult, error)
	ObserveScreen(ctx context.Context, includeScreenshot bool, format string, pruneDepth int) (*ObserveResult, error)
	DispatchAction(ctx context.Context, action Action) (*ActionResult, error)
	StepTime(ctx context.Context, deltaMs int64) (*StepResult, error)
	InspectState(ctx context.Context, target string) (*InspectResult, error)
	CheckHealth(ctx context.Context) (*HealthStatus, error)
	SaveSnapshot(name string) error
	RestoreSnapshot(name string) error
}

// --- MOCK IMPLEMENTATIONS FOR INDEPENDENT TESTING ---

// MockRuntimeModule provides an in-memory runtime sandbox mock.
type MockRuntimeModule struct {
	mu            sync.RWMutex
	appID         string
	activity      string
	pid           int
	virtualTimeMs int64
	running       bool
	LaunchCalls   int
	StepCalls     int
}

func NewMockRuntimeModule() *MockRuntimeModule {
	return &MockRuntimeModule{
		appID:         "com.prism.v",
		activity:      "com.prism.v.MainActivity",
		pid:           1024,
		virtualTimeMs: 1000,
	}
}

func (m *MockRuntimeModule) Launch(ctx context.Context, req LaunchRequest) (*LaunchResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LaunchCalls++
	if req.Package != "" {
		m.appID = req.Package
	} else if req.APKPath != "" && strings.Contains(req.APKPath, "prism") {
		m.appID = "com.prism.v"
	} else if req.APKPath != "" {
		m.appID = "com.example.app"
	}
	if req.Activity != "" {
		m.activity = req.Activity
	}
	m.running = true
	m.virtualTimeMs += 100

	return &LaunchResult{
		AppID:            m.appID,
		Activity:         m.activity,
		PID:              m.pid,
		Status:           "running",
		SandboxID:        "sandbox_virtual_01",
		VirtualTimeMs:    m.virtualTimeMs,
		LaunchDurationMs: 45,
	}, nil
}

func (m *MockRuntimeModule) StepLooper(ctx context.Context, deltaMs int64) (*StepResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.StepCalls++
	m.virtualTimeMs += deltaMs
	tasks := int(deltaMs / 16)
	if tasks == 0 && deltaMs > 0 {
		tasks = 1
	}
	return &StepResult{
		VirtualTimeMs: m.virtualTimeMs,
		SteppedMs:     deltaMs,
		TasksExecuted: tasks,
		IsIdle:        true,
	}, nil
}

func (m *MockRuntimeModule) GetVirtualTime() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.virtualTimeMs
}

func (m *MockRuntimeModule) IsAppRunning(appID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running && (appID == "" || m.appID == appID)
}

// MockRenderModule simulates headless rendering and base64 WebP screenshot generation.
type MockRenderModule struct {
	mu           sync.RWMutex
	CaptureCalls int
	SampleWebP   string
}

func NewMockRenderModule() *MockRenderModule {
	// 1x1 transparent dummy WebP base64
	dummyWebP := base64.StdEncoding.EncodeToString([]byte("RIFF\x1a\x00\x00\x00WEBPVP8L\x0e\x00\x00\x00/\x00\x00\x00\x00\x07\x88\x85%\xa4\x00\x03\x70\x00\xfe"))
	return &MockRenderModule{
		SampleWebP: dummyWebP,
	}
}

func (m *MockRenderModule) Capture(ctx context.Context, req RenderRequest) (*RenderResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CaptureCalls++
	fmtStr := req.Format
	if fmtStr == "" {
		fmtStr = "webp"
	}
	return &RenderResult{
		Format:      fmtStr,
		Base64Data:  m.SampleWebP,
		Width:       1080,
		Height:      2400,
		SizeBytes:   len(m.SampleWebP),
		TimestampMs: time.Now().UnixMilli(),
	}, nil
}

// MockASTInputModule simulates UI hierarchy management and action dispatch.
type MockASTInputModule struct {
	mu            sync.RWMutex
	root          *ASTNode
	ActionCalls   int
	DispatchCalls []Action
}

func NewMockASTInputModule() *MockASTInputModule {
	return &MockASTInputModule{
		root: BuildPrismSampleAST(),
	}
}

// BuildPrismSampleAST creates a standard test UI tree matching the Prism Video Studio reference workload.
func BuildPrismSampleAST() *ASTNode {
	return &ASTNode{
		ID:         "root_view",
		Type:       "ViewGroup",
		Class:      "android.widget.FrameLayout",
		Package:    "com.prism.v",
		Visible:    true,
		Bounds:     [4]int{0, 0, 1080, 2400},
		Children: []*ASTNode{
			{
				ID:                 "tab_studio",
				Type:               "Button",
				Text:               "Studio",
				ResourceID:         "com.prism.v:id/tab_studio",
				ContentDescription: "Studio Tab",
				Clickable:          true,
				Visible:            true,
				Bounds:             [4]int{50, 100, 300, 200},
			},
			{
				ID:         "lbl_resolution_preset",
				Type:       "TextView",
				Text:       "Resolution Preset",
				ResourceID: "com.prism.v:id/lbl_preset",
				Visible:    true,
				Bounds:     [4]int{50, 300, 500, 380},
			},
			{
				ID:                 "btn_ultra_4k",
				Type:               "RadioButton",
				Text:               "Ultra 4K",
				ResourceID:         "com.prism.v:id/opt_ultra_4k",
				ContentDescription: "Ultra 4K Preset",
				Clickable:          true,
				Checkable:          true,
				Checked:            false,
				Visible:            true,
				Bounds:             [4]int{50, 400, 450, 480},
			},
			{
				ID:         "lbl_estimated_size",
				Type:       "TextView",
				Text:       "Estimated Size: 148.6 MB",
				ResourceID: "com.prism.v:id/txt_est_size",
				Visible:    true,
				Bounds:     [4]int{50, 500, 600, 560},
			},
			{
				ID:         "input_project_title",
				Type:       "EditText",
				Text:       "My Video",
				ResourceID: "com.prism.v:id/edt_title",
				Clickable:  true,
				Visible:    true,
				Bounds:     [4]int{50, 600, 800, 700},
			},
		},
	}
}

func (m *MockASTInputModule) GetAST(ctx context.Context, pruneDepth int) (*ASTNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.root.Clone(), nil
}

func (m *MockASTInputModule) Dispatch(ctx context.Context, act Action) (*ActionResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ActionCalls++
	m.DispatchCalls = append(m.DispatchCalls, act)

	resolvedText := ""
	var targetNode *ASTNode
	if act.NodeID != "" {
		targetNode = m.root.FindByID(act.NodeID)
		if targetNode == nil {
			targetNode = m.root.FindByText(act.NodeID)
		}
		if targetNode != nil {
			resolvedText = targetNode.Text
			if resolvedText == "" {
				resolvedText = targetNode.ContentDescription
			}
		}
	}

	// Interactive state updates for mock realism
	if act.Type == "tap" && targetNode != nil {
		if targetNode.Checkable {
			targetNode.Checked = true
		}
		if targetNode.Text == "Ultra 4K" {
			// Update estimated size label dynamically
			if estNode := m.root.FindByID("lbl_estimated_size"); estNode != nil {
				estNode.Text = "Estimated Size: 148.6 MB"
			}
		}
	} else if act.Type == "type" && targetNode != nil {
		targetNode.Text = act.Text
	}

	assertionMet := true
	if act.AssertVisible != "" {
		found := m.root.FindByText(act.AssertVisible)
		assertionMet = (found != nil)
	}

	return &ActionResult{
		Status:        "success",
		Action:        act.Type,
		TargetNodeID:  act.NodeID,
		ResolvedText:  resolvedText,
		AssertionMet:  assertionMet,
		Message:       fmt.Sprintf("Dispatched %s on %s", act.Type, act.NodeID),
		VirtualTimeMs: time.Now().UnixMilli(),
	}, nil
}

func (m *MockASTInputModule) ResolveNode(nodeID string) (*ASTNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	node := m.root.FindByID(nodeID)
	if node == nil {
		node = m.root.FindByText(nodeID)
	}
	if node == nil {
		return nil, fmt.Errorf("node not found: %s", nodeID)
	}
	return node.Clone(), nil
}

func (m *MockASTInputModule) SetRootAST(root *ASTNode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.root = root
}

// MockProfilerMediaModule simulates heap and diagnostic telemetry.
type MockProfilerMediaModule struct {
	mu           sync.RWMutex
	InspectCalls int
}

func NewMockProfilerMediaModule() *MockProfilerMediaModule {
	return &MockProfilerMediaModule{}
}

func (m *MockProfilerMediaModule) InspectState(ctx context.Context, target string) (*InspectResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.InspectCalls++

	heap := m.GetHeapMetrics()
	return &InspectResult{
		Target:        target,
		AppID:         "com.prism.v",
		VirtualTimeMs: 1500,
		Heap:          heap,
		MemoryMB: map[string]any{
			"dalvik_allocated": heap.AllocatedMB,
			"dalvik_free":      heap.FreeMB,
			"native_allocated": 32.4,
		},
		ViewTreeStats: map[string]any{
			"total_views":   5,
			"active_window": "MainActivity",
		},
		LooperState: map[string]any{
			"virtual_time_ms": 1500,
			"pending_tasks":   0,
		},
	}, nil
}

func (m *MockProfilerMediaModule) GetHeapMetrics() *HeapInfo {
	return &HeapInfo{
		AllocatedMB: 48.2,
		FreeMB:      79.8,
		MaxMB:       128.0,
		ObjectsLive: 14200,
		GCCycles:    3,
	}
}

// MockCoordinator implements the full Coordinator interface backed by sub-mocks.
type MockCoordinator struct {
	mu             sync.RWMutex
	runtime        *MockRuntimeModule
	render         *MockRenderModule
	astInput       *MockASTInputModule
	profiler       *MockProfilerMediaModule
	activeAppID    string
	activeActivity string
	totalCalls     int64
	snapshots      map[string]*ASTNode
}

// NewMockCoordinator creates a ready-to-test Coordinator with realistic sample data.
func NewMockCoordinator() *MockCoordinator {
	return &MockCoordinator{
		runtime:        NewMockRuntimeModule(),
		render:         NewMockRenderModule(),
		astInput:       NewMockASTInputModule(),
		profiler:       NewMockProfilerMediaModule(),
		activeAppID:    "com.prism.v",
		activeActivity: "com.prism.v.MainActivity",
		snapshots:      make(map[string]*ASTNode),
	}
}

func (c *MockCoordinator) Runtime() RuntimeModule             { return c.runtime }
func (c *MockCoordinator) Render() RenderModule               { return c.render }
func (c *MockCoordinator) ASTInput() ASTInputModule           { return c.astInput }
func (c *MockCoordinator) Profiler() ProfilerMediaModule       { return c.profiler }

func (c *MockCoordinator) LaunchApp(ctx context.Context, apkPath string, activity string, clearState bool) (*LaunchResult, error) {
	atomic.AddInt64(&c.totalCalls, 1)
	req := LaunchRequest{
		APKPath:    apkPath,
		Activity:   activity,
		ClearState: clearState,
	}
	res, err := c.runtime.Launch(ctx, req)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.activeAppID = res.AppID
	c.activeActivity = res.Activity
	c.mu.Unlock()
	return res, nil
}

func (c *MockCoordinator) ObserveScreen(ctx context.Context, includeScreenshot bool, format string, pruneDepth int) (*ObserveResult, error) {
	atomic.AddInt64(&c.totalCalls, 1)
	ast, err := c.astInput.GetAST(ctx, pruneDepth)
	if err != nil {
		return nil, err
	}

	var screenshot *RenderResult
	if includeScreenshot {
		sRes, sErr := c.render.Capture(ctx, RenderRequest{Format: format})
		if sErr != nil {
			return nil, sErr
		}
		screenshot = sRes
	}

	c.mu.RLock()
	appID := c.activeAppID
	c.mu.RUnlock()

	return &ObserveResult{
		AppID:         appID,
		CurrentScreen: "MainActivity",
		AST:           ast,
		Screenshot:    screenshot,
		VisibleTexts:  ast.CollectVisibleTexts(),
		NodeCount:     len(ast.CollectVisibleTexts()),
		VirtualTimeMs: c.runtime.GetVirtualTime(),
	}, nil
}

func (c *MockCoordinator) DispatchAction(ctx context.Context, action Action) (*ActionResult, error) {
	atomic.AddInt64(&c.totalCalls, 1)
	return c.astInput.Dispatch(ctx, action)
}

func (c *MockCoordinator) StepTime(ctx context.Context, deltaMs int64) (*StepResult, error) {
	atomic.AddInt64(&c.totalCalls, 1)
	return c.runtime.StepLooper(ctx, deltaMs)
}

func (c *MockCoordinator) InspectState(ctx context.Context, target string) (*InspectResult, error) {
	atomic.AddInt64(&c.totalCalls, 1)
	return c.profiler.InspectState(ctx, target)
}

func (c *MockCoordinator) CheckHealth(ctx context.Context) (*HealthStatus, error) {
	atomic.AddInt64(&c.totalCalls, 1)
	c.mu.RLock()
	defer c.mu.RUnlock()

	heap := c.profiler.GetHeapMetrics()
	return &HealthStatus{
		OverallHealthy:  true,
		ActiveAppID:     c.activeAppID,
		VirtualTimeMs:   c.runtime.GetVirtualTime(),
		RuntimeHealthy:  true,
		RenderHealthy:   true,
		ASTHealthy:      true,
		ProfilerHealthy: true,
		SubsystemInfo: map[string]any{
			"runtime_running": c.runtime.IsAppRunning(c.activeAppID),
			"heap_allocated":  heap.AllocatedMB,
			"render_sample":   c.render.SampleWebP != "",
			"snapshots_count": len(c.snapshots),
		},
	}, nil
}

func (c *MockCoordinator) SaveSnapshot(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	ast, err := c.astInput.GetAST(context.Background(), 0)
	if err != nil {
		return err
	}
	c.snapshots[name] = ast.Clone()
	return nil
}

func (c *MockCoordinator) RestoreSnapshot(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	snap, ok := c.snapshots[name]
	if !ok {
		return fmt.Errorf("snapshot not found: %s", name)
	}
	c.astInput.SetRootAST(snap.Clone())
	return nil
}
