package intelhooks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"nano_android/engine/apk/ingest"
	"nano_android/engine/intelligence/ai"
	"nano_android/engine/intelligence/graph"
	"nano_android/engine/mcp_compiler"
	"nano_android/engine/security/audit"
	"nano_android/engine/security/pathcontainment"
	"nano_android/engine/security/policy"
	"nano_android/engine/testing/differential"
)

// Attach wires Phase 5 Behavior Graph & AI Autonomous Engineer MCP tools onto reg cleanly.
func Attach(reg *mcp_compiler.ToolRegistry, workspaceRoot string) {
	if reg == nil {
		return
	}
	if strings.TrimSpace(workspaceRoot) == "" {
		workspaceRoot, _ = os.Getwd()
	}
	allowed := []string{
		filepath.Join(workspaceRoot, "cache"),
		filepath.Join(workspaceRoot, "tools"),
		filepath.Join(workspaceRoot, "fixtures"),
		filepath.Join(workspaceRoot, "downloads"),
	}
	intelDir := filepath.Join(workspaceRoot, "cache", "intelligence")
	graphFile := filepath.Join(intelDir, "behavior_graph.json")
	propFile := filepath.Join(intelDir, "proposals.json")
	memFile := filepath.Join(intelDir, "memory.json")
	approvedRoot := filepath.Join(workspaceRoot, "cache", "ai_approved")

	ledger, _ := audit.OpenLedger(filepath.Join(workspaceRoot, "cache", "audit", "nano_ai_mcp_audit.jsonl"))
	bundleJar := filepath.Join(workspaceRoot, "tools", "libs", "bundletool-all-1.18.1.jar")
	guard := policy.NewGuard(policy.DefaultConfig())

	analyzeArtifact := func(raw string) (string, *ingest.IntelligenceReport, error) {
		p, err := pathcontainment.ResolveExistingContained(strings.TrimSpace(raw), allowed, []string{".apk", ".aab", ".apks", ".xapk", ".jar"})
		if err != nil {
			return "", nil, fmt.Errorf("BLOCKED_POLICY artifact containment error: %w", err)
		}
		eng := ingest.NewEngine(filepath.Join(workspaceRoot, "cache", "apk_intel"), bundleJar)
		rep, err := eng.Analyze(p)
		if err != nil {
			return "", nil, err
		}
		return p, rep, nil
	}

	reg.SetPhase5Hooks(mcp_compiler.Phase5Hooks{
		BuildGraph: func(ctx context.Context, artifactPath, sourceRoot string) (any, error) {
			_, rep, err := analyzeArtifact(artifactPath)
			if err != nil {
				return nil, err
			}
			safeSrc := ""
			if sourceRoot != "" {
				safeSrc = workspaceRoot
			}
			g := graph.BuildFromArtifact(rep, safeSrc)
			_ = g.Save(graphFile)
			return map[string]any{
				"graph_file":   graphFile,
				"architecture": g.ExploreArchitecture(),
			}, nil
		},
		AnalyzeImpact: func(ctx context.Context, artifactPath string, changed []string) (any, error) {
			var g *graph.BehaviorGraph
			if artifactPath != "" {
				if _, rep, err := analyzeArtifact(artifactPath); err == nil {
					g = graph.BuildFromArtifact(rep, "")
				}
			}
			if g == nil {
				loaded, err := graph.Load(graphFile)
				if err != nil {
					return nil, fmt.Errorf("no graph loaded or artifact specified: %w", err)
				}
				g = loaded
			}
			return g.AnalyzeChangeImpact(changed), nil
		},
		Understand: func(ctx context.Context, artifactPath string, maxSteps int) (any, error) {
			_, rep, err := analyzeArtifact(artifactPath)
			if err != nil {
				return nil, err
			}
			g := graph.BuildFromArtifact(rep, "")
			pkg := "unknown.app"
			if rep.Manifest != nil && rep.Manifest.Package != "" {
				pkg = rep.Manifest.Package
			}
			gov := ai.NewGovernor("", "mcp_ai", pkg, ai.ExecutionBudget{MaxSteps: maxSteps}, guard, ledger)
			return ai.UnderstandAPK(rep, g, gov)
		},
		Explore: func(ctx context.Context, artifactPath, activity string, maxSteps, maxScreens int) (any, error) {
			p, rep, err := analyzeArtifact(artifactPath)
			if err != nil {
				return nil, err
			}
			ic, ok := reg.GetCoordinator().(*mcp_compiler.IntegratedCoordinator)
			if !ok {
				return nil, fmt.Errorf("INFRASTRUCTURE_ERROR: live IntegratedCoordinator required for autonomous exploration")
			}
			g, _ := graph.Load(graphFile)
			if g == nil {
				g = graph.BuildFromArtifact(rep, "")
			}
			pkg := ""
			if rep.Manifest != nil {
				pkg = rep.Manifest.Package
			}
			gov := ai.NewGovernor("", "mcp_ai_explorer", pkg, ai.ExecutionBudget{MaxSteps: maxSteps, MaxScreens: maxScreens}, guard, ledger)
			store, err := ai.OpenProposalStore(propFile, ledger)
			if err != nil {
				return nil, err
			}
			res, err := ai.ExploreAndHunt(ctx, ic, p, activity, rep, g, gov, store)
			_ = g.Save(graphFile)
			return res, err
		},
		GenerateTests: func(ctx context.Context, artifactPath string, maxSteps int) (any, error) {
			_, rep, err := analyzeArtifact(artifactPath)
			if err != nil {
				return nil, err
			}
			g, _ := graph.Load(graphFile)
			if g == nil {
				g = graph.BuildFromArtifact(rep, "")
			}
			pkg := ""
			if rep.Manifest != nil {
				pkg = rep.Manifest.Package
			}
			gov := ai.NewGovernor("", "mcp_ai_tests", pkg, ai.ExecutionBudget{MaxSteps: maxSteps}, guard, ledger)
			store, err := ai.OpenProposalStore(propFile, ledger)
			if err != nil {
				return nil, err
			}
			var liveAST *mcp_compiler.ASTNode
			if obs, oErr := reg.GetCoordinator().ObserveScreen(ctx, false, "none", 0); oErr == nil {
				liveAST = obs.AST
			}
			props, err := ai.GenerateTests(rep, g, liveAST, gov, store)
			if err != nil {
				return nil, err
			}
			return map[string]any{"proposals": props, "budget": gov.Status()}, nil
		},
		Diagnose: func(ctx context.Context, trace, mappingPath, suspectedAPI string) (any, error) {
			mapTxt := ""
			if mappingPath != "" {
				cm, err := pathcontainment.ResolveExistingContained(mappingPath, allowed, []string{".txt", ".map"})
				if err != nil {
					return nil, fmt.Errorf("BLOCKED_POLICY mapping_path: %w", err)
				}
				if raw, rErr := os.ReadFile(cm); rErr == nil {
					mapTxt = string(raw)
				}
			}
			g, _ := graph.Load(graphFile)
			var diff *differential.DivergenceFinding
			if suspectedAPI != "" {
				diff = &differential.DivergenceFinding{
					FirstStepIndex: 1,
					StepLabel:      "reported_step",
					Dimension:      differential.DimUIHierarchy,
					SuspectedAPI:   suspectedAPI,
					Summary:        "Suspected compatibility difference on " + suspectedAPI,
				}
			}
			gov := ai.NewGovernor("", "mcp_ai_diagnose", "", ai.DefaultBudget(), guard, ledger)
			store, _ := ai.OpenProposalStore(propFile, ledger)
			return ai.DiagnoseFailure(trace, mapTxt, diff, g, gov, store)
		},
		Approve: func(ctx context.Context, proposalID, approvedBy string, apply bool) (any, error) {
			store, err := ai.OpenProposalStore(propFile, ledger)
			if err != nil {
				return nil, err
			}
			return store.Approve(proposalID, approvedBy, apply, approvedRoot)
		},
		QueryGraph: func(ctx context.Context, kind, match string, exportedOnly bool, limit int) (any, error) {
			g, err := graph.Load(graphFile)
			if err != nil {
				return nil, fmt.Errorf("load behavior graph: %w", err)
			}
			return g.Query(graph.QueryFilter{
				Kind:         graph.NodeKind(kind),
				NameContains: match,
				ExportedOnly: exportedOnly,
				Limit:        limit,
			}), nil
		},
		SelfHeal: func(ctx context.Context, resourceID, text, contentDesc, className string) (any, error) {
			obs, err := reg.GetCoordinator().ObserveScreen(ctx, false, "none", 0)
			if err != nil || obs.AST == nil {
				return nil, fmt.Errorf("live AST unavailable: %v", err)
			}
			gov := ai.NewGovernor("", "mcp_ai_heal", "", ai.DefaultBudget(), guard, ledger)
			store, _ := ai.OpenProposalStore(propFile, ledger)
			cands, prop, hErr := ai.ProposeSelfHealingSelectors(ai.BrokenSelector{
				ResourceID:         resourceID,
				Text:               text,
				ContentDescription: contentDesc,
				ClassName:          className,
			}, obs.AST, gov, store)
			if hErr != nil {
				return nil, hErr
			}
			return map[string]any{"candidates": cands, "proposal": prop, "budget": gov.Status()}, nil
		},
		Coordinate: func(ctx context.Context, artifactPath string, maxSteps, maxScreens int) (any, error) {
			p, rep, err := analyzeArtifact(artifactPath)
			if err != nil {
				return nil, err
			}
			ic, _ := reg.GetCoordinator().(*mcp_compiler.IntegratedCoordinator)
			pkg := ""
			if rep.Manifest != nil {
				pkg = rep.Manifest.Package
			}
			gov := ai.NewGovernor("", "mcp_ai_coordinator", pkg, ai.ExecutionBudget{MaxSteps: maxSteps, MaxScreens: maxScreens}, guard, ledger)
			store, _ := ai.OpenProposalStore(propFile, ledger)
			mem, _ := ai.OpenMemoryStore(memFile)
			return ai.MultiAgentCoordinate(ctx, ic, p, rep, "", gov, store, mem)
		},
		Memory: func(ctx context.Context, op, key, category, value, actor string) (any, error) {
			ms, err := ai.OpenMemoryStore(memFile)
			if err != nil {
				return nil, err
			}
			if strings.EqualFold(op, "put") {
				if err := ms.Put(key, category, value, actor); err != nil {
					return nil, err
				}
			}
			return ms, nil
		},
	})
}
