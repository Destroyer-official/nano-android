package mcp_compiler

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// MaestroCommandType defines standard Maestro commands.
type MaestroCommandType string

const (
	CmdLaunchApp          MaestroCommandType = "launchApp"
	CmdTapOn              MaestroCommandType = "tapOn"
	CmdDoubleTapOn        MaestroCommandType = "doubleTapOn"
	CmdLongPressOn        MaestroCommandType = "longPressOn"
	CmdInputText          MaestroCommandType = "inputText"
	CmdEraseText          MaestroCommandType = "eraseText"
	CmdPressKey           MaestroCommandType = "pressKey"
	CmdSwipe              MaestroCommandType = "swipe"
	CmdBack               MaestroCommandType = "back"
	CmdScroll             MaestroCommandType = "scroll"
	CmdScrollUntilVisible MaestroCommandType = "scrollUntilVisible"
	CmdAssertVisible      MaestroCommandType = "assertVisible"
	CmdAssertNotVisible   MaestroCommandType = "assertNotVisible"
	CmdAssertTrue         MaestroCommandType = "assertTrue"
	CmdSleep              MaestroCommandType = "sleep"
	CmdRepeat             MaestroCommandType = "repeat"
	CmdTakeScreenshot     MaestroCommandType = "takeScreenshot"
	CmdRunFlow            MaestroCommandType = "runFlow"
)

// MaestroCommand represents a single step in a Maestro test flow.
type MaestroCommand struct {
	Type           MaestroCommandType `json:"type"`
	Target         string             `json:"target,omitempty"`         // Human-readable text or selector string
	TargetID       string             `json:"target_id,omitempty"`      // Android resource ID or node ID
	Text           string             `json:"text,omitempty"`           // Text to input
	Direction      string             `json:"direction,omitempty"`      // UP, DOWN, LEFT, RIGHT
	DurationMs     int64              `json:"duration_ms,omitempty"`    // Wait / sleep duration
	RepeatTimes    int                `json:"repeat_times,omitempty"`   // For repeat blocks
	Commands       []MaestroCommand   `json:"commands,omitempty"`       // Nested commands (e.g. repeat)
	ClearState     bool               `json:"clear_state,omitempty"`    // For launchApp
	StopApp        bool               `json:"stop_app,omitempty"`       // For launchApp
	Key            string             `json:"key,omitempty"`            // For pressKey (ENTER, BACK, etc.)
	Condition      string             `json:"condition,omitempty"`      // For assertTrue (${...})
	ScreenshotPath string             `json:"screenshot_path,omitempty"` // For takeScreenshot
	FlowFile       string             `json:"flow_file,omitempty"`      // For runFlow
	Point          [2]float64         `json:"point,omitempty"`          // Coordinate tap [x, y]
	HasPoint       bool               `json:"has_point,omitempty"`
	Optional       bool               `json:"optional,omitempty"`
	TimeoutMs      int64              `json:"timeout_ms,omitempty"`
}

// CompileOptions configures the Maestro YAML generation.
type CompileOptions struct {
	AppID           string            `json:"app_id"`            // Target package ID (e.g. "com.prism.v")
	FlowName        string            `json:"flow_name"`         // Descriptive flow name
	RepeatCount     int               `json:"repeat_count"`      // Repetition count (default 3)
	WrapRepeat      bool              `json:"wrap_repeat"`       // Whether to wrap action commands in a Maestro repeat block
	IncludeComments bool              `json:"include_comments"`  // Include header comments
	OutputPath      string            `json:"output_path"`       // Optional file path to write YAML
	Optimize        bool              `json:"optimize"`          // Enable optimizer (sleep coalescing, assertion deduplication)
	Tags            []string          `json:"tags,omitempty"`    // Maestro tags (e.g. ["smoke", "regression"])
	Env             map[string]string `json:"env,omitempty"`     // Maestro environment variables
}

// DefaultCompileOptions provides sensible defaults.
func DefaultCompileOptions() CompileOptions {
	return CompileOptions{
		AppID:           "com.prism.v",
		FlowName:        "nano_regression_flow",
		RepeatCount:     3,
		WrapRepeat:      false,
		IncludeComments: true,
		Optimize:        true,
	}
}

// ValidationError records a flaw found in the test flow structure.
type ValidationError struct {
	Index   int    `json:"index"`
	Command string `json:"command"`
	Message string `json:"message"`
}

// SelectorScore rates confidence in a UI selector candidate.
type SelectorScore struct {
	Selector string  `json:"selector"`
	Type     string  `json:"type"` // "id", "text", "desc", "point"
	Score    float64 `json:"score"`
}

// MaestroCompiler manages recorded interaction sessions and compiles them to Maestro YAML.
type MaestroCompiler struct {
	mu        sync.RWMutex
	appID     string
	commands  []MaestroCommand
	lastAST   *ASTNode
	nodeIndex map[string]*ASTNode
}

// NewMaestroCompiler creates a new test compiler session.
func NewMaestroCompiler(defaultAppID string) *MaestroCompiler {
	if defaultAppID == "" {
		defaultAppID = "com.prism.v"
	}
	return &MaestroCompiler{
		appID:     defaultAppID,
		commands:  make([]MaestroCommand, 0),
		nodeIndex: make(map[string]*ASTNode),
	}
}

// SetAppID updates the current target application ID.
func (c *MaestroCompiler) SetAppID(appID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if appID != "" {
		c.appID = appID
	}
}

// GetAppID returns the current application ID.
func (c *MaestroCompiler) GetAppID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.appID
}

// UpdateAST updates the known UI tree and rebuilds the node index for smart selector matching.
func (c *MaestroCompiler) UpdateAST(ast *ASTNode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastAST = ast
	c.nodeIndex = make(map[string]*ASTNode)
	c.indexNodeRecursive(ast)
}

func (c *MaestroCompiler) indexNodeRecursive(node *ASTNode) {
	if node == nil {
		return
	}
	if node.ID != "" {
		c.nodeIndex[node.ID] = node
	}
	if node.ResourceID != "" {
		c.nodeIndex[node.ResourceID] = node
		if idx := strings.Index(node.ResourceID, ":id/"); idx != -1 {
			shortID := node.ResourceID[idx+4:]
			c.nodeIndex[shortID] = node
		}
	}
	if node.Text != "" {
		c.nodeIndex["text:"+node.Text] = node
	}
	for _, child := range node.Children {
		c.indexNodeRecursive(child)
	}
}

// RankSelector calculates the best selector strategy for a given AST node.
func (c *MaestroCompiler) RankSelector(node *ASTNode) SelectorScore {
	if node == nil {
		return SelectorScore{Selector: "", Type: "unknown", Score: 0}
	}

	// 1. Concise visible text is top priority for readable Maestro flows
	if node.Text != "" && len(node.Text) <= 40 {
		return SelectorScore{Selector: node.Text, Type: "text", Score: 1.0}
	}

	// 2. Accessibility Content Description
	if node.ContentDescription != "" {
		return SelectorScore{Selector: node.ContentDescription, Type: "desc", Score: 0.90}
	}

	// 3. Short Resource ID
	if node.ResourceID != "" {
		sel := node.ResourceID
		if idx := strings.Index(node.ResourceID, ":id/"); idx != -1 {
			sel = node.ResourceID[idx+4:]
		}
		return SelectorScore{Selector: sel, Type: "id", Score: 0.85}
	}

	// 4. Long text fallback
	if node.Text != "" {
		return SelectorScore{Selector: node.Text, Type: "text", Score: 0.70}
	}

	// 5. Node ID
	return SelectorScore{Selector: node.ID, Type: "node_id", Score: 0.50}
}

// ResolveBestSelector finds the cleanest, most robust Maestro selector for a target.
func (c *MaestroCompiler) ResolveBestSelector(nodeID string, fallbackText string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if fallbackText != "" {
		return fallbackText
	}
	if nodeID == "" {
		return ""
	}

	if node, ok := c.nodeIndex[nodeID]; ok {
		rank := c.RankSelector(node)
		if rank.Selector != "" {
			return rank.Selector
		}
	}

	if idx := strings.Index(nodeID, ":id/"); idx != -1 {
		return nodeID[idx+4:]
	}
	return nodeID
}

// RecordLaunch records an application launch command.
func (c *MaestroCompiler) RecordLaunch(appID string, clearState bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if appID != "" {
		c.appID = appID
	}
	c.commands = append(c.commands, MaestroCommand{
		Type:       CmdLaunchApp,
		ClearState: clearState,
	})
}

// RecordTap records a tap on a target node or text.
func (c *MaestroCompiler) RecordTap(nodeID string, fallbackText string, point [2]float64, hasPoint bool) {
	target := c.ResolveBestSelector(nodeID, fallbackText)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.commands = append(c.commands, MaestroCommand{
		Type:     CmdTapOn,
		Target:   target,
		TargetID: nodeID,
		Point:    point,
		HasPoint: hasPoint,
	})
}

// RecordDoubleTap records a double tap gesture.
func (c *MaestroCompiler) RecordDoubleTap(nodeID string, fallbackText string) {
	target := c.ResolveBestSelector(nodeID, fallbackText)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.commands = append(c.commands, MaestroCommand{
		Type:     CmdDoubleTapOn,
		Target:   target,
		TargetID: nodeID,
	})
}

// RecordLongPress records a long press gesture.
func (c *MaestroCompiler) RecordLongPress(nodeID string, fallbackText string) {
	target := c.ResolveBestSelector(nodeID, fallbackText)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.commands = append(c.commands, MaestroCommand{
		Type:     CmdLongPressOn,
		Target:   target,
		TargetID: nodeID,
	})
}

// RecordType records text entry.
func (c *MaestroCompiler) RecordType(text string, nodeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.commands = append(c.commands, MaestroCommand{
		Type:     CmdInputText,
		Text:     text,
		TargetID: nodeID,
	})
}

// RecordEraseText records text field deletion.
func (c *MaestroCompiler) RecordEraseText() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.commands = append(c.commands, MaestroCommand{
		Type: CmdEraseText,
	})
}

// RecordPressKey records a hardware or virtual key press.
func (c *MaestroCompiler) RecordPressKey(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	keyNorm := strings.ToUpper(strings.TrimSpace(key))
	c.commands = append(c.commands, MaestroCommand{
		Type: CmdPressKey,
		Key:  keyNorm,
	})
}

// RecordSwipe records a directional swipe gesture.
func (c *MaestroCompiler) RecordSwipe(direction string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	dir := strings.ToUpper(direction)
	if dir == "" {
		dir = "UP"
	}
	c.commands = append(c.commands, MaestroCommand{
		Type:      CmdSwipe,
		Direction: dir,
	})
}

// RecordBack records the Android back button press.
func (c *MaestroCompiler) RecordBack() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.commands = append(c.commands, MaestroCommand{
		Type: CmdBack,
	})
}

// RecordScroll records a scroll gesture.
func (c *MaestroCompiler) RecordScroll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.commands = append(c.commands, MaestroCommand{
		Type: CmdScroll,
	})
}

// RecordScrollUntilVisible records scrolling until target becomes visible.
func (c *MaestroCompiler) RecordScrollUntilVisible(target string, direction string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	dir := strings.ToUpper(direction)
	if dir == "" {
		dir = "DOWN"
	}
	c.commands = append(c.commands, MaestroCommand{
		Type:      CmdScrollUntilVisible,
		Target:    target,
		Direction: dir,
	})
}

// RecordSleep records a deterministic looper wait in milliseconds.
func (c *MaestroCompiler) RecordSleep(durationMs int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if durationMs <= 0 {
		return
	}
	c.commands = append(c.commands, MaestroCommand{
		Type:       CmdSleep,
		DurationMs: durationMs,
	})
}

// RecordAssertVisible records an expectation that an element/text is visible.
func (c *MaestroCompiler) RecordAssertVisible(text string, nodeID string) {
	target := c.ResolveBestSelector(nodeID, text)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.commands = append(c.commands, MaestroCommand{
		Type:     CmdAssertVisible,
		Target:   target,
		TargetID: nodeID,
	})
}

// RecordAssertNotVisible records an expectation that an element/text is absent.
func (c *MaestroCompiler) RecordAssertNotVisible(text string, nodeID string) {
	target := c.ResolveBestSelector(nodeID, text)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.commands = append(c.commands, MaestroCommand{
		Type:     CmdAssertNotVisible,
		Target:   target,
		TargetID: nodeID,
	})
}

// RecordAssertTrue records an assertion on an arbitrary expression.
func (c *MaestroCompiler) RecordAssertTrue(condition string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.commands = append(c.commands, MaestroCommand{
		Type:      CmdAssertTrue,
		Condition: condition,
	})
}

// RecordTakeScreenshot records a screenshot step.
func (c *MaestroCompiler) RecordTakeScreenshot(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.commands = append(c.commands, MaestroCommand{
		Type:           CmdTakeScreenshot,
		ScreenshotPath: path,
	})
}

// Reset clears recorded commands and starts a fresh session.
func (c *MaestroCompiler) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.commands = make([]MaestroCommand, 0)
}

// CommandCount returns the number of recorded actions.
func (c *MaestroCompiler) CommandCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.commands)
}

// GetCommands returns a copy of all recorded commands.
func (c *MaestroCompiler) GetCommands() []MaestroCommand {
	c.mu.RLock()
	defer c.mu.RUnlock()
	copied := make([]MaestroCommand, len(c.commands))
	copy(copied, c.commands)
	return copied
}

// Optimize removes redundant sleeps and duplicate assertions in-place.
func (c *MaestroCompiler) Optimize() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	initialCount := len(c.commands)
	if initialCount <= 1 {
		return 0
	}

	optimized := make([]MaestroCommand, 0, initialCount)
	for _, cmd := range c.commands {
		if len(optimized) == 0 {
			optimized = append(optimized, cmd)
			continue
		}

		lastIdx := len(optimized) - 1
		lastCmd := &optimized[lastIdx]

		// 1. Coalesce sequential sleep commands: sleep 100 + sleep 200 => sleep 300
		if cmd.Type == CmdSleep && lastCmd.Type == CmdSleep {
			lastCmd.DurationMs += cmd.DurationMs
			continue
		}

		// 2. Eliminate consecutive identical assertions: assertVisible "X" + assertVisible "X" => single assert
		if (cmd.Type == CmdAssertVisible && lastCmd.Type == CmdAssertVisible) ||
			(cmd.Type == CmdAssertNotVisible && lastCmd.Type == CmdAssertNotVisible) {
			if cmd.Target == lastCmd.Target && cmd.TargetID == lastCmd.TargetID {
				continue
			}
		}

		optimized = append(optimized, cmd)
	}

	c.commands = optimized
	return initialCount - len(optimized)
}

// Validate lints recorded commands against Maestro YAML standards.
func (c *MaestroCompiler) Validate() []ValidationError {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var errors []ValidationError
	pkgRegex := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*(\.[a-zA-Z][a-zA-Z0-9_]*)+$`)
	if c.appID != "" && !pkgRegex.MatchString(c.appID) {
		errors = append(errors, ValidationError{
			Index:   -1,
			Command: "appId",
			Message: fmt.Sprintf("invalid application package ID: %q", c.appID),
		})
	}

	for i, cmd := range c.commands {
		switch cmd.Type {
		case CmdTapOn, CmdDoubleTapOn, CmdLongPressOn:
			if cmd.Target == "" && !cmd.HasPoint && cmd.TargetID == "" {
				errors = append(errors, ValidationError{
					Index:   i,
					Command: string(cmd.Type),
					Message: "tap requires either a target text, id, or point coordinate",
				})
			}
		case CmdInputText:
			if cmd.Text == "" {
				errors = append(errors, ValidationError{
					Index:   i,
					Command: string(cmd.Type),
					Message: "inputText requires non-empty text",
				})
			}
		case CmdAssertVisible, CmdAssertNotVisible:
			if cmd.Target == "" && cmd.TargetID == "" {
				errors = append(errors, ValidationError{
					Index:   i,
					Command: string(cmd.Type),
					Message: "assertion requires target selector or text",
				})
			}
		case CmdSleep:
			if cmd.DurationMs <= 0 {
				errors = append(errors, ValidationError{
					Index:   i,
					Command: string(cmd.Type),
					Message: "sleep duration must be positive",
				})
			}
		case CmdRepeat:
			if cmd.RepeatTimes <= 0 || len(cmd.Commands) == 0 {
				errors = append(errors, ValidationError{
					Index:   i,
					Command: string(cmd.Type),
					Message: "repeat block requires repeat_times > 0 and at least 1 nested command",
				})
			}
		}
	}

	return errors
}

// Compile generates a production-ready Maestro YAML flow string.
func (c *MaestroCompiler) Compile(opts CompileOptions) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	appID := opts.AppID
	if appID == "" {
		appID = c.appID
	}
	if appID == "" {
		appID = "com.prism.v"
	}

	repeatCount := opts.RepeatCount
	if repeatCount <= 0 {
		repeatCount = 3
	}

	var buf bytes.Buffer

	// Top comments and metadata
	if opts.IncludeComments {
		flowName := opts.FlowName
		if flowName == "" {
			flowName = "nano_regression_flow"
		}
		buf.WriteString(fmt.Sprintf("# Maestro Regression Flow: %s\n", flowName))
		buf.WriteString(fmt.Sprintf("# repeat_count: %d\n", repeatCount))
	}

	// Tags
	if len(opts.Tags) > 0 {
		buf.WriteString("tags:\n")
		for _, tag := range opts.Tags {
			buf.WriteString(fmt.Sprintf("  - %s\n", tag))
		}
	}

	// Environment variables
	if len(opts.Env) > 0 {
		buf.WriteString("env:\n")
		for k, v := range opts.Env {
			buf.WriteString(fmt.Sprintf("  %s: %q\n", k, v))
		}
	}

	// Maestro appId header
	buf.WriteString(fmt.Sprintf("appId: %s\n", appID))
	buf.WriteString("---\n")

	// Check for recorded launch command
	cmds := c.commands
	if opts.Optimize {
		cmds = c.optimizeCommands(cmds)
	}

	var launchCmd *MaestroCommand
	for i := range cmds {
		if cmds[i].Type == CmdLaunchApp {
			launchCmd = &cmds[i]
			break
		}
	}

	if opts.WrapRepeat && repeatCount > 1 {
		if launchCmd != nil {
			c.formatCommand(&buf, *launchCmd, "")
		} else {
			buf.WriteString("- launchApp\n")
		}

		buf.WriteString(fmt.Sprintf("- repeat:\n    times: %d\n    commands:\n", repeatCount))
		for _, cmd := range cmds {
			if cmd.Type == CmdLaunchApp {
				continue // Don't repeat launch inside repeat block
			}
			c.formatCommand(&buf, cmd, "      ")
		}
	} else {
		if launchCmd == nil {
			buf.WriteString("- launchApp\n")
		}
		for _, cmd := range cmds {
			c.formatCommand(&buf, cmd, "")
		}
	}

	return buf.String(), nil
}

func (c *MaestroCompiler) optimizeCommands(cmds []MaestroCommand) []MaestroCommand {
	if len(cmds) <= 1 {
		return cmds
	}
	out := make([]MaestroCommand, 0, len(cmds))
	for _, cmd := range cmds {
		if len(out) == 0 {
			out = append(out, cmd)
			continue
		}
		last := &out[len(out)-1]
		if cmd.Type == CmdSleep && last.Type == CmdSleep {
			last.DurationMs += cmd.DurationMs
			continue
		}
		if (cmd.Type == CmdAssertVisible && last.Type == CmdAssertVisible) ||
			(cmd.Type == CmdAssertNotVisible && last.Type == CmdAssertNotVisible) {
			if cmd.Target == last.Target && cmd.TargetID == last.TargetID {
				continue
			}
		}
		out = append(out, cmd)
	}
	return out
}

func (c *MaestroCompiler) formatCommand(buf *bytes.Buffer, cmd MaestroCommand, indent string) {
	prefix := indent + "- "
	nestedIndent := indent + "    "

	switch cmd.Type {
	case CmdLaunchApp:
		if cmd.ClearState || cmd.StopApp {
			buf.WriteString(fmt.Sprintf("%slaunchApp:\n", prefix))
			if cmd.ClearState {
				buf.WriteString(fmt.Sprintf("%sclearState: true\n", nestedIndent))
			}
			if cmd.StopApp {
				buf.WriteString(fmt.Sprintf("%sstopApp: true\n", nestedIndent))
			}
		} else {
			buf.WriteString(fmt.Sprintf("%slaunchApp\n", prefix))
		}

	case CmdTapOn:
		if cmd.Target != "" {
			buf.WriteString(fmt.Sprintf("%stapOn: %q\n", prefix, cmd.Target))
		} else if cmd.HasPoint {
			buf.WriteString(fmt.Sprintf("%stapOn:\n%spoint: \"%.0f,%.0f\"\n", prefix, nestedIndent, cmd.Point[0], cmd.Point[1]))
		} else if cmd.TargetID != "" {
			buf.WriteString(fmt.Sprintf("%stapOn:\n%sid: %q\n", prefix, nestedIndent, cmd.TargetID))
		}

	case CmdDoubleTapOn:
		if cmd.Target != "" {
			buf.WriteString(fmt.Sprintf("%sdoubleTapOn: %q\n", prefix, cmd.Target))
		} else if cmd.TargetID != "" {
			buf.WriteString(fmt.Sprintf("%sdoubleTapOn:\n%sid: %q\n", prefix, nestedIndent, cmd.TargetID))
		}

	case CmdLongPressOn:
		if cmd.Target != "" {
			buf.WriteString(fmt.Sprintf("%slongPressOn: %q\n", prefix, cmd.Target))
		} else if cmd.TargetID != "" {
			buf.WriteString(fmt.Sprintf("%slongPressOn:\n%sid: %q\n", prefix, nestedIndent, cmd.TargetID))
		}

	case CmdInputText:
		buf.WriteString(fmt.Sprintf("%sinputText: %q\n", prefix, cmd.Text))

	case CmdEraseText:
		buf.WriteString(fmt.Sprintf("%seraseText\n", prefix))

	case CmdPressKey:
		key := cmd.Key
		if key == "" {
			key = "ENTER"
		}
		buf.WriteString(fmt.Sprintf("%spressKey: %s\n", prefix, key))

	case CmdSwipe:
		dir := cmd.Direction
		if dir == "" {
			dir = "UP"
		}
		buf.WriteString(fmt.Sprintf("%sswipe:\n%sdirection: %s\n", prefix, nestedIndent, dir))

	case CmdBack:
		buf.WriteString(fmt.Sprintf("%sback\n", prefix))

	case CmdScroll:
		buf.WriteString(fmt.Sprintf("%sscroll\n", prefix))

	case CmdScrollUntilVisible:
		buf.WriteString(fmt.Sprintf("%sscrollUntilVisible:\n%selement: %q\n%sdirection: %s\n", prefix, nestedIndent, cmd.Target, nestedIndent, cmd.Direction))

	case CmdAssertVisible:
		buf.WriteString(fmt.Sprintf("%sassertVisible: %q\n", prefix, cmd.Target))

	case CmdAssertNotVisible:
		buf.WriteString(fmt.Sprintf("%sassertNotVisible: %q\n", prefix, cmd.Target))

	case CmdAssertTrue:
		buf.WriteString(fmt.Sprintf("%sassertTrue: %q\n", prefix, cmd.Condition))

	case CmdSleep:
		buf.WriteString(fmt.Sprintf("%ssleep: %d\n", prefix, cmd.DurationMs))

	case CmdTakeScreenshot:
		buf.WriteString(fmt.Sprintf("%stakeScreenshot: %q\n", prefix, cmd.ScreenshotPath))

	case CmdRunFlow:
		buf.WriteString(fmt.Sprintf("%srunFlow: %q\n", prefix, cmd.FlowFile))

	default:
		if cmd.Target != "" {
			buf.WriteString(fmt.Sprintf("%s%s: %q\n", prefix, string(cmd.Type), cmd.Target))
		} else {
			buf.WriteString(fmt.Sprintf("%s%s\n", prefix, string(cmd.Type)))
		}
	}
}

// CompileToFile compiles the current session and writes it atomically to disk.
func (c *MaestroCompiler) CompileToFile(outputPath string, opts CompileOptions) error {
	yamlContent, err := c.Compile(opts)
	if err != nil {
		return fmt.Errorf("compile error: %w", err)
	}

	dir := filepath.Dir(outputPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	if err := os.WriteFile(outputPath, []byte(yamlContent), 0644); err != nil {
		return fmt.Errorf("failed to write Maestro flow to %s: %w", outputPath, err)
	}

	return nil
}
