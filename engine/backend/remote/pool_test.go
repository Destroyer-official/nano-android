package remote

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"nano_android/engine/security/audit"
	"nano_android/engine/testing/suite"
)

func TestRemotePoolDiscoveryHealthReserveShardAndQuotas(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, aErr := ln.Accept()
			if aErr != nil {
				return
			}
			_ = c.Close()
		}
	}()

	tmp := t.TempDir()
	ledger, _ := audit.OpenLedger(filepath.Join(tmp, "pool_audit.jsonl"))
	managed := ManagedPoolProvider{
		Endpoints: []ManagedEndpoint{
			{ID: "rem_1", Provider: ProviderRemoteADB, HostPort: ln.Addr().String(), Model: "Cuttlefish-1", APILevel: 35},
		},
	}
	ftl := FirebaseTestLabProvider{ProjectID: "", Tier: "spark"}
	pm := NewPoolManager(QuotaConfig{MaxConcurrentJobs: 1, MaxShardsPerJob: 2, MaxDailyJobs: 5}, ledger, managed, ftl)
	pm.RegisterDevice(PoolDevice{ID: "nano_w2", Provider: ProviderManagedOnPrem, SerialOrAddr: "local_w2", Model: "NanoWorker-2", APILevel: 35, Healthy: true})

	devs := pm.Discover(context.Background())
	if len(devs) < 3 {
		t.Fatalf("expected managed + ftl + registered worker, got %d", len(devs))
	}

	// Empty actor must be blocked
	if _, _, err := pm.Reserve("", "", time.Minute); err == nil {
		t.Fatalf("expected empty actor reservation rejected")
	}

	// Parallel sharded run across 2 healthy devices + automatic lease cleanup
	jobRes, err := pm.SubmitAndShard(context.Background(), ShardRequest{
		JobName:   "Phase6ShardedRun",
		Actor:     "ci_runner",
		MaxShards: 2,
		Suite: suite.Suite{
			Name: "ShardedSuite",
			Cases: []suite.TestCase{
				{Name: "Case1"}, {Name: "Case2"}, {Name: "Case3"},
			},
		},
	}, func(ctx context.Context, dev PoolDevice, cases []suite.TestCase) ([]suite.CaseOutcome, []string, error) {
		var out []suite.CaseOutcome
		for _, tc := range cases {
			out = append(out, suite.CaseOutcome{Name: tc.Name, Status: "NANO_SIMULATION_PASS", Iterations: 1, PassedCount: 1})
		}
		return out, []string{"trace_" + dev.ID + ".json"}, nil
	})
	if err != nil {
		t.Fatalf("SubmitAndShard failed: %v", err)
	}
	if jobRes.ShardsUsed != 2 || jobRes.PassedCases != 3 || jobRes.CleanedUpLeases != 2 {
		t.Fatalf("unexpected shard job result: %+v", jobRes)
	}

	// Quota exceeded check (MaxShardsPerJob = 2)
	if _, err := pm.SubmitAndShard(context.Background(), ShardRequest{
		JobName:   "OverQuota",
		Actor:     "ci_runner",
		MaxShards: 5,
		Suite:     suite.Suite{Cases: []suite.TestCase{{Name: "X"}}},
	}, func(ctx context.Context, dev PoolDevice, cases []suite.TestCase) ([]suite.CaseOutcome, []string, error) {
		return nil, nil, nil
	}); err == nil {
		t.Fatalf("expected MaxShardsPerJob quota enforcement error")
	}
}
