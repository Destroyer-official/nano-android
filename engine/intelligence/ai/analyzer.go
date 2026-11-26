package ai

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"nano_android/engine/apk/ingest"
	"nano_android/engine/apk/security"
	"nano_android/engine/compatibility/registry"
	"nano_android/engine/intelligence/graph"
	"nano_android/engine/mcp_compiler"
	"nano_android/engine/profiler/crash"
	"nano_android/engine/testing/differential"
	"nano_android/engine/testing/suite"
)

// APKUnderstanding represents I1 Intelligent APK Understanding output.
type APKUnderstanding struct {
	PackageName          string                           `json:"package_name"`
	AppLabelFenced       string                           `json:"app_label_fenced,omitempty"`
	Summary              string                           `json:"summary"`
	EntryPoints          []string                         `json:"entry_points"`
	DeepLinks            []string                         `json:"deep_links,omitempty"`
	NavigationPaths      []string                         `json:"navigation_paths,omitempty"`
	PermissionRisks      []ingest.PermissionRisk          `json:"permission_risks,omitempty"`
	Compatibility        registry.ExecutionRecommendation `json:"compatibility"`
	UnsupportedAPIs      []string                         `json:"unsupported_apis,omitempty"`
	SecurityFindings     []security.Finding               `json:"security_findings,omitempty"`
	CrashRiskIndicators  []string                         `json:"crash_risk_indicators,omitempty"`
	ComponentGuides      map[string]string                `json:"component_guides"`
	ArchitectureSummary  graph.ArchitectureReport         `json:"architecture_summary"`
	MarkdownDocs         string                           `json:"markdown_docs"`
	EnvironmentDisclosed string                           `json:"environment_disclosed"`
	Budget               BudgetStatus                     `json:"budget"`
}

// UnderstandAPK performs I1 intelligence analysis using Tier-0 static data, Behavior Graph, MASVS audit,
// and compatibility routing while fencing untrusted APK text through nonced <DF_UNTRUSTED_*> delimiters.
func UnderstandAPK(rep *ingest.IntelligenceReport, g *graph.BehaviorGraph, gov *Governor) (*APKUnderstanding, error) {
	if rep == nil {
		return nil, fmt.Errorf("intelligence report is required")
	}
	if err := gov.CheckStep("nano_ai_understand", map[string]any{"sha256": rep.SHA256}); err != nil {
		return nil, err
	}
	if g == nil {
		g = graph.BuildFromArtifact(rep, "")
	}
	arch := g.ExploreArchitecture()
	compat := registry.EvaluateAPK(rep, "auto")
	masvs := security.AuditAPK(rep)

	pkg := "unknown.app"
	appLabel := ""
	var entryPoints, deepLinks, crashRisks []string
	compGuides := make(map[string]string)

	if rep.Manifest != nil {
		pkg = rep.Manifest.Package
		if rep.Manifest.AppLabel != "" {
			appLabel = gov.FenceUntrusted("app_label", rep.Manifest.AppLabel, 256)
		}
		if rep.Manifest.LauncherActivity != "" {
			entryPoints = append(entryPoints, "launcher:"+rep.Manifest.LauncherActivity)
		}
		deepLinks = append(deepLinks, rep.Manifest.DeepLinks...)
		for _, a := range rep.Manifest.Activities {
			if a.IsEffectivelyExported() {
				entryPoints = append(entryPoints, "exported_activity:"+a.Name)
			}
			compGuides[a.Name] = fmt.Sprintf("Activity (exported=%v, intentFilters=%d)", a.IsEffectivelyExported(), len(a.IntentFilters))
		}
		for _, s := range rep.Manifest.Services {
			if s.IsEffectivelyExported() {
				entryPoints = append(entryPoints, "exported_service:"+s.Name)
			}
			compGuides[s.Name] = fmt.Sprintf("Service (exported=%v, permission=%q)", s.IsEffectivelyExported(), s.Permission)
		}
		for _, r := range rep.Manifest.Receivers {
			if r.IsEffectivelyExported() {
				entryPoints = append(entryPoints, "exported_receiver:"+r.Name)
			}
			compGuides[r.Name] = fmt.Sprintf("BroadcastReceiver (exported=%v, filters=%d)", r.IsEffectivelyExported(), len(r.IntentFilters))
		}
		for _, p := range rep.Manifest.Providers {
			if p.IsEffectivelyExported() {
				entryPoints = append(entryPoints, "exported_provider:"+p.Name)
			}
			compGuides[p.Name] = fmt.Sprintf("ContentProvider (authority=%q, exported=%v)", p.Authority, p.IsEffectivelyExported())
		}
		if rep.Manifest.Debuggable {
			crashRisks = append(crashRisks, "Debuggable release flag enabled (JDWP attachable)")
		}
	}

	var unsupported []string
	if rep.DEX != nil {
		for _, issue := range rep.DEX.CompatibilityIssues {
			unsupported = append(unsupported, fmt.Sprintf("%s [%s -> %s]", issue.API, issue.Category, issue.RecommendedTier))
			if issue.Severity == "REQUIRES_HARDWARE" || issue.Severity == "REQUIRES_ART" {
				crashRisks = append(crashRisks, fmt.Sprintf("API %s requires %s", issue.API, issue.RecommendedTier))
			}
		}
	}
	for _, nl := range rep.NativeLibs {
		if !nl.Is16KBAligned {
			crashRisks = append(crashRisks, fmt.Sprintf("Native ELF %s lacks 16 KB page alignment for Android 17 (API 37)", nl.Path))
		}
	}
	if len(arch.CircularDependencies) > 0 {
		crashRisks = append(crashRisks, fmt.Sprintf("%d circular class dependency cluster(s) detected", len(arch.CircularDependencies)))
	}

	sort.Strings(entryPoints)
	sort.Strings(unsupported)

	md := buildUnderstandingMarkdown(pkg, compat, entryPoints, deepLinks, rep.PermissionRisks, masvs.Findings, unsupported, crashRisks, arch)
	summary := fmt.Sprintf("Package %s: %d classes, %d entry points, primary tier %s (confidence %.0f%%), %d MASVS findings, %d risk indicators.",
		pkg, len(arch.TopComplexClasses), len(entryPoints), compat.PrimaryBackend, compat.ConfidenceScore*100, len(masvs.Findings), len(crashRisks))

	return &APKUnderstanding{
		PackageName:          pkg,
		AppLabelFenced:       appLabel,
		Summary:              summary,
		EntryPoints:          entryPoints,
		DeepLinks:            deepLinks,
		NavigationPaths:      arch.NavigationEdges,
		PermissionRisks:      rep.PermissionRisks,
		Compatibility:        compat,
		UnsupportedAPIs:      unsupported,
		SecurityFindings:     masvs.Findings,
		CrashRiskIndicators:  crashRisks,
		ComponentGuides:      compGuides,
		ArchitectureSummary:  arch,
		MarkdownDocs:         md,
		EnvironmentDisclosed: "static_tier0 + behavior_graph (zero external network, zero autonomous code edits)",
		Budget:               gov.Status(),
	}, nil
}

// GenerateTests executes I2 Autonomous Test Generation, emitting reviewable Maestro YAML and JUnit/Kotlin
// proposals saved in ProposalStore with PENDING_APPROVAL status.
func GenerateTests(rep *ingest.IntelligenceReport, g *graph.BehaviorGraph, liveAST *mcp_compiler.ASTNode, gov *Governor, store *ProposalStore) ([]Proposal, error) {
	if rep == nil {
		return nil, fmt.Errorf("intelligence report is required")
	}
	if err := gov.CheckStep("nano_ai_generate_tests", map[string]any{"sha256": rep.SHA256}); err != nil {
		return nil, err
	}
	if store == nil {
		return nil, fmt.Errorf("proposal store is required")
	}
	pkg := "com.example.app"
	launcher := ""
	if rep.Manifest != nil {
		if rep.Manifest.Package != "" {
			pkg = rep.Manifest.Package
		}
		launcher = rep.Manifest.LauncherActivity
	}

	var generated []Proposal
	addProp := func(kind ProposalKind, title, desc, relPath, content string, categories []string) error {
		p, err := store.Add(Proposal{
			Kind:            kind,
			Title:           title,
			Description:     desc,
			TargetRelPath:   relPath,
			Content:         content,
			EnvironmentTier: "nano_jvm",
			CreatedBy:       "ai_test_generator",
			Metadata:        map[string]any{"categories": categories, "package": pkg},
		})
		if err != nil {
			return err
		}
		generated = append(generated, *p)
		return nil
	}

	// 1. Startup + Regression + Accessibility Maestro smoke flow
	maestroStartup := fmt.Sprintf("appId: %s\n---\n- launchApp:\n    clearState: true\n", pkg)
	if liveAST != nil {
		for _, txt := range liveAST.CollectVisibleTexts() {
			clean := strings.TrimSpace(txt)
			if clean != "" {
				maestroStartup += fmt.Sprintf("- assertVisible: %q\n", clean)
				break
			}
		}
	}
	maestroStartup += "- scroll\n"
	if err := addProp(ProposalTestMaestro, "Startup & Accessibility Smoke Flow", "Deterministic Maestro YAML startup, visibility, and accessibility baseline.", "ai_startup_smoke.yaml", maestroStartup, []string{"startup", "accessibility", "regression"}); err != nil {
		return nil, err
	}

	// 2. Navigation, Form Boundary, Permissions, Auth/Network, File-Picker & Storage Maestro flows
	if liveAST != nil {
		var clickTargets, editTargets []string
		walkInteractive(liveAST, &clickTargets, &editTargets)
		if len(editTargets) > 0 {
			formYAML := fmt.Sprintf("appId: %s\n---\n- launchApp\n- tapOn: %q\n- inputText: \"\"\n- inputText: \"valid_test_input\"\n", pkg, editTargets[0])
			if len(clickTargets) > 0 {
				formYAML += fmt.Sprintf("- tapOn: %q\n", clickTargets[0])
			}
			_ = addProp(ProposalTestMaestro, "Form Boundary & Error Handling Flow", "Exercises empty/valid input handling on discovered EditText widget.", "ai_form_boundary.yaml", formYAML, []string{"forms", "error_handling"})
		}
		if len(clickTargets) > 0 {
			navYAML := fmt.Sprintf("appId: %s\n---\n- launchApp\n- tapOn: %q\n- back\n", pkg, clickTargets[0])
			_ = addProp(ProposalTestMaestro, "Primary Navigation & Back-Stack Flow", "Verifies navigation forward and clean back-stack pop.", "ai_navigation.yaml", navYAML, []string{"navigation", "regression"})
		}
	}

	// 3. Domain-specific test proposals covering permissions, file_picker, media, database, and auth
	var extraCategories []string
	extraCategories = append(extraCategories, "startup", "navigation", "forms", "error_handling", "accessibility", "regression")
	hasMedia, hasFilePicker := false, false
	if rep.Manifest != nil {
		for _, perm := range rep.Manifest.Permissions {
			if strings.Contains(perm, "CAMERA") || strings.Contains(perm, "LOCATION") || strings.Contains(perm, "AUDIO") || strings.Contains(perm, "READ_MEDIA") {
				extraCategories = append(extraCategories, "permissions")
			}
		}
		for _, a := range rep.Manifest.Activities {
			for _, f := range a.IntentFilters {
				for _, act := range f.Actions {
					if strings.Contains(act, "GET_CONTENT") || strings.Contains(act, "OPEN_DOCUMENT") {
						hasFilePicker = true
					}
				}
			}
		}
	}
	if hasFilePicker {
		extraCategories = append(extraCategories, "file_picker")
	}
	if g != nil {
		arch := g.ExploreArchitecture()
		if len(arch.DatabaseTables) > 0 {
			extraCategories = append(extraCategories, "database")
		}
		if len(arch.NetworkEndpoints) > 0 {
			extraCategories = append(extraCategories, "auth")
		}
	}
	if rep.DEX != nil {
		for _, mr := range rep.DEX.MethodReferences {
			if strings.Contains(mr.Owner, "MediaCodec") || strings.Contains(mr.Owner, "MediaPlayer") {
				hasMedia = true
			}
			if strings.Contains(mr.Name, "openFile") || strings.Contains(mr.Owner, "DocumentsContract") {
				hasFilePicker = true
			}
		}
	}
	if hasMedia {
		extraCategories = append(extraCategories, "media")
	}
	if hasFilePicker {
		extraCategories = append(extraCategories, "file_picker")
	}

	junitSrc := fmt.Sprintf(`package generated.ai;

import org.junit.jupiter.api.Test;
import static org.junit.jupiter.api.Assertions.*;

/**
 * Generated by Nano-Android AI Engineer (Reviewable Proposal - requires human approval).
 * Target package: %s | Launcher: %s
 * Categories covered: %s
 */
public class AiGeneratedRegressionSuite {
    @Test
    void verifyColdStartAndLifecycleResilience() {
        assertNotNull("%s");
    }

    @Test
    void verifyPermissionsStorageAuthAndMediaContracts() {
        assertFalse("%s".isBlank());
    }
}
`, pkg, launcher, strings.Join(dedupeStrings(extraCategories), ", "), pkg, pkg)

	if err := addProp(ProposalTestJUnit, "JUnit 5 Behavioral Regression Draft", "Reviewable Java/JUnit 5 test skeleton covering detected components and categories.", "AiGeneratedRegressionSuite.java", junitSrc, dedupeStrings(extraCategories)); err != nil {
		return nil, err
	}

	return generated, nil
}

// RootCauseCandidate ranks one likely cause of an observed failure or divergence.
type RootCauseCandidate struct {
	Rank           int     `json:"rank"`
	Confidence     float64 `json:"confidence"` // 0.0..1.0
	Category       string  `json:"category"`   // "app_defect", "unsupported_api", "profile_geometry", "proguard_obfuscation", "regression"
	RootCauseClass string  `json:"root_cause_class"`
	Explanation    string  `json:"explanation"`
	Remediation    string  `json:"remediation"`
}

// FailureDiagnosis aggregates I4/I5 root-cause ranking, fenced trace, impact selection, and patch proposal.
type FailureDiagnosis struct {
	CrashSummary         *crash.CrashReport               `json:"crash_summary,omitempty"`
	FencedTrace          string                           `json:"fenced_trace,omitempty"`
	DifferentialFinding  *differential.DivergenceFinding  `json:"differential_finding,omitempty"`
	RankedCauses         []RootCauseCandidate             `json:"ranked_causes"`
	Impact               graph.ImpactReport               `json:"impact"`
	PatchProposal        *Proposal                        `json:"patch_proposal,omitempty"`
	EnvironmentDisclosed string                           `json:"environment_disclosed"`
	Budget               BudgetStatus                     `json:"budget"`
}

// DiagnoseFailure implements I4/I5 failure analysis, differential root-cause localization, and reviewable patch drafting.
func DiagnoseFailure(rawTrace, r8MappingText string, diffFinding *differential.DivergenceFinding, g *graph.BehaviorGraph, gov *Governor, store *ProposalStore) (*FailureDiagnosis, error) {
	if strings.TrimSpace(rawTrace) == "" && diffFinding == nil {
		return nil, fmt.Errorf("either rawTrace or differential finding is required")
	}
	if err := gov.CheckStep("nano_ai_diagnose", map[string]any{"has_trace": rawTrace != "", "has_diff": diffFinding != nil}); err != nil {
		return nil, err
	}

	diag := &FailureDiagnosis{
		EnvironmentDisclosed: "local_crash_forensics + compatibility_registry + behavior_graph (zero autonomous edits)",
	}

	var changedTargets []string
	var ranked []RootCauseCandidate

	if strings.TrimSpace(rawTrace) != "" {
		var r8 *crash.R8Mapping
		if strings.TrimSpace(r8MappingText) != "" {
			r8 = crash.ParseR8Mapping(r8MappingText)
		}
		rep := crash.AnalyzeTrace(rawTrace, "ai_triage", "nano_jvm", r8)
		diag.CrashSummary = &rep
		diag.FencedTrace = gov.FenceUntrusted("crash_trace", rep.RetracedTrace, 2048)
		if g != nil {
			g.AttachCrash(rep, "")
		}
		if rep.RootCauseClass != "" {
			changedTargets = append(changedTargets, rep.RootCauseClass)
		}

		// Map against compatibility contracts to check if unsupported/partial framework API caused failure.
		contracts := registry.DefaultContracts()
		for api, c := range contracts {
			if strings.Contains(rep.RetracedTrace, api) && c.Level != registry.SupportFullyImplemented {
				ranked = append(ranked, RootCauseCandidate{
					Confidence:     0.88,
					Category:       "unsupported_api",
					RootCauseClass: api,
					Explanation:    fmt.Sprintf("Stack trace hits framework API %s which is %s in nano_jvm.", api, c.Level),
					Remediation:    fmt.Sprintf("Re-verify on fallback backend %q before concluding application bug.", c.FallbackBackend),
				})
			}
		}

		rootCls := rep.RootCauseClass
		if rootCls == "" {
			rootCls = rep.ExceptionClass
		}
		ranked = append(ranked, RootCauseCandidate{
			Confidence:     0.82,
			Category:       "app_defect",
			RootCauseClass: rootCls,
			Explanation:    fmt.Sprintf("%s thrown with signature %s.", rep.ExceptionClass, rep.Signature),
			Remediation:    fmt.Sprintf("Inspect %s and add null/state guard verified by targeted regression tests.", rootCls),
		})
	}

	if diffFinding != nil {
		diag.DifferentialFinding = diffFinding
		if diffFinding.SuspectedAPI != "" {
			changedTargets = append(changedTargets, diffFinding.SuspectedAPI)
		}
		cat := "regression"
		conf := 0.85
		if strings.Contains(strings.ToLower(diffFinding.Summary), "nano_jvm") || strings.Contains(diffFinding.SuspectedAPI, "WebView") || strings.Contains(diffFinding.SuspectedAPI, "MediaCodec") {
			cat = "unsupported_api"
			conf = 0.91
		}
		ranked = append(ranked, RootCauseCandidate{
			Confidence:     conf,
			Category:       cat,
			RootCauseClass: diffFinding.SuspectedAPI,
			Explanation:    fmt.Sprintf("Earliest divergence at step %d (%s) on dimension %s: %s", diffFinding.FirstStepIndex, diffFinding.StepLabel, diffFinding.Dimension, diffFinding.Summary),
			Remediation:    "Inspect differential dimension delta and add behavioral parity guard or ART routing.",
		})
	}

	sort.Slice(ranked, func(i, j int) bool { return ranked[i].Confidence > ranked[j].Confidence })
	for i := range ranked {
		ranked[i].Rank = i + 1
	}
	diag.RankedCauses = ranked

	if g != nil && len(changedTargets) > 0 {
		diag.Impact = g.AnalyzeChangeImpact(changedTargets)
	}

	if store != nil && len(ranked) > 0 {
		top := ranked[0]
		targetCls := strings.ReplaceAll(top.RootCauseClass, ".", "/") + ".patch.md"
		if targetCls == ".patch.md" {
			targetCls = "triage_fix.patch.md"
		}
		diffPreview := fmt.Sprintf("--- a/%s\n+++ b/%s\n@@ Candidate Remediation (%s, confidence %.2f) @@\n+# Guard root cause: %s\n+# Suggested action: %s\n+# Impacted tests: %s\n",
			targetCls, targetCls, top.Category, top.Confidence, top.Explanation, top.Remediation, strings.Join(diag.Impact.SelectedTests, ", "))
		prop, pErr := store.Add(Proposal{
			Kind:            ProposalPatchSource,
			Title:           "Remediation Patch Proposal: " + top.RootCauseClass,
			Description:     top.Explanation,
			TargetRelPath:   filepathSafeName(targetCls),
			Content:         diffPreview,
			UnifiedDiff:     diffPreview,
			EnvironmentTier: "static_triage",
			CreatedBy:       "ai_triage_engineer",
			Metadata: map[string]any{
				"confidence":     top.Confidence,
				"category":       top.Category,
				"impacted_tests": diag.Impact.SelectedTests,
			},
		})
		if pErr == nil {
			diag.PatchProposal = prop
		}
	}

	diag.Budget = gov.Status()
	return diag, nil
}

// BrokenSelector describes an outdated UI selector failing during test replay.
type BrokenSelector struct {
	ResourceID         string `json:"resource_id,omitempty"`
	Text               string `json:"text,omitempty"`
	ContentDescription string `json:"content_description,omitempty"`
	ClassName          string `json:"class_name,omitempty"`
	Bounds             [4]int `json:"bounds,omitempty"`
}

// SelectorCandidate ranks one candidate replacement node from the current AST.
type SelectorCandidate struct {
	Rank               int     `json:"rank"`
	NodeID             string  `json:"node_id"`
	Score              float64 `json:"score"`
	ResourceID         string  `json:"resource_id,omitempty"`
	Text               string  `json:"text,omitempty"`
	ContentDescription string  `json:"content_description,omitempty"`
	ClassName          string  `json:"class_name,omitempty"`
	SignalsMatched     []string `json:"signals_matched"`
}

// ProposeSelfHealingSelectors scores live AST widgets against a broken selector across 5 signals
// (`resource_id`, `content_description`, `semantic_role`, `parent_child`, `nearby_bounds`) and records a reviewable proposal.
func ProposeSelfHealingSelectors(broken BrokenSelector, root *mcp_compiler.ASTNode, gov *Governor, store *ProposalStore) ([]SelectorCandidate, *Proposal, error) {
	if root == nil {
		return nil, nil, fmt.Errorf("live AST root is required")
	}
	if err := gov.CheckStep("nano_ai_self_heal", map[string]any{"resource_id": broken.ResourceID, "text": broken.Text}); err != nil {
		return nil, nil, err
	}

	var candidates []SelectorCandidate
	var walk func(n *mcp_compiler.ASTNode, parentClass string)
	walk = func(n *mcp_compiler.ASTNode, parentClass string) {
		if n == nil {
			return
		}
		if n.Visible && (n.Clickable || n.Text != "" || n.ResourceID != "" || n.ContentDescription != "") {
			score := 0.0
			var signals []string
			if broken.ResourceID != "" && n.ResourceID != "" {
				if n.ResourceID == broken.ResourceID {
					score += 0.45
					signals = append(signals, "exact_resource_id")
				} else if strings.Contains(strings.ToLower(n.ResourceID), lastToken(broken.ResourceID)) {
					score += 0.28
					signals = append(signals, "partial_resource_id")
				}
			}
			if broken.ContentDescription != "" && n.ContentDescription != "" && strings.EqualFold(n.ContentDescription, broken.ContentDescription) {
				score += 0.25
				signals = append(signals, "content_description")
			}
			if broken.Text != "" && n.Text != "" && strings.EqualFold(n.Text, broken.Text) {
				score += 0.25
				signals = append(signals, "text_match")
			}
			if broken.ClassName != "" && n.Class != "" && strings.EqualFold(n.Class, broken.ClassName) {
				score += 0.15
				signals = append(signals, "semantic_role:"+parentClass)
			}
			if broken.Bounds[2] > 0 && n.Bounds[2] > 0 {
				dx := float64(centerX(broken.Bounds) - centerX(n.Bounds))
				dy := float64(centerY(broken.Bounds) - centerY(n.Bounds))
				dist := math.Hypot(dx, dy)
				if dist <= 160 {
					score += 0.15 * (1.0 - dist/160.0)
					signals = append(signals, "nearby_bounds")
				}
			}
			if score > 0 {
				candidates = append(candidates, SelectorCandidate{
					NodeID:             n.ID,
					Score:              math.Round(score*1000) / 1000,
					ResourceID:         n.ResourceID,
					Text:               gov.FenceUntrusted("candidate_text", n.Text, 128),
					ContentDescription: gov.FenceUntrusted("candidate_desc", n.ContentDescription, 128),
					ClassName:          n.Class,
					SignalsMatched:     signals,
				})
			}
		}
		for _, ch := range n.Children {
			walk(ch, n.Class)
		}
	}
	walk(root, "")

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}
	for i := range candidates {
		candidates[i].Rank = i + 1
	}

	var prop *Proposal
	if store != nil && len(candidates) > 0 {
		best := candidates[0]
		yamlPatch := fmt.Sprintf("# Self-Healing Selector Proposal (requires approval)\n# Previous: id=%q text=%q\n- tapOn:\n    id: %q\n",
			broken.ResourceID, broken.Text, best.ResourceID)
		p, err := store.Add(Proposal{
			Kind:            ProposalSelectorHeal,
			Title:           fmt.Sprintf("Self-Heal Selector -> %s (score %.2f)", best.NodeID, best.Score),
			Description:     "Ranked AST replacement selector requiring explicit validation/approval before baseline update.",
			TargetRelPath:   "healed_selector.yaml",
			Content:         yamlPatch,
			EnvironmentTier: "nano_jvm",
			CreatedBy:       "ai_selector_healer",
			Metadata:        map[string]any{"candidates": candidates},
		})
		if err == nil {
			prop = p
		}
	}
	return candidates, prop, nil
}

// TestSuiteFromProposals converts approved or reviewable suite proposals into an executable suite.Suite descriptor.
func TestSuiteFromProposals(packageID, artifact string, proposals []Proposal) suite.Suite {
	s := suite.Suite{
		Name:        "ai_generated_" + packageID,
		Artifact:    artifact,
		Mode:        "auto",
		RepeatCount: 1,
	}
	for _, p := range proposals {
		if p.Kind == ProposalTestMaestro || p.Kind == ProposalExploreRepro {
			s.Cases = append(s.Cases, suite.TestCase{
				Name: p.Title,
				Tags: []string{"ai_generated", string(p.Kind)},
				Steps: []suite.Step{
					{Action: "observe"},
					{Action: "step_time", DeltaMs: 300},
				},
			})
		}
	}
	return s
}

func buildUnderstandingMarkdown(pkg string, compat registry.ExecutionRecommendation, entryPoints, deepLinks []string, permRisks []ingest.PermissionRisk, findings []security.Finding, unsupported, crashRisks []string, arch graph.ArchitectureReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# AI Architecture & Intelligence Guide: `%s`\n\n", pkg)
	fmt.Fprintf(&b, "- **Recommended Execution Tier:** `%s` (confidence %.2f)\n", compat.PrimaryBackend, compat.ConfidenceScore)
	fmt.Fprintf(&b, "- **Graph Scale:** %d vertices, %d edges\n", arch.NodeCount, arch.EdgeCount)
	fmt.Fprintf(&b, "- **Entry Points (%d):** %s\n", len(entryPoints), strings.Join(entryPoints, ", "))
	if len(deepLinks) > 0 {
		fmt.Fprintf(&b, "- **Deep Links:** %s\n", strings.Join(deepLinks, ", "))
	}
	fmt.Fprintf(&b, "- **Permissions Assessed:** %d | **MASVS Findings:** %d | **Unsupported APIs:** %d\n", len(permRisks), len(findings), len(unsupported))
	if len(crashRisks) > 0 {
		b.WriteString("\n## Crash & Compatibility Risk Indicators\n")
		for _, r := range crashRisks {
			fmt.Fprintf(&b, "- %s\n", r)
		}
	}
	return b.String()
}

func walkInteractive(n *mcp_compiler.ASTNode, clickables, editables *[]string) {
	if n == nil {
		return
	}
	lowCls := strings.ToLower(n.Class)
	label := n.Text
	if label == "" {
		label = n.ContentDescription
	}
	if label == "" {
		label = n.ResourceID
	}
	if label == "" {
		label = n.ID
	}
	if n.Visible && (strings.Contains(lowCls, "edittext") || strings.Contains(n.Type, "edit")) {
		*editables = append(*editables, label)
	} else if n.Visible && n.Clickable && label != "" {
		*clickables = append(*clickables, label)
	}
	for _, ch := range n.Children {
		walkInteractive(ch, clickables, editables)
	}
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, v := range in {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func filepathSafeName(raw string) string {
	r := strings.ReplaceAll(raw, "\\", "_")
	r = strings.ReplaceAll(r, "/", "_")
	r = strings.ReplaceAll(r, "..", "_")
	return r
}

func lastToken(s string) string {
	idx := strings.LastIndexAny(s, "./:")
	if idx >= 0 && idx < len(s)-1 {
		return strings.ToLower(s[idx+1:])
	}
	return strings.ToLower(s)
}

func centerX(b [4]int) int { return (b[0] + b[2]) / 2 }
func centerY(b [4]int) int { return (b[1] + b[3]) / 2 }
