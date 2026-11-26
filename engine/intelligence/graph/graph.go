package graph

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"nano_android/engine/apk/ingest"
	"nano_android/engine/apk/manifest"
	"nano_android/engine/compatibility/registry"
	"nano_android/engine/mcp_compiler"
	"nano_android/engine/profiler/crash"
	"nano_android/engine/testing/suite"
)

// NodeKind identifies every entity type modeled in the persistent Application Behavior Graph.
type NodeKind string

const (
	NodePackage         NodeKind = "package"
	NodeActivity        NodeKind = "activity"
	NodeFragment        NodeKind = "fragment"
	NodeService         NodeKind = "service"
	NodeReceiver        NodeKind = "receiver"
	NodeProvider        NodeKind = "provider"
	NodeClass           NodeKind = "class"
	NodeMethod          NodeKind = "method"
	NodeDependency      NodeKind = "dependency"
	NodeResource        NodeKind = "resource"
	NodePermission      NodeKind = "permission"
	NodeIntent          NodeKind = "intent"
	NodeScreen          NodeKind = "screen"
	NodeNavigationFlow  NodeKind = "navigation_flow"
	NodeNetworkEndpoint NodeKind = "network_endpoint"
	NodeDBTable         NodeKind = "db_table"
	NodeNativeLib       NodeKind = "native_lib"
	NodeRuntimeEvent    NodeKind = "runtime_event"
	NodeCrash           NodeKind = "crash"
	NodeTestCase        NodeKind = "test_case"
)

// EdgeRelation defines directed semantic relationships in the Behavior Graph.
type EdgeRelation string

const (
	RelDeclares           EdgeRelation = "declares"
	RelImplements         EdgeRelation = "implements"
	RelExtends            EdgeRelation = "extends"
	RelCalls              EdgeRelation = "calls"
	RelDependsOn          EdgeRelation = "depends_on"
	RelUsesResource       EdgeRelation = "uses_resource"
	RelRequiresPermission EdgeRelation = "requires_permission"
	RelHandlesIntent      EdgeRelation = "handles_intent"
	RelNavigatesTo        EdgeRelation = "navigates_to"
	RelRendersScreen      EdgeRelation = "renders_screen"
	RelCallsEndpoint      EdgeRelation = "calls_endpoint"
	RelMutatesTable       EdgeRelation = "mutates_table"
	RelLoadsNative        EdgeRelation = "loads_native"
	RelEmitsEvent         EdgeRelation = "emits_event"
	RelThrowsCrash        EdgeRelation = "throws_crash"
	RelVerifiedBy         EdgeRelation = "verified_by"
)

// Node represents a single vertex in the persistent Behavior Graph.
type Node struct {
	ID         string            `json:"id"`
	Kind       NodeKind          `json:"kind"`
	Name       string            `json:"name"`
	Label      string            `json:"label,omitempty"`
	Package    string            `json:"package,omitempty"`
	Exported   bool              `json:"exported,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// Edge represents a directed relationship between two vertices.
type Edge struct {
	ID       string            `json:"id"`
	From     string            `json:"from"`
	To       string            `json:"to"`
	Relation EdgeRelation      `json:"relation"`
	Weight   int               `json:"weight,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ComplexityMetric captures structural coupling and method reference density per class.
type ComplexityMetric struct {
	ClassName        string `json:"class_name"`
	OutDegree        int    `json:"out_degree"`
	InDegree         int    `json:"in_degree"`
	MethodRefCount   int    `json:"method_ref_count"`
	ComplexityScore  int    `json:"complexity_score"`
	IsComponent      bool   `json:"is_component"`
}

// CompatibilityNode maps a class or framework API inside the graph to its execution tier requirement.
type CompatibilityNode struct {
	API             string                `json:"api"`
	Category        string                `json:"category"`
	SupportLevel    registry.SupportLevel `json:"support_level"`
	RecommendedTier registry.Backend      `json:"recommended_tier"`
	CallerCount     int                   `json:"caller_count"`
}

// ArchitectureReport exposes project-wide static + dynamic architecture insights.
type ArchitectureReport struct {
	PackageName           string              `json:"package_name"`
	GeneratedUTC          string              `json:"generated_utc"`
	NodeCount             int                 `json:"node_count"`
	EdgeCount             int                 `json:"edge_count"`
	NodeCountsByKind      map[NodeKind]int    `json:"node_counts_by_kind"`
	PackageHierarchy      map[string][]string `json:"package_hierarchy"`
	InheritanceTree       map[string][]string `json:"inheritance_tree"`
	CircularDependencies  [][]string          `json:"circular_dependencies,omitempty"`
	DuplicateClasses      []string            `json:"duplicate_classes,omitempty"`
	UnusedResources       []string            `json:"unused_resources,omitempty"`
	JNIBoundaries         []string            `json:"jni_boundaries,omitempty"`
	PermissionToCodeMap   map[string][]string `json:"permission_to_code_map,omitempty"`
	IntentResolutionMap   map[string][]string `json:"intent_resolution_map,omitempty"`
	APICompatibilityGraph []CompatibilityNode `json:"api_compatibility_graph,omitempty"`
	NavigationEdges       []string            `json:"navigation_edges,omitempty"`
	NetworkEndpoints      []string            `json:"network_endpoints,omitempty"`
	DatabaseTables        []string            `json:"database_tables,omitempty"`
	TopComplexClasses     []ComplexityMetric  `json:"top_complex_classes,omitempty"`
}

// ImpactedItem describes an entity reached by reverse dependency/impact traversal.
type ImpactedItem struct {
	NodeID   string   `json:"node_id"`
	Kind     NodeKind `json:"kind"`
	Name     string   `json:"name"`
	Distance int      `json:"distance"`
	Path     []string `json:"path"`
}

// ImpactReport summarizes which components, screens, and tests are impacted by edited symbols or files.
type ImpactReport struct {
	ChangedTargets     []string       `json:"changed_targets"`
	MatchedSeedNodes   []string       `json:"matched_seed_nodes"`
	ImpactedNodes      []ImpactedItem `json:"impacted_nodes"`
	ImpactedComponents []string       `json:"impacted_components,omitempty"`
	ImpactedScreens    []string       `json:"impacted_screens,omitempty"`
	SelectedTests      []string       `json:"selected_tests,omitempty"`
	RiskLevel          string         `json:"risk_level"` // LOW, MEDIUM, HIGH
	Rationale          string         `json:"rationale"`
}

// BehaviorGraph persists the multi-layer application intelligence graph and serves architecture/impact queries.
type BehaviorGraph struct {
	mu          sync.RWMutex
	Version     string            `json:"version"`
	PackageName string            `json:"package_name"`
	ArtifactSHA string            `json:"artifact_sha256,omitempty"`
	UpdatedUTC  string            `json:"updated_utc"`
	Nodes       map[string]*Node  `json:"nodes"`
	Edges       map[string]*Edge  `json:"edges"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// NewBehaviorGraph creates an empty thread-safe BehaviorGraph.
func NewBehaviorGraph(packageName string) *BehaviorGraph {
	return &BehaviorGraph{
		Version:     "1.0",
		PackageName: strings.TrimSpace(packageName),
		UpdatedUTC:  time.Now().UTC().Format(time.RFC3339),
		Nodes:       make(map[string]*Node),
		Edges:       make(map[string]*Edge),
		Metadata:    make(map[string]string),
	}
}

// NodeID creates a deterministic stable vertex identifier.
func NodeID(kind NodeKind, name string) string {
	return string(kind) + ":" + strings.TrimSpace(name)
}

func edgeID(from, to string, rel EdgeRelation) string {
	sum := sha256.Sum256([]byte(from + "|" + string(rel) + "|" + to))
	return "edge:" + hex.EncodeToString(sum[:8])
}

// UpsertNode adds or merges a graph vertex deterministically.
func (g *BehaviorGraph) UpsertNode(n Node) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if n.ID == "" {
		n.ID = NodeID(n.Kind, n.Name)
	}
	if existing, ok := g.Nodes[n.ID]; ok {
		if n.Label != "" {
			existing.Label = n.Label
		}
		if n.Package != "" {
			existing.Package = n.Package
		}
		if n.Exported {
			existing.Exported = true
		}
		if len(n.Properties) > 0 {
			if existing.Properties == nil {
				existing.Properties = make(map[string]string)
			}
			for k, v := range n.Properties {
				existing.Properties[k] = v
			}
		}
		g.UpdatedUTC = time.Now().UTC().Format(time.RFC3339)
		return n.ID
	}
	cp := n
	if cp.Properties != nil {
		props := make(map[string]string, len(cp.Properties))
		for k, v := range cp.Properties {
			props[k] = v
		}
		cp.Properties = props
	}
	g.Nodes[cp.ID] = &cp
	g.UpdatedUTC = time.Now().UTC().Format(time.RFC3339)
	return cp.ID
}

// AddEdge connects two existing or newly referenced node IDs idempotently and increments weight.
func (g *BehaviorGraph) AddEdge(fromID, toID string, rel EdgeRelation, meta map[string]string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	eid := edgeID(fromID, toID, rel)
	if ex, ok := g.Edges[eid]; ok {
		ex.Weight++
		for k, v := range meta {
			if ex.Metadata == nil {
				ex.Metadata = make(map[string]string)
			}
			ex.Metadata[k] = v
		}
		g.UpdatedUTC = time.Now().UTC().Format(time.RFC3339)
		return eid
	}
	cpMeta := make(map[string]string, len(meta))
	for k, v := range meta {
		cpMeta[k] = v
	}
	if len(cpMeta) == 0 {
		cpMeta = nil
	}
	g.Edges[eid] = &Edge{
		ID:       eid,
		From:     fromID,
		To:       toID,
		Relation: rel,
		Weight:   1,
		Metadata: cpMeta,
	}
	g.UpdatedUTC = time.Now().UTC().Format(time.RFC3339)
	return eid
}

var (
	reEndpointURL = regexp.MustCompile(`https?://[A-Za-z0-9._\-/:?=&%]+`)
	reResRef      = regexp.MustCompile(`@(?:layout|string|drawable|xml|id)/([a-zA-Z0-9_]+)`)
	reJavaResRef  = regexp.MustCompile(`R\.(?:layout|string|drawable|id|xml)\.([a-zA-Z0-9_]+)`)
	reSQLTable    = regexp.MustCompile(`(?i)(?:CREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?|INTO|FROM|UPDATE)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	reGradleDep   = regexp.MustCompile(`(?m)(?:implementation|api|kapt|ksp)\s*\(?["']([^"'\s:]+:[^"'\s:]+(?::[^"'\s]+)?)["']\)?`)
	rePermHint    = regexp.MustCompile(`android\.permission\.[A-Z0-9_]+`)
)

// BuildFromArtifact populates the Behavior Graph from static APK/AAB/DEX/Manifest/Resource intelligence
// and optional Gradle/Kotlin/Java workspace sources.
func BuildFromArtifact(rep *ingest.IntelligenceReport, sourceRoot string) *BehaviorGraph {
	pkgName := "unknown.app"
	if rep != nil && rep.Manifest != nil && strings.TrimSpace(rep.Manifest.Package) != "" {
		pkgName = rep.Manifest.Package
	}
	g := NewBehaviorGraph(pkgName)
	if rep != nil {
		g.ArtifactSHA = rep.SHA256
		g.Metadata["format"] = string(rep.Format)
		g.Metadata["artifact_path"] = rep.Path
	}

	rootPkgID := g.UpsertNode(Node{
		Kind: NodePackage, Name: pkgName, Label: pkgName, Package: pkgName,
	})

	// 1. Manifest components, permissions, intents, deep links.
	if rep != nil && rep.Manifest != nil {
		m := rep.Manifest
		for _, perm := range m.Permissions {
			pID := g.UpsertNode(Node{Kind: NodePermission, Name: perm, Label: perm})
			g.AddEdge(rootPkgID, pID, RelRequiresPermission, nil)
		}
		ingestComponents := func(kind NodeKind, comps []manifestKindAdapter) {
			for _, c := range comps {
				cID := g.UpsertNode(Node{
					Kind:     kind,
					Name:     c.Name,
					Label:    shortName(c.Name),
					Package:  packageOfClass(c.Name),
					Exported: c.Exported,
					Properties: map[string]string{
						"permission": c.Permission,
						"authority":  c.Authority,
					},
				})
				g.AddEdge(rootPkgID, cID, RelDeclares, nil)
				if c.Permission != "" {
					pID := g.UpsertNode(Node{Kind: NodePermission, Name: c.Permission, Label: c.Permission})
					g.AddEdge(cID, pID, RelRequiresPermission, nil)
				}
				for _, action := range c.Actions {
					iID := g.UpsertNode(Node{Kind: NodeIntent, Name: action, Label: action})
					g.AddEdge(cID, iID, RelHandlesIntent, nil)
				}
				for _, dl := range c.DeepLinks {
					iID := g.UpsertNode(Node{Kind: NodeIntent, Name: dl, Label: dl, Properties: map[string]string{"type": "deep_link"}})
					g.AddEdge(cID, iID, RelHandlesIntent, nil)
				}
			}
		}

		var acts, svcs, recvs, provs []manifestKindAdapter
		for _, a := range m.Activities {
			acts = append(acts, fromManifestComponent(a.Name, a.IsEffectivelyExported(), a.Permission, a.Authority, a.IntentFilters))
		}
		for _, s := range m.Services {
			svcs = append(svcs, fromManifestComponent(s.Name, s.IsEffectivelyExported(), s.Permission, s.Authority, s.IntentFilters))
		}
		for _, r := range m.Receivers {
			recvs = append(recvs, fromManifestComponent(r.Name, r.IsEffectivelyExported(), r.Permission, r.Authority, r.IntentFilters))
		}
		for _, p := range m.Providers {
			provs = append(provs, fromManifestComponent(p.Name, p.IsEffectivelyExported(), p.Permission, p.Authority, p.IntentFilters))
		}
		ingestComponents(NodeActivity, acts)
		ingestComponents(NodeService, svcs)
		ingestComponents(NodeReceiver, recvs)
		ingestComponents(NodeProvider, provs)
	}

	// 2. DEX classes, fragments, inheritance, dependency graph, methods, third-party libraries & API compatibility.
	if rep != nil && rep.DEX != nil {
		for _, cls := range rep.DEX.Classes {
			kind := NodeClass
			if strings.Contains(cls.SuperClass, "Fragment") || strings.HasSuffix(cls.JavaName, "Fragment") {
				kind = NodeFragment
			} else if strings.Contains(cls.SuperClass, "Activity") {
				kind = NodeActivity
			} else if strings.Contains(cls.SuperClass, "Service") {
				kind = NodeService
			}
			clsPkg := packageOfClass(cls.JavaName)
			subPkgID := g.UpsertNode(Node{Kind: NodePackage, Name: clsPkg, Label: clsPkg, Package: clsPkg})
			clsID := g.UpsertNode(Node{
				Kind:    kind,
				Name:    cls.JavaName,
				Label:   shortName(cls.JavaName),
				Package: clsPkg,
				Properties: map[string]string{
					"dex_file":    cls.DexFile,
					"super_class": cls.SuperClass,
					"source_file": cls.SourceFile,
				},
			})
			g.AddEdge(subPkgID, clsID, RelDeclares, nil)
			if cls.SuperClass != "" && !strings.HasPrefix(cls.SuperClass, "java.lang.Object") {
				supKind := NodeClass
				supID := g.UpsertNode(Node{Kind: supKind, Name: cls.SuperClass, Label: shortName(cls.SuperClass)})
				g.AddEdge(clsID, supID, RelExtends, nil)
			}
			for _, iface := range cls.Interfaces {
				ifID := g.UpsertNode(Node{Kind: NodeClass, Name: iface, Label: shortName(iface), Properties: map[string]string{"interface": "true"}})
				g.AddEdge(clsID, ifID, RelImplements, nil)
			}
			// Infer navigation edges between Activity/Fragment classes referencing one another via DEX dependency graph
			for _, dep := range rep.DEX.ClassDependencyGraph[cls.JavaName] {
				depID := g.UpsertNode(Node{Kind: NodeClass, Name: dep, Label: shortName(dep)})
				g.AddEdge(clsID, depID, RelDependsOn, nil)
				if strings.HasSuffix(dep, "Activity") || strings.HasSuffix(dep, "Fragment") {
					g.AddEdge(clsID, depID, RelNavigatesTo, map[string]string{"inferred": "dex_reference"})
				}
			}
		}

		// Sample/bounded method nodes (top 500 methods to keep JSON graphs snappy & readable)
		maxMethods := len(rep.DEX.MethodReferences)
		if maxMethods > 500 {
			maxMethods = 500
		}
		for i := 0; i < maxMethods; i++ {
			mr := rep.DEX.MethodReferences[i]
			owner := strings.TrimPrefix(strings.TrimSuffix(strings.ReplaceAll(mr.Owner, "/", "."), ";"), "L")
			mFull := owner + "#" + mr.Name
			mID := g.UpsertNode(Node{Kind: NodeMethod, Name: mFull, Label: mr.Name, Package: owner})
			ownerID := g.UpsertNode(Node{Kind: NodeClass, Name: owner, Label: shortName(owner)})
			g.AddEdge(ownerID, mID, RelDeclares, nil)

			// Map sensitive framework method owners to permission hints
			if perm := inferPermissionFromMethod(owner, mr.Name); perm != "" {
				pID := g.UpsertNode(Node{Kind: NodePermission, Name: perm, Label: perm})
				g.AddEdge(mID, pID, RelRequiresPermission, map[string]string{"inferred": "api_call"})
			}
			// Detect SQLite table operations from standard SQLite API signatures
			if strings.Contains(owner, "SQLiteDatabase") && (mr.Name == "execSQL" || mr.Name == "rawQuery" || mr.Name == "insert" || mr.Name == "query") {
				tID := g.UpsertNode(Node{Kind: NodeDBTable, Name: "sqlite_dynamic", Label: "SQLite API (" + mr.Name + ")"})
				g.AddEdge(ownerID, tID, RelMutatesTable, nil)
			}
		}

		if len(rep.DEX.DuplicateClasses) > 0 {
			g.Metadata["duplicate_classes"] = strings.Join(rep.DEX.DuplicateClasses, ",")
		}
		for _, lib := range rep.DEX.DetectedLibraries {
			dID := g.UpsertNode(Node{Kind: NodeDependency, Name: lib, Label: lib})
			g.AddEdge(rootPkgID, dID, RelDependsOn, nil)
		}
	}

	// 3. Resources, endpoints, and SQL strings embedded in resources.arsc
	if rep != nil && rep.Resources != nil {
		for _, entry := range rep.Resources.Entries {
			resKey := entry.Type + "/" + entry.Name
			rID := g.UpsertNode(Node{
				Kind: NodeResource, Name: resKey, Label: entry.Name,
				Properties: map[string]string{"id_hex": entry.IDHex, "type": entry.Type, "value": entry.Value},
			})
			if entry.Type == "layout" {
				sID := g.UpsertNode(Node{Kind: NodeScreen, Name: "layout:" + entry.Name, Label: entry.Name})
				g.AddEdge(sID, rID, RelUsesResource, nil)
			}
		}
		for _, s := range rep.Resources.Strings {
			for _, u := range reEndpointURL.FindAllString(s, 3) {
				uID := g.UpsertNode(Node{Kind: NodeNetworkEndpoint, Name: u, Label: u})
				g.AddEdge(rootPkgID, uID, RelCallsEndpoint, nil)
			}
			if m := reSQLTable.FindStringSubmatch(s); len(m) == 2 && !isSQLKeyword(m[1]) {
				tID := g.UpsertNode(Node{Kind: NodeDBTable, Name: m[1], Label: m[1]})
				g.AddEdge(rootPkgID, tID, RelMutatesTable, nil)
			}
		}
	}

	// 4. Native .so libraries & JNI boundary mapping
	if rep != nil {
		for _, nl := range rep.NativeLibs {
			nID := g.UpsertNode(Node{
				Kind:  NodeNativeLib,
				Name:  nl.Path,
				Label: filepath.Base(nl.Path),
				Properties: map[string]string{
					"abi":         nl.ABI,
					"machine":     nl.Machine,
					"page_16kb":   fmt.Sprintf("%v", nl.Is16KBAligned),
					"has_jni_sym": fmt.Sprintf("%v", len(nl.JNISymbols) > 0),
				},
			})
			g.AddEdge(rootPkgID, nID, RelLoadsNative, nil)
		}
	}

	// 5. Optional source-root scanning (Kotlin / Java / AndroidManifest.xml / layouts) to enrich source->resource/table/endpoint links
	if strings.TrimSpace(sourceRoot) != "" {
		g.enrichFromSourceTree(rootPkgID, sourceRoot)
	}
	return g
}

type manifestKindAdapter struct {
	Name       string
	Exported   bool
	Permission string
	Authority  string
	Actions    []string
	DeepLinks  []string
}

func fromManifestComponent(name string, exported bool, perm, auth string, filters []manifest.IntentFilter) manifestKindAdapter {
	ad := manifestKindAdapter{
		Name:       name,
		Exported:   exported,
		Permission: perm,
		Authority:  auth,
	}
	for _, f := range filters {
		ad.Actions = append(ad.Actions, f.Actions...)
		for _, sc := range f.Schemes {
			host := "*"
			if len(f.Hosts) > 0 {
				host = f.Hosts[0]
			}
			ad.DeepLinks = append(ad.DeepLinks, sc+"://"+host)
		}
	}
	return ad
}

func (g *BehaviorGraph) enrichFromSourceTree(rootPkgID, sourceRoot string) {
	_ = filepath.WalkDir(sourceRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "build" || base == "bin" || base == "node_modules" || base == "cache" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext != ".kt" && ext != ".java" && ext != ".xml" && ext != ".gradle" && d.Name() != "build.gradle.kts" {
			return nil
		}
		info, sErr := d.Info()
		if sErr != nil || info.Size() > 512*1024 {
			return nil
		}
		raw, rErr := os.ReadFile(p)
		if rErr != nil {
			return nil
		}
		txt := string(raw)
		if d.Name() == "build.gradle" || d.Name() == "build.gradle.kts" {
			for _, m := range reGradleDep.FindAllStringSubmatch(txt, 50) {
				depID := g.UpsertNode(Node{Kind: NodeDependency, Name: m[1], Label: m[1], Properties: map[string]string{"gradle_file": p}})
				g.AddEdge(rootPkgID, depID, RelDependsOn, nil)
			}
			return nil
		}
		clsName := strings.TrimSuffix(d.Name(), ext)
		kind := NodeClass
		if strings.Contains(txt, "extends Fragment") || strings.Contains(txt, ": Fragment(") || strings.HasSuffix(clsName, "Fragment") {
			kind = NodeFragment
		} else if strings.Contains(txt, "@Composable") {
			kind = NodeScreen
		}
		srcNodeID := g.UpsertNode(Node{
			Kind:       kind,
			Name:       clsName,
			Label:      d.Name(),
			Properties: map[string]string{"source_path": p},
		})
		g.AddEdge(rootPkgID, srcNodeID, RelDeclares, nil)
		for _, perm := range rePermHint.FindAllString(txt, 15) {
			pID := g.UpsertNode(Node{Kind: NodePermission, Name: perm, Label: perm})
			g.AddEdge(srcNodeID, pID, RelRequiresPermission, map[string]string{"source": "code_reference"})
		}
		for _, m := range reJavaResRef.FindAllStringSubmatch(txt, 25) {
			rID := g.UpsertNode(Node{Kind: NodeResource, Name: m[1], Label: m[1]})
			g.AddEdge(srcNodeID, rID, RelUsesResource, nil)
		}
		for _, m := range reResRef.FindAllStringSubmatch(txt, 25) {
			rID := g.UpsertNode(Node{Kind: NodeResource, Name: m[1], Label: m[1]})
			g.AddEdge(srcNodeID, rID, RelUsesResource, nil)
		}
		for _, u := range reEndpointURL.FindAllString(txt, 10) {
			uID := g.UpsertNode(Node{Kind: NodeNetworkEndpoint, Name: u, Label: u})
			g.AddEdge(srcNodeID, uID, RelCallsEndpoint, nil)
		}
		for _, m := range reSQLTable.FindAllStringSubmatch(txt, 10) {
			if len(m) == 2 && !isSQLKeyword(m[1]) {
				tID := g.UpsertNode(Node{Kind: NodeDBTable, Name: m[1], Label: m[1]})
				g.AddEdge(srcNodeID, tID, RelMutatesTable, nil)
			}
		}
		return nil
	})
}

// EnrichFromRuntime records live observed screens, UTG navigation transitions, timeline events, and SQLite tables.
func (g *BehaviorGraph) EnrichFromRuntime(activity, fromScreen, toScreen, actionLabel string, ast *mcp_compiler.ASTNode, timeline []mcp_compiler.HostTimelineEvent, storage *mcp_compiler.HostStorageSnapshot) {
	actID := ""
	if strings.TrimSpace(activity) != "" {
		actID = g.UpsertNode(Node{Kind: NodeActivity, Name: activity, Label: shortName(activity)})
	}
	if strings.TrimSpace(toScreen) != "" {
		props := map[string]string{}
		if ast != nil {
			props["widget_count"] = fmt.Sprintf("%d", countASTNodes(ast))
			props["visible_texts"] = strings.Join(ast.CollectVisibleTexts(), " | ")
		}
		toID := g.UpsertNode(Node{Kind: NodeScreen, Name: toScreen, Label: shortName(toScreen), Properties: props})
		if actID != "" {
			g.AddEdge(actID, toID, RelRendersScreen, nil)
		}
		if strings.TrimSpace(fromScreen) != "" && fromScreen != toScreen {
			fromID := g.UpsertNode(Node{Kind: NodeScreen, Name: fromScreen, Label: shortName(fromScreen)})
			flowName := fromScreen + " -> " + toScreen
			flowID := g.UpsertNode(Node{Kind: NodeNavigationFlow, Name: flowName, Label: flowName, Properties: map[string]string{"action": actionLabel}})
			g.AddEdge(fromID, flowID, RelNavigatesTo, map[string]string{"action": actionLabel})
			g.AddEdge(flowID, toID, RelNavigatesTo, map[string]string{"action": actionLabel})
		}
	}
	for i, ev := range timeline {
		if i >= 100 {
			break
		}
		evID := g.UpsertNode(Node{
			Kind:       NodeRuntimeEvent,
			Name:       fmt.Sprintf("%d:%s:%s", ev.Seq, ev.EventType, ev.Source),
			Label:      ev.EventType,
			Properties: map[string]string{"backend": ev.Backend, "source": ev.Source},
		})
		if actID != "" {
			g.AddEdge(actID, evID, RelEmitsEvent, nil)
		}
	}
	if storage != nil {
		for tbl, val := range storage.SQLiteTables {
			tID := g.UpsertNode(Node{Kind: NodeDBTable, Name: tbl, Label: tbl, Properties: map[string]string{"summary": val}})
			if actID != "" {
				g.AddEdge(actID, tID, RelMutatesTable, nil)
			}
		}
	}
}

// AttachCrash links a analyzed CrashReport to its root-cause class and component vertices.
func (g *BehaviorGraph) AttachCrash(rep crash.CrashReport, componentName string) string {
	cID := g.UpsertNode(Node{
		Kind:  NodeCrash,
		Name:  rep.Signature,
		Label: rep.ExceptionClass + " @ " + rep.RootCauseClass,
		Properties: map[string]string{
			"kind":             rep.Kind,
			"exception_class":  rep.ExceptionClass,
			"root_cause_class": rep.RootCauseClass,
			"environment_tier": rep.EnvironmentTier,
		},
	})
	if rep.RootCauseClass != "" {
		clsID := g.UpsertNode(Node{Kind: NodeClass, Name: rep.RootCauseClass, Label: shortName(rep.RootCauseClass)})
		g.AddEdge(clsID, cID, RelThrowsCrash, nil)
	}
	if componentName != "" {
		compID := g.UpsertNode(Node{Kind: NodeActivity, Name: componentName, Label: shortName(componentName)})
		g.AddEdge(compID, cID, RelThrowsCrash, nil)
	}
	return cID
}

// AttachTestCase registers a suite.TestCase and connects it to the Activity / target vertices it verifies.
func (g *BehaviorGraph) AttachTestCase(tc suite.TestCase) string {
	tID := g.UpsertNode(Node{
		Kind:  NodeTestCase,
		Name:  tc.Name,
		Label: tc.Name,
		Properties: map[string]string{
			"activity": tc.Activity,
			"tags":     strings.Join(tc.Tags, ","),
			"steps":    fmt.Sprintf("%d", len(tc.Steps)),
		},
	})
	if tc.Activity != "" {
		aID := g.UpsertNode(Node{Kind: NodeActivity, Name: tc.Activity, Label: shortName(tc.Activity)})
		g.AddEdge(aID, tID, RelVerifiedBy, nil)
	}
	for _, st := range tc.Steps {
		if st.Target != "" {
			sID := g.UpsertNode(Node{Kind: NodeScreen, Name: st.Target, Label: st.Target})
			g.AddEdge(sID, tID, RelVerifiedBy, map[string]string{"step_action": st.Action})
		}
	}
	return tID
}

// ExploreArchitecture computes package hierarchy, inheritance, circular dependencies, unused resources,
// JNI boundaries, permission-to-code mappings, API compatibility summary, and complexity metrics.
func (g *BehaviorGraph) ExploreArchitecture() ArchitectureReport {
	g.mu.RLock()
	defer g.mu.RUnlock()

	rep := ArchitectureReport{
		PackageName:         g.PackageName,
		GeneratedUTC:        time.Now().UTC().Format(time.RFC3339),
		NodeCount:           len(g.Nodes),
		EdgeCount:           len(g.Edges),
		NodeCountsByKind:    make(map[NodeKind]int),
		PackageHierarchy:    make(map[string][]string),
		InheritanceTree:     make(map[string][]string),
		PermissionToCodeMap: make(map[string][]string),
		IntentResolutionMap: make(map[string][]string),
	}

	if dupRaw := g.Metadata["duplicate_classes"]; dupRaw != "" {
		rep.DuplicateClasses = strings.Split(dupRaw, ",")
	}

	// Track in-degree, out-degree, method counts, used resources, class adjacency for SCCs.
	inDeg := make(map[string]int)
	outDeg := make(map[string]int)
	methodCnt := make(map[string]int)
	usedResources := make(map[string]bool)
	classAdj := make(map[string][]string)

	for _, n := range g.Nodes {
		rep.NodeCountsByKind[n.Kind]++
		switch n.Kind {
		case NodeClass, NodeActivity, NodeFragment, NodeService, NodeReceiver, NodeProvider:
			if n.Package != "" {
				rep.PackageHierarchy[n.Package] = append(rep.PackageHierarchy[n.Package], n.Name)
			}
		case NodeNativeLib:
			rep.JNIBoundaries = append(rep.JNIBoundaries, fmt.Sprintf("%s (%s, 16KB=%s)", n.Name, n.Properties["abi"], n.Properties["page_16kb"]))
		case NodeNetworkEndpoint:
			rep.NetworkEndpoints = append(rep.NetworkEndpoints, n.Name)
		case NodeDBTable:
			rep.DatabaseTables = append(rep.DatabaseTables, n.Name)
		case NodeNavigationFlow:
			rep.NavigationEdges = append(rep.NavigationEdges, n.Name)
		}
	}

	compatCallers := make(map[string]int)
	for _, e := range g.Edges {
		outDeg[e.From]++
		inDeg[e.To]++
		fromNode := g.Nodes[e.From]
		toNode := g.Nodes[e.To]
		if fromNode == nil || toNode == nil {
			continue
		}
		switch e.Relation {
		case RelDeclares:
			if toNode.Kind == NodeMethod {
				methodCnt[fromNode.Name]++
			}
		case RelExtends:
			rep.InheritanceTree[toNode.Name] = append(rep.InheritanceTree[toNode.Name], fromNode.Name)
		case RelDependsOn:
			if fromNode.Kind == NodeClass && toNode.Kind == NodeClass {
				classAdj[fromNode.Name] = append(classAdj[fromNode.Name], toNode.Name)
			}
		case RelUsesResource:
			usedResources[toNode.ID] = true
		case RelRequiresPermission:
			rep.PermissionToCodeMap[toNode.Name] = append(rep.PermissionToCodeMap[toNode.Name], fromNode.Name)
		case RelHandlesIntent:
			rep.IntentResolutionMap[toNode.Name] = append(rep.IntentResolutionMap[toNode.Name], fromNode.Name)
		}
		if toNode.Kind == NodeClass {
			for apiPrefix := range registry.DefaultContracts() {
				if strings.HasPrefix(toNode.Name, apiPrefix) {
					compatCallers[apiPrefix]++
				}
			}
		}
	}

	for id, n := range g.Nodes {
		if n.Kind == NodeResource && !usedResources[id] {
			rep.UnusedResources = append(rep.UnusedResources, n.Name)
		}
	}

	contracts := registry.DefaultContracts()
	for api, count := range compatCallers {
		c := contracts[api]
		tier := registry.BackendNanoJVM
		if c.FallbackBackend != "" {
			tier = c.FallbackBackend
		}
		rep.APICompatibilityGraph = append(rep.APICompatibilityGraph, CompatibilityNode{
			API:             api,
			Category:        "framework",
			SupportLevel:    c.Level,
			RecommendedTier: tier,
			CallerCount:     count,
		})
	}

	rep.CircularDependencies = findSCCCycles(classAdj)

	for _, n := range g.Nodes {
		if n.Kind != NodeClass && n.Kind != NodeActivity && n.Kind != NodeFragment && n.Kind != NodeService {
			continue
		}
		in := inDeg[n.ID]
		out := outDeg[n.ID]
		mc := methodCnt[n.Name]
		score := out*2 + mc + in
		if score == 0 {
			continue
		}
		rep.TopComplexClasses = append(rep.TopComplexClasses, ComplexityMetric{
			ClassName:       n.Name,
			OutDegree:       out,
			InDegree:        in,
			MethodRefCount:  mc,
			ComplexityScore: score,
			IsComponent:     n.Kind != NodeClass,
		})
	}

	// Deterministic sorting across slices & maps
	for k := range rep.PackageHierarchy {
		sort.Strings(rep.PackageHierarchy[k])
	}
	for k := range rep.InheritanceTree {
		sort.Strings(rep.InheritanceTree[k])
	}
	for k := range rep.PermissionToCodeMap {
		sort.Strings(rep.PermissionToCodeMap[k])
	}
	for k := range rep.IntentResolutionMap {
		sort.Strings(rep.IntentResolutionMap[k])
	}
	sort.Strings(rep.UnusedResources)
	sort.Strings(rep.JNIBoundaries)
	sort.Strings(rep.NetworkEndpoints)
	sort.Strings(rep.DatabaseTables)
	sort.Strings(rep.NavigationEdges)
	sort.Slice(rep.APICompatibilityGraph, func(i, j int) bool {
		return rep.APICompatibilityGraph[i].API < rep.APICompatibilityGraph[j].API
	})
	sort.Slice(rep.TopComplexClasses, func(i, j int) bool {
		if rep.TopComplexClasses[i].ComplexityScore == rep.TopComplexClasses[j].ComplexityScore {
			return rep.TopComplexClasses[i].ClassName < rep.TopComplexClasses[j].ClassName
		}
		return rep.TopComplexClasses[i].ComplexityScore > rep.TopComplexClasses[j].ComplexityScore
	})
	if len(rep.TopComplexClasses) > 25 {
		rep.TopComplexClasses = rep.TopComplexClasses[:25]
	}
	return rep
}

// AnalyzeChangeImpact traverses dependencies and verification edges from edited files/classes/APIs
// to determine impacted components, screens, risk level, and tests to run.
func (g *BehaviorGraph) AnalyzeChangeImpact(changedTargets []string) ImpactReport {
	g.mu.RLock()
	defer g.mu.RUnlock()

	rep := ImpactReport{
		ChangedTargets: append([]string(nil), changedTargets...),
		RiskLevel:      "LOW",
	}
	if len(changedTargets) == 0 {
		rep.Rationale = "No changed targets supplied."
		return rep
	}

	// Build undirected/impact propagation adjacency:
	// - forward verification/navigation/render/crash edges (from -> to)
	// - reverse dependency/calls/resource edges (to -> from, i.e., if A depends on B and B changes, A is impacted)
	impactAdj := make(map[string][]string)
	for _, e := range g.Edges {
		switch e.Relation {
		case RelDependsOn, RelExtends, RelImplements, RelCalls, RelUsesResource, RelRequiresPermission, RelHandlesIntent, RelLoadsNative, RelDeclares:
			impactAdj[e.To] = append(impactAdj[e.To], e.From)
		case RelRendersScreen, RelNavigatesTo, RelVerifiedBy, RelThrowsCrash:
			impactAdj[e.From] = append(impactAdj[e.From], e.To)
		}
	}

	seeds := make([]string, 0)
	seen := make(map[string]bool)
	for _, rawTarget := range changedTargets {
		t := strings.ToLower(strings.TrimSpace(rawTarget))
		if t == "" {
			continue
		}
		base := strings.ToLower(strings.TrimSuffix(filepath.Base(rawTarget), filepath.Ext(rawTarget)))
		for id, n := range g.Nodes {
			nameLow := strings.ToLower(n.Name)
			srcLow := strings.ToLower(n.Properties["source_path"])
			if nameLow == t || strings.Contains(nameLow, t) || (base != "" && (nameLow == base || strings.HasSuffix(nameLow, "."+base))) || (srcLow != "" && strings.Contains(srcLow, t)) {
				if !seen[id] {
					seen[id] = true
					seeds = append(seeds, id)
					rep.MatchedSeedNodes = append(rep.MatchedSeedNodes, id)
				}
			}
		}
	}
	sort.Strings(seeds)
	sort.Strings(rep.MatchedSeedNodes)

	type bfsItem struct {
		id   string
		dist int
		path []string
	}
	queue := make([]bfsItem, 0, len(seeds))
	for _, sid := range seeds {
		queue = append(queue, bfsItem{id: sid, dist: 0, path: []string{sid}})
	}

	compSet := make(map[string]bool)
	screenSet := make(map[string]bool)
	testSet := make(map[string]bool)

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		n := g.Nodes[cur.id]
		if n != nil {
			rep.ImpactedNodes = append(rep.ImpactedNodes, ImpactedItem{
				NodeID:   n.ID,
				Kind:     n.Kind,
				Name:     n.Name,
				Distance: cur.dist,
				Path:     cur.path,
			})
			switch n.Kind {
			case NodeActivity, NodeFragment, NodeService, NodeReceiver, NodeProvider:
				compSet[n.Name] = true
			case NodeScreen:
				screenSet[n.Name] = true
			case NodeTestCase:
				testSet[n.Name] = true
			}
		}
		if cur.dist >= 5 {
			continue
		}
		for _, nxt := range impactAdj[cur.id] {
			if !seen[nxt] {
				seen[nxt] = true
				np := make([]string, len(cur.path)+1)
				copy(np, cur.path)
				np[len(cur.path)] = nxt
				queue = append(queue, bfsItem{id: nxt, dist: cur.dist + 1, path: np})
			}
		}
	}

	for c := range compSet {
		rep.ImpactedComponents = append(rep.ImpactedComponents, c)
	}
	for s := range screenSet {
		rep.ImpactedScreens = append(rep.ImpactedScreens, s)
	}
	for t := range testSet {
		rep.SelectedTests = append(rep.SelectedTests, t)
	}
	sort.Strings(rep.ImpactedComponents)
	sort.Strings(rep.ImpactedScreens)
	sort.Strings(rep.SelectedTests)

	if len(rep.ImpactedComponents) >= 3 || len(rep.ImpactedNodes) >= 15 {
		rep.RiskLevel = "HIGH"
	} else if len(rep.ImpactedComponents) >= 1 || len(rep.ImpactedNodes) >= 4 {
		rep.RiskLevel = "MEDIUM"
	}
	rep.Rationale = fmt.Sprintf("Matched %d seed graph vertices; reverse dependency/test traversal identified %d impacted nodes, %d Android components, %d screens, and %d regression tests.",
		len(rep.MatchedSeedNodes), len(rep.ImpactedNodes), len(rep.ImpactedComponents), len(rep.ImpactedScreens), len(rep.SelectedTests))
	return rep
}

// QueryFilter filters Behavior Graph vertices and incident edges.
type QueryFilter struct {
	Kind         NodeKind `json:"kind,omitempty"`
	NameContains string   `json:"name_contains,omitempty"`
	ExportedOnly bool     `json:"exported_only,omitempty"`
	Limit        int      `json:"limit,omitempty"`
}

// QueryResult returns matching graph nodes and connecting edges.
type QueryResult struct {
	TotalMatching int    `json:"total_matching"`
	Nodes         []Node `json:"nodes"`
	Edges         []Edge `json:"edges"`
}

// Query slices the Behavior Graph deterministically by kind, substring, or exported status.
func (g *BehaviorGraph) Query(f QueryFilter) QueryResult {
	g.mu.RLock()
	defer g.mu.RUnlock()

	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	sub := strings.ToLower(strings.TrimSpace(f.NameContains))

	var matched []Node
	matchedIDs := make(map[string]bool)
	var keys []string
	for id := range g.Nodes {
		keys = append(keys, id)
	}
	sort.Strings(keys)

	for _, id := range keys {
		n := g.Nodes[id]
		if f.Kind != "" && n.Kind != f.Kind {
			continue
		}
		if f.ExportedOnly && !n.Exported {
			continue
		}
		if sub != "" && !strings.Contains(strings.ToLower(n.Name), sub) && !strings.Contains(strings.ToLower(n.Label), sub) {
			continue
		}
		if len(matched) < limit {
			matched = append(matched, *n)
			matchedIDs[id] = true
		}
	}

	var edges []Edge
	for _, e := range g.Edges {
		if matchedIDs[e.From] && matchedIDs[e.To] {
			edges = append(edges, *e)
		}
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })
	return QueryResult{
		TotalMatching: len(matched),
		Nodes:         matched,
		Edges:         edges,
	}
}

// Save writes the BehaviorGraph atomically to disk with restrictive permissions.
func (g *BehaviorGraph) Save(path string) error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	b, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load reads a persisted BehaviorGraph JSON file from disk.
func Load(path string) (*BehaviorGraph, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var g BehaviorGraph
	if err := json.Unmarshal(b, &g); err != nil {
		return nil, err
	}
	if g.Nodes == nil {
		g.Nodes = make(map[string]*Node)
	}
	if g.Edges == nil {
		g.Edges = make(map[string]*Edge)
	}
	if g.Metadata == nil {
		g.Metadata = make(map[string]string)
	}
	return &g, nil
}

// findSCCCycles implements Tarjan's strongly connected components algorithm to find circular dependencies.
func findSCCCycles(adj map[string][]string) [][]string {
	index := 0
	indices := make(map[string]int)
	lowlink := make(map[string]int)
	onStack := make(map[string]bool)
	var stack []string
	var cycles [][]string

	var keys []string
	for k := range adj {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var strongConnect func(v string)
	strongConnect = func(v string) {
		indices[v] = index
		lowlink[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true

		neighbors := append([]string(nil), adj[v]...)
		sort.Strings(neighbors)
		for _, w := range neighbors {
			if _, seen := indices[w]; !seen {
				strongConnect(w)
				if lowlink[w] < lowlink[v] {
					lowlink[v] = lowlink[w]
				}
			} else if onStack[w] {
				if indices[w] < lowlink[v] {
					lowlink[v] = indices[w]
				}
			}
		}

		if lowlink[v] == indices[v] {
			var scc []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				scc = append(scc, w)
				if w == v {
					break
				}
			}
			if len(scc) > 1 {
				sort.Strings(scc)
				cycles = append(cycles, scc)
			}
		}
	}

	for _, k := range keys {
		if _, seen := indices[k]; !seen {
			strongConnect(k)
		}
	}
	return cycles
}

func shortName(fqn string) string {
	if idx := strings.LastIndex(fqn, "."); idx >= 0 && idx < len(fqn)-1 {
		return fqn[idx+1:]
	}
	return fqn
}

func packageOfClass(fqn string) string {
	if idx := strings.LastIndex(fqn, "."); idx > 0 {
		return fqn[:idx]
	}
	return "(default)"
}

func countASTNodes(n *mcp_compiler.ASTNode) int {
	if n == nil {
		return 0
	}
	c := 1
	for _, ch := range n.Children {
		c += countASTNodes(ch)
	}
	return c
}

func inferPermissionFromMethod(owner, method string) string {
	lowOwner := strings.ToLower(owner)
	lowMethod := strings.ToLower(method)
	switch {
	case strings.Contains(lowOwner, "locationmanager"):
		return "android.permission.ACCESS_FINE_LOCATION"
	case strings.Contains(lowOwner, "camera") || strings.Contains(lowMethod, "opencamera"):
		return "android.permission.CAMERA"
	case strings.Contains(lowOwner, "telephonymanager") || strings.Contains(lowOwner, "smsmanager"):
		return "android.permission.READ_PHONE_STATE"
	case strings.Contains(lowOwner, "audiorecord"):
		return "android.permission.RECORD_AUDIO"
	case strings.Contains(lowOwner, "httpurlconnection") || strings.Contains(lowOwner, "okhttp"):
		return "android.permission.INTERNET"
	default:
		return ""
	}
}

func isSQLKeyword(tok string) bool {
	switch strings.ToUpper(tok) {
	case "SELECT", "WHERE", "VALUES", "SET", "INTO", "FROM", "TABLE", "EXISTS", "AND", "OR":
		return true
	default:
		return false
	}
}
