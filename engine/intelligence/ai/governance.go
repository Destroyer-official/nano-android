package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"nano_android/engine/security/audit"
	"nano_android/engine/security/pathcontainment"
	"nano_android/engine/security/policy"
)

// ExecutionBudget caps autonomous AI steps, screen visits, wall time, and UI retry loops.
type ExecutionBudget struct {
	MaxSteps             int  `json:"max_steps"`
	MaxScreens           int  `json:"max_screens"`
	MaxDurationSec       int  `json:"max_duration_sec"`
	MaxIdenticalUIStreak int  `json:"max_identical_ui_streak"`
	MaxProposals         int  `json:"max_proposals"`
	BlockExternalNetwork bool `json:"block_external_network"`
}

// DefaultBudget returns conservative P2/P3 AI autonomous engineering guardrails.
func DefaultBudget() ExecutionBudget {
	return ExecutionBudget{
		MaxSteps:             30,
		MaxScreens:           20,
		MaxDurationSec:       60,
		MaxIdenticalUIStreak: 5,
		MaxProposals:         25,
		BlockExternalNetwork: true,
	}
}

func normalizeBudget(b ExecutionBudget) ExecutionBudget {
	def := DefaultBudget()
	if b.MaxSteps <= 0 {
		b.MaxSteps = def.MaxSteps
	}
	if b.MaxSteps > 100 {
		b.MaxSteps = 100
	}
	if b.MaxScreens <= 0 {
		b.MaxScreens = def.MaxScreens
	}
	if b.MaxScreens > 50 {
		b.MaxScreens = 50
	}
	if b.MaxDurationSec <= 0 {
		b.MaxDurationSec = def.MaxDurationSec
	}
	if b.MaxDurationSec > 300 {
		b.MaxDurationSec = 300
	}
	if b.MaxIdenticalUIStreak <= 0 {
		b.MaxIdenticalUIStreak = def.MaxIdenticalUIStreak
	}
	if b.MaxProposals <= 0 {
		b.MaxProposals = def.MaxProposals
	}
	if b.MaxProposals > 100 {
		b.MaxProposals = 100
	}
	// External internet access is permanently disabled during autonomous runs.
	b.BlockExternalNetwork = true
	return b
}

// InjectionSignal records a detected prompt-injection attempt quarantined inside untrusted device data.
type InjectionSignal struct {
	SourceField string `json:"source_field"`
	Nonce       string `json:"nonce"`
	Pattern     string `json:"pattern"`
	Snippet     string `json:"snippet"`
}

// BudgetStatus reports live consumption of a bounded AI session.
type BudgetStatus struct {
	SessionID         string            `json:"session_id"`
	Actor             string            `json:"actor"`
	PackageID         string            `json:"package_id"`
	StepsUsed         int               `json:"steps_used"`
	MaxSteps          int               `json:"max_steps"`
	ScreensDiscovered int               `json:"screens_discovered"`
	MaxScreens        int               `json:"max_screens"`
	ElapsedMs         int64             `json:"elapsed_ms"`
	Halted            bool              `json:"halted"`
	HaltReason        string            `json:"halt_reason,omitempty"`
	InjectionSignals  []InjectionSignal `json:"injection_signals,omitempty"`
}

var injectionPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"instruction_override", regexp.MustCompile(`(?i)(ignore\s+(all\s+)?previous\s+instructions|system\s+override|you\s+are\s+now\s+in\s+developer\s+mode)`)},
	{"fake_fence_tag", regexp.MustCompile(`(?i)</?DF_UNTRUSTED`)},
	{"shell_exfil", regexp.MustCompile(`(?i)(curl\s+https?://|wget\s+https?://|rm\s+-rf|powershell\s+-e|nc\s+-e)`)},
	{"secret_probe", regexp.MustCompile(`(?i)(print\s+process\.env|read\s+/etc/passwd|exfiltrate|api_key)`)},
}

// DetectPromptInjection inspects untrusted device text for adversarial prompt-injection payloads.
func DetectPromptInjection(raw string) []string {
	var matched []string
	for _, p := range injectionPatterns {
		if p.re.MatchString(raw) {
			matched = append(matched, p.name)
		}
	}
	return matched
}

// Governor enforces budgets, policy allowlists, external-network blocks, nonced fencing, and audit attribution.
type Governor struct {
	mu         sync.Mutex
	sessionID  string
	actor      string
	packageID  string
	budget     ExecutionBudget
	guard      *policy.Guard
	ledger     *audit.Ledger
	startedAt  time.Time
	stepsUsed  int
	screens    map[string]bool
	lastUIHash string
	uiStreak   int
	halted     bool
	haltReason string
	injections []InjectionSignal
}

// NewGovernor initializes a session-scoped AI governance gate backed by Policy Guard + Audit Ledger.
func NewGovernor(sessionID, actor, packageID string, budget ExecutionBudget, guard *policy.Guard, ledger *audit.Ledger) *Governor {
	if strings.TrimSpace(sessionID) == "" {
		sessionID = fmt.Sprintf("ai_%d", time.Now().UnixNano())
	}
	if strings.TrimSpace(actor) == "" {
		actor = "ai_engineer"
	}
	if guard == nil {
		guard = policy.NewGuard(policy.DefaultConfig())
	}
	return &Governor{
		sessionID: sessionID,
		actor:     actor,
		packageID: strings.TrimSpace(packageID),
		budget:    normalizeBudget(budget),
		guard:     guard,
		ledger:    ledger,
		startedAt: time.Now(),
		screens:   make(map[string]bool),
	}
}

// CheckStep verifies budget caps, external-network isolation, and Policy Guard before running an AI action.
func (g *Governor) CheckStep(toolName string, args map[string]any) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.halted {
		return fmt.Errorf("BLOCKED_POLICY budget halted: %s", g.haltReason)
	}
	if g.stepsUsed >= g.budget.MaxSteps {
		g.halted = true
		g.haltReason = fmt.Sprintf("max steps reached (%d)", g.budget.MaxSteps)
		g.auditLocked(toolName, "DENY", g.haltReason, args)
		return fmt.Errorf("BLOCKED_POLICY %s", g.haltReason)
	}
	if int(time.Since(g.startedAt).Seconds()) >= g.budget.MaxDurationSec {
		g.halted = true
		g.haltReason = fmt.Sprintf("max wall duration reached (%ds)", g.budget.MaxDurationSec)
		g.auditLocked(toolName, "DENY", g.haltReason, args)
		return fmt.Errorf("BLOCKED_POLICY %s", g.haltReason)
	}
	if g.budget.BlockExternalNetwork {
		if rawURL, _ := args["url"].(string); rawURL != "" && !isLoopbackURL(rawURL) {
			g.auditLocked(toolName, "DENY", "external network egress forbidden for autonomous AI", args)
			return fmt.Errorf("BLOCKED_POLICY external network access to %q forbidden", rawURL)
		}
	}
	// Defense-in-depth path traversal check across path-like parameters
	for _, pk := range []string{"path", "file", "artifact", "mapping_path", "output_path", "target_rel_path"} {
		if pv, ok := args[pk].(string); ok && containsTraversalSegment(pv) {
			g.auditLocked(toolName, "DENY", "path traversal segment rejected", args)
			return fmt.Errorf("BLOCKED_POLICY argument %q contains path traversal", pk)
		}
	}

	evalArgs := make(map[string]any, len(args)+1)
	for k, v := range args {
		evalArgs[k] = v
	}
	if g.packageID != "" {
		if _, exists := evalArgs["package"]; !exists {
			evalArgs["package"] = g.packageID
		}
	}
	ev := g.guard.EvaluateTool(g.sessionID, toolName, evalArgs)
	if ev.Decision != policy.DecisionAllow {
		g.auditLocked(toolName, string(ev.Decision), ev.Reason, evalArgs)
		return fmt.Errorf("%s: %s", ev.Decision, ev.Reason)
	}
	g.stepsUsed++
	g.auditLocked(toolName, "ALLOW", "governed ai step", evalArgs)
	return nil
}

// ObserveScreenState records screen hash progression and trips loop detection if UI freezes identically.
func (g *Governor) ObserveScreenState(screenKey, uiHash string) (loopDetected bool, reason string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if screenKey != "" && !g.screens[screenKey] {
		if len(g.screens) >= g.budget.MaxScreens {
			g.halted = true
			g.haltReason = fmt.Sprintf("max discovered screens reached (%d)", g.budget.MaxScreens)
			return true, g.haltReason
		}
		g.screens[screenKey] = true
	}
	if uiHash != "" && uiHash == g.lastUIHash {
		g.uiStreak++
		if g.uiStreak >= g.budget.MaxIdenticalUIStreak {
			return true, fmt.Sprintf("identical UI state repeated %d times", g.uiStreak)
		}
	} else {
		g.lastUIHash = uiHash
		g.uiStreak = 1
	}
	return false, ""
}

var reANSIEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// FenceUntrusted wraps raw device/APK text in a cryptographic random nonce tag and flags any injection payload.
func (g *Governor) FenceUntrusted(field, raw string, maxBytes int) string {
	cleanANSI := reANSIEscape.ReplaceAllString(raw, "")
	fenced, nonce := policy.FenceUntrustedText(cleanANSI, maxBytes)
	sigs := DetectPromptInjection(cleanANSI)
	if len(sigs) > 0 {
		g.mu.Lock()
		for _, p := range sigs {
			snip := policy.StripObfuscation(raw)
			if len(snip) > 96 {
				snip = snip[:96]
			}
			g.injections = append(g.injections, InjectionSignal{
				SourceField: field,
				Nonce:       nonce,
				Pattern:     p,
				Snippet:     snip,
			})
		}
		g.auditLocked("untrusted_injection_quarantined", "ALLOW", "hostile payload quarantined inside nonce delimiter", map[string]any{
			"field":    field,
			"nonce":    nonce,
			"patterns": strings.Join(sigs, ","),
		})
		g.mu.Unlock()
	}
	return fenced
}

// AuditAction writes an explicit attributed record into the immutable audit ledger.
func (g *Governor) AuditAction(action, decision, reason string, meta map[string]any) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.auditLocked(action, decision, reason, meta)
}

func (g *Governor) auditLocked(action, decision, reason string, meta map[string]any) {
	if g.ledger == nil {
		return
	}
	m := make(map[string]any, len(meta)+3)
	for k, v := range meta {
		m[k] = v
	}
	m["ai_session_id"] = g.sessionID
	m["steps_used"] = g.stepsUsed
	m["budget_max_steps"] = g.budget.MaxSteps
	_, _ = g.ledger.Record("ai_engineer", g.actor, action, g.packageID, decision, reason, m)
}

// Status returns a snapshot of current AI budget usage and quarantined injection signals.
func (g *Governor) Status() BudgetStatus {
	g.mu.Lock()
	defer g.mu.Unlock()
	return BudgetStatus{
		SessionID:         g.sessionID,
		Actor:             g.actor,
		PackageID:         g.packageID,
		StepsUsed:         g.stepsUsed,
		MaxSteps:          g.budget.MaxSteps,
		ScreensDiscovered: len(g.screens),
		MaxScreens:        g.budget.MaxScreens,
		ElapsedMs:         time.Since(g.startedAt).Milliseconds(),
		Halted:            g.halted,
		HaltReason:        g.haltReason,
		InjectionSignals:  append([]InjectionSignal(nil), g.injections...),
	}
}

func isLoopbackURL(raw string) bool {
	low := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(low, "http://127.0.0.1") ||
		strings.HasPrefix(low, "http://localhost") ||
		strings.HasPrefix(low, "http://[::1]") ||
		strings.HasPrefix(low, "https://127.0.0.1") ||
		strings.HasPrefix(low, "https://localhost")
}

func containsTraversalSegment(raw string) bool {
	cleaned := strings.ReplaceAll(raw, "\\", "/")
	for _, seg := range strings.Split(cleaned, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// ProposalKind identifies reviewable AI outputs.
type ProposalKind string

const (
	ProposalTestMaestro  ProposalKind = "test_maestro"
	ProposalTestJUnit    ProposalKind = "test_junit"
	ProposalPatchSource  ProposalKind = "patch_source"
	ProposalSelectorHeal ProposalKind = "selector_heal"
	ProposalExploreRepro ProposalKind = "exploration_repro"
)

// Proposal represents a reviewable AI artifact. It is ALWAYS created PENDING_APPROVAL and never alters files silently.
type Proposal struct {
	ID              string         `json:"id"`
	Kind            ProposalKind   `json:"kind"`
	Title           string         `json:"title"`
	Description     string         `json:"description"`
	TargetRelPath   string         `json:"target_rel_path,omitempty"`
	Content         string         `json:"content"`
	UnifiedDiff     string         `json:"unified_diff,omitempty"`
	EnvironmentTier string         `json:"environment_tier"`
	Status          string         `json:"status"` // PENDING_APPROVAL, APPROVED, APPLIED, REJECTED
	CreatedBy       string         `json:"created_by"`
	CreatedUTC      string         `json:"created_utc"`
	ApprovedBy      string         `json:"approved_by,omitempty"`
	ApprovedUTC     string         `json:"approved_utc,omitempty"`
	SHA256          string         `json:"sha256"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// ProposalStore persists human-reviewable AI proposals safely under a contained workspace cache directory.
type ProposalStore struct {
	mu        sync.Mutex
	storeFile string
	ledger    *audit.Ledger
	items     map[string]*Proposal
	order     []string
}

// OpenProposalStore loads or creates a persistent AI proposal approval queue.
func OpenProposalStore(storeFile string, ledger *audit.Ledger) (*ProposalStore, error) {
	if strings.TrimSpace(storeFile) == "" {
		return nil, fmt.Errorf("storeFile is required")
	}
	if err := os.MkdirAll(filepath.Dir(storeFile), 0750); err != nil {
		return nil, err
	}
	ps := &ProposalStore{
		storeFile: storeFile,
		ledger:    ledger,
		items:     make(map[string]*Proposal),
	}
	if raw, err := os.ReadFile(storeFile); err == nil && len(raw) > 0 {
		var loaded []*Proposal
		if uErr := json.Unmarshal(raw, &loaded); uErr == nil {
			for _, p := range loaded {
				ps.items[p.ID] = p
				ps.order = append(ps.order, p.ID)
			}
		}
	}
	return ps, nil
}

// Add registers a new proposal forced into Status="PENDING_APPROVAL".
func (ps *ProposalStore) Add(p Proposal) (*Proposal, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%s|%s|%d", len(ps.order), string(p.Kind), p.Title, p.Content, time.Now().UTC().UnixNano())))
	if p.ID == "" {
		p.ID = fmt.Sprintf("prop_%d_%s", len(ps.order)+1, hex.EncodeToString(sum[:4]))
	}
	contentSum := sha256.Sum256([]byte(p.Content))
	p.SHA256 = hex.EncodeToString(contentSum[:])
	p.Status = "PENDING_APPROVAL"
	p.ApprovedBy = ""
	p.ApprovedUTC = ""
	if p.CreatedUTC == "" {
		p.CreatedUTC = time.Now().UTC().Format(time.RFC3339)
	}
	if p.CreatedBy == "" {
		p.CreatedBy = "ai_engineer"
	}
	cp := p
	ps.items[cp.ID] = &cp
	ps.order = append(ps.order, cp.ID)
	if err := ps.saveLocked(); err != nil {
		return nil, err
	}
	if ps.ledger != nil {
		_, _ = ps.ledger.Record("ai_engineer", cp.CreatedBy, "proposal_created", cp.ID, "ALLOW", "awaiting human approval", map[string]any{
			"kind":   string(cp.Kind),
			"title":  cp.Title,
			"sha256": cp.SHA256,
		})
	}
	return &cp, nil
}

// List returns all recorded proposals ordered chronologically.
func (ps *ProposalStore) List() []Proposal {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	out := make([]Proposal, 0, len(ps.order))
	for _, id := range ps.order {
		if p, ok := ps.items[id]; ok {
			out = append(out, *p)
		}
	}
	return out
}

// Approve requires explicit non-empty human approver identity before marking a proposal APPROVED (or writing to an allowlisted root).
func (ps *ProposalStore) Approve(id, approvedBy string, writeOutput bool, allowedOutputRoot string) (*Proposal, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	approvedBy = strings.TrimSpace(approvedBy)
	if approvedBy == "" {
		if ps.ledger != nil {
			_, _ = ps.ledger.Record("ai_engineer", "anonymous", "proposal_approve", id, "DENY", "explicit approved_by actor required", nil)
		}
		return nil, fmt.Errorf("BLOCKED_POLICY explicit non-empty approved_by actor is required to approve AI proposal")
	}
	p, ok := ps.items[id]
	if !ok {
		return nil, fmt.Errorf("proposal %q not found", id)
	}
	if p.Status == "APPLIED" {
		return nil, fmt.Errorf("BLOCKED_POLICY cannot reject already APPLIED proposal %q", id)
	}
	if p.Status == "REJECTED" {
		return nil, fmt.Errorf("BLOCKED_POLICY cannot approve already REJECTED proposal %q", id)
	}
	if p.Status == "APPLIED" && writeOutput {
		cp := *p
		return &cp, nil
	}
	p.Status = "APPROVED"
	p.ApprovedBy = approvedBy
	p.ApprovedUTC = time.Now().UTC().Format(time.RFC3339)

	if writeOutput {
		if strings.TrimSpace(allowedOutputRoot) == "" || strings.TrimSpace(p.TargetRelPath) == "" {
			return nil, fmt.Errorf("BLOCKED_POLICY allowedOutputRoot and TargetRelPath required when applying proposal")
		}
		dst, err := pathcontainment.ResolveWriteContained(allowedOutputRoot, p.TargetRelPath, []string{".yaml", ".yml", ".kt", ".java", ".md", ".json"})
		if err != nil {
			return nil, fmt.Errorf("BLOCKED_POLICY %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0750); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dst, []byte(p.Content), 0600); err != nil {
			return nil, err
		}
		p.Status = "APPLIED"
		if p.Metadata == nil {
			p.Metadata = make(map[string]any)
		}
		p.Metadata["applied_path"] = dst
	}

	if err := ps.saveLocked(); err != nil {
		return nil, err
	}
	if ps.ledger != nil {
		_, _ = ps.ledger.Record("ai_engineer", approvedBy, "proposal_approve", id, "ALLOW", "human approved proposal", map[string]any{
			"status": p.Status,
			"kind":   string(p.Kind),
			"sha256": p.SHA256,
		})
	}
	cp := *p
	return &cp, nil
}

// Reject marks a pending proposal REJECTED with an explicit reviewer reason.
func (ps *ProposalStore) Reject(id, actor, reason string) (*Proposal, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if strings.TrimSpace(actor) == "" {
		return nil, fmt.Errorf("BLOCKED_POLICY reviewer actor is required")
	}
	p, ok := ps.items[id]
	if !ok {
		return nil, fmt.Errorf("proposal %q not found", id)
	}
	if p.Status == "APPLIED" {
		return nil, fmt.Errorf("BLOCKED_POLICY cannot reject already APPLIED proposal %q", id)
	}
	p.Status = "REJECTED"
	p.ApprovedBy = actor
	p.ApprovedUTC = time.Now().UTC().Format(time.RFC3339)
	if p.Metadata == nil {
		p.Metadata = make(map[string]any)
	}
	p.Metadata["reject_reason"] = reason
	if err := ps.saveLocked(); err != nil {
		return nil, err
	}
	if ps.ledger != nil {
		_, _ = ps.ledger.Record("ai_engineer", actor, "proposal_reject", id, "ALLOW", reason, nil)
	}
	cp := *p
	return &cp, nil
}

func (ps *ProposalStore) saveLocked() error {
	list := make([]*Proposal, 0, len(ps.order))
	for _, id := range ps.order {
		if p, ok := ps.items[id]; ok {
			list = append(list, p)
		}
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := ps.storeFile + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, ps.storeFile)
}

// MemoryEntry captures durable project-level engineering knowledge learned across sessions.
type MemoryEntry struct {
	Key        string `json:"key"`
	Category   string `json:"category"` // "selector_alias", "known_quirk", "screen_invariant", "crash_resolution"
	Value      string `json:"value"`
	UpdatedBy  string `json:"updated_by"`
	UpdatedUTC string `json:"updated_utc"`
}

// MemoryStore persists vetted project engineering memory.
type MemoryStore struct {
	mu      sync.Mutex
	path    string
	Entries map[string]MemoryEntry `json:"entries"`
}

// OpenMemoryStore loads or initializes a JSON engineering memory file.
func OpenMemoryStore(path string) (*MemoryStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("memory store path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return nil, err
	}
	ms := &MemoryStore{path: path, Entries: make(map[string]MemoryEntry)}
	if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 {
		var loaded struct {
			Entries map[string]MemoryEntry `json:"entries"`
		}
		if json.Unmarshal(raw, &loaded) == nil && loaded.Entries != nil {
			ms.Entries = loaded.Entries
		}
	}
	return ms, nil
}

// Put stores a vetted engineering memory entry (requires non-empty actor).
func (ms *MemoryStore) Put(key, category, value, actor string) error {
	if strings.TrimSpace(actor) == "" {
		return fmt.Errorf("BLOCKED_POLICY actor is required to mutate project engineering memory")
	}
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.Entries[key] = MemoryEntry{
		Key:        key,
		Category:   category,
		Value:      audit.RedactString(value),
		UpdatedBy:  actor,
		UpdatedUTC: time.Now().UTC().Format(time.RFC3339),
	}
	b, err := json.MarshalIndent(struct {
		Entries map[string]MemoryEntry `json:"entries"`
	}{Entries: ms.Entries}, "", "  ")
	if err != nil {
		return err
	}
	tmp := ms.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, ms.path)
}
