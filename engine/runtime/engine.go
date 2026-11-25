package runtime

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ActivityContext holds the runtime environment and state for an active Activity
type ActivityContext struct {
	mu               sync.RWMutex
	ActivityName     string
	PackageName      string
	State            ActivityState
	Intent           *Intent
	Context          *Context
	Activity         *Activity
	DexClass         *DexClass
	LifecycleHistory []ActivityState
	LaunchDuration   time.Duration
}

// GetState returns the current lifecycle state
func (ac *ActivityContext) GetState() ActivityState {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.State
}

// GetLifecycleHistory returns a snapshot copy of lifecycle transitions
func (ac *ActivityContext) GetLifecycleHistory() []ActivityState {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	res := make([]ActivityState, len(ac.LifecycleHistory))
	copy(res, ac.LifecycleHistory)
	return res
}

// GetIntent returns the launching Intent
func (ac *ActivityContext) GetIntent() *Intent {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.Intent
}

// GetContext returns the associated Android Context
func (ac *ActivityContext) GetContext() *Context {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.Context
}

// GetLaunchDuration returns the wall-clock time elapsed to enter onResume
func (ac *ActivityContext) GetLaunchDuration() time.Duration {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.LaunchDuration
}

// RuntimeController defines the headless Android runtime interface
type RuntimeController interface {
	LaunchActivity(apkPath string, activityName string, extras map[string]any) (*ActivityContext, error)
	StepLooper(deltaMs int64) (remainingMessages int, err error)
	GetLiveActivity() *ActivityContext
	DestroyActivity() error
}

// HeadlessDalvikEngine implements RuntimeController
type HeadlessDalvikEngine struct {
	mu           sync.RWMutex
	registry     *InMemoryClassRegistry
	looper       *VirtualLooper
	liveActivity *ActivityContext
	loadedAPKs   map[string]bool
}

// NewHeadlessDalvikEngine creates an initialized headless Dalvik engine
func NewHeadlessDalvikEngine(opts ...EngineOption) *HeadlessDalvikEngine {
	engine := &HeadlessDalvikEngine{
		registry:   NewInMemoryClassRegistry(),
		looper:     NewVirtualLooper(0),
		loadedAPKs: make(map[string]bool),
	}
	for _, opt := range opts {
		opt(engine)
	}
	return engine
}

// EngineOption allows customizing HeadlessDalvikEngine
type EngineOption func(*HeadlessDalvikEngine)

// WithVirtualLooper injects a custom VirtualLooper
func WithVirtualLooper(looper *VirtualLooper) EngineOption {
	return func(e *HeadlessDalvikEngine) {
		e.looper = looper
	}
}

// WithClassRegistry injects a pre-populated InMemoryClassRegistry
func WithClassRegistry(registry *InMemoryClassRegistry) EngineOption {
	return func(e *HeadlessDalvikEngine) {
		e.registry = registry
	}
}

// LaunchActivity loads the APK (if specified), resolves the activity class,
// initializes Context and Intent, and transitions through onCreate -> onStart -> onResume
func (e *HeadlessDalvikEngine) LaunchActivity(apkPath string, activityName string, extras map[string]any) (*ActivityContext, error) {
	start := time.Now()

	e.mu.Lock()
	defer e.mu.Unlock()

	// 1. Ingest APK if path is provided and not already cached
	if apkPath != "" && !e.loadedAPKs[apkPath] {
		if err := e.registry.LoadAPK(apkPath); err != nil {
			return nil, fmt.Errorf("failed to ingest APK: %w", err)
		}
		e.loadedAPKs[apkPath] = true
	}

	// 2. Resolve target Activity class
	cleanName := strings.TrimSpace(activityName)
	discoveredPackage := ""
	if apkPath != "" {
		if manifest, err := ParseManifestFromAPK(apkPath); err == nil && manifest != nil {
			if manifest.PackageName != "" {
				discoveredPackage = manifest.PackageName
			}
			if cleanName == "" && manifest.LauncherActivity != "" {
				cleanName = manifest.LauncherActivity
			}
		}
	}
	if cleanName == "" {
		cleanName = "MainActivity"
	}

	dexClass, err := e.registry.ResolveClass(cleanName)
	if err != nil {
		// If class is not in registry, synthesize a mock Activity class
		desc := DottedToDescriptor(cleanName)
		dexClass = NewDexClass(desc)
		dexClass.SuperClassName = "Landroid/app/Activity;"
		dexClass.VirtualMethods["onCreate"] = &DexMethod{Name: "onCreate"}
		dexClass.VirtualMethods["onStart"] = &DexMethod{Name: "onStart"}
		dexClass.VirtualMethods["onResume"] = &DexMethod{Name: "onResume"}
		dexClass.VirtualMethods["onPause"] = &DexMethod{Name: "onPause"}
		dexClass.VirtualMethods["onDestroy"] = &DexMethod{Name: "onDestroy"}
		e.registry.RegisterClass(dexClass)
	}

	// 3. Derive package name
	packageName := "com.example.app"
	if discoveredPackage != "" {
		packageName = discoveredPackage
	} else if idx := strings.LastIndex(dexClass.DottedName, "."); idx != -1 {
		packageName = dexClass.DottedName[:idx]
	}

	// 4. Construct Intent & Extras
	intent := NewIntent("android.intent.action.MAIN")
	intent.AddCategory("android.intent.category.LAUNCHER")
	intent.component = dexClass.DottedName
	if extras != nil {
		for k, v := range extras {
			intent.PutExtra(k, v)
		}
	}

	// 5. Build Context and Activity instances
	ctx := NewContext(packageName, e.looper)
	activity := NewActivity()
	activity.Attach(ctx, intent)

	// 6. Execute Activity Lifecycle Transitions deterministically
	// Bundle for savedInstanceState
	savedState := NewBundle()

	// onCreate
	activity.OnCreate(savedState)
	_, _ = e.registry.InvokeMethod(dexClass, "onCreate", activity, savedState)

	// onStart
	activity.OnStart()
	_, _ = e.registry.InvokeMethod(dexClass, "onStart", activity)

	// onResume
	activity.OnResume()
	_, _ = e.registry.InvokeMethod(dexClass, "onResume", activity)

	duration := time.Since(start)

	// 7. Store active context
	actCtx := &ActivityContext{
		ActivityName:     dexClass.DottedName,
		PackageName:      packageName,
		State:            activity.GetState(),
		Intent:           intent,
		Context:          ctx,
		Activity:         activity,
		DexClass:         dexClass,
		LifecycleHistory: activity.GetLifecycleHistory(),
		LaunchDuration:   duration,
	}

	e.liveActivity = actCtx
	return actCtx, nil
}

// StepLooper advances virtual looper by deltaMs and processes all scheduled tasks
func (e *HeadlessDalvikEngine) StepLooper(deltaMs int64) (remainingMessages int, err error) {
	return e.looper.StepLooper(deltaMs)
}

// GetLiveActivity retrieves the current active ActivityContext
func (e *HeadlessDalvikEngine) GetLiveActivity() *ActivityContext {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.liveActivity
}

// DestroyActivity tears down the live activity through onPause -> onStop -> onDestroy
func (e *HeadlessDalvikEngine) DestroyActivity() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.liveActivity == nil {
		return nil
	}

	act := e.liveActivity.Activity
	if act != nil {
		act.OnPause()
		_, _ = e.registry.InvokeMethod(e.liveActivity.DexClass, "onPause", act)

		act.OnStop()
		_, _ = e.registry.InvokeMethod(e.liveActivity.DexClass, "onStop", act)

		act.OnDestroy()
		_, _ = e.registry.InvokeMethod(e.liveActivity.DexClass, "onDestroy", act)

		e.liveActivity.mu.Lock()
		e.liveActivity.State = act.GetState()
		e.liveActivity.LifecycleHistory = act.GetLifecycleHistory()
		e.liveActivity.mu.Unlock()
	}

	e.liveActivity = nil
	return nil
}

// GetRegistry returns the engine's class registry
func (e *HeadlessDalvikEngine) GetRegistry() *InMemoryClassRegistry {
	return e.registry
}

// GetLooper returns the engine's virtual looper
func (e *HeadlessDalvikEngine) GetLooper() *VirtualLooper {
	return e.looper
}
