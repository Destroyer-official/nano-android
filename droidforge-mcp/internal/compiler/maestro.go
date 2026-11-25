package compiler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/anthropic/droidforge-mcp/internal/types"
	"gopkg.in/yaml.v3"
)

var (
	// Regex patterns for dynamic text detection
	datePattern       = regexp.MustCompile(`(?i)\b(\d{1,2}[/-]\d{1,2}[/-]\d{2,4}|\d{4}[/-]\d{1,2}[/-]\d{1,2}|jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec|today|yesterday|tomorrow)\b`)
	timePattern       = regexp.MustCompile(`(?i)\b(\d{1,2}:\d{2}(:\d{2})?\s*(am|pm)?|\d+\s*(sec|min|hr|hour|day)s?\s*ago|just\s*now)\b`)
	currencyPattern   = regexp.MustCompile(`(?i)[\$€£¥₹]\s*\d+([.,]\d{2})?`)
	percentagePattern = regexp.MustCompile(`\b\d+([.,]\d+)?%`)
	uuidPattern       = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	digitPattern      = regexp.MustCompile(`\d`)
	coordinateRegexp  = regexp.MustCompile(`^(\d+)\s*,\s*(\d+)$`)
	percentageRegexp  = regexp.MustCompile(`^(\d{1,3})%\s*,\s*(\d{1,3})%$`)
)

// SelectorResult contains the resolved selector, its rubric tier, and any warning.
type SelectorResult struct {
	SelectorType string // "id", "text", "point"
	Value        string
	Warning      string
}

// HookStep represents an external stimulus step to be executed alongside Maestro.
type HookStep struct {
	StepIndex int            `json:"step_index"`
	Action    string         `json:"action"`
	Target    string         `json:"target,omitempty"`
	Text      string         `json:"text,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
}

// HooksManifest is the structure of companion .hooks.json files.
type HooksManifest struct {
	FlowName string     `json:"flow_name"`
	YAMLPath string     `json:"yaml_path"`
	Hooks    []HookStep `json:"hooks"`
}

// MaestroFlow represents the top-level structure of a Maestro YAML file.
type MaestroFlow struct {
	AppID string   `yaml:"appId"`
	Name  string   `yaml:"name"`
	Tags  []string `yaml:"tags,omitempty"`
}

// ResolveSelector applies the strict Selector Priority Rubric:
// 1. id: (resourceId / testTag) — gold standard
// 2. text: from static contentDescription — acceptable if unique
// 3. text: from visible label — emit weak_selector_warning if dynamic
// 4. point: (percentage coordinates) — last resort, always warns
func ResolveSelector(target string, nodes []types.PrunedNode, stepIndex int) SelectorResult {
	// 1. If target matches a node ID ("n1", "n2", etc.) in the provided nodes
	var matchedNode *types.PrunedNode
	for i := range nodes {
		if nodes[i].ID == target {
			matchedNode = &nodes[i]
			break
		}
	}

	if matchedNode != nil {
		return resolveNodeSelector(*matchedNode, nodes, stepIndex)
	}

	// 2. Target was not a node ID, analyze target string directly
	if strings.HasPrefix(target, "id:") {
		idVal := strings.TrimPrefix(target, "id:")
		idVal = cleanResourceID(strings.TrimSpace(idVal))
		return SelectorResult{SelectorType: "id", Value: idVal}
	}

	// If target looks like a full Android resource ID (e.g., com.example:id/foo)
	if strings.Contains(target, ":id/") || (strings.HasPrefix(target, "btn_") || strings.HasPrefix(target, "txt_") || strings.HasPrefix(target, "input_") || strings.HasPrefix(target, "card_")) {
		return SelectorResult{SelectorType: "id", Value: cleanResourceID(target)}
	}

	// Check if target is a coordinate pair (e.g., "540,960" or "50%,60%")
	if percentageRegexp.MatchString(target) {
		return SelectorResult{
			SelectorType: "point",
			Value:        target,
			Warning:      fmt.Sprintf("Step %d: raw percentage point %q used as selector (last resort); add testTag in Compose", stepIndex, target),
		}
	}
	if m := coordinateRegexp.FindStringSubmatch(target); m != nil {
		x, _ := strconv.Atoi(m[1])
		y, _ := strconv.Atoi(m[2])
		// Convert to estimated percentage (assuming 1080x2400 standard emulator if dimensions unknown)
		pctX := (x * 100) / 1080
		pctY := (y * 100) / 2400
		ptVal := fmt.Sprintf("%d%%,%d%%", pctX, pctY)
		return SelectorResult{
			SelectorType: "point",
			Value:        ptVal,
			Warning:      fmt.Sprintf("Step %d: raw pixel coordinates %q converted to %q (last resort); add testTag in Compose", stepIndex, target, ptVal),
		}
	}

	// Default fallback for raw string target: treat as text selector
	warn := checkDynamicText(target, stepIndex, "target text")
	return SelectorResult{
		SelectorType: "text",
		Value:        target,
		Warning:      warn,
	}
}

func resolveNodeSelector(node types.PrunedNode, allNodes []types.PrunedNode, stepIndex int) SelectorResult {
	// Priority 1: Resource ID / Compose testTag
	if node.ResID != "" {
		return SelectorResult{
			SelectorType: "id",
			Value:        cleanResourceID(node.ResID),
		}
	}

	// Priority 2: Static contentDescription (acceptable if unique on screen)
	if node.ContentDesc != "" {
		isUnique := true
		count := 0
		for _, other := range allNodes {
			if other.ContentDesc == node.ContentDesc {
				count++
			}
		}
		if count > 1 {
			isUnique = false
		}

		if isUnique {
			return SelectorResult{
				SelectorType: "text",
				Value:        node.ContentDesc,
			}
		}
	}

	// Priority 3: Visible text label
	if node.Text != "" {
		warn := checkDynamicText(node.Text, stepIndex, "visible label")
		return SelectorResult{
			SelectorType: "text",
			Value:        node.Text,
			Warning:      warn,
		}
	}

	// If contentDescription exists but was not unique, use it with a warning
	if node.ContentDesc != "" {
		return SelectorResult{
			SelectorType: "text",
			Value:        node.ContentDesc,
			Warning:      fmt.Sprintf("Step %d: contentDescription %q is not unique on screen", stepIndex, node.ContentDesc),
		}
	}

	// Priority 4: Coordinate point from bounds [left, top, right, bottom]
	cx := (node.Bounds[0] + node.Bounds[2]) / 2
	cy := (node.Bounds[1] + node.Bounds[3]) / 2
	pctX := (cx * 100) / 1080
	pctY := (cy * 100) / 2400
	if pctX <= 0 {
		pctX = 50
	}
	if pctY <= 0 {
		pctY = 50
	}
	ptVal := fmt.Sprintf("%d%%,%d%%", pctX, pctY)

	return SelectorResult{
		SelectorType: "point",
		Value:        ptVal,
		Warning:      fmt.Sprintf("Step %d: node %s has no id/desc/text; raw coordinates %q used as last resort; add testTag in Compose", stepIndex, node.ID, ptVal),
	}
}

func checkDynamicText(text string, stepIndex int, labelType string) string {
	isDynamic := false
	var reasons []string

	if uuidPattern.MatchString(text) {
		isDynamic = true
		reasons = append(reasons, "contains UUID")
	}
	if currencyPattern.MatchString(text) {
		isDynamic = true
		reasons = append(reasons, "contains currency price")
	}
	if percentagePattern.MatchString(text) {
		isDynamic = true
		reasons = append(reasons, "contains percentage")
	}
	if timePattern.MatchString(text) {
		isDynamic = true
		reasons = append(reasons, "contains timestamp or relative time")
	}
	if datePattern.MatchString(text) {
		isDynamic = true
		reasons = append(reasons, "contains date or relative day")
	}
	if digitPattern.MatchString(text) {
		isDynamic = true
		reasons = append(reasons, "contains numbers")
	}
	if len(text) > 32 {
		isDynamic = true
		reasons = append(reasons, "exceeds 32 characters")
	}

	if isDynamic {
		return fmt.Sprintf("Step %d: %s %q may be dynamic (%s)", stepIndex, labelType, text, strings.Join(reasons, ", "))
	}
	return ""
}

func cleanResourceID(resID string) string {
	if idx := strings.Index(resID, ":id/"); idx != -1 {
		return resID[idx+4:]
	}
	return resID
}

// CompileToMaestro compiles a slice of SessionSteps into Maestro YAML and optional hooks JSON.
func CompileToMaestro(appID, flowName string, steps []types.SessionStep) (yamlContent string, hooksJSON []byte, warnings []string, err error) {
	if appID == "" {
		appID = "com.example.app"
	}
	if flowName == "" {
		flowName = "Frozen Exploration Flow"
	}

	header := MaestroFlow{
		AppID: appID,
		Name:  flowName,
		Tags: []string{
			"frozen-by-droidforge",
		},
	}

	var commands []map[string]any

	// Initial launch command with clear state
	commands = append(commands, map[string]any{
		"launchApp": map[string]any{
			"clearState": true,
		},
	})

	var hooks []HookStep
	var priorNodes []types.PrunedNode

	for idx, step := range steps {
		stepNum := idx + 1

		// External stimuli steps
		if isStimulusAction(step.Action) {
			hook := HookStep{
				StepIndex: stepNum,
				Action:    step.Action,
				Target:    step.Target,
				Text:      step.Text,
			}
			hooks = append(hooks, hook)
			continue
		}

		switch step.Action {
		case "tap_node", "tap", "click":
			sel := ResolveSelector(step.Target, priorNodes, stepNum)
			if sel.Warning != "" {
				warnings = append(warnings, sel.Warning)
			}
			targetMap := map[string]any{
				sel.SelectorType: sel.Value,
			}
			commands = append(commands, map[string]any{
				"tapOn": targetMap,
			})

		case "long_press", "long_click":
			sel := ResolveSelector(step.Target, priorNodes, stepNum)
			if sel.Warning != "" {
				warnings = append(warnings, sel.Warning)
			}
			commands = append(commands, map[string]any{
				"longPressOn": map[string]any{
					sel.SelectorType: sel.Value,
				},
			})

		case "type", "inputText":
			if step.Target != "" && step.Target != "keyboard" {
				sel := ResolveSelector(step.Target, priorNodes, stepNum)
				if sel.Warning != "" {
					warnings = append(warnings, sel.Warning)
				}
				commands = append(commands, map[string]any{
					"tapOn": map[string]any{
						sel.SelectorType: sel.Value,
					},
				})
				// Erase existing text if modifying an input
				commands = append(commands, map[string]any{
					"eraseText": nil,
				})
			}
			commands = append(commands, map[string]any{
				"inputText": step.Text,
			})

		case "scroll_until_visible":
			sel := ResolveSelector(step.Target, step.NodesAfter, stepNum)
			if sel.Warning != "" {
				warnings = append(warnings, sel.Warning)
			}
			commands = append(commands, map[string]any{
				"scrollUntilVisible": map[string]any{
					"element": map[string]any{
						sel.SelectorType: sel.Value,
					},
					"direction": "DOWN",
				},
			})

		case "swipe":
			direction := "DOWN"
			if step.Text != "" {
				direction = strings.ToUpper(step.Text)
			}
			commands = append(commands, map[string]any{
				"swipe": map[string]any{
					"direction": direction,
				},
			})

		case "key", "press_key", "system_key":
			key := strings.ToLower(step.Target)
			if key == "back" {
				commands = append(commands, map[string]any{
					"back": nil,
				})
			} else if key == "home" {
				commands = append(commands, map[string]any{
					"pressKey": "Home",
				})
			} else if key == "hide_keyboard" {
				commands = append(commands, map[string]any{
					"hideKeyboard": nil,
				})
			} else {
				commands = append(commands, map[string]any{
					"pressKey": step.Target,
				})
			}

		case "intent", "open_link", "deep_link":
			uri := step.Target
			if uri == "" {
				uri = step.Text
			}
			commands = append(commands, map[string]any{
				"openLink": uri,
			})

		case "observe", "assert":
			if step.Target != "" {
				sel := ResolveSelector(step.Target, step.NodesAfter, stepNum)
				if sel.Warning != "" {
					warnings = append(warnings, sel.Warning)
				}
				commands = append(commands, map[string]any{
					"assertVisible": map[string]any{
						sel.SelectorType: sel.Value,
					},
				})
			}
		}

		// Update priorNodes for the next step
		if len(step.NodesAfter) > 0 {
			priorNodes = step.NodesAfter
		}
	}

	headerBytes, err := yaml.Marshal(header)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to marshal flow header: %w", err)
	}

	bodyBytes, err := yaml.Marshal(commands)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to marshal flow commands: %w", err)
	}

	yamlContent = string(headerBytes) + "---\n" + string(bodyBytes)

	if len(hooks) > 0 {
		manifest := HooksManifest{
			FlowName: flowName,
			YAMLPath: flowName + ".yaml",
			Hooks:    hooks,
		}
		hooksJSON, err = json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return "", nil, nil, fmt.Errorf("failed to marshal hooks manifest: %w", err)
		}
	}

	return yamlContent, hooksJSON, warnings, nil
}

func isStimulusAction(action string) bool {
	switch action {
	case "sim_inject_sms_or_call", "sim_post_notification", "sms_inject", "notification_post", "sim_set_permissions":
		return true
	default:
		return false
	}
}

// FreezeSessionToMaestro compiles a recorded session into a Maestro YAML file and optional companion .hooks.json file.
func FreezeSessionToMaestro(session *SessionRecorder, flowName, outputPath string) (*types.FreezeResult, error) {
	steps := session.NetSteps()

	appID := session.AppID()
	if appID == "" {
		appID = "com.example.app"
	}
	if flowName == "" {
		flowName = session.SessionID()
	}
	if outputPath == "" {
		outputPath = filepath.Join(".droidforge", "flows", flowName+".yaml")
	}

	yamlContent, hooksData, warnings, err := CompileToMaestro(appID, flowName, steps)
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create output directory %s: %w", dir, err)
	}

	if err := os.WriteFile(outputPath, []byte(yamlContent), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write Maestro YAML to %s: %w", outputPath, err)
	}

	var hooksPath string
	if len(hooksData) > 0 {
		ext := filepath.Ext(outputPath)
		base := strings.TrimSuffix(outputPath, ext)
		hooksPath = base + ".hooks.json"
		if err := os.WriteFile(hooksPath, hooksData, 0o644); err != nil {
			return nil, fmt.Errorf("failed to write companion hooks JSON to %s: %w", hooksPath, err)
		}
	}

	return &types.FreezeResult{
		YAMLPath:          outputPath,
		StepCount:         len(steps),
		WeakSelectorWarns: warnings,
		HooksJSONPath:     hooksPath,
	}, nil
}
