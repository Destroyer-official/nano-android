package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"nano_android/engine/apk/ingest"
	"nano_android/engine/intelligence/ai"
	"nano_android/engine/intelligence/graph"
	"nano_android/engine/mcp_compiler"
	"nano_android/engine/testing/differential"
)

func (s *Service) mountPhase5Routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/graph/build", s.handleGraphBuild)
	mux.HandleFunc("/api/v1/graph/query", s.handleGraphQuery)
	mux.HandleFunc("/api/v1/graph/impact", s.handleGraphImpact)
	mux.HandleFunc("/api/v1/ai/understand", s.handleAIUnderstand)
	mux.HandleFunc("/api/v1/ai/explore", s.handleAIExplore)
	mux.HandleFunc("/api/v1/ai/generate_tests", s.handleAIGenerateTests)
	mux.HandleFunc("/api/v1/ai/coordinate", s.handleAICoordinate)
	mux.HandleFunc("/api/v1/ai/diagnose", s.handleAIDiagnose)
	mux.HandleFunc("/api/v1/ai/self_heal", s.handleAISelfHeal)
	mux.HandleFunc("/api/v1/ai/proposals", s.handleAIProposals)
	mux.HandleFunc("/api/v1/ai/proposals/approve", s.handleAIProposalApprove)
	mux.HandleFunc("/api/v1/ai/memory", s.handleAIMemory)
}

func (s *Service) intelPaths() (graphFile, propFile, memFile, approvedRoot string) {
	dir := filepath.Join(s.WorkspaceRoot, "cache", "intelligence")
	return filepath.Join(dir, "behavior_graph.json"),
		filepath.Join(dir, "proposals.json"),
		filepath.Join(dir, "memory.json"),
		filepath.Join(s.WorkspaceRoot, "cache", "ai_approved")
}

func (s *Service) resolveOrFirstArtifact(rawID string) (ArtifactEntry, *ingest.IntelligenceReport, error) {
	id := strings.TrimSpace(rawID)
	if id == "" {
		arts := s.discoverArtifacts()
		if len(arts) == 0 {
			return ArtifactEntry{}, nil, os.ErrNotExist
		}
		id = arts[0].Path
	}
	art, err := s.resolveArtifactByID(id)
	if err != nil {
		return ArtifactEntry{}, nil, err
	}
	bundleJar := filepath.Join(s.WorkspaceRoot, "tools", "libs", "bundletool-all-1.18.1.jar")
	eng := ingest.NewEngine(filepath.Join(s.WorkspaceRoot, "cache", "apk_intel"), bundleJar)
	rep, aErr := eng.Analyze(art.Path)
	if aErr != nil {
		return ArtifactEntry{}, nil, aErr
	}
	return art, rep, nil
}

func (s *Service) handleGraphBuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	var req struct {
		Artifact   string `json:"artifact"`
		ScanSource bool   `json:"scan_source"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	art, rep, err := s.resolveOrFirstArtifact(req.Artifact)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if strings.Contains(err.Error(), "BLOCKED_POLICY") {
			status = http.StatusForbidden
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}

	srcRoot := ""
	if req.ScanSource {
		srcRoot = s.WorkspaceRoot
	}
	g := graph.BuildFromArtifact(rep, srcRoot)
	graphFile, _, _, _ := s.intelPaths()
	_ = g.Save(graphFile)

	arch := g.ExploreArchitecture()
	s.recordAudit("graph_build", art.Name, "ALLOW", "built application behavior graph", map[string]any{
		"nodes": arch.NodeCount,
		"edges": arch.EdgeCount,
	})
	s.Bus.Broadcast("GRAPH_BUILT", map[string]any{"package": arch.PackageName, "nodes": arch.NodeCount, "edges": arch.EdgeCount})
	writeJSON(w, http.StatusOK, map[string]any{
		"artifact":     art,
		"graph_file":   graphFile,
		"architecture": arch,
	})
}

func (s *Service) handleGraphQuery(w http.ResponseWriter, r *http.Request) {
	graphFile, _, _, _ := s.intelPaths()
	g, err := graph.Load(graphFile)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "behavior graph not built yet; call POST /api/v1/graph/build"})
		return
	}
	var f graph.QueryFilter
	if r.Method == http.MethodPost && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
			return
		}
	} else {
		q := r.URL.Query()
		f.Kind = graph.NodeKind(q.Get("kind"))
		f.NameContains = q.Get("name_contains")
		f.ExportedOnly = q.Get("exported_only") == "true"
	}
	writeJSON(w, http.StatusOK, g.Query(f))
}

func (s *Service) handleAICoordinate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	var req struct {
		Artifact   string `json:"artifact"`
		MaxSteps   int    `json:"max_steps"`
		MaxScreens int    `json:"max_screens"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
			return
		}
	}
	art, rep, err := s.resolveOrFirstArtifact(req.Artifact)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}
	_, propFile, memFile, _ := s.intelPaths()
	pkg := ""
	if rep.Manifest != nil {
		pkg = rep.Manifest.Package
	}
	gov := ai.NewGovernor("", "studio_ai_coordinator", pkg, ai.ExecutionBudget{MaxSteps: req.MaxSteps, MaxScreens: req.MaxScreens}, s.Guard, s.Ledger)
	store, _ := ai.OpenProposalStore(propFile, s.Ledger)
	mem, _ := ai.OpenMemoryStore(memFile)
	out, cErr := ai.MultiAgentCoordinate(r.Context(), s.Coord, art.Path, rep, "", gov, store, mem)
	if cErr != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": cErr.Error()})
		return
	}
	s.Bus.Broadcast("AI_COORDINATED", map[string]any{"artifact": art.Name})
	writeJSON(w, http.StatusOK, out)
}

func (s *Service) handleGraphImpact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	var req struct {
		Artifact string   `json:"artifact"`
		Changed  []string `json:"changed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	graphFile, _, _, _ := s.intelPaths()
	var g *graph.BehaviorGraph
	if req.Artifact != "" {
		if _, rep, err := s.resolveOrFirstArtifact(req.Artifact); err == nil {
			g = graph.BuildFromArtifact(rep, "")
		}
	}
	if g == nil {
		g, _ = graph.Load(graphFile)
	}
	if g == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "no behavior graph available; run /api/v1/graph/build first or pass artifact"})
		return
	}
	impact := g.AnalyzeChangeImpact(req.Changed)
	s.recordAudit("graph_impact", strings.Join(req.Changed, ","), "ALLOW", impact.Rationale, nil)
	writeJSON(w, http.StatusOK, impact)
}

func (s *Service) handleAIUnderstand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	var req struct {
		Artifact string `json:"artifact"`
		MaxSteps int    `json:"max_steps"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	art, rep, err := s.resolveOrFirstArtifact(req.Artifact)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}
	graphFile, _, _, _ := s.intelPaths()
	g := graph.BuildFromArtifact(rep, "")
	_ = g.Save(graphFile)

	pkg := "unknown.app"
	if rep.Manifest != nil && rep.Manifest.Package != "" {
		pkg = rep.Manifest.Package
	}
	gov := ai.NewGovernor("", "studio_ai", pkg, ai.ExecutionBudget{MaxSteps: req.MaxSteps}, s.Guard, s.Ledger)
	u, uErr := ai.UnderstandAPK(rep, g, gov)
	if uErr != nil {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": uErr.Error()})
		return
	}
	s.storeReport("ai_understand", "AI Understanding: "+art.Name, "completed", u)
	writeJSON(w, http.StatusOK, u)
}

func (s *Service) handleAIExplore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	var req struct {
		Artifact   string `json:"artifact"`
		Activity   string `json:"activity"`
		MaxSteps   int    `json:"max_steps"`
		MaxScreens int    `json:"max_screens"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	art, rep, err := s.resolveOrFirstArtifact(req.Artifact)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}
	graphFile, propFile, _, _ := s.intelPaths()
	g, _ := graph.Load(graphFile)
	if g == nil {
		g = graph.BuildFromArtifact(rep, "")
	}
	pkg := ""
	if rep.Manifest != nil {
		pkg = rep.Manifest.Package
	}
	gov := ai.NewGovernor("", "studio_ai_explorer", pkg, ai.ExecutionBudget{MaxSteps: req.MaxSteps, MaxScreens: req.MaxScreens}, s.Guard, s.Ledger)
	store, sErr := ai.OpenProposalStore(propFile, s.Ledger)
	if sErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": sErr.Error()})
		return
	}
	exp, eErr := ai.ExploreAndHunt(r.Context(), s.Coord, art.Path, req.Activity, rep, g, gov, store)
	if eErr != nil {
		status := http.StatusUnprocessableEntity
		if strings.Contains(eErr.Error(), "BLOCKED_POLICY") {
			status = http.StatusForbidden
		}
		writeJSON(w, status, map[string]any{"error": eErr.Error()})
		return
	}
	_ = g.Save(graphFile)
	s.storeReport("ai_explore", "AI Exploration: "+art.Name, "completed", exp)
	s.Bus.Broadcast("AI_EXPLORATION_COMPLETED", map[string]any{"package": exp.PackageName, "steps": len(exp.StepsExecuted), "findings": len(exp.Findings)})
	writeJSON(w, http.StatusOK, exp)
}

func (s *Service) handleAIGenerateTests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	var req struct {
		Artifact string `json:"artifact"`
		MaxSteps int    `json:"max_steps"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	art, rep, err := s.resolveOrFirstArtifact(req.Artifact)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}
	graphFile, propFile, _, _ := s.intelPaths()
	g, _ := graph.Load(graphFile)
	if g == nil {
		g = graph.BuildFromArtifact(rep, "")
	}
	pkg := ""
	if rep.Manifest != nil {
		pkg = rep.Manifest.Package
	}
	gov := ai.NewGovernor("", "studio_ai_tests", pkg, ai.ExecutionBudget{MaxSteps: req.MaxSteps}, s.Guard, s.Ledger)
	store, err := ai.OpenProposalStore(propFile, s.Ledger)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	var liveAST *mcp_compiler.ASTNode
	if obs, oErr := s.Coord.ObserveScreen(r.Context(), false, "none", 0); oErr == nil {
		liveAST = obs.AST
	}
	props, gErr := ai.GenerateTests(rep, g, liveAST, gov, store)
	if gErr != nil {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": gErr.Error()})
		return
	}
	s.Bus.Broadcast("AI_PROPOSAL_CREATED", map[string]any{"artifact": art.Name, "count": len(props)})
	writeJSON(w, http.StatusOK, map[string]any{"proposals": props, "budget": gov.Status()})
}

func (s *Service) handleAIDiagnose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	var req struct {
		Trace        string `json:"trace"`
		R8Mapping    string `json:"r8_mapping"`
		SuspectedAPI string `json:"suspected_api"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	graphFile, propFile, _, _ := s.intelPaths()
	g, _ := graph.Load(graphFile)
	store, _ := ai.OpenProposalStore(propFile, s.Ledger)
	gov := ai.NewGovernor("", "studio_ai_triage", "", ai.DefaultBudget(), s.Guard, s.Ledger)
	var diff *differential.DivergenceFinding
	if req.SuspectedAPI != "" {
		diff = &differential.DivergenceFinding{
			FirstStepIndex: 1,
			StepLabel:      "reported_step",
			Dimension:      differential.DimUIHierarchy,
			SuspectedAPI:   req.SuspectedAPI,
			Summary:        "Suspected API divergence: " + req.SuspectedAPI,
		}
	}
	diag, err := ai.DiagnoseFailure(req.Trace, req.R8Mapping, diff, g, gov, store)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}
	s.Bus.Broadcast("AI_PROPOSAL_CREATED", map[string]any{"kind": "diagnosis", "causes": len(diag.RankedCauses)})
	writeJSON(w, http.StatusOK, diag)
}

func (s *Service) handleAISelfHeal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	var req ai.BrokenSelector
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	_, propFile, _, _ := s.intelPaths()
	store, _ := ai.OpenProposalStore(propFile, s.Ledger)
	gov := ai.NewGovernor("", "studio_ai_heal", "", ai.DefaultBudget(), s.Guard, s.Ledger)
	obs, oErr := s.Coord.ObserveScreen(r.Context(), false, "none", 0)
	if oErr != nil || obs.AST == nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "live AST unavailable for self-healing"})
		return
	}
	cands, prop, err := ai.ProposeSelfHealingSelectors(req, obs.AST, gov, store)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": cands, "proposal": prop, "budget": gov.Status()})
}

func (s *Service) handleAIProposals(w http.ResponseWriter, r *http.Request) {
	_, propFile, _, _ := s.intelPaths()
	store, err := ai.OpenProposalStore(propFile, s.Ledger)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"proposals": store.List()})
}

func (s *Service) handleAIProposalApprove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	var req struct {
		ID         string `json:"id"`
		ApprovedBy string `json:"approved_by"`
		Apply      bool   `json:"apply"`
		Reject     bool   `json:"reject"`
		Reason     string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	_, propFile, _, approvedRoot := s.intelPaths()
	store, err := ai.OpenProposalStore(propFile, s.Ledger)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	var res *ai.Proposal
	if req.Reject {
		res, err = store.Reject(req.ID, req.ApprovedBy, req.Reason)
	} else {
		res, err = store.Approve(req.ID, req.ApprovedBy, req.Apply, approvedRoot)
	}
	if err != nil {
		status := http.StatusUnprocessableEntity
		if strings.Contains(err.Error(), "BLOCKED_POLICY") {
			status = http.StatusForbidden
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	s.Bus.Broadcast("AI_PROPOSAL_APPROVED", map[string]any{"id": res.ID, "status": res.Status, "approved_by": res.ApprovedBy})
	writeJSON(w, http.StatusOK, res)
}

func (s *Service) handleAIMemory(w http.ResponseWriter, r *http.Request) {
	_, _, memFile, _ := s.intelPaths()
	ms, err := ai.OpenMemoryStore(memFile)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, ms)
		return
	}
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		var req struct {
			Key      string `json:"key"`
			Category string `json:"category"`
			Value    string `json:"value"`
			Actor    string `json:"actor"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
			return
		}
		if err := ms.Put(req.Key, req.Category, req.Value, req.Actor); err != nil {
			status := http.StatusUnprocessableEntity
			if strings.Contains(err.Error(), "BLOCKED_POLICY") {
				status = http.StatusForbidden
			}
			writeJSON(w, status, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, ms)
		return
	}
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET or PUT/POST required"})
}
