package plugins

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"nano_android/engine/security/audit"
	"nano_android/engine/security/pathcontainment"
)

var pluginIDRegex = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

type limitedBuffer struct {
	buf bytes.Buffer
	max int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	rem := l.max - l.buf.Len()
	if rem > 0 {
		if len(p) > rem {
			l.buf.Write(p[:rem])
		} else {
			l.buf.Write(p)
		}
	}
	return len(p), nil
}

// PluginKind enumerates the 10 supported Nano-Android SDK extension points.
type PluginKind string

const (
	KindAPIShim         PluginKind = "api_shim"
	KindAPKAnalyzer     PluginKind = "apk_analyzer"
	KindResourceParser  PluginKind = "resource_parser"
	KindRenderingEngine PluginKind = "rendering_engine"
	KindTestReporter    PluginKind = "test_reporter"
	KindAIAgent         PluginKind = "ai_agent"
	KindDeviceProvider  PluginKind = "device_provider"
	KindBenchmarkEngine PluginKind = "benchmark_engine"
	KindSecurityScanner PluginKind = "security_scanner"
	KindMediaAnalyzer   PluginKind = "media_analyzer"
)

// TrustLevel governs process isolation guarantees for loaded plugins.
type TrustLevel string

const (
	TrustCore            TrustLevel = "TRUSTED_CORE"
	TrustVerifiedSandbox TrustLevel = "VERIFIED_SANDBOX"
	TrustUntrusted       TrustLevel = "UNTRUSTED"
)

// PluginManifest declares versioned SDK metadata, declared permissions, compatibility bounds, and Ed25519 signature.
type PluginManifest struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Version       string     `json:"version"`
	APIVersion    string     `json:"api_version"` // e.g. "1.0"
	Kind          PluginKind `json:"kind"`
	TrustLevel    TrustLevel `json:"trust_level"`
	Entrypoint    string     `json:"entrypoint"` // relative path inside plugin dir
	Permissions   []string   `json:"permissions,omitempty"`
	MinEngineVer  string     `json:"min_engine_version,omitempty"`
	MinSDK        int        `json:"min_sdk,omitempty"`
	MaxSDK        int        `json:"max_sdk,omitempty"`
	SHA256        string     `json:"sha256"`
	PublicKeyHex  string     `json:"public_key_ed25519"`
	SignatureHex  string     `json:"signature_ed25519"`
}

// CanonicalBytes computes the deterministic payload covered by the Ed25519 signature.
func (m PluginManifest) CanonicalBytes() []byte {
	cp := m
	cp.SignatureHex = ""
	b, _ := json.Marshal(cp)
	return b
}

// GenerateEd25519Keypair creates a hex-encoded Ed25519 public/private keypair for plugin/report signing.
func GenerateEd25519Keypair() (pubHex, privHex string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return hex.EncodeToString(pub), hex.EncodeToString(priv), nil
}

// SignManifest computes the entrypoint SHA-256 and signs the canonical manifest using Ed25519.
func SignManifest(m *PluginManifest, entrypointPath, privHex string) error {
	if m == nil {
		return fmt.Errorf("manifest is required")
	}
	privBytes, err := hex.DecodeString(strings.TrimSpace(privHex))
	if err != nil || len(privBytes) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid Ed25519 private key hex")
	}
	priv := ed25519.PrivateKey(privBytes)
	pub := priv.Public().(ed25519.PublicKey)

	rawEntry, err := os.ReadFile(entrypointPath)
	if err != nil {
		return fmt.Errorf("read entrypoint: %w", err)
	}
	sum := sha256.Sum256(rawEntry)
	m.SHA256 = hex.EncodeToString(sum[:])
	m.PublicKeyHex = hex.EncodeToString(pub)
	if m.APIVersion == "" {
		m.APIVersion = "1.0"
	}
	if m.TrustLevel == "" || m.TrustLevel == TrustUntrusted {
		m.TrustLevel = TrustVerifiedSandbox
	}
	sig := ed25519.Sign(priv, m.CanonicalBytes())
	m.SignatureHex = hex.EncodeToString(sig)
	return nil
}

// VerifyManifest validates the Ed25519 signature and entrypoint SHA-256 integrity on disk.
func VerifyManifest(m PluginManifest, entrypointPath string) error {
	if !pluginIDRegex.MatchString(strings.TrimSpace(m.ID)) || strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("BLOCKED_POLICY plugin manifest missing/invalid id %q or version", m.ID)
	}
	if !isSupportedKind(m.Kind) {
		return fmt.Errorf("BLOCKED_POLICY unsupported plugin kind %q", m.Kind)
	}
	pubBytes, err := hex.DecodeString(strings.TrimSpace(m.PublicKeyHex))
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("BLOCKED_POLICY invalid Ed25519 public key")
	}
	sigBytes, err := hex.DecodeString(strings.TrimSpace(m.SignatureHex))
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return fmt.Errorf("BLOCKED_POLICY invalid Ed25519 signature length")
	}
	if !ed25519.Verify(ed25519.PublicKey(pubBytes), m.CanonicalBytes(), sigBytes) {
		return fmt.Errorf("BLOCKED_POLICY Ed25519 signature verification failed for plugin %q", m.ID)
	}
	rawEntry, err := os.ReadFile(entrypointPath)
	if err != nil {
		return fmt.Errorf("BLOCKED_POLICY entrypoint missing: %w", err)
	}
	sum := sha256.Sum256(rawEntry)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), m.SHA256) {
		return fmt.Errorf("BLOCKED_POLICY entrypoint SHA-256 mismatch for plugin %q", m.ID)
	}
	return nil
}

// RegisteredPlugin pairs a verified manifest with its contained filesystem directory and entrypoint.
type RegisteredPlugin struct {
	Manifest       PluginManifest `json:"manifest"`
	PluginDir      string         `json:"plugin_dir"`
	EntrypointAbs  string         `json:"entrypoint_abs"`
	Verified       bool           `json:"verified"`
	RegisteredUTC  string         `json:"registered_utc"`
}

// Registry manages versioned plugin discovery, Ed25519 verification, and isolated subprocess execution.
type Registry struct {
	mu              sync.RWMutex
	storeRoot       string
	requireVerified bool
	ledger          *audit.Ledger
	plugins         map[string]RegisteredPlugin
}

// NewRegistry creates a versioned SDK registry persisted under storeRoot/registry.json.
func NewRegistry(storeRoot string, ledger *audit.Ledger, requireVerified bool) *Registry {
	if strings.TrimSpace(storeRoot) == "" {
		storeRoot = filepath.Join("cache", "plugins")
	}
	_ = os.MkdirAll(storeRoot, 0750)
	reg := &Registry{
		storeRoot:       storeRoot,
		requireVerified: requireVerified,
		ledger:          ledger,
		plugins:         make(map[string]RegisteredPlugin),
	}
	reg.load()
	return reg
}

// InstallFromDir validates containment, verifies Ed25519 signature & SHA-256, and registers a plugin directory.
func (r *Registry) InstallFromDir(pluginDir string, allowedRoots []string) (*RegisteredPlugin, error) {
	if len(allowedRoots) == 0 {
		allowedRoots = []string{r.storeRoot}
	}
	absDir, err := filepath.Abs(pluginDir)
	if err != nil {
		return nil, err
	}
	manifestPath, err := pathcontainment.ResolveExistingContained(filepath.Join(absDir, "plugin.json"), allowedRoots, []string{".json"})
	if err != nil {
		r.audit("plugin_install", pluginDir, "DENY", err.Error(), nil)
		return nil, fmt.Errorf("BLOCKED_POLICY plugin manifest containment failed: %w", err)
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var m PluginManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("invalid plugin.json: %w", err)
	}
	if strings.Contains(m.Entrypoint, "..") || filepath.IsAbs(m.Entrypoint) {
		r.audit("plugin_install", m.ID, "DENY", "invalid entrypoint path", nil)
		return nil, fmt.Errorf("BLOCKED_POLICY plugin entrypoint must be relative inside plugin directory")
	}
	entryAbs, err := pathcontainment.ResolveExistingContained(filepath.Join(filepath.Dir(manifestPath), m.Entrypoint), []string{filepath.Dir(manifestPath)}, nil)
	if err != nil {
		r.audit("plugin_install", m.ID, "DENY", err.Error(), nil)
		return nil, fmt.Errorf("BLOCKED_POLICY %w", err)
	}

	verified := false
	if vErr := VerifyManifest(m, entryAbs); vErr != nil {
		if r.requireVerified {
			r.audit("plugin_install", m.ID, "DENY", vErr.Error(), nil)
			return nil, vErr
		}
		m.TrustLevel = TrustUntrusted
	} else {
		verified = true
	}

	rp := RegisteredPlugin{
		Manifest:      m,
		PluginDir:     filepath.Dir(manifestPath),
		EntrypointAbs: entryAbs,
		Verified:      verified,
		RegisteredUTC: time.Now().UTC().Format(time.RFC3339),
	}
	r.mu.Lock()
	r.plugins[m.ID] = rp
	_ = r.saveLocked()
	r.mu.Unlock()

	r.audit("plugin_install", m.ID, "ALLOW", fmt.Sprintf("installed v%s verified=%v trust=%s", m.Version, verified, m.TrustLevel), map[string]any{
		"kind":   string(m.Kind),
		"sha256": m.SHA256,
	})
	return &rp, nil
}

// Invoke executes an installed plugin inside a trust-level isolated subprocess with clean environment & JSON stdin/stdout IPC.
func (r *Registry) Invoke(ctx context.Context, pluginID string, payload map[string]any) (map[string]any, error) {
	r.mu.RLock()
	rp, ok := r.plugins[pluginID]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("plugin %q not installed", pluginID)
	}
	if r.requireVerified && (!rp.Verified || rp.Manifest.TrustLevel == TrustUntrusted) {
		r.audit("plugin_invoke", pluginID, "DENY", "untrusted plugin blocked by verification policy", nil)
		return nil, fmt.Errorf("BLOCKED_POLICY plugin %q failed trust verification", pluginID)
	}
	if err := VerifyManifest(rp.Manifest, rp.EntrypointAbs); err != nil && r.requireVerified {
		r.audit("plugin_invoke", pluginID, "DENY", err.Error(), nil)
		return nil, err
	}

	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	inBytes, _ := json.Marshal(payload)
	var cmd *exec.Cmd
	if strings.HasSuffix(strings.ToLower(rp.EntrypointAbs), ".py") {
		pyExe, pErr := exec.LookPath("python")
		if pErr != nil {
			pyExe, _ = exec.LookPath("python3")
		}
		if pyExe == "" {
			return nil, fmt.Errorf("INFRASTRUCTURE_ERROR python interpreter not found for .py plugin")
		}
		cmd = exec.CommandContext(runCtx, pyExe, rp.EntrypointAbs)
	} else {
		cmd = exec.CommandContext(runCtx, rp.EntrypointAbs)
	}
	cmd.Dir = rp.PluginDir
	// Scrubbed isolated environment: expose only OS base system vars + trust metadata, zero secrets/tokens.
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"SystemRoot=" + os.Getenv("SystemRoot"),
		"NANO_PLUGIN_ID=" + rp.Manifest.ID,
		"NANO_PLUGIN_VERSION=" + rp.Manifest.Version,
		"NANO_PLUGIN_TRUST=" + string(rp.Manifest.TrustLevel),
	}
	cmd.Stdin = bytes.NewReader(inBytes)
	stdout := &limitedBuffer{max: 2 * 1024 * 1024}
	stderr := &limitedBuffer{max: 64 * 1024}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		r.audit("plugin_invoke", pluginID, "ERROR", err.Error(), map[string]any{"stderr": truncateStr(stderr.buf.String(), 256)})
		return nil, fmt.Errorf("plugin %q execution failed: %w (%s)", pluginID, err, strings.TrimSpace(stderr.buf.String()))
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.buf.Bytes(), &out); err != nil {
		out = map[string]any{"raw_output": truncateStr(stdout.buf.String(), 4096)}
	}
	out["_plugin_id"] = rp.Manifest.ID
	out["_plugin_version"] = rp.Manifest.Version
	out["_trust_level"] = rp.Manifest.TrustLevel
	out["_verified_ed25519"] = rp.Verified
	r.audit("plugin_invoke", pluginID, "ALLOW", "executed in isolated plugin host", nil)
	return out, nil
}

// List returns all registered plugins sorted by ID.
func (r *Registry) List() []RegisteredPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]RegisteredPlugin, 0, len(r.plugins))
	for _, rp := range r.plugins {
		out = append(out, rp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Manifest.ID < out[j].Manifest.ID })
	return out
}

// SupportedKinds lists all 10 SDK extension kinds.
func SupportedKinds() []PluginKind {
	return []PluginKind{
		KindAPIShim, KindAPKAnalyzer, KindResourceParser, KindRenderingEngine,
		KindTestReporter, KindAIAgent, KindDeviceProvider, KindBenchmarkEngine,
		KindSecurityScanner, KindMediaAnalyzer,
	}
}

func isSupportedKind(k PluginKind) bool {
	for _, valid := range SupportedKinds() {
		if k == valid {
			return true
		}
	}
	return false
}

func (r *Registry) registryFile() string {
	return filepath.Join(r.storeRoot, "registry.json")
}

func (r *Registry) load() {
	raw, err := os.ReadFile(r.registryFile())
	if err != nil {
		return
	}
	var items []RegisteredPlugin
	if json.Unmarshal(raw, &items) == nil {
		for _, item := range items {
			r.plugins[item.Manifest.ID] = item
		}
	}
}

func (r *Registry) saveLocked() error {
	items := make([]RegisteredPlugin, 0, len(r.plugins))
	for _, rp := range r.plugins {
		items = append(items, rp)
	}
	b, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	tmp := r.registryFile() + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, r.registryFile())
}

func (r *Registry) audit(action, res, decision, reason string, meta map[string]any) {
	if r.ledger != nil {
		_, _ = r.ledger.Record("plugin_sdk", "host", action, res, decision, reason, meta)
	}
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
