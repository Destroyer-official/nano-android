package registry

import (
	"fmt"
	"sort"
	"strings"

	"nano_android/engine/apk/dex"
	"nano_android/engine/apk/ingest"
)

// ResultStatus is the mandatory explicit classification taxonomy (never bare PASS/FAIL).
type ResultStatus string

const (
	StatusNanoSimulationPass ResultStatus = "NANO_SIMULATION_PASS"
	StatusRobolectricPass    ResultStatus = "ROBOLECTRIC_PASS"
	StatusArtEmulatorPass    ResultStatus = "ART_EMULATOR_PASS"
	StatusPhysicalDevicePass ResultStatus = "PHYSICAL_DEVICE_PASS"
	StatusFail               ResultStatus = "FAIL"
	StatusUnsupported        ResultStatus = "UNSUPPORTED"
	StatusInconclusive       ResultStatus = "INCONCLUSIVE"
	StatusBlockedPolicy      ResultStatus = "BLOCKED_POLICY"
	StatusInfrastructureErr  ResultStatus = "INFRASTRUCTURE_ERROR"
)

// Backend identifies an explicit Android execution tier.
type Backend string

const (
	BackendStatic         Backend = "static"
	BackendNanoJVM        Backend = "nano_jvm"
	BackendRobolectric    Backend = "robolectric"
	BackendAndroidART     Backend = "android_art"
	BackendPhysicalDevice Backend = "physical_device"
)

// SupportLevel classifies an Android framework API's fidelity in Nano JVM.
type SupportLevel string

const (
	SupportFullyImplemented SupportLevel = "FULLY_IMPLEMENTED"
	SupportBehavioralDiff   SupportLevel = "BEHAVIORAL_DIFF"
	SupportPartial          SupportLevel = "PARTIAL"
	SupportStubbed          SupportLevel = "STUBBED"
	SupportUnsupported      SupportLevel = "UNSUPPORTED"
	SupportRequiresART      SupportLevel = "REQUIRES_ART"
	SupportRequiresHardware SupportLevel = "REQUIRES_HARDWARE"
)

// APIContract describes behavioral coverage for one Android framework class/subsystem.
type APIContract struct {
	API             string       `json:"api"`
	Level           SupportLevel `json:"level"`
	SinceAPI        int          `json:"since_api"`
	UntilAPI        int          `json:"until_api"`
	FallbackBackend Backend      `json:"fallback_backend,omitempty"`
	Notes           string       `json:"notes"`
}

// ExecutionRecommendation summarizes backend routing & compatibility gaps for an APK.
type ExecutionRecommendation struct {
	PackageName      string        `json:"package_name"`
	PrimaryBackend   Backend       `json:"primary_backend"`
	FallbackBackend  Backend       `json:"fallback_backend,omitempty"`
	ConfidenceScore  float64       `json:"confidence_score"` // 0.0..1.0
	MatchedContracts []APIContract `json:"matched_contracts,omitempty"`
	Limitations      []string      `json:"limitations,omitempty"`
	Rationale        string        `json:"rationale"`
}

// DefaultContracts returns the canonical Nano-Android compatibility matrix.
func DefaultContracts() map[string]APIContract {
	list := []APIContract{
		{API: "android.app.Application", Level: SupportFullyImplemented, SinceAPI: 26, UntilAPI: 37, Notes: "Application init + lifecycle callbacks"},
		{API: "android.app.Activity", Level: SupportFullyImplemented, SinceAPI: 26, UntilAPI: 37, Notes: "Full lifecycle, back-stack, results, recreate, savedInstanceState"},
		{API: "android.app.Service", Level: SupportPartial, SinceAPI: 26, UntilAPI: 37, FallbackBackend: BackendAndroidART, Notes: "Local start/bind/foreground simulation"},
		{API: "android.content.Context", Level: SupportBehavioralDiff, SinceAPI: 26, UntilAPI: 37, Notes: "Canonical sandbox containment + stateful permissions"},
		{API: "android.content.ContentProvider", Level: SupportPartial, SinceAPI: 26, UntilAPI: 37, FallbackBackend: BackendAndroidART, Notes: "In-process authority registration & CRUD"},
		{API: "android.content.BroadcastReceiver", Level: SupportPartial, SinceAPI: 26, UntilAPI: 37, FallbackBackend: BackendAndroidART, Notes: "In-process exported/not-exported broadcast dispatch"},
		{API: "android.database.sqlite.SQLiteDatabase", Level: SupportFullyImplemented, SinceAPI: 26, UntilAPI: 37, Notes: "Real SQLite 3.54.1 via sqlite-jdbc with ACID rollback & savepoints"},
		{API: "android.os.Looper", Level: SupportFullyImplemented, SinceAPI: 26, UntilAPI: 37, Notes: "Deterministic virtual clock & MessageQueue"},
		{API: "android.view.View", Level: SupportPartial, SinceAPI: 26, UntilAPI: 37, FallbackBackend: BackendAndroidART, Notes: "2-pass measure/layout + semantic AST"},
		{API: "android.media.MediaCodec", Level: SupportRequiresHardware, SinceAPI: 26, UntilAPI: 37, FallbackBackend: BackendPhysicalDevice, Notes: "Hardware codec silicon requires physical device"},
		{API: "androidx.compose.ui", Level: SupportRequiresART, SinceAPI: 26, UntilAPI: 37, FallbackBackend: BackendRobolectric, Notes: "Requires Robolectric ComposeTestRule or Android ART"},
		{API: "android.webkit.WebView", Level: SupportRequiresART, SinceAPI: 26, UntilAPI: 37, FallbackBackend: BackendAndroidART, Notes: "Chromium WebView requires Android ART"},
	}
	out := make(map[string]APIContract, len(list))
	for _, c := range list {
		out[c.API] = c
	}
	return out
}

// EvaluateAPK inspects static APK intelligence and returns a deterministic routing recommendation.
func EvaluateAPK(rep *ingest.IntelligenceReport, requestedMode string) ExecutionRecommendation {
	rec := ExecutionRecommendation{
		PrimaryBackend:  BackendNanoJVM,
		ConfidenceScore: 0.90,
	}
	if rep == nil {
		rec.PrimaryBackend = BackendStatic
		rec.ConfidenceScore = 0.0
		rec.Rationale = "missing APK intelligence report"
		return rec
	}
	if rep.Manifest != nil {
		rec.PackageName = rep.Manifest.Package
	}

	// User-selected mode NEVER gets overridden silently.
	if requestedMode != "" && requestedMode != "auto" {
		rec.PrimaryBackend = Backend(requestedMode)
		rec.Rationale = fmt.Sprintf("user explicitly requested backend %q (auto-routing overridden)", requestedMode)
	}

	contracts := DefaultContracts()
	matchedMap := map[string]APIContract{}
	var reasons []string
	requiresHardware := false
	requiresART := false
	hasCompose := false

	if len(rep.NativeLibs) > 0 {
		requiresART = true
		rec.Limitations = append(rec.Limitations, fmt.Sprintf("%d native JNI .so libraries detected (non-JVM ABI)", len(rep.NativeLibs)))
		reasons = append(reasons, "native JNI .so libraries require ART/physical ABI")
	}

	if rep.DEX != nil {
		for _, ci := range rep.DEX.CompatibilityIssues {
			switch ci.Category {
			case "HARDWARE_REQUIRED":
				requiresHardware = true
				rec.Limitations = append(rec.Limitations, ci.API+" requires physical hardware")
				reasons = append(reasons, ci.API)
			case "COMPOSE":
				hasCompose = true
				requiresART = true
				rec.Limitations = append(rec.Limitations, "Jetpack Compose UI requires Robolectric or Android ART")
				reasons = append(reasons, "Jetpack Compose")
			case "ART_REQUIRED":
				requiresART = true
				rec.Limitations = append(rec.Limitations, ci.API+" requires Android ART")
				reasons = append(reasons, ci.API)
			}
		}
		for _, m := range rep.DEX.MethodReferences {
			for api, c := range contracts {
				if strings.HasPrefix(m.Owner, api) {
					matchedMap[api] = c
				}
			}
		}
		for _, cl := range rep.DEX.Classes {
			for api, c := range contracts {
				if strings.HasPrefix(cl.SuperClass, api) {
					matchedMap[api] = c
				}
			}
		}
	}

	keys := make([]string, 0, len(matchedMap))
	for k := range matchedMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		rec.MatchedContracts = append(rec.MatchedContracts, matchedMap[k])
	}

	if requestedMode == "" || requestedMode == "auto" {
		switch {
		case requiresHardware:
			rec.PrimaryBackend = BackendPhysicalDevice
			rec.FallbackBackend = BackendAndroidART
			rec.ConfidenceScore = 0.85
			rec.Rationale = "Selected physical_device due to hardware dependencies: " + strings.Join(reasons, ", ")
		case hasCompose && len(rep.NativeLibs) == 0:
			rec.PrimaryBackend = BackendRobolectric
			rec.FallbackBackend = BackendAndroidART
			rec.ConfidenceScore = 0.88
			rec.Rationale = "Selected robolectric/android_art for Jetpack Compose semantics"
		case requiresART:
			rec.PrimaryBackend = BackendAndroidART
			rec.FallbackBackend = BackendPhysicalDevice
			rec.ConfidenceScore = 0.86
			rec.Rationale = "Selected android_art due to: " + strings.Join(reasons, ", ")
		default:
			rec.PrimaryBackend = BackendNanoJVM
			rec.FallbackBackend = BackendAndroidART
			rec.ConfidenceScore = 0.92
			rec.Rationale = "Pure JVM/View/SQLite workload supported by Nano JVM"
		}
	}
	return rec
}

// ValidateStatusResult ensures a test result uses the explicit status taxonomy.
func ValidateStatusResult(status ResultStatus) error {
	switch status {
	case StatusNanoSimulationPass, StatusRobolectricPass, StatusArtEmulatorPass,
		StatusPhysicalDevicePass, StatusFail, StatusUnsupported,
		StatusInconclusive, StatusBlockedPolicy, StatusInfrastructureErr:
		return nil
	default:
		return fmt.Errorf("ambiguous or invalid test status %q (must use canonical Nano-Android taxonomy)", status)
	}
}

var _ = dex.Inventory{}
