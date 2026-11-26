package remote

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	"nano_android/engine/security/audit"
	"nano_android/engine/testing/suite"
)

// ProviderKind identifies local, wireless, remote on-prem, or cloud Android execution providers (§10.3).
type ProviderKind string

const (
	ProviderLocalAVD         ProviderKind = "local_avd"
	ProviderUSBDevice        ProviderKind = "usb_device"
	ProviderTLSWireless      ProviderKind = "tls_wireless"
	ProviderRemoteADB        ProviderKind = "remote_adb"
	ProviderManagedOnPrem    ProviderKind = "managed_onprem_pool"
	ProviderFirebaseTestLab  ProviderKind = "cloud_firebase_test_lab"
)

// PoolDevice represents a schedulable physical, AVD, wireless, remote, or cloud target.
type PoolDevice struct {
	ID            string       `json:"id"`
	Provider      ProviderKind `json:"provider"`
	SerialOrAddr  string       `json:"serial_or_addr"`
	Model         string       `json:"model"`
	APILevel      int          `json:"api_level"`
	Healthy       bool         `json:"healthy"`
	ReservedBy    string       `json:"reserved_by,omitempty"`
	LeaseExpires  string       `json:"lease_expires,omitempty"`
	LastError     string       `json:"last_error,omitempty"`
}

// HealthStatus summarizes live probe readiness of one device target.
type HealthStatus struct {
	DeviceID   string `json:"device_id"`
	Healthy    bool   `json:"healthy"`
	Detail     string `json:"detail"`
	CheckedUTC string `json:"checked_utc"`
}

// DeviceProvider abstracts discovery & health checks across heterogeneous execution providers.
type DeviceProvider interface {
	Kind() ProviderKind
	Discover(ctx context.Context) ([]PoolDevice, error)
	HealthCheck(ctx context.Context, serialOrAddr string) HealthStatus
}

// ADBProvider discovers attached USB/AVD/wireless ADB targets via real `adb devices -l`.
type ADBProvider struct {
	ADBPath string
}

func (p ADBProvider) resolveADB() string {
	if strings.TrimSpace(p.ADBPath) != "" {
		return p.ADBPath
	}
	path, _ := exec.LookPath("adb")
	return path
}

func (ADBProvider) Kind() ProviderKind { return ProviderUSBDevice }

func (p ADBProvider) Discover(ctx context.Context) ([]PoolDevice, error) {
	adbExe := p.resolveADB()
	if adbExe == "" {
		return nil, nil
	}
	cctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, adbExe, "devices", "-l").Output()
	if err != nil {
		return nil, err
	}
	var res []PoolDevice
	for _, rawLine := range strings.Split(string(out), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "List of devices") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		serial, state := fields[0], fields[1]
		prov := ProviderUSBDevice
		if strings.HasPrefix(serial, "emulator-") {
			prov = ProviderLocalAVD
		} else if strings.Contains(serial, ":") {
			prov = ProviderTLSWireless
		}
		model := serial
		for _, f := range fields[2:] {
			if strings.HasPrefix(f, "model:") {
				model = strings.TrimPrefix(f, "model:")
			}
		}
		res = append(res, PoolDevice{
			ID:           "adb:" + serial,
			Provider:     prov,
			SerialOrAddr: serial,
			Model:        model,
			Healthy:      state == "device",
		})
	}
	return res, nil
}

func (p ADBProvider) HealthCheck(ctx context.Context, serialOrAddr string) HealthStatus {
	adbExe := p.resolveADB()
	hs := HealthStatus{DeviceID: serialOrAddr, CheckedUTC: time.Now().UTC().Format(time.RFC3339)}
	if adbExe == "" {
		hs.Detail = "adb not installed on host"
		return hs
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, adbExe, "-s", serialOrAddr, "get-state").CombinedOutput()
	txt := strings.TrimSpace(string(out))
	if err == nil && txt == "device" {
		hs.Healthy = true
		hs.Detail = "adb state=device"
	} else {
		hs.Detail = fmt.Sprintf("adb state=%q err=%v", txt, err)
	}
	return hs
}

// ManagedEndpoint describes one registered remote/on-prem pool node (e.g. tcp Redroid/Cuttlefish/device rack).
type ManagedEndpoint struct {
	ID       string
	Provider ProviderKind
	HostPort string
	Model    string
	APILevel int
}

// ManagedPoolProvider validates registered remote TCP endpoints using real net.DialTimeout probes.
type ManagedPoolProvider struct {
	Endpoints []ManagedEndpoint
}

func (ManagedPoolProvider) Kind() ProviderKind { return ProviderManagedOnPrem }

func (m ManagedPoolProvider) Discover(ctx context.Context) ([]PoolDevice, error) {
	out := make([]PoolDevice, 0, len(m.Endpoints))
	for _, ep := range m.Endpoints {
		hs := m.HealthCheck(ctx, ep.HostPort)
		prov := ep.Provider
		if prov == "" {
			prov = ProviderManagedOnPrem
		}
		out = append(out, PoolDevice{
			ID:           ep.ID,
			Provider:     prov,
			SerialOrAddr: ep.HostPort,
			Model:        ep.Model,
			APILevel:     ep.APILevel,
			Healthy:      hs.Healthy,
			LastError:    hs.Detail,
		})
	}
	return out, nil
}

func (ManagedPoolProvider) HealthCheck(ctx context.Context, hostPort string) HealthStatus {
	hs := HealthStatus{DeviceID: hostPort, CheckedUTC: time.Now().UTC().Format(time.RFC3339)}
	conn, err := (&net.Dialer{Timeout: 1500 * time.Millisecond}).DialContext(ctx, "tcp", hostPort)
	if err != nil {
		hs.Detail = err.Error()
		return hs
	}
	_ = conn.Close()
	hs.Healthy = true
	hs.Detail = "tcp reachable"
	return hs
}

// FirebaseTestLabProvider exposes Spark/Blaze FTL capability discovery (gcloud CLI check without silent egress).
type FirebaseTestLabProvider struct {
	ProjectID string
	Tier      string // "spark" or "blaze"
}

func (FirebaseTestLabProvider) Kind() ProviderKind { return ProviderFirebaseTestLab }

func (f FirebaseTestLabProvider) Discover(ctx context.Context) ([]PoolDevice, error) {
	gcloudExe, err := exec.LookPath("gcloud")
	healthy := err == nil && strings.TrimSpace(f.ProjectID) != ""
	detail := "gcloud CLI missing or Firebase ProjectID unset"
	if healthy {
		detail = "gcloud ready at " + gcloudExe
	}
	tier := f.Tier
	if tier == "" {
		tier = "spark"
	}
	return []PoolDevice{
		{
			ID:           "ftl:" + tier,
			Provider:     ProviderFirebaseTestLab,
			SerialOrAddr: "firebase-test-lab://" + f.ProjectID,
			Model:        "FirebaseTestLab-" + tier,
			APILevel:     35,
			Healthy:      healthy,
			LastError:    detail,
		},
	}, nil
}

func (f FirebaseTestLabProvider) HealthCheck(ctx context.Context, _ string) HealthStatus {
	gcloudExe, err := exec.LookPath("gcloud")
	return HealthStatus{
		DeviceID:   "ftl:" + f.ProjectID,
		Healthy:    err == nil && f.ProjectID != "",
		Detail:     fmt.Sprintf("gcloud=%q project=%q", gcloudExe, f.ProjectID),
		CheckedUTC: time.Now().UTC().Format(time.RFC3339),
	}
}

// QuotaConfig caps pool concurrency, shard count, and daily runtime budgets per actor/project.
type QuotaConfig struct {
	MaxConcurrentJobs int
	MaxShardsPerJob   int
	MaxDailyJobs      int
}

// DefaultQuota returns safe default pool quotas.
func DefaultQuota() QuotaConfig {
	return QuotaConfig{
		MaxConcurrentJobs: 4,
		MaxShardsPerJob:   8,
		MaxDailyJobs:      200,
	}
}

// Reservation represents an exclusive device lease inside the PoolManager.
type Reservation struct {
	ID         string       `json:"id"`
	DeviceID   string       `json:"device_id"`
	Provider   ProviderKind `json:"provider"`
	Actor      string       `json:"actor"`
	ExpiresAt  time.Time    `json:"expires_at"`
}

// ShardRequest configures parallel sharded suite execution across registered pool devices.
type ShardRequest struct {
	JobName    string           `json:"job_name"`
	Actor      string           `json:"actor"`
	Suite      suite.Suite      `json:"suite"`
	MaxShards  int              `json:"max_shards"`
	MaxRetries int              `json:"max_retries"`
}

// ShardOutcome summarizes execution on one shard device.
type ShardOutcome struct {
	ShardIndex   int                 `json:"shard_index"`
	DeviceID     string              `json:"device_id"`
	Provider     ProviderKind        `json:"provider"`
	Attempts     int                 `json:"attempts"`
	CasesRun     int                 `json:"cases_run"`
	CasesPassed  int                 `json:"cases_passed"`
	CasesFailed  int                 `json:"cases_failed"`
	CaseOutcomes []suite.CaseOutcome `json:"case_outcomes"`
	Artifacts    []string            `json:"artifacts,omitempty"`
	Error        string              `json:"error,omitempty"`
}

// PoolJobResult aggregates sharded pool execution results and cleanup state.
type PoolJobResult struct {
	JobID            string         `json:"job_id"`
	JobName          string         `json:"job_name"`
	Status           string         `json:"status"` // COMPLETED, FAILED, CANCELED, BLOCKED_POLICY
	ShardsUsed       int            `json:"shards_used"`
	TotalCases       int            `json:"total_cases"`
	PassedCases      int            `json:"passed_cases"`
	FailedCases      int            `json:"failed_cases"`
	DurationMs       int64          `json:"duration_ms"`
	CleanedUpLeases  int            `json:"cleaned_up_leases"`
	Shards           []ShardOutcome `json:"shards"`
}

// ShardRunner executes one slice of TestCases on a reserved PoolDevice and optionally collects artifact paths.
type ShardRunner func(ctx context.Context, dev PoolDevice, cases []suite.TestCase) ([]suite.CaseOutcome, []string, error)

// PoolManager orchestrates multi-provider discovery, reservations, health checks, sharding, and quotas.
type PoolManager struct {
	mu           sync.Mutex
	providers    []DeviceProvider
	devices      map[string]*PoolDevice
	reservations map[string]*Reservation
	quota        QuotaConfig
	activeJobs   map[string]int
	dailyJobs    map[string]int
	ledger       *audit.Ledger
}

// NewPoolManager creates a pool scheduler backed by real DeviceProviders and QuotaConfig.
func NewPoolManager(quota QuotaConfig, ledger *audit.Ledger, providers ...DeviceProvider) *PoolManager {
	if quota.MaxConcurrentJobs <= 0 {
		quota = DefaultQuota()
	}
	return &PoolManager{
		providers:    providers,
		devices:      make(map[string]*PoolDevice),
		reservations: make(map[string]*Reservation),
		quota:        quota,
		activeJobs:   make(map[string]int),
		dailyJobs:    make(map[string]int),
		ledger:       ledger,
	}
}

// RegisterDevice adds or refreshes a device slot explicitly (e.g. a local Nano JVM or managed test slot).
func (pm *PoolManager) RegisterDevice(d PoolDevice) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	cp := d
	pm.devices[cp.ID] = &cp
}

// Discover refreshes device inventory from all attached providers while preserving active reservations.
func (pm *PoolManager) Discover(ctx context.Context) []PoolDevice {
	var discovered []PoolDevice
	for _, prov := range pm.providers {
		list, err := prov.Discover(ctx)
		if err == nil {
			discovered = append(discovered, list...)
		}
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for _, d := range discovered {
		if existing, ok := pm.devices[d.ID]; ok && existing.ReservedBy != "" {
			d.ReservedBy = existing.ReservedBy
			d.LeaseExpires = existing.LeaseExpires
		}
		cp := d
		pm.devices[cp.ID] = &cp
	}
	var out []PoolDevice
	for _, d := range pm.devices {
		pm.expireIfNeededLocked(d)
		out = append(out, *d)
	}
	return out
}

// Reserve locks one healthy unreserved device matching optional provider preference.
func (pm *PoolManager) Reserve(actor string, preferred ProviderKind, leaseTTL time.Duration) (*Reservation, *PoolDevice, error) {
	if strings.TrimSpace(actor) == "" {
		return nil, nil, fmt.Errorf("BLOCKED_POLICY non-empty actor required for device reservation")
	}
	if leaseTTL <= 0 {
		leaseTTL = 5 * time.Minute
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, d := range pm.devices {
		pm.expireIfNeededLocked(d)
		if !d.Healthy || d.ReservedBy != "" {
			continue
		}
		if preferred != "" && d.Provider != preferred {
			continue
		}
		rid := "res_" + randHex(5)
		exp := time.Now().UTC().Add(leaseTTL)
		d.ReservedBy = rid
		d.LeaseExpires = exp.Format(time.RFC3339)
		res := &Reservation{
			ID:        rid,
			DeviceID:  d.ID,
			Provider:  d.Provider,
			Actor:     actor,
			ExpiresAt: exp,
		}
		pm.reservations[rid] = res
		if pm.ledger != nil {
			_, _ = pm.ledger.Record("remote_pool", actor, "reserve_device", d.ID, "ALLOW", "leased", map[string]any{"reservation_id": rid})
		}
		cpDev := *d
		return res, &cpDev, nil
	}
	return nil, nil, fmt.Errorf("no healthy unreserved device available (provider=%q)", preferred)
}

// Release releases a device lease cleanly (auto-cleanup).
func (pm *PoolManager) Release(reservationID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	res, ok := pm.reservations[reservationID]
	if !ok {
		return fmt.Errorf("reservation %q not found", reservationID)
	}
	if d, exists := pm.devices[res.DeviceID]; exists && d.ReservedBy == reservationID {
		d.ReservedBy = ""
		d.LeaseExpires = ""
	}
	delete(pm.reservations, reservationID)
	if pm.ledger != nil {
		_, _ = pm.ledger.Record("remote_pool", res.Actor, "release_device", res.DeviceID, "ALLOW", "released", map[string]any{"reservation_id": reservationID})
	}
	return nil
}

// SubmitAndShard enforces quotas, reserves up to MaxShards devices, executes shards in parallel with retries & cancellation, and cleans up all leases.
func (pm *PoolManager) SubmitAndShard(ctx context.Context, req ShardRequest, runner ShardRunner) (*PoolJobResult, error) {
	actor := strings.TrimSpace(req.Actor)
	if actor == "" {
		return nil, fmt.Errorf("BLOCKED_POLICY non-empty actor required")
	}
	if runner == nil {
		return nil, fmt.Errorf("shard runner is required")
	}
	shardsRequested := req.MaxShards
	if shardsRequested <= 0 {
		shardsRequested = 1
	}

	pm.mu.Lock()
	if pm.activeJobs[actor] >= pm.quota.MaxConcurrentJobs {
		pm.mu.Unlock()
		return nil, fmt.Errorf("BLOCKED_POLICY concurrent job quota exceeded (%d/%d) for %q", pm.activeJobs[actor], pm.quota.MaxConcurrentJobs, actor)
	}
	if pm.dailyJobs[actor] >= pm.quota.MaxDailyJobs {
		pm.mu.Unlock()
		return nil, fmt.Errorf("BLOCKED_POLICY daily job quota exceeded (%d/%d) for %q", pm.dailyJobs[actor], pm.quota.MaxDailyJobs, actor)
	}
	if shardsRequested > pm.quota.MaxShardsPerJob {
		pm.mu.Unlock()
		return nil, fmt.Errorf("BLOCKED_POLICY shard count %d exceeds MaxShardsPerJob (%d)", shardsRequested, pm.quota.MaxShardsPerJob)
	}
	pm.activeJobs[actor]++
	pm.dailyJobs[actor]++
	pm.mu.Unlock()

	defer func() {
		pm.mu.Lock()
		pm.activeJobs[actor]--
		pm.mu.Unlock()
	}()

	start := time.Now()
	// Reserve up to shardsRequested healthy devices
	var leases []*Reservation
	var leasedDevs []PoolDevice
	for i := 0; i < shardsRequested && i < max1(len(req.Suite.Cases)); i++ {
		res, dev, rErr := pm.Reserve(actor, "", 5*time.Minute)
		if rErr != nil {
			break
		}
		leases = append(leases, res)
		leasedDevs = append(leasedDevs, *dev)
	}
	cleaned := 0
	defer func() {
		for _, l := range leases {
			if pm.Release(l.ID) == nil {
				cleaned++
			}
		}
	}()

	if len(leasedDevs) == 0 {
		return nil, fmt.Errorf("no healthy devices available in pool")
	}

	// Partition suite test cases across leased shard devices round-robin
	buckets := make([][]suite.TestCase, len(leasedDevs))
	for i, tc := range req.Suite.Cases {
		buckets[i%len(leasedDevs)] = append(buckets[i%len(leasedDevs)], tc)
	}

	outcomes := make([]ShardOutcome, len(leasedDevs))
	var wg sync.WaitGroup
	for idx, dev := range leasedDevs {
		wg.Add(1)
		go func(shardIdx int, target PoolDevice, cases []suite.TestCase) {
			defer wg.Done()
			so := ShardOutcome{
				ShardIndex: shardIdx,
				DeviceID:   target.ID,
				Provider:   target.Provider,
				CasesRun:   len(cases),
			}
			attempts := 0
			var lastErr error
			var caseOut []suite.CaseOutcome
			var arts []string
			for attempts <= req.MaxRetries {
				if ctx.Err() != nil {
					lastErr = ctx.Err()
					break
				}
				attempts++
				co, a, err := runner(ctx, target, cases)
				caseOut = co
				arts = a
				lastErr = err
				if err == nil && allCasesPassed(co) {
					break
				}
			}
			so.Attempts = attempts
			so.CaseOutcomes = caseOut
			so.Artifacts = arts
			if lastErr != nil {
				so.Error = lastErr.Error()
			}
			for _, c := range caseOut {
				if c.FailedCount == 0 && (c.Status == "NANO_SIMULATION_PASS" || c.Status == "ART_EMULATOR_PASS" || c.Status == "PHYSICAL_DEVICE_PASS" || c.Status == "ROBOLECTRIC_PASS") {
					so.CasesPassed++
				} else {
					so.CasesFailed++
				}
			}
			outcomes[shardIdx] = so
		}(idx, dev, buckets[idx])
	}
	wg.Wait()

	// Release before building final struct so CleanedUpLeases reflects actual releases
	for _, l := range leases {
		if pm.Release(l.ID) == nil {
			cleaned++
		}
	}
	leases = nil

	res := &PoolJobResult{
		JobID:           "job_" + randHex(5),
		JobName:         req.JobName,
		Status:          "COMPLETED",
		ShardsUsed:      len(leasedDevs),
		TotalCases:      len(req.Suite.Cases),
		DurationMs:      time.Since(start).Milliseconds(),
		CleanedUpLeases: cleaned,
		Shards:          outcomes,
	}
	for _, so := range outcomes {
		res.PassedCases += so.CasesPassed
		res.FailedCases += so.CasesFailed
		if so.Error != "" && res.Status == "COMPLETED" {
			res.Status = "FAILED"
		}
	}
	if ctx.Err() != nil {
		res.Status = "CANCELED"
	} else if res.FailedCases > 0 {
		res.Status = "FAILED"
	}
	return res, nil
}

func (pm *PoolManager) expireIfNeededLocked(d *PoolDevice) {
	if d.ReservedBy == "" || d.LeaseExpires == "" {
		return
	}
	exp, err := time.Parse(time.RFC3339, d.LeaseExpires)
	if err == nil && time.Now().UTC().After(exp) {
		delete(pm.reservations, d.ReservedBy)
		d.ReservedBy = ""
		d.LeaseExpires = ""
	}
}

func allCasesPassed(co []suite.CaseOutcome) bool {
	for _, c := range co {
		if c.FailedCount > 0 || c.Status == "FAIL" || c.Status == "UNSUPPORTED" || c.Status == "BLOCKED_POLICY" {
			return false
		}
	}
	return len(co) > 0
}

func max1(v int) int {
	if v < 1 {
		return 1
	}
	return v
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
