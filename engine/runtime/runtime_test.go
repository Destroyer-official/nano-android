package runtime

import (
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLaunchActivitySpeedAndLifecycle(t *testing.T) {
	// 1. Create a synthetic test APK with a valid classes.dex
	tempDir := t.TempDir()
	apkPath := filepath.Join(tempDir, "test_app.apk")

	err := CreateTestAPK(apkPath, SyntheticClassSpec{
		Descriptor: "Lcom/example/nano/MainActivity;",
		SuperClass: "Landroid/app/Activity;",
		Methods:    []string{"onCreate", "onStart", "onResume", "onPause", "onDestroy"},
	})
	if err != nil {
		t.Fatalf("Failed to create test APK: %v", err)
	}

	// 2. Instantiate engine
	engine := NewHeadlessDalvikEngine()

	// Register a method hook to verify Dalvik method interception
	hookCalled := false
	engine.GetRegistry().RegisterHook("com.example.nano.MainActivity", "onResume", func(instance any, args ...any) (any, error) {
		hookCalled = true
		return nil, nil
	})

	// 3. Launch Activity with extras
	extras := map[string]any{
		"session_id": "sess_12345",
		"debug_mode": true,
		"counter":    42,
	}

	start := time.Now()
	actCtx, err := engine.LaunchActivity(apkPath, "com.example.nano.MainActivity", extras)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("LaunchActivity failed: %v", err)
	}

	// Requirement: Activity successfully enters onResume() in < 35ms
	if elapsed >= 35*time.Millisecond {
		t.Errorf("LaunchActivity took %v, which exceeds the required < 35ms limit", elapsed)
	} else {
		t.Logf("PASS: LaunchActivity completed in %v (< 35ms requirement)", elapsed)
	}

	// Assert onResume state
	if actCtx.GetState() != StateResumed {
		t.Fatalf("Expected state %s, got %s", StateResumed, actCtx.GetState())
	}

	// Assert lifecycle history
	expectedHistory := []ActivityState{StateCreated, StateStarted, StateResumed}
	history := actCtx.GetLifecycleHistory()
	if len(history) != len(expectedHistory) {
		t.Fatalf("Expected lifecycle history len %d, got %d: %v", len(expectedHistory), len(history), history)
	}
	for i, exp := range expectedHistory {
		if history[i] != exp {
			t.Errorf("Lifecycle step %d: expected %s, got %s", i, exp, history[i])
		}
	}

	// Assert Intent & Extras
	intent := actCtx.GetIntent()
	if intent == nil {
		t.Fatal("Intent is nil")
	}
	if intent.GetStringExtra("session_id") != "sess_12345" {
		t.Errorf("Expected session_id sess_12345, got %s", intent.GetStringExtra("session_id"))
	}
	if !intent.GetBooleanExtra("debug_mode", false) {
		t.Errorf("Expected debug_mode true, got false")
	}
	if intent.GetIntExtra("counter", 0) != 42 {
		t.Errorf("Expected counter 42, got %d", intent.GetIntExtra("counter", 0))
	}

	// Assert hook called
	if !hookCalled {
		t.Error("Expected onResume method hook to be invoked")
	}

	// Assert GetLiveActivity
	live := engine.GetLiveActivity()
	if live == nil || live != actCtx {
		t.Error("GetLiveActivity did not return the active activity context")
	}

	// 4. Test DestroyActivity lifecycle teardown
	err = engine.DestroyActivity()
	if err != nil {
		t.Fatalf("DestroyActivity failed: %v", err)
	}
	if actCtx.GetState() != StateDestroyed {
		t.Fatalf("Expected state %s after destruction, got %s", StateDestroyed, actCtx.GetState())
	}
	if engine.GetLiveActivity() != nil {
		t.Fatal("Expected live activity to be nil after DestroyActivity()")
	}
}

func TestStepLooperExecution(t *testing.T) {
	// Requirement: StepLooper(500) correctly executes scheduled Runnables
	engine := NewHeadlessDalvikEngine()
	looper := engine.GetLooper()
	handler := NewHandler(looper)

	var (
		r100Executed atomic.Bool
		r250Executed atomic.Bool
		r500Executed atomic.Bool
		r750Executed atomic.Bool
		executionSeq []string
		seqMu        sync.Mutex
	)

	record := func(name string) {
		seqMu.Lock()
		defer seqMu.Unlock()
		executionSeq = append(executionSeq, name)
	}

	// Schedule 4 tasks at distinct virtual delays
	handler.PostDelayed(func() {
		r100Executed.Store(true)
		record("R100")
	}, 100)

	handler.PostDelayed(func() {
		r250Executed.Store(true)
		record("R250")
	}, 250)

	handler.PostDelayed(func() {
		r500Executed.Store(true)
		record("R500")
	}, 500)

	handler.PostDelayed(func() {
		r750Executed.Store(true)
		record("R750")
	}, 750)

	// Before stepping: initial queue should have 4 messages
	if looper.GetQueue().Count() != 4 {
		t.Fatalf("Expected 4 queued messages, got %d", looper.GetQueue().Count())
	}

	// Step by 50ms: none should have executed
	remaining, err := engine.StepLooper(50)
	if err != nil {
		t.Fatalf("StepLooper(50) failed: %v", err)
	}
	if remaining != 4 || r100Executed.Load() {
		t.Fatalf("Expected 4 remaining at 50ms, got %d", remaining)
	}

	// Advance remaining 450ms so total advanced is 500ms
	remaining, err = engine.StepLooper(450)
	if err != nil {
		t.Fatalf("StepLooper(450) failed: %v", err)
	}

	// At 500ms: R100, R250, and R500 MUST have executed; R750 must NOT have executed
	if !r100Executed.Load() {
		t.Error("R100 runnable was not executed by 500ms mark")
	}
	if !r250Executed.Load() {
		t.Error("R250 runnable was not executed by 500ms mark")
	}
	if !r500Executed.Load() {
		t.Error("R500 runnable was not executed by 500ms mark")
	}
	if r750Executed.Load() {
		t.Error("R750 runnable should not have executed at 500ms")
	}
	if remaining != 1 {
		t.Errorf("Expected 1 remaining message (R750), got %d", remaining)
	}

	// Verify deterministic sequence
	seqMu.Lock()
	if len(executionSeq) != 3 || executionSeq[0] != "R100" || executionSeq[1] != "R250" || executionSeq[2] != "R500" {
		t.Errorf("Unexpected execution order at 500ms: %v", executionSeq)
	}
	seqMu.Unlock()

	// Step another 300ms (reaching virtual uptime 800ms)
	remaining, err = engine.StepLooper(300)
	if err != nil {
		t.Fatalf("StepLooper(300) failed: %v", err)
	}
	if !r750Executed.Load() {
		t.Error("R750 runnable was not executed at 800ms mark")
	}
	if remaining != 0 {
		t.Errorf("Expected 0 remaining messages, got %d", remaining)
	}
}

func TestNestedLooperScheduling(t *testing.T) {
	looper := NewVirtualLooper(0)
	handler := NewHandler(looper)

	var outerRan, innerRan bool
	var innerExecTime int64

	handler.PostDelayed(func() {
		outerRan = true
		// Schedule nested runnable during execution of outer runnable
		handler.PostDelayed(func() {
			innerRan = true
			innerExecTime = looper.UptimeMillis()
		}, 100)
	}, 200)

	// Step 250ms -> outer runs at 200ms, inner is scheduled for 300ms
	_ = looper.Step(250)
	if !outerRan {
		t.Fatal("Outer runnable should have executed at 200ms")
	}
	if innerRan {
		t.Fatal("Inner runnable should not have executed at 250ms")
	}

	// Step another 100ms (total 350ms) -> inner runs at 300ms
	_ = looper.Step(100)
	if !innerRan {
		t.Fatal("Inner runnable should have executed by 350ms")
	}
	if innerExecTime != 300 {
		t.Errorf("Expected innerExecTime 300ms, got %d", innerExecTime)
	}
}

func TestChoreographerFrameCallbacks(t *testing.T) {
	looper := NewVirtualLooper(0)
	frameExecuted := false
	var frameNanos int64

	looper.PostFrameCallback(func(nanos int64) {
		frameExecuted = true
		frameNanos = nanos
	})

	_ = looper.Step(16)
	if !frameExecuted {
		t.Fatal("Frame callback was not triggered on step")
	}
	expectedNanos := int64(16 * 1_000_000)
	if frameNanos != expectedNanos {
		t.Errorf("Expected frameNanos %d, got %d", expectedNanos, frameNanos)
	}
}

func TestDexLoaderDirectBytesAndResolution(t *testing.T) {
	dexBytes, err := BuildSyntheticDEX(
		SyntheticClassSpec{
			Descriptor: "Lcom/test/FirstActivity;",
			SuperClass: "Landroid/app/Activity;",
			Methods:    []string{"onCreate", "onResume"},
		},
		SyntheticClassSpec{
			Descriptor: "Lcom/test/SecondActivity;",
			SuperClass: "Landroid/app/Activity;",
			Methods:    []string{"onCreate", "onPause"},
		},
	)
	if err != nil {
		t.Fatalf("BuildSyntheticDEX failed: %v", err)
	}

	registry := NewInMemoryClassRegistry()
	err = registry.LoadDEXBytes(dexBytes)
	if err != nil {
		t.Fatalf("LoadDEXBytes failed: %v", err)
	}

	// Test resolution by descriptor
	c1, err := registry.ResolveClass("Lcom/test/FirstActivity;")
	if err != nil || c1 == nil {
		t.Fatalf("Failed to resolve Lcom/test/FirstActivity;: %v", err)
	}

	// Test resolution by dotted name
	c2, err := registry.ResolveClass("com.test.SecondActivity")
	if err != nil || c2 == nil {
		t.Fatalf("Failed to resolve com.test.SecondActivity: %v", err)
	}

	// Test resolution by simple name
	c3, err := registry.ResolveClass("FirstActivity")
	if err != nil || c3 == nil {
		t.Fatalf("Failed to resolve FirstActivity by short name: %v", err)
	}

	// Test instance allocation & field hooks
	c1.InstanceFields["mCount"] = &DexField{Name: "mCount", Type: "I"}
	inst := NewDexInstance(c1)
	err = registry.SetField(inst, "mCount", int32(99))
	if err != nil {
		t.Fatalf("SetField failed: %v", err)
	}
	val, err := registry.GetField(inst, "mCount")
	if err != nil || val != int32(99) {
		t.Fatalf("GetField failed, expected 99, got %v (err: %v)", val, err)
	}
}

func TestFrameworkShims(t *testing.T) {
	// 1. Context and SharedPreferences
	looper := NewVirtualLooper(100)
	ctx := NewContext("com.example.mock", looper)

	if ctx.GetPackageName() != "com.example.mock" {
		t.Errorf("Unexpected package name: %s", ctx.GetPackageName())
	}

	prefs := ctx.GetSharedPreferences("app_settings", 0)
	prefs.Edit().
		PutString("api_url", "https://api.nano.internal").
		PutInt("timeout_ms", 5000).
		PutBoolean("use_ssl", true).
		Apply()

	if prefs.GetString("api_url", "") != "https://api.nano.internal" {
		t.Errorf("SharedPreferences getString failed: %s", prefs.GetString("api_url", ""))
	}
	if prefs.GetInt("timeout_ms", 0) != 5000 {
		t.Errorf("SharedPreferences getInt failed: %d", prefs.GetInt("timeout_ms", 0))
	}
	if !prefs.GetBoolean("use_ssl", false) {
		t.Errorf("SharedPreferences getBoolean failed")
	}

	// Test removal
	prefs.Edit().Remove("api_url").Commit()
	if prefs.Contains("api_url") {
		t.Errorf("Expected api_url to be removed")
	}

	// 2. Resources
	res := ctx.GetResources()
	res.SetString("app_name", "NanoAndroid")
	if res.GetString("app_name") != "NanoAndroid" {
		t.Errorf("Resources getString failed: %s", res.GetString("app_name"))
	}

	// 3. SystemClock
	clock := NewSystemClock(looper)
	if clock.UptimeMillis() != 100 {
		t.Errorf("Expected clock 100ms, got %d", clock.UptimeMillis())
	}
	_ = clock.Sleep(200)
	if clock.UptimeMillis() != 300 {
		t.Errorf("Expected clock 300ms after sleep, got %d", clock.UptimeMillis())
	}
}

func TestConcurrentRaceSafety(t *testing.T) {
	// Verifies concurrency safety under `go test -race`
	looper := NewVirtualLooper(0)
	handler := NewHandler(looper)

	var wg sync.WaitGroup
	const numGoroutines = 10
	const numMessages = 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(gId int) {
			defer wg.Done()
			for m := 0; m < numMessages; m++ {
				delay := int64((gId * 10) + m)
				handler.PostDelayed(func() {}, delay)
			}
		}(i)
	}

	wg.Wait()

	// Step looper through entire range
	err := looper.Step(1000)
	if err != nil {
		t.Fatalf("Concurrent Step failed: %v", err)
	}
}

func TestJavaCompilation(t *testing.T) {
	// Verify that host OpenJDK 25 javac compiles the Java stubs cleanly
	javacPath, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac not found on host PATH, skipping Java compilation check")
	}

	tempOut := t.TempDir()
	cmd := exec.Command(
		javacPath,
		"-d", tempOut,
		"stubs/Bundle.java",
		"stubs/Intent.java",
		"stubs/Resources.java",
		"stubs/SharedPreferences.java",
		"stubs/Message.java",
		"stubs/MessageQueue.java",
		"stubs/Looper.java",
		"stubs/Handler.java",
		"stubs/SystemClock.java",
		"stubs/Context.java",
		"stubs/View.java",
		"stubs/ViewGroup.java",
		"stubs/Activity.java",
		"NanoLooper.java",
		"DexClassLoader.java",
		"NanoHost.java",
	)
	cmd.Dir = "."
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("javac compilation failed: %v\nOutput:\n%s", err, string(output))
	}

	t.Log("PASS: OpenJDK 25 successfully compiled all Java stubs and classes without errors")
}

func TestBytecodeInterpreterArithmeticAndBranches(t *testing.T) {
	registry := NewInMemoryClassRegistry()
	interp := NewDalvikInterpreter(registry)

	// Dalvik bytecode:
	// const/4 v0, 5          -> 0x5012 (v0 = 5)
	// const/4 v1, 5          -> 0x5112 (v1 = 5)
	// add-int v2, v0, v1     -> 0x0290, 0x0100 (v2 = 10)
	// mul-int/lit8 v2, v2, 3 -> 0x03da, 0x0202 (v2 = 30)
	// return v2              -> 0x020f
	method := &DexMethod{
		Name: "compute",
		CodeItem: &DexCodeItem{
			RegistersSize: 4,
			InsSize:       0,
			Insns: []uint16{
				0x5012,         // const/4 v0, 5
				0x5112,         // const/4 v1, 5
				0x0290, 0x0100, // add-int v2, v0, v1
				0x02da, 0x0302, // mul-int/lit8 v2, v2, #+3
				0x020f,         // return v2
			},
		},
	}

	result, err := interp.Execute(method, nil)
	if err != nil {
		t.Fatalf("Interpreter failed: %v", err)
	}

	expected := 30
	if toInt(result) != expected {
		t.Fatalf("Expected interpreter result %d, got %v", expected, result)
	}
	t.Logf("PASS: Dalvik bytecode interpreter executed successfully with result: %v", result)
}

func TestAndroidManifestParsingAndLauncherAutoDetect(t *testing.T) {
	tempDir := t.TempDir()
	apkPath := filepath.Join(tempDir, "auto_launcher.apk")

	err := CreateTestAPKWithManifest(
		apkPath,
		"com.example.production",
		"com.example.production.SplashActivity",
		SyntheticClassSpec{
			Descriptor: "Lcom/example/production/SplashActivity;",
			SuperClass: "Landroid/app/Activity;",
			Methods:    []string{"onCreate", "onStart", "onResume"},
		},
	)
	if err != nil {
		t.Fatalf("Failed to create APK with manifest: %v", err)
	}

	engine := NewHeadlessDalvikEngine()
	// Pass empty activityName -> engine should auto-discover SplashActivity from manifest
	actCtx, err := engine.LaunchActivity(apkPath, "", nil)
	if err != nil {
		t.Fatalf("LaunchActivity with auto-discovery failed: %v", err)
	}

	if actCtx.ActivityName != "com.example.production.SplashActivity" {
		t.Errorf("Expected auto-discovered SplashActivity, got %s", actCtx.ActivityName)
	}
	if actCtx.PackageName != "com.example.production" {
		t.Errorf("Expected package com.example.production, got %s", actCtx.PackageName)
	}
	if actCtx.GetState() != StateResumed {
		t.Errorf("Expected RESUMED state, got %s", actCtx.GetState())
	}
	t.Logf("PASS: Auto-detected launcher activity %s in %v", actCtx.ActivityName, actCtx.GetLaunchDuration())
}

func TestViewHierarchyAndClickEvents(t *testing.T) {
	ctx := NewContext("com.example.viewtest", NewVirtualLooper(0))
	act := NewActivity()
	act.Attach(ctx, NewIntent("android.intent.action.MAIN"))

	// Create UI hierarchy:
	// root (ViewGroup)
	//   ├── TextView (id=101, text="Initial")
	//   └── Button (id=102, text="Click Me")
	root := NewViewGroup(ctx, 100)
	tv := NewTextView(ctx, 101)
	tv.SetText("Initial")

	btn := NewButton(ctx, 102)
	btn.SetOnClickListener(func(v *View) {
		tv.SetText("Clicked Successfully!")
	})

	root.AddView(tv.View)
	root.AddView(btn.View)
	act.SetContentView(root.View)

	// Assert root retrieval & find by ID
	foundTv := act.FindViewById(101)
	if foundTv == nil {
		t.Fatal("Could not find TextView by ID 101")
	}

	foundBtn := act.FindViewById(102)
	if foundBtn == nil {
		t.Fatal("Could not find Button by ID 102")
	}

	// Trigger simulated click
	clicked := foundBtn.PerformClick()
	if !clicked {
		t.Fatal("PerformClick returned false")
	}

	// Verify text mutation
	if tv.GetText() != "Clicked Successfully!" {
		t.Fatalf("Expected text 'Clicked Successfully!', got '%s'", tv.GetText())
	}
	t.Log("PASS: View hierarchy and click listener dispatched successfully")
}

func TestIdleHandlersAndTokenCancellation(t *testing.T) {
	looper := NewVirtualLooper(0)
	handler := NewHandler(looper)

	idleRan := false
	looper.GetQueue().AddIdleHandler(func() bool {
		idleRan = true
		return false // run once
	})

	token := "special_task_token"
	cancelledRan := false
	handler.SendMessageDelayed(&Message{
		Target: handler,
		Obj:    token,
		Callback: func() {
			cancelledRan = true
		},
	}, 100)

	// Cancel by token
	handler.RemoveCallbacksAndMessages(token)

	// Step forward
	_ = looper.Step(200)

	if cancelledRan {
		t.Fatal("Cancelled message should not have run")
	}
	if !idleRan {
		t.Fatal("IdleHandler should have run when queue became idle")
	}
	t.Log("PASS: Token cancellation and IdleHandlers verified")
}

func TestVirtualHandlerThread(t *testing.T) {
	ht := NewVirtualHandlerThread("BackgroundWorker", 0)
	bgLooper := ht.GetLooper()
	bgHandler := NewHandler(bgLooper)

	bgRan := false
	bgHandler.PostDelayed(func() {
		bgRan = true
	}, 150)

	_ = ht.Step(200)
	if !bgRan {
		t.Fatal("Background handler runnable did not run on VirtualHandlerThread")
	}
	ht.Quit()
	t.Log("PASS: VirtualHandlerThread deterministic execution verified")
}

func BenchmarkLaunchActivity(b *testing.B) {
	tempDir := b.TempDir()
	apkPath := filepath.Join(tempDir, "bench_app.apk")
	_ = CreateTestAPK(apkPath, SyntheticClassSpec{
		Descriptor: "Lcom/example/bench/BenchActivity;",
		SuperClass: "Landroid/app/Activity;",
		Methods:    []string{"onCreate", "onStart", "onResume", "onDestroy"},
	})

	engine := NewHeadlessDalvikEngine()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := engine.LaunchActivity(apkPath, "com.example.bench.BenchActivity", nil)
		if err != nil {
			b.Fatal(err)
		}
		_ = engine.DestroyActivity()
	}
}

func BenchmarkStepLooper(b *testing.B) {
	engine := NewHeadlessDalvikEngine()
	handler := NewHandler(engine.GetLooper())

	for i := 0; i < 1000; i++ {
		handler.PostDelayed(func() {}, int64(i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.StepLooper(1)
	}
}

func BenchmarkBytecodeInterpreter(b *testing.B) {
	registry := NewInMemoryClassRegistry()
	interp := NewDalvikInterpreter(registry)
	method := &DexMethod{
		Name: "loop",
		CodeItem: &DexCodeItem{
			RegistersSize: 3,
			Insns: []uint16{
				0x1012,         // const/4 v0, 1
				0x2112,         // const/4 v1, 2
				0x0290, 0x0100, // add-int v2, v0, v1
				0x020f,         // return v2
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = interp.Execute(method, nil)
	}
}
