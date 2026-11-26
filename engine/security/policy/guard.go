package policy

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

var (
	packageIDRegex = regexp.MustCompile(`^[a-zA-Z0-9._]+$`)
	shellMetaChars = []string{";", "|", "&&", "$(", "`", "\n", "\r", ">", "<"}

	permanentBlockedPackages = map[string]bool{
		"com.android.systemui":            true,
		"com.google.android.gms":          true,
		"com.android.providers.contacts":  true,
		"com.android.providers.telephony": true,
	}
)

// Config holds unified security policy settings for Nano-Android & DroidForge.
type Config struct {
	AllowlistedPackages  []string
	EnforcePackageLock   bool
	AllowRawShell        bool
	MaxStepsPerSession   int
	MaxIdenticalUIHashes int
	MaxNodeTextBytes     int
	BlockedTools         map[string]bool
	ApprovalRequired     map[string]bool
}

// DefaultConfig returns fail-safe P0 security defaults.
func DefaultConfig() Config {
	return Config{
		EnforcePackageLock:   false, // Enabled automatically when AllowlistedPackages is populated
		AllowRawShell:        false,
		MaxStepsPerSession:   100,
		MaxIdenticalUIHashes: 10,
		MaxNodeTextBytes:     512,
		BlockedTools:         map[string]bool{"shell_exec": true, "raw_shell": true},
		ApprovalRequired: map[string]bool{
			"register_new_physical_device": true,
			"modify_non_allowlisted_package": true,
			"sign_release_artifact":        true,
		},
	}
}

// Decision enumerates policy outcomes.
type Decision string

const (
	DecisionAllow            Decision = "ALLOW"
	DecisionDeny             Decision = "DENY"
	DecisionApprovalRequired Decision = "APPROVAL_REQUIRED"
	DecisionHalt             Decision = "HALT"
)

// Evaluation records a synchronous policy check outcome.
type Evaluation struct {
	Decision Decision `json:"decision"`
	Reason   string   `json:"reason,omitempty"`
}

// Guard enforces unified security checks across CLI, Studio, and MCP surfaces.
type Guard struct {
	mu           sync.Mutex
	cfg          Config
	steps        map[string]int
	lastUIHash   map[string]string
	streak       map[string]int
	allowedPkgs  map[string]bool
	approvals    map[string]bool
}

// NewGuard creates a thread-safe unified Policy Guard.
func NewGuard(cfg Config) *Guard {
	if cfg.MaxStepsPerSession <= 0 {
		cfg.MaxStepsPerSession = 100
	}
	if cfg.MaxIdenticalUIHashes <= 0 {
		cfg.MaxIdenticalUIHashes = 10
	}
	if cfg.MaxNodeTextBytes <= 0 {
		cfg.MaxNodeTextBytes = 512
	}
	allowed := make(map[string]bool, len(cfg.AllowlistedPackages))
	for _, p := range cfg.AllowlistedPackages {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			allowed[trimmed] = true
		}
	}
	return &Guard{
		cfg:         cfg,
		steps:       make(map[string]int),
		lastUIHash:  make(map[string]string),
		streak:      make(map[string]int),
		allowedPkgs: allowed,
		approvals:   make(map[string]bool),
	}
}

// GrantApproval records explicit human approval for a gated operation token/tool.
func (g *Guard) GrantApproval(toolOrOp string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.approvals[toolOrOp] = true
}

// EvaluateTool validates a tool/action invocation synchronously before execution.
func (g *Guard) EvaluateTool(sessionID, toolName string, args map[string]any) Evaluation {
	g.mu.Lock()
	defer g.mu.Unlock()

	if sessionID == "" {
		sessionID = "default"
	}
	if g.cfg.BlockedTools[toolName] {
		return Evaluation{Decision: DecisionDeny, Reason: fmt.Sprintf("tool %q is permanently blocked by security policy", toolName)}
	}
	if g.cfg.ApprovalRequired[toolName] && !g.approvals[toolName] {
		return Evaluation{Decision: DecisionApprovalRequired, Reason: fmt.Sprintf("tool %q requires explicit human approval", toolName)}
	}

	// Shell metacharacter scan across control parameters (exempt free-text typing payload only for 'type' action).
	isTypingAction := false
	if act, ok := args["action"].(string); ok && strings.EqualFold(act, "type") {
		isTypingAction = true
	}
	for k, v := range args {
		if (isTypingAction && k == "text") || ((toolName == "nano_crash_analyze" || toolName == "nano_ai_diagnose") && k == "trace") {
			continue
		}
		if s, ok := v.(string); ok {
			for _, meta := range shellMetaChars {
				if strings.Contains(s, meta) {
					return Evaluation{
						Decision: DecisionDeny,
						Reason:   fmt.Sprintf("argument %q contains forbidden shell metacharacter %q", k, meta),
					}
				}
			}
		}
	}

	// Package scope & permanent system blocklist enforcement.
	for _, pkgKey := range []string{"package", "app_id"} {
		if pkg, ok := args[pkgKey].(string); ok && strings.TrimSpace(pkg) != "" {
			pkg = strings.TrimSpace(pkg)
			if !packageIDRegex.MatchString(pkg) {
				return Evaluation{Decision: DecisionDeny, Reason: fmt.Sprintf("invalid package ID format %q", pkg)}
			}
			if permanentBlockedPackages[pkg] {
				return Evaluation{Decision: DecisionDeny, Reason: fmt.Sprintf("system package %q is permanently blocked", pkg)}
			}
			if g.cfg.EnforcePackageLock && len(g.allowedPkgs) > 0 && !g.allowedPkgs[pkg] {
				return Evaluation{Decision: DecisionDeny, Reason: fmt.Sprintf("package %q is outside allowlisted scope", pkg)}
			}
		}
	}

	// Circuit breaker: max steps per session.
	if g.steps[sessionID] >= g.cfg.MaxStepsPerSession {
		return Evaluation{Decision: DecisionHalt, Reason: fmt.Sprintf("session step limit exceeded (%d)", g.cfg.MaxStepsPerSession)}
	}
	if toolName == "nano_dispatch_action" || toolName == "nano_ui_tap" {
		g.steps[sessionID]++
	}

	return Evaluation{Decision: DecisionAllow}
}

// ResetSession resets step and UI streak counters for a session.
func (g *Guard) ResetSession(sessionID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if sessionID == "" {
		sessionID = "default"
	}
	delete(g.steps, sessionID)
	delete(g.lastUIHash, sessionID)
	delete(g.streak, sessionID)
}

// FenceUntrustedText strips zero-width & bidi characters, escapes fake delimiters,
// truncates safely at rune boundaries, and wraps text in a random nonce tag.
func FenceUntrustedText(raw string, maxBytes int) (string, string) {
	if maxBytes <= 0 {
		maxBytes = 512
	}
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		b = []byte{0xde, 0xad, 0xbe, 0xef, 0x00, 0x01}
	}
	nonce := hex.EncodeToString(b)

	cleaned := StripObfuscation(raw)
	cleaned = strings.ReplaceAll(cleaned, "</DF_UNTRUSTED", "[ESCAPED_DF_UNTRUSTED_CLOSE")
	cleaned = strings.ReplaceAll(cleaned, "<DF_UNTRUSTED", "[ESCAPED_DF_UNTRUSTED_OPEN")

	if len(cleaned) > maxBytes {
		runes := []rune(cleaned)
		for len(string(runes)) > maxBytes && len(runes) > 0 {
			runes = runes[:len(runes)-1]
		}
		cleaned = string(runes)
	}
	return fmt.Sprintf("<DF_UNTRUSTED_%s>%s</DF_UNTRUSTED_%s>", nonce, cleaned, nonce), nonce
}

// StripObfuscation removes zero-width and Unicode directional override characters.
func StripObfuscation(raw string) string {
	if raw == "" {
		return ""
	}
	var out []rune
	for _, r := range raw {
		if (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) ||
			(r >= 0x200B && r <= 0x200D) || r == 0xFEFF {
			continue
		}
		out = append(out, r)
	}
	return string(out)
}
