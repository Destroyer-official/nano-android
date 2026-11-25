# Nano-Android

Zero-emulator Android APK execution runtime, Go MCP server, and live browser testing studio.

Nano-Android translates Android `.apk` DEX bytecode to standard JVM `.class` bytecode, decodes compiled AAPT2 binary resources (`AndroidManifest.xml`, `resources.arsc`, binary `res/layout/*.xml`), and executes pure-JVM Android `Activity` and `View` hierarchies directly on a host OpenJDK JVM with a 1080x2400 Java2D framebuffer, deterministic Looper clock, persistent sandbox storage, and an optional ADB device backend.

---

## Why This Exists

Automated Android testing typically forces an awkward trade-off:
1. **Heavy QEMU/AVD Emulators:** Require multi-gigabyte system images, nested virtualization / hypervisor setup, slow cold boots, and brittle polling sleeps for async animations.
2. **Pure Static / Vision-Only Wrappers:** Treat apps as screenshots or XML templates without executing the app's actual Java/Kotlin state machine, listeners, dialogs, or database writes.

Nano-Android takes a third approach: run the real compiled Java/Kotlin APK code in-process on OpenJDK 25 against a pure-Java reimplementation of the Android SDK surface (`nano_framework`), extract an accessible semantic View AST (`~500–2000` tokens instead of a vision-model image), and render the live 1080x2400 View tree via Java2D whenever visual inspection is needed. When an app depends on native ARM/C++ code (`.so` JNI) or Android system services that cannot run on host JVM, the coordinator switches cleanly to an attached ADB device without changing tool schemas.

---

## End-to-End Architecture

```text
   ┌────────────────────────────────────────────────────────────────────┐
   │  AI Agent (MCP stdio)  <─── JSON-RPC 2.0 ───>  nano-android-mcp    │
   │  Human / Browser UI    <─── HTTP / PNG   ───>  nano-studio (:8088) │
   └───────────────────────────────────┬────────────────────────────────┘
                                       │
                                       ▼
                        IntegratedCoordinator (Go)
                   ┌───────────────────┴───────────────────┐
                   │                                       │
           mode=nano_jvm                            mode=adb_device
                   │                                       │
                   ▼                                       ▼
         LiveHeadlessBridge                       `adb` CLI Wrapper
      (stdin/stdout JSON IPC +                  (uiautomator dump +
      dex2jar SHA-256 caching)                   screencap + input)
                   │
                   ▼
     HeadlessAppHost (OpenJDK 25 JVM)
   ┌──────────────────────────────────────────────────────────────┐
   │ 1. ApkResourceTable  -> AndroidManifest, resources.arsc, PNG │
   │ 2. BinaryXmlInflater -> Inflates binary AXML res/layout XML  │
   │ 3. NanoClassLoader   -> ASM 9.5 verification repair & hooks  │
   │ 4. Android Micro-SDK -> Activity stack, View measure/layout, │
   │                         Dialog/AlertDialog, Toast, SQLite,   │
   │                         SharedPreferences, File Picker, MP4  │
   │ 5. Java2D Renderer   -> 1080x2400 ARGB PNG framebuffer       │
   └──────────────────────────────────────────────────────────────┘
```

### How an APK Executes Without an Emulator

1. **DEX to JVM Translation (`engine/mcp_compiler/headless_bridge.go`)**
   - Given a `.apk` path, `LiveHeadlessBridge.PrepareArtifacts` hashes the APK (`SHA-256` prefix) and checks `cache/<hash>_classes.jar`.
   - On cache miss, it extracts `classes*.dex` and converts them via `dex2jar` (`d2j-dex2jar.bat` / `d2j-dex2jar.sh`), using `--force --skip-exceptions` fallback if any single method fails translation.
   - Standalone `.jar` targets skip `dex2jar` and boot directly.

2. **Bytecode Instrumentation (`engine/runtime/nano_framework/src/nanoandroid/host/NanoClassLoader.java`)**
   - Dalvik bytecode compiled back to JVM `.class` files via R8/D8 + `dex2jar` often triggers JVM `VerifyError` or missing-superclass failures on host OpenJDK (for example, uninitialized object flow mismatches, R8 interface/type merging, or AndroidX `View` subclasses missing a matching `(Context, AttributeSet)` constructor).
   - `NanoClassLoader` runs ASM 9.5 (`asm-9.5.jar`, `asm-tree-9.5.jar`, `asm-analysis-9.5.jar`) over loaded classes before `defineClass`:
     - Repairs stack/local verification type mismatches (`insertCheckcastAtStackOffset`, `repairMethodTypesWithAnalyzer`).
     - Synthesizes missing forwarding constructors (`injectForwardingInit`) when subclasses invoke constructors omitted by stripped super stubs.
     - Falls back safely to `sun.misc.Unsafe.allocateInstance` + recursive View/ViewBinding field population when custom third-party views fail standard instantiation.
     - Installs runtime hooks (`onStartActivityHook`, `onStartActivityForResultHook`, `onFinishActivityHook`) to intercept intents cleanly without an Android binder daemon.

3. **Binary Resource Decoding (`ApkResourceTable.java`, `BinaryXmlInflater.java`)**
   - Pure-Java ZIP/binary chunk parsers decode `AndroidManifest.xml` (package name, `Application` class, theme IDs, exported activities, launcher intent filter) and `resources.arsc` (including AAPT2 sparse/offset16 resource type chunks, string pools, dimension/color values, and theme style attributes).
   - `BinaryXmlInflater` parses compiled `res/layout/*.xml` binary AXML and builds live `android.view.View` / `ViewGroup` instances, resolving `@string/...`, `@color/...`, `@drawable/...`, margins, paddings, layout weights, and text styling.

4. **Measure, Layout, Event Loop & Render (`HeadlessAppHost.java`)**
   - Boots `Application.onCreate()`, then pushes the target `Activity` onto a live `Deque<ActivityFrame>` stack and runs `attach()`, `onCreate(Bundle)`, `onStart()`, `onResume()`.
   - Advances asynchronous tasks cleanly via deterministic `SystemClock.advanceUptimeMillis(deltaMs)` + `Looper.getMainLooper().getQueue()` polling (`STEP <ms>`), eliminating real-time wall-clock flakiness.
   - Runs two-pass Android layout (`measure(EXACTLY 1080, EXACTLY 2264)` -> `layout(0, 68, 1080, 2332)`) and renders the `View` tree, active `Dialog`/`AlertDialog` overlays (with dark scrim), and `Toast` notifications onto a 1080x2400 `BufferedImage` (`RENDER <path>`).
   - Dispatches `click`/`tap` (hit-testing top dialog first, bubbling up to clickable parents or falling back to `SeekBar` scrubbing / `MotionEvent` sequences), `type` (firing `TextWatcher` & `OnEditorActionListener`), `scroll`/`swipe` (directional or `x1,y1->x2,y2` `MotionEvent` sequences + `ScrollView`/`HorizontalScrollView`), `keyevent`, and `back`.

---

## Functional Runtime Subsystems (`nano_framework`)

Instead of stubbing UI setters to no-ops, `engine/runtime/nano_framework/src/android/` implements stateful subsystems:

- **Per-Package Disk Sandbox:** `Context.getFilesDir()`, `getCacheDir()`, `getDatabasePath()`, `getExternalFilesDir()`, `getExternalCacheDir()`, `getDataDir()`, and `Environment.getExternalStorageDirectory()` resolve to writable directories under `cache/sandbox/<package>/`.
- **Persistent `SharedPreferences`:** `SharedPreferences.MemorySharedPreferences` reads and writes typed key/value pairs (`String`, `Set<String>`, `int`, `long`, `float`, `boolean`) to `cache/sandbox/<package>/shared_prefs/<name>.properties` on `commit()`/`apply()`.
- **Pure-Java `SQLiteDatabase` CRUD Engine:** Supports `CREATE TABLE`, `ALTER TABLE ... ADD COLUMN`, `DROP TABLE`, `CREATE [UNIQUE] INDEX`, `PRAGMA table_info/index_list/index_info/foreign_key_list`, `ContentValues` `insert`/`insertOrThrow`/`replace`/`update`/`delete`, `SQLiteStatement` parameter binding, and `SELECT`/`query`/`rawQuery` with `WHERE` (`=`, `!=`, `<>`, `<`, `<=`, `>`, `>=`, `LIKE`, `IS [NOT] NULL`, `AND`), `ORDER BY`, `LIMIT`, and `COUNT(*)`. Non-`:memory:` databases persist to `cache/sandbox/<package>/databases/<name>`.
- **Multi-Activity Navigation Stack:** `startActivity`, `startActivityForResult`, `setResult`, `finish()`, and `back` push/pop `ActivityFrame` entries and deliver `onActivityResult(requestCode, resultCode, data)` back to callers.
- **Interactive File/Media Picker & MP4 Probe:** Intercepts `ACTION_OPEN_DOCUMENT`, `ACTION_GET_CONTENT`, `ACTION_PICK`, `android.provider.action.PICK_IMAGES`, `ACTION_OPEN_DOCUMENT_TREE`, and `ACTION_CREATE_DOCUMENT`, presenting a live `AlertDialog` file selector over `cache/staged_files/` (auto-seeding `sample_1080p_clip.mp4` and `sample_frame.png` if empty). `ContentResolver` serves `query()` (`_display_name`, `_size`, `mime_type`, `_data`), `openInputStream()`, `openOutputStream()`, and `ParcelFileDescriptor`. `MediaMetadataRetriever` includes a lightweight ISOBMFF/MP4 atom parser (`mvhd`/`tkhd`) to extract actual duration, video width/height, and computed bitrate from staged MP4s.

---

## Honest Capabilities and Known Limitations

Nano-Android is a user-space Java framework runtime, not a Linux kernel or ARM emulator. Understanding where the boundary lies is essential:

| Workload Type | Behavior on `nano_jvm` Mode | Notes |
|---|---|---|
| Pure JVM Java/Kotlin UI (`Activity`, `LinearLayout`, `FrameLayout`, `ScrollView`, `HorizontalScrollView`, `TextView`, `Button`, `EditText`, `CheckBox`, `Switch`, `SeekBar`, `AlertDialog`, `Dialog`, `Toast`) | **Executes live logic, layout, rendering & persistence** | Verified interactively with real app flows (e.g., Prism-V file picker -> `MediaMetadataRetriever` metadata update -> AI Copilot dialog -> navigation tabs). |
| Compiled AXML layouts (`res/layout/*.xml`) in third-party APKs | **Inflates & renders visually** | Even when an Activity fails startup, `HeadlessAppHost` falls back to static AXML preview (`preview_layout` API / Inspector) and exposes exact JVM stack traces honestly. |
| Native JNI / C++ libraries (`lib/*.so` ARM64/x86) | **Does not execute on host JVM** | `ApkResourceTable` detects and counts `.so` libs and emits an explicit runtime warning; `adb_device` mode should be used for JNI/codec/FFmpeg paths. |
| Jetpack Compose / SurfaceView / Vulkan / WebView GPU rendering | **Cannot execute Skia/Vulkan native UI** | Requires `adb_device` execution or static layout inspection. |
| Unimplemented Android SDK / Material / OEM classes or complex R8 obfuscation | **Partial startup or `ClassNotFoundException` / `NoSuchMethodError` / `VerifyError`** | Example observed in testing: `SimpleCalendar` attaches its `CoordinatorLayout` + `MyFloatingActionButton` (`New Event`), while `MaterialToolbar` initialization halts on an unimplemented `android.graphics.PointF` constructor (`Stub!`) in `android.jar`. |

---

## Repository Layout

```text
.
├── cmd/nano-studio/              # Web Studio HTTP server + single-page interactive UI (:8088)
├── engine/
│   ├── ast_input/                # Semantic AST pruning, raycasting, key/touch synthesizers, diffs
│   ├── mcp_compiler/             # MCP JSON-RPC 2.0 server, tool registry, coordinator, Maestro compiler
│   │   ├── cmd/                  # `nano-android-mcp` CLI binary entrypoint
│   │   ├── headless_bridge.go    # Manages `HeadlessAppHost` JVM subprocess + dex2jar cache
│   │   ├── integrated_coordinator.go # Dual-mode orchestrator (`nano_jvm` and `adb_device`)
│   │   └── maestro_compiler.go   # Compiles session actions into `repeat: 3` Maestro YAML
│   ├── profiler_media/           # Memory telemetry, APK compliance checks, media verification helpers
│   ├── render/                   # Host layout/rasterizer/webp helpers
│   └── runtime/
│       ├── nano_framework/src/   # Pure-Java Android SDK surface + `nanoandroid.host.*` runtime
│       └── rebuild_nano_framework.ps1 # Compiles 186 Java files + bundles ASM 9.5 into nano-framework.jar
├── droidforge-mcp/               # Companion DroidForge ADB/device/emulator/audit Go MCP module
├── flows/                        # Saved Maestro YAML flows
└── go.work                       # Go workspace linking all 6 Go modules
```

---

## Nano-Android Studio (`cmd/nano-studio`)

Running `nano-studio` starts a 3-column local web console (`http://localhost:8088`):

- **Left Column (Live Viewport):** 1080x2400 live APK framebuffer with element bounding-box hover inspection, click-to-tap, pointer drag-to-swipe (`dist >= 18px` dispatches `swipe(start_x, start_y -> end_x, end_y)`), mouse-wheel scroll, text input box (`Enter` submits), `📎 Stage File` uploader (adds media/files to `cache/staged_files/` for Android file pickers), and Back/Up/Down/`+300ms` Looper controls.
- **Middle Column (What the AI Agent Sees):** Live searchable View AST (`clickable-only` filter + node token estimate), raw JSON payload, and a compiled AXML Layout Explorer listing all `res/layout/*.xml` entries from `resources.arsc`.
- **Right Column (Diagnostics, Action Trace & Maestro Export):** Activity stack path, active `Dialog`/`Toast` state, staged sandbox files, JVM heap & loaded class telemetry, explicit `Honest Runtime Diagnostic Trace` stack traces on errors, action log, and downloadable `repeat: 3` Maestro YAML.

### HTTP API Endpoints

| Endpoint | Method | Purpose |
|---|---|---|
| `/` | `GET` | Serves Studio web UI |
| `/api/apps` | `GET` | Discovers local `.apk`/`.jar` files (`cache/uploads`, `downloads`, `tools`) |
| `/api/upload` | `POST` | Multipart `.apk`/`.jar` upload + immediate install & boot |
| `/api/launch` | `POST` | Boots `{ "app": "...", "activity": "auto", "clear_state": false }` |
| `/api/observe` | `GET` | Returns full snapshot (`observe`, `inspect`, `layouts`, `staged_files`, `actions`) |
| `/api/screen.png` | `GET` | Streams current 1080x2400 PNG framebuffer (`no-store`) |
| `/api/action` | `POST` | Dispatches `tap`, `type`, `scroll`, `swipe`, `keyevent`, or `back` |
| `/api/step` | `POST` | Advances deterministic virtual Looper (`{ "delta_ms": 300 }`) |
| `/api/stage_file` | `POST` | Stages multipart `file` or JSON `path` into `cache/staged_files/` |
| `/api/files` | `GET` | Lists staged sandbox input files |
| `/api/layouts` / `/api/preview_layout` | `GET` / `POST` | Lists or inflates static binary AXML layouts from `resources.arsc` |
| `/api/devices` / `/api/mode` | `GET` / `POST` | Probes ADB devices / AVDs and toggles `nano_jvm` vs `adb_device` |
| `/api/export_maestro` | `GET` | Downloads recorded session as Maestro YAML (`repeat: 3`) |

---

## MCP Server & Tool Reference (`nano-android-mcp`)

The MCP server speaks **JSON-RPC 2.0** over `stdin`/`stdout` (line-delimited, protocol versions supported in `engine/mcp_compiler/server.go`).

### Registered Tools

- `nano_launch_app` — Boots an APK/JAR into the sandbox (`apk_path`, optional `activity`, `clear_state`).
- `nano_observe_screen` — Returns pruned AST tree (`AppID`, `CurrentScreen`, `ActiveActivity`, `ActivityStack`, `ActiveDialog`, `Toast`, visible texts, bounds) and optional base64 PNG/WebP screenshot.
- `nano_dispatch_action` — Dispatches `tap`, `double_tap`, `long_press`, `type`, `erase_text`, `press_key`, `swipe`, `scroll`, `back` by `node_id`, text, direction, or coordinates with optional `assert_visible`.
- `nano_step_time` — Advances virtual Looper time by `delta_ms`.
- `nano_inspect_state` — Returns JVM heap allocation (`used_mb`, `free_mb`, `max_mb`), GC count, loaded class count, view count, and virtual uptime.
- `nano_freeze_test` — Compiles the recorded session actions into a deterministic Maestro YAML flow with `repeat_count` (default `3`).
- `nano_assert_ui` — Asserts text/ID presence, absence, or exact text checks against the active UI AST.
- `nano_get_session_flow` — Returns actions recorded in the current session.
- `nano_clear_session` — Clears recorded session actions.
- `nano_doctor` — Checks ADB/JDK/bridge toolchain availability and execution readiness.

---

## Prerequisites & Building from Source

### Prerequisites
- **Go:** `1.26+` (workspace uses Go `1.27.4` in `go.work`).
- **Java JDK:** OpenJDK 25+ (`javac`, `java`, `jar`).
- **Third-Party Runtime Dependencies (not tracked in Git):**
  1. Place an Android SDK platform `android.jar` (e.g., API 34–36) at `engine/runtime/android.jar`.
  2. Place `dex-tools-v2.4` (including `d2j-dex2jar` and `lib/asm-9.5.jar`, `asm-tree-9.5.jar`, `asm-analysis-9.5.jar`) at `tools/dex-tools-v2.4/`.

### Build Commands (Windows PowerShell)

1. **Build `engine/runtime/nano-framework.jar`**
   ```powershell
   .\engine\runtime\rebuild_nano_framework.ps1
   ```

2. **Run Go Unit & Integration Tests**
   ```powershell
   Push-Location engine\mcp_compiler
   go test -count=1 -v ./...
   Pop-Location
   ```

3. **Build Binaries**
   ```powershell
   go build -o bin\nano-studio.exe ./cmd/nano-studio/main.go
   go build -o bin\nano-android-mcp.exe ./engine/mcp_compiler/cmd
   ```

### Running

- **Start Studio Web UI:**
  ```powershell
  .\bin\nano-studio.exe -port 8088
  ```
  Then open `http://localhost:8088`.

- **Run MCP Server (stdio):**
  ```powershell
  .\bin\nano-android-mcp.exe --app com.prism.v --log-level INFO
  ```
  Flags supported by `cmd/main.go`: `--app`, `--log-level`, `--repeat`, `--mock`, `--version`, `--help`.

- **Direct `HeadlessAppHost` CLI (stdin IPC):**
  ```powershell
  java -cp "engine\runtime\nano-framework.jar;engine\runtime\android.jar" nanoandroid.host.HeadlessAppHost
  ```
  Commands: `LAUNCH <jarOrApkPath> [activity] [apkPath]`, `OBSERVE`, `RENDER <pngPath>`, `ACTION <click|tap|type|scroll|swipe|keyevent|back> [target] [extra]`, `STEP <ms>`, `STAGE_FILE <path>`, `LIST_STAGED`, `INSPECT`, `LIST_LAYOUTS`, `PREVIEW_LAYOUT <nameOrId>`, `EXIT`.
