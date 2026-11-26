package suite

import (
	"bytes"
	"encoding/xml"
	"fmt"

	"nano_android/engine/network/mock"
)

// Assertion verifies post-step screen, storage, network, or audit state.
type Assertion struct {
	Type     string `json:"type"` // "visible", "not_visible", "text_matches", "a11y_passed", "http_captured"
	Target   string `json:"target"`
	Expected string `json:"expected,omitempty"`
}

// Step represents a single declarative UI/runtime action inside a test case.
type Step struct {
	Action    string     `json:"action"` // "tap", "type", "long_click", "swipe", "back", "step_time", "observe", "simulate_process_death", "assert"
	Target    string     `json:"target,omitempty"`
	Text      string     `json:"text,omitempty"`
	Direction string     `json:"direction,omitempty"`
	DeltaMs   int64      `json:"delta_ms,omitempty"`
	Assert    *Assertion `json:"assert,omitempty"`
}

// TestCase defines one isolated reproducible Android test flow.
type TestCase struct {
	Name           string   `json:"name"`
	Activity       string   `json:"activity,omitempty"`
	ProfileID      string   `json:"profile_id,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	RepeatCount    int      `json:"repeat_count,omitempty"`
	RetryOnFailure int      `json:"retry_on_failure,omitempty"`
	Steps          []Step   `json:"steps"`
}

// Suite bundles multiple test cases targeting one artifact.
type Suite struct {
	Name         string           `json:"name"`
	Artifact     string           `json:"artifact"`
	Mode         string           `json:"mode,omitempty"` // "auto", "static", "nano_jvm", "robolectric", "android_art", "physical_device"
	RepeatCount  int              `json:"repeat_count,omitempty"`
	WorkerCount  int              `json:"worker_count,omitempty"` // parallel worker concurrency (defaults to 1)
	NetworkStubs []mock.StubRoute `json:"network_stubs,omitempty"`
	Offline      bool             `json:"offline,omitempty"`
	Cases        []TestCase       `json:"cases"`
}

// CaseOutcome records single or repeated run results for flake detection.
type CaseOutcome struct {
	Name                   string   `json:"name"`
	Status                 string   `json:"status"` // canonical ResultStatus ("NANO_SIMULATION_PASS", "FAIL", "UNSUPPORTED", "INCONCLUSIVE", "BLOCKED_POLICY")
	Flaky                  bool     `json:"flaky"`
	Iterations             int      `json:"iterations"`
	PassedCount            int      `json:"passed_count"`
	FailedCount            int      `json:"failed_count"`
	RetriesUsed            int      `json:"retries_used,omitempty"`
	DurationMs             int64    `json:"duration_ms"`
	BackendUsed            string   `json:"backend_used"`
	EnvironmentDisclosure  string   `json:"environment_disclosure"`
	ReproducedBehaviors    []string `json:"reproduced_behaviors,omitempty"`
	NonReproducedBehaviors []string `json:"non_reproduced_behaviors,omitempty"`
	MockServerURL          string   `json:"mock_server_url,omitempty"`
	Error                  string   `json:"error,omitempty"`
}

// SuiteReport summarizes overall suite execution and flake telemetry.
type SuiteReport struct {
	SuiteName      string        `json:"suite_name"`
	OverallStatus  string        `json:"overall_status"`
	PrimaryBackend string        `json:"primary_backend"`
	Passed         int           `json:"passed"`
	Failed         int           `json:"failed"`
	Flaky          int           `json:"flaky"`
	DurationMs     int64         `json:"duration_ms"`
	Outcomes       []CaseOutcome `json:"outcomes"`
}

type junitTestSuites struct {
	XMLName  xml.Name       `xml:"testsuites"`
	Tests    int            `xml:"tests,attr"`
	Failures int            `xml:"failures,attr"`
	Time     string         `xml:"time,attr"`
	Suite    junitTestSuite `xml:"testsuite"`
}

type junitTestSuite struct {
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Time     string          `xml:"time,attr"`
	Cases    []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"`
}

// ExportJUnitXML converts a SuiteReport into standard JUnit XML for CI pipelines.
func (r SuiteReport) ExportJUnitXML() ([]byte, error) {
	cases := make([]junitTestCase, 0, len(r.Outcomes))
	failures := 0
	for _, o := range r.Outcomes {
		tc := junitTestCase{
			Name:      o.Name,
			Classname: r.PrimaryBackend + "." + r.SuiteName,
			Time:      fmt.Sprintf("%.3f", float64(o.DurationMs)/1000.0),
		}
		if o.Status != "NANO_SIMULATION_PASS" && o.Status != "ROBOLECTRIC_PASS" && o.Status != "ART_EMULATOR_PASS" && o.Status != "PHYSICAL_DEVICE_PASS" {
			failures++
			tc.Failure = &junitFailure{
				Message: o.Status + ": " + o.Error,
				Body:    o.EnvironmentDisclosure,
			}
		}
		cases = append(cases, tc)
	}
	doc := junitTestSuites{
		Tests:    len(r.Outcomes),
		Failures: failures,
		Time:     fmt.Sprintf("%.3f", float64(r.DurationMs)/1000.0),
		Suite: junitTestSuite{
			Name:     r.SuiteName,
			Tests:    len(r.Outcomes),
			Failures: failures,
			Time:     fmt.Sprintf("%.3f", float64(r.DurationMs)/1000.0),
			Cases:    cases,
		},
	}
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
