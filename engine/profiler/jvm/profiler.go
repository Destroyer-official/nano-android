package jvm

import (
	"fmt"
	"time"

	"nano_android/engine/mcp_compiler"
	"nano_android/engine/profiler/timeline"
)

// QueryTiming tracks individual SQLite/file/network latency measurements recorded during JVM execution.
type QueryTiming struct {
	Operation  string  `json:"operation"`
	Target     string  `json:"target"`
	DurationMs float64 `json:"duration_ms"`
}

// Snapshot captures live JVM memory, GC, classloading, thread/deadlock, Looper depth, and forensic timeline data.
type Snapshot struct {
	CapturedAtUTC         string                         `json:"captured_at_utc"`
	Backend               string                         `json:"backend"` // strictly "nano_jvm"
	EnvironmentDisclosure string                         `json:"environment_disclosure"`
	Metrics               *mcp_compiler.RuntimeMetrics   `json:"metrics"`
	Threads               *mcp_compiler.HostThreadReport `json:"threads"`
	Timeline              []timeline.Event               `json:"timeline"`
	QueryTimings          []QueryTiming                  `json:"query_timings,omitempty"`
	Warnings              []string                       `json:"warnings,omitempty"`
}

// CaptureSnapshot pulls real JVM heap, GC, thread/deadlock, Looper queue, and ForensicTimeline metrics
// directly from an active LiveHeadlessBridge worker.
func CaptureSnapshot(bridge *mcp_compiler.LiveHeadlessBridge, extraTimings ...QueryTiming) (*Snapshot, error) {
	if bridge == nil {
		return nil, fmt.Errorf("headless bridge is required")
	}

	metrics, err := bridge.InspectRuntime()
	if err != nil {
		return nil, fmt.Errorf("inspect runtime metrics: %w", err)
	}
	threads, tErr := bridge.InspectThreads()
	rawTimeline, tlErr := bridge.InspectTimeline()

	tl := timeline.New("jvm-profile")
	if tlErr == nil {
		tl.IngestHostEvents(rawTimeline)
	}

	snap := &Snapshot{
		CapturedAtUTC:         time.Now().UTC().Format(time.RFC3339),
		Backend:               "nano_jvm",
		EnvironmentDisclosure: "Host OpenJDK 25 JVM metrics only (not equivalent to Android ART native RSS, RenderThread, GPU, or ODPM power)",
		Metrics:               metrics,
		Threads:               threads,
		Timeline:              tl.Events(),
		QueryTimings:          append([]QueryTiming(nil), extraTimings...),
	}
	if tErr != nil {
		snap.Warnings = append(snap.Warnings, "thread inspection warning: "+tErr.Error())
	}
	if tlErr != nil {
		snap.Warnings = append(snap.Warnings, "timeline inspection warning: "+tlErr.Error())
	}
	if metrics.DeadlockedThreads > 0 || (threads != nil && threads.DeadlockedCount > 0) {
		snap.Warnings = append(snap.Warnings, fmt.Sprintf("CRITICAL: %d deadlocked JVM threads detected", metrics.DeadlockedThreads))
	}
	return snap, nil
}
