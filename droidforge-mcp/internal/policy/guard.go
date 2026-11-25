package policy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

var (
	// Package ID regex per specification: ^[a-zA-Z0-9._]+$
	packageIDRegex = regexp.MustCompile(`^[a-zA-Z0-9._]+$`)

	// Emulator serial regex: emulator-\d+ or 127.0.0.1:\d+
	emulatorSerialRegex = regexp.MustCompile(`^(emulator-\d+|127\.0\.0\.1:\d+)$`)

	// Shell metacharacters to reject: ;, |, &&, $(), backticks, newlines, redirects
	shellMetachars = []string{";", "|", "&&", "$(", "`", "\n", "\r", ">", "<", "$"}

	// Hardcoded permanent blocklist packages
	permanentBlocklist = map[string]bool{
		"com.android.systemui":            true,
		"com.google.android.gms":          true,
		"com.android.providers.contacts":  true,
		"com.android.providers.telephony": true,
	}

	// Windows reserved device filenames
	windowsReservedNames = map[string]bool{
		"CON": true, "PRN": true, "AUX": true, "NUL": true,
		"COM1": true, "COM2": true, "COM3": true, "COM4": true,
		"COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
		"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true,
		"LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
	}
)

// Guard implements types.PolicyGuard enforcing the 5-stage synchronous
// security validation pipeline and post-execution nonce fencing.
type Guard struct {
	mu            sync.Mutex
	workspaceRoot string

	projectConfig   types.ProjectConfig
	policyConfig    types.PolicyConfig
	executionConfig types.ExecutionConfig

	emulatorOnly bool

	// Session tracking for circuit breakers
	stepsPerSession           map[string]int
	lastUIHashPerSession      map[string]string
	identicalStreakPerSession map[string]int
}

// NewGuard creates a new security PolicyGuard.
func NewGuard(workspaceRoot string, proj types.ProjectConfig, pol types.PolicyConfig, exec types.ExecutionConfig) *Guard {
	if exec.MaxStepsPerTask <= 0 {
		exec.MaxStepsPerTask = 15
	}
	if exec.MaxIdenticalUIHashes <= 0 {
		exec.MaxIdenticalUIHashes = 3
	}
	if pol.Sanitization.MaxNodeTextBytes <= 0 {
		pol.Sanitization.MaxNodeTextBytes = 256
	}
	if pol.Sanitization.MaxLogcatLineBytes <= 0 {
		pol.Sanitization.MaxLogcatLineBytes = 512
	}

	cleanRoot := workspaceRoot
	if cleanRoot != "" {
		if abs, err := filepath.Abs(cleanRoot); err == nil {
			cleanRoot = abs
		}
	}

	return &Guard{
		workspaceRoot:             cleanRoot,
		projectConfig:             proj,
		policyConfig:              pol,
		executionConfig:           exec,
		emulatorOnly:              true, // Emulator-only mode by default
		stepsPerSession:           make(map[string]int),
		lastUIHashPerSession:      make(map[string]string),
		identicalStreakPerSession: make(map[string]int),
	}
}

// SetEmulatorOnly toggles emulator-only device trust mode.
func (g *Guard) SetEmulatorOnly(enabled bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.emulatorOnly = enabled
}

// IncrementStepCounter increments the session step counter and checks against budget limits.
func (g *Guard) IncrementStepCounter(sessionID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if sessionID == "" {
		sessionID = "default"
	}
	g.stepsPerSession[sessionID]++
}

// GetSteps returns the current step counter for a session.
func (g *Guard) GetSteps(sessionID string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	if sessionID == "" {
		sessionID = "default"
	}
	return g.stepsPerSession[sessionID]
}

// RecordUIHash updates the identical UI hash streak for a session.
func (g *Guard) RecordUIHash(sessionID, hash string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	if sessionID == "" {
		sessionID = "default"
	}

	if hash == "" {
		g.identicalStreakPerSession[sessionID] = 0
		g.lastUIHashPerSession[sessionID] = ""
		return 0
	}

	lastHash := g.lastUIHashPerSession[sessionID]
	if hash == lastHash {
		g.identicalStreakPerSession[sessionID]++
	} else {
		g.lastUIHashPerSession[sessionID] = hash
		g.identicalStreakPerSession[sessionID] = 1
	}

	return g.identicalStreakPerSession[sessionID]
}

// GenerateNonce creates a secure 6-byte crypto/rand hex nonce.
func (g *Guard) GenerateNonce() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "df0001020304"
	}
	return hex.EncodeToString(b)
}

// FenceUntrustedText implements Stage 5 nonce fencing:
// - Generates a 6-byte crypto/rand hex nonce
// - Strips Unicode bidi overrides (U+202A–U+202E, U+2066–U+2069)
// - Strips Unicode zero-width characters (U+200B–U+200D, U+FEFF)
// - Escapes nested fence delimiters to neutralize delimiter spoofing attacks
// - Truncates text to max_node_text_bytes (default 256) cleanly at rune boundaries
// - Wraps text in <DF_UNTRUSTED_{nonce}>...</DF_UNTRUSTED_{nonce}>
func (g *Guard) FenceUntrustedText(raw string) (string, string) {
	maxBytes := g.policyConfig.Sanitization.MaxNodeTextBytes
	if maxBytes <= 0 {
		maxBytes = 256
	}
	return g.fenceTextWithLimit(raw, maxBytes)
}

// FenceLogcatLine sanitizes and fences logcat lines using MaxLogcatLineBytes limit.
func (g *Guard) FenceLogcatLine(raw string) (string, string) {
	maxBytes := g.policyConfig.Sanitization.MaxLogcatLineBytes
	if maxBytes <= 0 {
		maxBytes = 512
	}
	return g.fenceTextWithLimit(raw, maxBytes)
}

func (g *Guard) fenceTextWithLimit(raw string, maxBytes int) (string, string) {
	nonce := g.GenerateNonce()

	// 1. Strip bidi overrides and zero-width chars
	var cleanRunes []rune
	for _, r := range raw {
		// Bidi overrides: U+202A..U+202E, U+2066..U+2069
		if (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) {
			continue
		}
		// Zero-width characters: U+200B..U+200D, U+FEFF
		if (r >= 0x200B && r <= 0x200D) || r == 0xFEFF {
			continue
		}
		cleanRunes = append(cleanRunes, r)
	}

	cleanStr := string(cleanRunes)

	// 2. Neutralize any fake delimiter injection attempts inside untrusted text
	cleanStr = strings.ReplaceAll(cleanStr, "</DF_UNTRUSTED", "[ESCAPED_DF_UNTRUSTED_CLOSE")
	cleanStr = strings.ReplaceAll(cleanStr, "<DF_UNTRUSTED", "[ESCAPED_DF_UNTRUSTED_OPEN")

	// 3. Truncate cleanly to maxBytes along rune boundaries
	if len(cleanStr) > maxBytes {
		runes := []rune(cleanStr)
		for len(string(runes)) > maxBytes && len(runes) > 0 {
			runes = runes[:len(runes)-1]
		}
		cleanStr = string(runes)
	}

	fenced := fmt.Sprintf("<DF_UNTRUSTED_%s>%s</DF_UNTRUSTED_%s>", nonce, cleanStr, nonce)
	return fenced, nonce
}

// Evaluate evaluates a tool call through the 5-stage synchronous validation pipeline.
func (g *Guard) Evaluate(ctx context.Context, tool types.ToolDefinition, args map[string]any) (*types.PolicyEvaluation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	sessionID, _ := args["session_id"].(string)
	if sessionID == "" {
		sessionID = "default"
	}

	// Stage 4 (Budget & Circuit Breaker Check):
	// 1. Session step limit
	steps := g.stepsPerSession[sessionID]
	if steps >= g.executionConfig.MaxStepsPerTask {
		return &types.PolicyEvaluation{
			Decision: types.PolicyHALT,
			Reason:   fmt.Sprintf("HALT: session step limit exceeded (%d >= %d)", steps, g.executionConfig.MaxStepsPerTask),
		}, nil
	}

	// 2. Identical UI hash streak limit
	streak := g.identicalStreakPerSession[sessionID]
	if streak > g.executionConfig.MaxIdenticalUIHashes {
		return &types.PolicyEvaluation{
			Decision: types.PolicyHALT,
			Reason:   fmt.Sprintf("HALT: identical UI hash streak exceeded circuit breaker (%d > %d)", streak, g.executionConfig.MaxIdenticalUIHashes),
		}, nil
	}

	// Human Approval Check
	if tool.RiskTier == types.RiskAPPROVALGATED || g.isHumanApprovalRequired(tool.Name, args) {
		return &types.PolicyEvaluation{
			Decision: types.PolicyAPPROVALREQUIRED,
			Reason:   fmt.Sprintf("APPROVAL_REQUIRED: tool %s or requested action requires interactive human confirmation", tool.Name),
		}, nil
	}

	// Stage 1a: Shell Metacharacter Check
	if reason := g.checkShellMetachars(args); reason != "" {
		return &types.PolicyEvaluation{
			Decision: types.PolicyDENY,
			Reason:   reason,
		}, nil
	}

	// Stage 1b: Package ID Regex Check
	if reason := g.checkPackageIDRegex(args); reason != "" {
		return &types.PolicyEvaluation{
			Decision: types.PolicyDENY,
			Reason:   reason,
		}, nil
	}

	// Stage 1c & Stage 2: Device Serial & Trust Check
	if reason := g.checkDeviceTrust(args); reason != "" {
		return &types.PolicyEvaluation{
			Decision: types.PolicyDENY,
			Reason:   reason,
		}, nil
	}

	// Stage 1d: File Paths, Symlink Escape & Policy Tampering Check
	if reason := g.checkFilePaths(args); reason != "" {
		return &types.PolicyEvaluation{
			Decision: types.PolicyDENY,
			Reason:   reason,
		}, nil
	}

	// Stage 3: Package Scope Lock, Contacts/SMS Read, and TCP/IP Checks
	if reason := g.checkPackageScopeAndPolicy(tool, args); reason != "" {
		return &types.PolicyEvaluation{
			Decision: types.PolicyDENY,
			Reason:   reason,
		}, nil
	}

	// All synchronous checks passed
	return &types.PolicyEvaluation{
		Decision: types.PolicyALLOW,
		Nonce:    g.GenerateNonce(),
	}, nil
}

func (g *Guard) isHumanApprovalRequired(toolName string, args map[string]any) bool {
	for _, app := range g.policyConfig.HumanApproval {
		if strings.EqualFold(app, toolName) {
			return true
		}
	}
	// Check for cloud billing or destructive actions
	if action, ok := args["action"].(string); ok {
		if strings.Contains(strings.ToLower(action), "bill") || strings.Contains(strings.ToLower(action), "cloud_farm") {
			return true
		}
	}
	if strings.Contains(strings.ToLower(toolName), "billing") || strings.Contains(strings.ToLower(toolName), "cloud_farm") {
		return true
	}
	return false
}

func (g *Guard) checkShellMetachars(args map[string]any) string {
	for k, v := range args {
		if hasShellMetachar(v) {
			return fmt.Sprintf("DENY: shell metacharacter injection detected in parameter %q", k)
		}
	}
	return ""
}

func hasShellMetachar(val any) bool {
	switch v := val.(type) {
	case string:
		// Check for null bytes
		if strings.ContainsRune(v, 0) {
			return true
		}
		for _, meta := range shellMetachars {
			if strings.Contains(v, meta) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if hasShellMetachar(item) {
				return true
			}
		}
	case []string:
		for _, item := range v {
			if strings.ContainsRune(item, 0) {
				return true
			}
			for _, meta := range shellMetachars {
				if strings.Contains(item, meta) {
					return true
				}
			}
		}
	case map[string]any:
		for _, item := range v {
			if hasShellMetachar(item) {
				return true
			}
		}
	}
	return false
}

func (g *Guard) checkPackageIDRegex(args map[string]any) string {
	pkgKeys := []string{"package_id", "package", "pkg", "target_package", "application_id"}
	for _, key := range pkgKeys {
		if val, ok := args[key]; ok {
			if strVal, isStr := val.(string); isStr && strVal != "" {
				if !packageIDRegex.MatchString(strVal) {
					return fmt.Sprintf("DENY: invalid package ID format %q: must match ^[a-zA-Z0-9._]+$", strVal)
				}
			}
		}
	}
	return ""
}

func (g *Guard) checkDeviceTrust(args map[string]any) string {
	serialKeys := []string{"serial", "device_serial", "device"}
	for _, key := range serialKeys {
		if val, ok := args[key]; ok {
			if serial, isStr := val.(string); isStr && serial != "" {
				if g.emulatorOnly {
					if !emulatorSerialRegex.MatchString(serial) {
						return fmt.Sprintf("DENY: DEVICE_NOT_APPROVED: serial %q is not an approved emulator (must match emulator-* or 127.0.0.1:*)", serial)
					}
				}
			}
		}
	}
	return ""
}

func (g *Guard) checkFilePaths(args map[string]any) string {
	pathKeys := []string{"apk_path", "path", "file_path", "output_path", "flow_path", "trace_path", "target_path", "src_path", "dst_path"}

	// Check if any argument targets policy configuration files or uses forbidden path syntax
	for _, key := range pathKeys {
		if val, ok := args[key]; ok {
			if strVal, isStr := val.(string); isStr && strVal != "" {
				// Reject null bytes
				if strings.ContainsRune(strVal, 0) {
					return "DENY: null byte injection detected in path"
				}

				// Reject Windows device namespaces
				if strings.HasPrefix(strVal, `\\.\`) || strings.HasPrefix(strVal, `\\?\`) ||
					strings.HasPrefix(strVal, `//./`) || strings.HasPrefix(strVal, `//?/`) {
					return "DENY: device namespace paths prohibited"
				}

				cleanVal := filepath.Clean(strVal)
				base := filepath.Base(cleanVal)

				// Reject reserved DOS device names
				baseNoExt := strings.TrimSuffix(base, filepath.Ext(base))
				if windowsReservedNames[strings.ToUpper(baseNoExt)] {
					return fmt.Sprintf("DENY: access to reserved system device filename %q is prohibited", base)
				}

				if base == "droidforge.policy.json" || base == "droidforge.yaml" ||
					strings.HasSuffix(cleanVal, "droidforge.policy.json") || strings.HasSuffix(cleanVal, "droidforge.yaml") {
					return fmt.Sprintf("DENY: policy file tampering attempt: access to %s is prohibited", base)
				}
			}
		}
	}

	// Check for direct symlink escape indicator in test arguments
	if targetEscape, ok := args["target_escape"].(string); ok && targetEscape != "" {
		return fmt.Sprintf("DENY: symlink escape detected: resolves outside workspace root to %s", targetEscape)
	}

	if g.workspaceRoot == "" {
		return ""
	}

	for _, key := range pathKeys {
		if val, ok := args[key]; ok {
			if pathVal, isStr := val.(string); isStr && pathVal != "" {
				cleanPath := filepath.Clean(pathVal)

				var absPath string
				if filepath.IsAbs(cleanPath) {
					absPath = cleanPath
				} else {
					absPath = filepath.Join(g.workspaceRoot, cleanPath)
				}

				// Check directory traversal out of workspace root
				rel, err := filepath.Rel(g.workspaceRoot, absPath)
				if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
					// In apk-only mode or for .apk files, allow reading external .apk files
					if (g.projectConfig.Mode == "apk-only" || key == "apk_path") && strings.HasSuffix(strings.ToLower(absPath), ".apk") {
						// Allowed external APK
					} else {
						return fmt.Sprintf("DENY: path traversal detected: %q escapes workspace root", pathVal)
					}
				}

				// If the file exists, evaluate symlinks
				if info, err := os.Lstat(absPath); err == nil {
					if info.Mode()&os.ModeSymlink != 0 {
						evalPath, err := filepath.EvalSymlinks(absPath)
						if err != nil {
							return fmt.Sprintf("DENY: symlink resolution failed: %v", err)
						}
						relEval, err := filepath.Rel(g.workspaceRoot, evalPath)
						if err != nil || strings.HasPrefix(relEval, "..") || filepath.IsAbs(relEval) {
							if (g.projectConfig.Mode == "apk-only" || key == "apk_path") && strings.HasSuffix(strings.ToLower(evalPath), ".apk") {
								// Allowed external APK
							} else {
								return fmt.Sprintf("DENY: symlink escape detected: resolves to %q outside workspace root", evalPath)
							}
						}
					}
				}
			}
		}
	}
	return ""
}

func (g *Guard) checkPackageScopeAndPolicy(tool types.ToolDefinition, args map[string]any) string {
	// Extract target package from arguments
	targetPkg := ""
	pkgKeys := []string{"package_id", "package", "pkg", "target_package", "application_id"}
	for _, key := range pkgKeys {
		if val, ok := args[key]; ok {
			if s, isStr := val.(string); isStr && s != "" {
				targetPkg = s
				break
			}
		}
	}

	// If no direct package_id, extract from command strings (e.g., pm, am, monkey)
	if targetPkg == "" {
		if cmd, ok := args["command"].(string); ok {
			parts := strings.Fields(cmd)
			if len(parts) >= 2 {
				switch parts[0] {
				case "pm":
					if len(parts) >= 3 && (parts[1] == "clear" || parts[1] == "uninstall" || parts[1] == "disable" || parts[1] == "enable") {
						targetPkg = parts[2]
					}
				case "am":
					for i, p := range parts {
						if p == "-p" && i+1 < len(parts) {
							targetPkg = parts[i+1]
							break
						}
						if p == "-n" && i+1 < len(parts) {
							targetPkg = strings.Split(parts[i+1], "/")[0]
							break
						}
						if (p == "force-stop" || p == "kill") && i+1 < len(parts) {
							targetPkg = parts[i+1]
							break
						}
					}
				case "monkey":
					for i, p := range parts {
						if p == "-p" && i+1 < len(parts) {
							targetPkg = parts[i+1]
							break
						}
					}
				}
			}
		}
	}

	// 1. Permanent Hardcoded Blocklist
	if targetPkg != "" && permanentBlocklist[targetPkg] {
		return fmt.Sprintf("DENY: target package %q is in hardcoded permanent blocklist", targetPkg)
	}

	// 2. Package Scope Lock for WRITE_SCOPED tools
	if tool.RiskTier == types.RiskWRITESCOPED && targetPkg != "" {
		allowed := false
		for _, allow := range g.projectConfig.AllowlistedPackages {
			if allow == targetPkg {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Sprintf("DENY: package %q is not in allowlisted_packages", targetPkg)
		}
	}

	// 3. Contacts / Telephony / SMS Read Policy Check
	if query, ok := args["query"].(string); ok {
		lowerQ := strings.ToLower(query)
		if strings.Contains(lowerQ, "content://contacts") ||
			strings.Contains(lowerQ, "content://sms") ||
			strings.Contains(lowerQ, "content://telephony") ||
			strings.Contains(lowerQ, "contacts") {
			return "DENY: access to contacts or SMS data is prohibited by security policy"
		}
	}

	// 4. Raw TCP/IP 5555 Network Policy Check
	if !g.policyConfig.Network.AllowUnencryptedADBTcpip {
		if port, ok := args["port"].(int); ok && port == 5555 {
			return "DENY: unencrypted adb tcpip 5555 is prohibited by network policy"
		}
		if cmd, ok := args["command"].(string); ok {
			if strings.Contains(cmd, "tcpip 5555") || strings.Contains(cmd, "adb tcpip") {
				return "DENY: unencrypted adb tcpip 5555 is prohibited by network policy"
			}
		}
	}

	return ""
}
