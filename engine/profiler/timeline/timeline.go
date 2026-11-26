package timeline

import (
	"encoding/json"
	"sort"
	"sync"
	"time"

	"nano_android/engine/mcp_compiler"
	"nano_android/engine/security/audit"
)

// Canonical correlated event types across Nano JVM, Robolectric, ART, and Physical backends.
const (
	EventAPKLaunched        = "APK_LAUNCHED"
	EventClassLoaded        = "CLASS_LOADED"
	EventActivityCreated    = "ACTIVITY_CREATED"
	EventViewInflated       = "VIEW_INFLATED"
	EventActionDispatched   = "ACTION_DISPATCHED"
	EventCallbackExecuted   = "CALLBACK_EXECUTED"
	EventNetworkEvent       = "NETWORK_EVENT"
	EventFileOp             = "FILE_OP"
	EventDBMutation         = "DB_MUTATION"
	EventExceptionRaised    = "EXCEPTION_RAISED"
	EventScreenshotCaptured = "SCREENSHOT_CAPTURED"
)

// Event represents a single correlated forensic event with monotonic sequence and timestamp.
type Event struct {
	Seq             int64  `json:"seq"`
	TimestampNs     int64  `json:"timestamp_ns"`
	VirtualUptimeMs int64  `json:"virtual_uptime_ms"`
	EventType       string `json:"event_type"`
	Source          string `json:"source"`
	Backend         string `json:"backend"`
	CorrelationID   string `json:"correlation_id"`
	Details         string `json:"details,omitempty"`
}

// Timeline maintains a thread-safe chronological sequence of secret-redacted forensic events.
type Timeline struct {
	mu            sync.Mutex
	correlationID string
	nextSeq       int64
	startTime     time.Time
	events        []Event
}

// New constructs a Timeline anchored to a correlation ID.
func New(correlationID string) *Timeline {
	if correlationID == "" {
		correlationID = "nano-timeline"
	}
	return &Timeline{
		correlationID: correlationID,
		startTime:     time.Now(),
	}
}

// Record appends a secret-redacted event with monotonic sequence and elapsed monotonic nanoseconds.
func (t *Timeline) Record(eventType, source, backend, details string, virtualUptimeMs int64) Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nextSeq++
	if backend == "" {
		backend = "nano_jvm"
	}
	ev := Event{
		Seq:             t.nextSeq,
		TimestampNs:     time.Since(t.startTime).Nanoseconds(),
		VirtualUptimeMs: virtualUptimeMs,
		EventType:       eventType,
		Source:          audit.RedactString(source),
		Backend:         backend,
		CorrelationID:   t.correlationID,
		Details:         audit.RedactString(details),
	}
	t.events = append(t.events, ev)
	return ev
}

// IngestHostEvents converts HeadlessAppHost ForensicTimeline events into this Timeline.
func (t *Timeline) IngestHostEvents(hostEvents []mcp_compiler.HostTimelineEvent) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	added := 0
	for _, he := range hostEvents {
		t.nextSeq++
		corr := he.CorrelationID
		if corr == "" {
			corr = t.correlationID
		}
		backend := he.Backend
		if backend == "" {
			backend = "nano_jvm"
		}
		t.events = append(t.events, Event{
			Seq:             t.nextSeq,
			TimestampNs:     he.TimestampNs,
			VirtualUptimeMs: he.VirtualUptimeMs,
			EventType:       he.EventType,
			Source:          audit.RedactString(he.Source),
			Backend:         backend,
			CorrelationID:   corr,
			Details:         audit.RedactString(he.Details),
		})
		added++
	}
	return added
}

// Events returns a snapshot slice ordered chronologically.
func (t *Timeline) Events() []Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Event, len(t.events))
	copy(out, t.events)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].VirtualUptimeMs != out[j].VirtualUptimeMs {
			return out[i].VirtualUptimeMs < out[j].VirtualUptimeMs
		}
		return out[i].Seq < out[j].Seq
	})
	return out
}

// FilterByType returns all events matching the given EventType.
func (t *Timeline) FilterByType(eventType string) []Event {
	all := t.Events()
	var filtered []Event
	for _, ev := range all {
		if ev.EventType == eventType {
			filtered = append(filtered, ev)
		}
	}
	return filtered
}

// ExportJSON serializes the forensic timeline for artifacts and CLI output.
func (t *Timeline) ExportJSON() ([]byte, error) {
	return json.MarshalIndent(map[string]any{
		"correlation_id": t.correlationID,
		"event_count":    len(t.events),
		"events":         t.Events(),
	}, "", "  ")
}
