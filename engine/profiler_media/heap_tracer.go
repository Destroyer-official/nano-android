package profiler_media

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LifecycleState defines Android Activity lifecycle progression.
type LifecycleState string

const (
	StateCreated   LifecycleState = "CREATED"
	StateResumed   LifecycleState = "RESUMED"
	StatePaused    LifecycleState = "PAUSED"
	StateStopped   LifecycleState = "STOPPED"
	StateDestroyed LifecycleState = "DESTROYED"
	StateRecreated LifecycleState = "RECREATED"
)

// GCRootType identifies the kind of garbage collection root retaining an object.
type GCRootType string

const (
	RootStaticField    GCRootType = "STATIC_FIELD"
	RootJNI_Global     GCRootType = "JNI_GLOBAL"
	RootJNI_Local      GCRootType = "JNI_LOCAL"
	RootThreadBlock    GCRootType = "THREAD_BLOCK"
	RootNativeStack    GCRootType = "NATIVE_STACK"
	RootSystemClass    GCRootType = "SYSTEM_CLASS"
	RootAsyncHandler   GCRootType = "ASYNC_HANDLER"
	RootSingletonScope GCRootType = "SINGLETON_SCOPE"
)

// HeapReference represents a directed edge from a parent object to a child object.
type HeapReference struct {
	FieldName string `json:"field_name"`
	TargetID  string `json:"target_id"`
}

// DalvikObject represents an object allocated in Dalvik / ART heap.
type DalvikObject struct {
	ID           string          `json:"id"`
	ClassName    string          `json:"class_name"`
	ShallowSize  int64           `json:"shallow_size"`  // Bytes allocated for object itself
	RetainedSize int64           `json:"retained_size"` // Bytes retained in heap by this object
	References   []HeapReference `json:"references"`    // Outgoing fields
	IsGCRoot     bool            `json:"is_gc_root"`
	RootType     GCRootType      `json:"root_type,omitempty"`
	RootDesc     string          `json:"root_desc,omitempty"`
	AllocStack   string          `json:"alloc_stack,omitempty"`
}

// ActivityLifecycleRecord tracks an activity instance through lifecycle cycles.
type ActivityLifecycleRecord struct {
	InstanceID       string         `json:"instance_id"`
	ClassName        string         `json:"class_name"`
	CycleIndex       int            `json:"cycle_index"`
	State            LifecycleState `json:"state"`
	CreationTime     time.Time      `json:"creation_time"`
	DestructionTime  time.Time      `json:"destruction_time,omitempty"`
	CreationStack    string         `json:"creation_stack,omitempty"`
	DestructionStack string         `json:"destruction_stack,omitempty"`
}

// LeakPathNode represents one link in the shortest path from GC root to leaked activity.
type LeakPathNode struct {
	ObjectID  string `json:"object_id"`
	ClassName string `json:"class_name"`
	FieldName string `json:"field_name,omitempty"`
	IsRoot    bool   `json:"is_root"`
	RootType  string `json:"root_type,omitempty"`
}

// ActivityLeakTrace details a confirmed Activity memory leak with call stacks and diagnoses.
type ActivityLeakTrace struct {
	ActivityID       string         `json:"activity_id"`
	ActivityClass    string         `json:"activity_class"`
	CycleIndex       int            `json:"cycle_index"`
	RetainedBytes    int64          `json:"retained_bytes"`
	LeakingRootID    string         `json:"leaking_root_id"`
	LeakingRootType  GCRootType     `json:"leaking_root_type"`
	LeakingRootDesc  string         `json:"leaking_root_desc"`
	LeakType         string         `json:"leak_type"` // STATIC_REFERENCE, ASYNC_HANDLER, VIEW_CONTEXT, UNREGISTERED_LISTENER, BACKGROUND_WORKER
	Diagnosis        string         `json:"diagnosis"`
	Remediation      string         `json:"remediation"`
	ReferenceChain   []LeakPathNode `json:"reference_chain"`
	FormattedPath    string         `json:"formatted_path"`
	StackTrace       string         `json:"stack_trace"`
	DetectionTime    time.Time      `json:"detection_time"`
}

// MultiCycleAnalysisReport aggregates heap growth and active linear leak detection across N recreation cycles.
type MultiCycleAnalysisReport struct {
	CyclesTested            int                      `json:"cycles_tested"`
	ActivityClass           string                   `json:"activity_class"`
	RetainedInstances       int                      `json:"retained_instances"`
	TotalRetainedBytes      int64                    `json:"total_retained_bytes"`
	GrowthRateBytesPerCycle int64                    `json:"growth_rate_bytes_per_cycle"`
	IsLinearLeak            bool                     `json:"is_linear_leak"`
	Cycles                  []*RecreationCycleReport `json:"cycles"`
}

// RecreationCycleReport details heap growth and detected leaks after an activity recreation.
type RecreationCycleReport struct {
	CycleIndex           int                  `json:"cycle_index"`
	ActivityClass        string               `json:"activity_class"`
	OldInstanceID        string               `json:"old_instance_id"`
	NewInstanceID        string               `json:"new_instance_id"`
	HeapDeltaBytes       int64                `json:"heap_delta_bytes"`
	TotalRetainedHeap    int64                `json:"total_retained_heap"`
	LeaksDetected        []ActivityLeakTrace  `json:"leaks_detected"`
	HasLeaks             bool                 `json:"has_leaks"`
	Timestamp            time.Time            `json:"timestamp"`
}

// MeminfoSnapshot holds metrics parsed from Android's `dumpsys meminfo`.
type MeminfoSnapshot struct {
	PackageName     string `json:"package_name"`
	PID             int    `json:"pid"`
	NativeHeapAlloc int64  `json:"native_heap_alloc_kb"`
	DalvikHeapAlloc int64  `json:"dalvik_heap_alloc_kb"`
	DalvikHeapPSS   int64  `json:"dalvik_heap_pss_kb"`
	TotalPSS        int64  `json:"total_pss_kb"`
	TotalPrivateDirty int64 `json:"total_private_dirty_kb"`
	Views           int    `json:"views_count"`
	Activities      int    `json:"activities_count"`
	AppContexts     int    `json:"app_contexts_count"`
}

// HeapTracer manages heap allocation tracking, GC graph analysis, and leak detection.
type HeapTracer struct {
	mu           sync.RWMutex
	objects      map[string]*DalvikObject
	activities   map[string]*ActivityLifecycleRecord
	cycleHistory []*RecreationCycleReport
}

// NewHeapTracer creates a new Dalvik heap tracer.
func NewHeapTracer() *HeapTracer {
	return &HeapTracer{
		objects:      make(map[string]*DalvikObject),
		activities:   make(map[string]*ActivityLifecycleRecord),
		cycleHistory: make([]*RecreationCycleReport, 0),
	}
}

// RegisterObject records an allocation in the Dalvik heap.
func (ht *HeapTracer) RegisterObject(obj DalvikObject) {
	ht.mu.Lock()
	defer ht.mu.Unlock()
	copied := obj
	ht.objects[obj.ID] = &copied
}

// AddReference adds a directed pointer from fromID to toID with a field name.
func (ht *HeapTracer) AddReference(fromID, toID, fieldName string) {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	if parent, ok := ht.objects[fromID]; ok {
		parent.References = append(parent.References, HeapReference{
			FieldName: fieldName,
			TargetID:  toID,
		})
	}
}

// TrackActivity records lifecycle transitions for an Activity instance.
func (ht *HeapTracer) TrackActivity(id, className string, cycle int, state LifecycleState, stack string) {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	rec, ok := ht.activities[id]
	if !ok {
		rec = &ActivityLifecycleRecord{
			InstanceID:   id,
			ClassName:    className,
			CycleIndex:   cycle,
			State:        state,
			CreationTime: time.Now().UTC(),
		}
		if state == StateCreated {
			rec.CreationStack = stack
		} else if state == StateDestroyed {
			rec.DestructionStack = stack
			rec.DestructionTime = time.Now().UTC()
		}
		ht.activities[id] = rec
	} else {
		rec.State = state
		if state == StateDestroyed {
			rec.DestructionTime = time.Now().UTC()
			rec.DestructionStack = stack
		}
	}

	// Ensure heap object representation exists
	if _, exists := ht.objects[id]; !exists {
		ht.objects[id] = &DalvikObject{
			ID:          id,
			ClassName:   className,
			ShallowSize: 2048, // typical baseline Activity shallow size
			AllocStack:  stack,
		}
	}
}

// SimulateRecreationCycle simulates destroying an old activity and creating a new one (e.g. rotation).
func (ht *HeapTracer) SimulateRecreationCycle(
	className string,
	oldID, newID string,
	cycle int,
	retainingField string,
	gcRootID string,
	destroyStack string,
) *RecreationCycleReport {
	// 1. Transition old activity to DESTROYED
	ht.TrackActivity(oldID, className, cycle, StateDestroyed, destroyStack)

	// 2. If a leak exists, wire the GC root reference to the old activity
	if gcRootID != "" && retainingField != "" {
		ht.AddReference(gcRootID, oldID, retainingField)
	}

	// 3. Create the new activity instance for the next cycle
	newStack := fmt.Sprintf("android.app.ActivityThread.handleLaunchActivity\n\tat %s.<init>(%s.kt:42)", className, className)
	ht.TrackActivity(newID, className, cycle+1, StateResumed, newStack)

	// 4. Run leak detection across all destroyed activities
	leaks := ht.DetectActivityLeaks()

	var totalRetained int64
	for _, l := range leaks {
		totalRetained += l.RetainedBytes
	}

	report := &RecreationCycleReport{
		CycleIndex:        cycle,
		ActivityClass:     className,
		OldInstanceID:     oldID,
		NewInstanceID:     newID,
		HeapDeltaBytes:    totalRetained,
		TotalRetainedHeap: totalRetained,
		LeaksDetected:     leaks,
		HasLeaks:          len(leaks) > 0,
		Timestamp:         time.Now().UTC(),
	}

	ht.mu.Lock()
	ht.cycleHistory = append(ht.cycleHistory, report)
	ht.mu.Unlock()

	return report
}

// DetectActivityLeaks finds paths from GC roots to DESTROYED activity instances.
func (ht *HeapTracer) DetectActivityLeaks() []ActivityLeakTrace {
	ht.mu.RLock()
	defer ht.mu.RUnlock()

	var leaks []ActivityLeakTrace

	// Find all destroyed activities
	var destroyedActivities []*ActivityLifecycleRecord
	for _, act := range ht.activities {
		if act.State == StateDestroyed {
			destroyedActivities = append(destroyedActivities, act)
		}
	}

	if len(destroyedActivities) == 0 {
		return leaks
	}

	// Identify all GC roots
	var gcRoots []*DalvikObject
	for _, obj := range ht.objects {
		if obj.IsGCRoot {
			gcRoots = append(gcRoots, obj)
		}
	}

	// For each destroyed activity, run BFS from all GC roots to find the shortest retaining path
	for _, act := range destroyedActivities {
		targetID := act.InstanceID
		shortestPath, root := ht.findShortestPathFromRoots(gcRoots, targetID)
		if len(shortestPath) > 0 && root != nil {
			retained := ht.computeRetainedSize(targetID)

			formatted := formatReferenceChain(shortestPath)
			stack := act.DestructionStack
			if stack == "" {
				stack = act.CreationStack
			}

			leakType, diag, remed := DiagnoseLeak(shortestPath, root.RootType)

			leak := ActivityLeakTrace{
				ActivityID:      targetID,
				ActivityClass:   act.ClassName,
				CycleIndex:      act.CycleIndex,
				RetainedBytes:   retained,
				LeakingRootID:   root.ID,
				LeakingRootType: root.RootType,
				LeakingRootDesc: root.RootDesc,
				LeakType:        leakType,
				Diagnosis:       diag,
				Remediation:     remed,
				ReferenceChain:  shortestPath,
				FormattedPath:   formatted,
				StackTrace:      stack,
				DetectionTime:   time.Now().UTC(),
			}

			leaks = append(leaks, leak)
		}
	}

	sort.Slice(leaks, func(i, j int) bool {
		return leaks[i].RetainedBytes > leaks[j].RetainedBytes
	})

	return leaks
}

// findShortestPathFromRoots performs multi-source BFS starting from all GC roots to find targetID.
func (ht *HeapTracer) findShortestPathFromRoots(roots []*DalvikObject, targetID string) ([]LeakPathNode, *DalvikObject) {
	type queueItem struct {
		id   string
		path []LeakPathNode
		root *DalvikObject
	}

	visited := make(map[string]bool)
	queue := make([]queueItem, 0)

	for _, r := range roots {
		visited[r.ID] = true
		rootNode := LeakPathNode{
			ObjectID:  r.ID,
			ClassName: r.ClassName,
			IsRoot:    true,
			RootType:  string(r.RootType),
		}
		queue = append(queue, queueItem{
			id:   r.ID,
			path: []LeakPathNode{rootNode},
			root: r,
		})
	}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if curr.id == targetID {
			return curr.path, curr.root
		}

		obj, exists := ht.objects[curr.id]
		if !exists {
			continue
		}

		for _, ref := range obj.References {
			if !visited[ref.TargetID] {
				visited[ref.TargetID] = true
				targetObj, ok := ht.objects[ref.TargetID]
				className := "UnknownClass"
				if ok {
					className = targetObj.ClassName
				}

				stepNode := LeakPathNode{
					ObjectID:  ref.TargetID,
					ClassName: className,
					FieldName: ref.FieldName,
					IsRoot:    false,
				}

				newPath := make([]LeakPathNode, len(curr.path), len(curr.path)+1)
				copy(newPath, curr.path)
				newPath = append(newPath, stepNode)

				queue = append(queue, queueItem{
					id:   ref.TargetID,
					path: newPath,
					root: curr.root,
				})
			}
		}
	}

	return nil, nil
}

// computeRetainedSize calculates the subgraph memory reachable from targetID.
func (ht *HeapTracer) computeRetainedSize(rootID string) int64 {
	visited := make(map[string]bool)
	var total int64

	var dfs func(id string)
	dfs = func(id string) {
		if visited[id] {
			return
		}
		visited[id] = true
		if obj, ok := ht.objects[id]; ok {
			total += obj.ShallowSize
			for _, ref := range obj.References {
				dfs(ref.TargetID)
			}
		}
	}

	dfs(rootID)
	return total
}

func formatReferenceChain(path []LeakPathNode) string {
	var parts []string
	for i, node := range path {
		if i == 0 {
			parts = append(parts, fmt.Sprintf("GC_ROOT(%s: %s [%s])", node.RootType, node.ClassName, node.ObjectID))
		} else {
			parts = append(parts, fmt.Sprintf("--> %s.%s (%s [%s])", path[i-1].ClassName, node.FieldName, node.ClassName, node.ObjectID))
		}
	}
	return strings.Join(parts, "\n\t")
}

// ParseDumpsysMeminfo extracts key Dalvik heap and PSS numbers from `dumpsys meminfo`.
func ParseDumpsysMeminfo(output string) (*MeminfoSnapshot, error) {
	snap := &MeminfoSnapshot{}
	scanner := bufio.NewScanner(strings.NewReader(output))

	pidRe := regexp.MustCompile(`MEMINFO in pid\s+(\d+)\s+\[([^\]]+)\]`)
	dalvikRe := regexp.MustCompile(`(?i)Dalvik Heap\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)`)
	nativeRe := regexp.MustCompile(`(?i)Native Heap\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)`)
	totalRe := regexp.MustCompile(`(?i)TOTAL\s+(\d+)\s+(\d+)\s+(\d+)`)
	viewsRe := regexp.MustCompile(`(?i)Views:\s+(\d+)`)
	activitiesRe := regexp.MustCompile(`(?i)Activities:\s+(\d+)`)
	appContextsRe := regexp.MustCompile(`(?i)AppContexts:\s+(\d+)`)

	for scanner.Scan() {
		line := scanner.Text()

		if m := pidRe.FindStringSubmatch(line); len(m) > 2 {
			snap.PID, _ = strconv.Atoi(m[1])
			snap.PackageName = m[2]
		}

		if m := dalvikRe.FindStringSubmatch(line); len(m) > 7 {
			snap.DalvikHeapPSS, _ = strconv.ParseInt(m[1], 10, 64)
			snap.DalvikHeapAlloc, _ = strconv.ParseInt(m[7], 10, 64)
		}

		if m := nativeRe.FindStringSubmatch(line); len(m) > 7 {
			snap.NativeHeapAlloc, _ = strconv.ParseInt(m[7], 10, 64)
		}

		if m := totalRe.FindStringSubmatch(line); len(m) > 3 {
			snap.TotalPSS, _ = strconv.ParseInt(m[1], 10, 64)
			snap.TotalPrivateDirty, _ = strconv.ParseInt(m[2], 10, 64)
		}

		if m := viewsRe.FindStringSubmatch(line); len(m) > 1 {
			snap.Views, _ = strconv.Atoi(m[1])
		}

		if m := activitiesRe.FindStringSubmatch(line); len(m) > 1 {
			snap.Activities, _ = strconv.Atoi(m[1])
		}

		if m := appContextsRe.FindStringSubmatch(line); len(m) > 1 {
			snap.AppContexts, _ = strconv.Atoi(m[1])
		}
	}

	return snap, nil
}

// DiagnoseLeak maps reference chains to concrete Android leak patterns and remediation recommendations.
func DiagnoseLeak(path []LeakPathNode, rootType GCRootType) (leakType, diagnosis, remediation string) {
	chainStr := ""
	for _, p := range path {
		chainStr += " " + p.ClassName + "." + p.FieldName
	}
	lower := strings.ToLower(chainStr)

	if rootType == RootStaticField || strings.Contains(lower, "companion") || strings.Contains(lower, "static") {
		return "STATIC_REFERENCE",
			"Static variable holds a direct reference to a destroyed Activity context.",
			"Nullify the static reference in Activity.onDestroy() or replace with Application context."
	}
	if strings.Contains(lower, "handler") || strings.Contains(lower, "runnable") || strings.Contains(lower, "message") {
		return "ASYNC_HANDLER",
			"Pending message/runnable in MessageQueue retains outer Activity via implicit $this reference.",
			"Declare Handler as static or removeCallbacksAndMessages(null) in Activity.onDestroy()."
	}
	if strings.Contains(lower, "view") || strings.Contains(lower, "mcontext") {
		return "VIEW_CONTEXT",
			"Detached View holds a reference to destroyed Activity context.",
			"Avoid holding views in long-lived caches or singletons; clear view references in onDestroyView()."
	}
	if strings.Contains(lower, "listener") || strings.Contains(lower, "callback") || strings.Contains(lower, "observer") {
		return "UNREGISTERED_LISTENER",
			"Event listener registered with a system service or bus was never unregistered.",
			"Call unregisterListener() / removeObserver() in Activity.onStop() or onDestroy()."
	}
	if strings.Contains(lower, "coroutine") || strings.Contains(lower, "job") || strings.Contains(lower, "thread") {
		return "BACKGROUND_WORKER",
			"Background task running beyond Activity lifecycle captures Activity reference.",
			"Use activity-scoped lifecycleScope or cancel jobs in onDestroy()."
	}

	return "GC_ROOT_REFERENCE",
		"Destroyed Activity reachable from GC root via reference chain.",
		"Break reference link upon lifecycle teardown."
}

// SimulateMultiCycleRecreations simulates N consecutive rotation/recreation cycles to detect linear leak slopes.
func (ht *HeapTracer) SimulateMultiCycleRecreations(
	className string,
	cycleCount int,
	leakRootID string,
	retainingField string,
) *MultiCycleAnalysisReport {
	var reports []*RecreationCycleReport
	for c := 0; c < cycleCount; c++ {
		oldID := fmt.Sprintf("%s_instance_%d", className, c)
		newID := fmt.Sprintf("%s_instance_%d", className, c+1)
		stack := fmt.Sprintf("android.app.Activity.onDestroy(Activity.java:100)\n\tat %s.onDestroy(%s.kt:50)", className, className)
		rep := ht.SimulateRecreationCycle(className, oldID, newID, c, retainingField, leakRootID, stack)
		reports = append(reports, rep)
	}

	var totalRetained int64
	for _, rep := range reports {
		totalRetained += rep.TotalRetainedHeap
	}

	lastCycleLeaks := 0
	if len(reports) > 0 {
		lastCycleLeaks = len(reports[len(reports)-1].LeaksDetected)
	}

	growthRate := int64(0)
	if cycleCount > 1 {
		growthRate = totalRetained / int64(cycleCount)
	}

	return &MultiCycleAnalysisReport{
		CyclesTested:            cycleCount,
		ActivityClass:           className,
		RetainedInstances:       lastCycleLeaks,
		TotalRetainedBytes:      totalRetained,
		GrowthRateBytesPerCycle: growthRate,
		IsLinearLeak:            lastCycleLeaks >= cycleCount && growthRate > 0,
		Cycles:                  reports,
	}
}

// ParseHPROF parses an Android HPROF memory dump file and populates the HeapTracer graph.
func ParseHPROF(r io.Reader) (*HeapTracer, error) {
	ht := NewHeapTracer()
	br := bufio.NewReader(r)

	// 1. Read header (null-terminated string, e.g. "JAVA PROFILE 1.0.3")
	headerStr, err := br.ReadString(0)
	if err != nil {
		return nil, fmt.Errorf("failed to read HPROF header: %w", err)
	}
	if !strings.HasPrefix(headerStr, "JAVA PROFILE") {
		return nil, fmt.Errorf("invalid HPROF magic: %s", headerStr)
	}

	// 2. Read identifier size (4 or 8)
	var idSize uint32
	if err := binary.Read(br, binary.BigEndian, &idSize); err != nil {
		return nil, fmt.Errorf("failed to read HPROF identifier size: %w", err)
	}
	if idSize != 4 && idSize != 8 {
		idSize = 8 // default to 64-bit ART heap
	}

	// 3. Read high & low timestamp (8 bytes)
	var timeHigh, timeLow uint32
	binary.Read(br, binary.BigEndian, &timeHigh)
	binary.Read(br, binary.BigEndian, &timeLow)

	stringTable := make(map[uint64]string)
	classNames := make(map[uint64]string)

	for {
		tagByte, err := br.ReadByte()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		var timeMicros uint32
		var length uint32
		if err := binary.Read(br, binary.BigEndian, &timeMicros); err != nil {
			break
		}
		if err := binary.Read(br, binary.BigEndian, &length); err != nil {
			break
		}

		recordData := make([]byte, length)
		if _, err := io.ReadFull(br, recordData); err != nil {
			break
		}
		recReader := bytes.NewReader(recordData)

		switch tagByte {
		case 0x01: // STRING
			var strID uint64
			if idSize == 4 {
				var id32 uint32
				binary.Read(recReader, binary.BigEndian, &id32)
				strID = uint64(id32)
			} else {
				binary.Read(recReader, binary.BigEndian, &strID)
			}
			if len(recordData) > int(idSize) {
				stringTable[strID] = string(recordData[idSize:])
			}

		case 0x02: // LOAD CLASS
			var serial, stackSerial uint32
			binary.Read(recReader, binary.BigEndian, &serial)
			if idSize == 4 {
				var c32, n32 uint32
				binary.Read(recReader, binary.BigEndian, &c32)
				binary.Read(recReader, binary.BigEndian, &stackSerial)
				binary.Read(recReader, binary.BigEndian, &n32)
				if name, ok := stringTable[uint64(n32)]; ok {
					classNames[uint64(c32)] = strings.ReplaceAll(name, "/", ".")
				}
			} else {
				var c64, n64 uint64
				binary.Read(recReader, binary.BigEndian, &c64)
				binary.Read(recReader, binary.BigEndian, &stackSerial)
				binary.Read(recReader, binary.BigEndian, &n64)
				if name, ok := stringTable[n64]; ok {
					classNames[c64] = strings.ReplaceAll(name, "/", ".")
				}
			}

		case 0x0C, 0x1C: // HEAP DUMP / HEAP DUMP SEGMENT
			subReader := bytes.NewReader(recordData)
			for subReader.Len() > 0 {
				subTag, err := subReader.ReadByte()
				if err != nil {
					break
				}
				switch subTag {
				case 0x01: // ROOT JNI GLOBAL
					var objID uint64
					if idSize == 4 {
						var id32, j32 uint32
						binary.Read(subReader, binary.BigEndian, &id32)
						binary.Read(subReader, binary.BigEndian, &j32)
						objID = uint64(id32)
					} else {
						var j64 uint64
						binary.Read(subReader, binary.BigEndian, &objID)
						binary.Read(subReader, binary.BigEndian, &j64)
					}
					ht.RegisterObject(DalvikObject{
						ID:          fmt.Sprintf("0x%x", objID),
						ClassName:   "java.lang.Object",
						ShallowSize: 32,
						IsGCRoot:    true,
						RootType:    RootJNI_Global,
						RootDesc:    "JNI Global Reference",
					})

				case 0x05: // ROOT STICKY CLASS
					var objID uint64
					if idSize == 4 {
						var id32 uint32
						binary.Read(subReader, binary.BigEndian, &id32)
						objID = uint64(id32)
					} else {
						binary.Read(subReader, binary.BigEndian, &objID)
					}
					ht.RegisterObject(DalvikObject{
						ID:          fmt.Sprintf("0x%x", objID),
						ClassName:   "java.lang.Class",
						ShallowSize: 64,
						IsGCRoot:    true,
						RootType:    RootSystemClass,
						RootDesc:    "System Sticky Class",
					})

				case 0x21: // INSTANCE DUMP
					var objID, classID uint64
					var stackSerial, bytesFollow uint32
					if idSize == 4 {
						var id32, c32 uint32
						binary.Read(subReader, binary.BigEndian, &id32)
						binary.Read(subReader, binary.BigEndian, &stackSerial)
						binary.Read(subReader, binary.BigEndian, &c32)
						objID = uint64(id32)
						classID = uint64(c32)
					} else {
						binary.Read(subReader, binary.BigEndian, &objID)
						binary.Read(subReader, binary.BigEndian, &stackSerial)
						binary.Read(subReader, binary.BigEndian, &classID)
					}
					binary.Read(subReader, binary.BigEndian, &bytesFollow)
					fieldBytes := make([]byte, bytesFollow)
					subReader.Read(fieldBytes)

					className := classNames[classID]
					if className == "" {
						className = "java.lang.Object"
					}

					ht.RegisterObject(DalvikObject{
						ID:          fmt.Sprintf("0x%x", objID),
						ClassName:   className,
						ShallowSize: int64(bytesFollow) + 16,
					})

				default:
					break
				}
			}
		}
	}

	return ht, nil
}
