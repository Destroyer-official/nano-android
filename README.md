# Nano-Android & DroidForge MCP

Technical reference for the Nano-Android multi-engine Android analysis, deterministic host-JVM execution runtime, automated testing platform, web workstation (**Nano-Android Studio**), unified CLI (`nano`), and dual MCP server suite (`nano-android-mcp` and `droidforge-mcp`).

---

## 1. System Architecture Overview

The repository is organized as a **22-module Go workspace** (`go.work`, Go `1.27.4`), a pure-Java Android framework micro-runtime (`engine/runtime/nano_framework`, OpenJDK `25.0.5`, **230 `.java` files** across **35 packages**), compiled into `engine/runtime/nano-framework.jar` bundled with **OW2 ASM `9.11.1`**, and **Python `3.12+` analysis sidecars** for media verification and trace parsing.

```text
+-------------------------------------------------------------------------------+
| Interfaces                                                                    |
|  - `nano` CLI (cmd/nano)                                                      |
|  - `nano-android-mcp` (cmd/nano-mcp, JSON-RPC 2.0 stdio, 73 MCP tools)        |
|  - `droidforge-mcp` (droidforge-mcp/cmd/droidforge-mcp, stdio, 26 MCP tools)  |
|  - Nano-Android Studio Web UI (:8088, cmd/nano-studio + studio/frontend)      |
|  - Shared HTTP/WS API (`engine/api`, /api/v1/* + /api/v1/ws)                  |
+---------------------------------------+---------------------------------------+
                                        |
                                        v
+-------------------------------------------------------------------------------+
| Security Governance Layer (`engine/security`)                                 |
|  - httpauth: CSPRNG bearer token, loopback/Host/Origin/CSRF & CSP headers     |
|  - pathcontainment: canonical symlink resolution, ADS/UNC/reserved-name block |
|  - archive: ZIP bomb, zip-slip, ZIP symlink, and Janus polyglot rejection     |
|  - policy: 5-stage validation, metachar block, package lock, <DF_UNTRUSTED_*> |
|  - audit: in-process singleton SHA-256 hash-chained JSONL ledger + redaction  |
|  - worker_isolation: Windows Job Objects / Linux bwrap + sanitized env        |
+---------------------------------------+---------------------------------------+
                                        |
        +-------------------------------+-------------------------------+
        |                               |                               |
        v                               v                               v
+-------------------+         +-------------------+         +-------------------+
| Tier 0 Static     |         | Tier 1 Nano JVM   |         | Tier 2/3 & Cloud  |
| `engine/apk`      |         | `HeadlessAppHost` |         | `droidforge-mcp`  |
|  - Manifest AXML  |         |  - OpenJDK 25 worker        | `engine/backend`  |
|  - DEX Inventory  |         |  - dex2jar / JAR  |         |  - ADB / AVD      |
|  - resources.arsc |         |  - ASM 9.11.1     |         |  - scrcpy / FTL   |
|  - Signing v1-v4  |         |  - sqlite-jdbc    |         |  - Managed Pools  |
|  - Native ELF/16K |         |  - Java2D 1080x2400         |  - Gradle / Perf  |
|  - OWASP MASVS    |         |  - Virtual Looper |         |  - FFprobe/Oracle |
+-------------------+         +-------------------+         +-------------------+
```

### Canonical Execution Tiers & Result Taxonomy

Every execution result explicitly discloses its backend and environment rather than returning an ambiguous binary `PASS`/`FAIL`:

- **Backends:** `static`, `nano_jvm`, `robolectric`, `android_art`, `physical_device`
- **Statuses:** `NANO_SIMULATION_PASS`, `ROBOLECTRIC_PASS`, `ART_EMULATOR_PASS`, `PHYSICAL_DEVICE_PASS`, `FAIL`, `UNSUPPORTED`, `INCONCLUSIVE`, `BLOCKED_POLICY`, `INFRASTRUCTURE_ERROR`

### Honest Technical Scope & Limitations

Nano-Android executes JVM bytecode in a host OpenJDK process against a Java reimplementation of Android APIs; it is **not** an ARM/Linux kernel emulator:

- **Supported directly in `nano_jvm`:** Pure Java/Kotlin `Activity`/`Application`/`Service`/`ContentProvider`/`BroadcastReceiver` lifecycles, `LinearLayout`/`FrameLayout`/`RelativeLayout`/`ConstraintLayout`/`ScrollView`/`HorizontalScrollView`/`RecyclerView`/standard `android.widget.*` views, `Dialog`/`AlertDialog`/`Toast`, Java2D `1080x2400` framebuffer rendering, deterministic `Looper`/`MessageQueue`/`Handler`/`Choreographer`, per-package `SharedPreferences`/`DataStore`, and ACID SQLite via `org.xerial:sqlite-jdbc:3.54.1.0`.
- **Not executed in `nano_jvm`:** Native JNI/C++ libraries (`lib/*.so` ARM64/x86_64), GPU Vulkan/SurfaceView/WebView/Skia rendering, Jetpack Compose runtime rendering, real hardware `MediaCodec`, real thermal sensors, or Pixel ODPM power rails. Native `.so` libraries and Compose symbols are detected during ingestion (`CompatibilityIssues` + `runtimeWarnings`) and routed to `robolectric`, `android_art`, or `physical_device` backends via `droidforge-mcp` and `engine/backend`.
- **JVM bytecode heuristics disclosure:** Whenever `NanoClassLoader` falls back to `Unsafe.allocateInstance` or dynamic interface `Proxy` synthesis (`createDummyInterfaceProxy`), `TransformationAuditLog` records `DEGRADED_HEURISTIC` and `ApiCompatibilityRegistry` logs a compatibility fault rather than reporting a clean pass.
- **Process/Sandbox boundaries:** Windows worker isolation enforces Windows Job Objects (`JOB_OBJECT_LIMIT_KILL_ON_CLOSE`, `ACTIVE_PROCESS`, `JOB_MEMORY`, CPU rate hard cap) and stripped environment variables (`SanitizedEnv`), not Windows AppContainer ACL profiles. Linux worker isolation prefers unprivileged `bubblewrap` (`bwrap` with `--unshare-pid/ipc/uts/net`) and falls back to `setpgid` negative-PID teardown when `bwrap` is absent.

---

## 2. Technology Stack & Pinned Toolchain

Verified against `tools/toolchain.lock.json` via `scripts/verify_toolchain.ps1` and `scripts/verify_toolchain.sh`:

| Component | Version / Artifact | Role |
| :--- | :--- | :--- |
| **Go Workspace** | `1.27.4` (`go.work`) | Core MCP servers, HTTP/WS API, CLI, static analyzers, orchestrator, worker isolation |
| **Java JDK** | OpenJDK `25.0.5` | Host JVM runtime (`nano-framework.jar`), javac compilation, JUnit 5 runner |
| **OW2 ASM** | `9.11.1` (`asm`, `asm-tree`, `asm-analysis`, `asm-commons`, `asm-util`) | Bytecode verification repair, constructor synthesis, abstract strip, Kotlin metadata inspection |
| **SQLite JDBC** | `org.xerial:sqlite-jdbc:3.54.1.0` | Real SQLite engine inside `android.database.sqlite.SQLiteDatabase` (ACID, savepoints, WAL, FKs) |
| **Bundletool** | `bundletool-all-1.18.1.jar` | Isolated `.aab` -> `.apks` universal conversion and split inspection |
| **JUnit Platform** | `junit-platform-console-standalone-1.11.4.jar` | Java security & runtime conformance suites (`tests/security/java`, `tests/conformance/java`) |
| **dex-tools** | `dex-tools-v2.4` (`tools/dex-tools-v2.4/`) | Isolated DEX -> JVM `.jar` translation (`d2j-dex2jar.bat` / `.sh`) with `--force --skip-exceptions` |
| **Python Sidecars** | Python `3.12+` (`droidforge-mcp/sidecar-python`, `engine/profiler_media`) | Trace analysis, PSNR/SSIM/MS-SSIM/VMAF perceptual scoring, ELF 16 KB check |
| **CI/CD** | GitHub Actions (`.github/workflows/ci.yml`, `release.yml`) | Ubuntu/Windows matrix tests + 6-OS release builds, CycloneDX SBOM, Cosign |

---

## 3. Repository & Module Map (22 Go Modules)

| Module Path (`go.work`) | Packages & Responsibilities |
| :--- | :--- |
| `.` (root module `nano_android`) | `cmd/nano` (Unified CLI), `cmd/nano-studio` (Web Studio + auth), `cmd/nano-mcp` (Full 73-tool MCP binary), `cmd/live-runtime-verify`, `cmd/audit-fix-verify`, `cmd/e2e-integration-test`, `internal/phase6hooks`, `studio/frontend` (`go:embed` web UI) |
| `./adapters` | `adapters/frameworks`: Probing & shell-free argument translation for 12 Android/test ecosystem tools |
| `./droidforge-mcp` | Independent ADB/Gradle/scrcpy/Perfetto/MASVS compliance MCP server (`cmd/droidforge-mcp`, `internal/{audit,build,compiler,device,harness,mcp,policy,profiler,types,uitree,vision}`, `schemas`, `redteam`) |
| `./engine/api` | Shared `/api/v1/*` REST service, Phase 5 AI/graph routes, Phase 6 plugin/adapter/pool/report routes, RFC 6455 WebSocket `EventBus` |
| `./engine/apk` | `ingest` (`.apk`, `.jar`, `.aab`, `.apks`, `.xapk`), `manifest` (AXML parser), `dex` (`classes*.dex` parser & merge), `resources` (`resources.arsc` & archive buckets), `signing` (v1/v2/v3/v4 X.509 + `.so` ELF 16 KB audit), `security` (OWASP MASVS v2.1.0 + SARIF export) |
| `./engine/ast_input` | AST compression, UI diffing, Set-of-Marks coordinate mapping |
| `./engine/backend` | `engine/backend/remote`: `DeviceProvider` abstraction (`ADBProvider`, `ManagedPoolProvider`, `FirebaseTestLabProvider`), TTL leases, quotas, parallel shard execution |
| `./engine/compatibility` | `registry` (`compatibility_registry.json`, result status taxonomy, adaptive router) and `intel` (SQLite compatibility DB, scenario quirks, coverage heatmaps) |
| `./engine/intelligence` | `graph` (Application Behavior Graph, dependency/navigation/permission edges, change-impact analysis), `ai` (governed APK understanding, bounded UTG exploration, test generation, crash diagnosis, self-healing selectors, multi-agent coordination, engineering memory, `PENDING_APPROVAL` proposal store), `intelhooks` |
| `./engine/mcp_compiler` | Core MCP JSON-RPC 2.0 server, `IntegratedCoordinator` (`nano_jvm`/`adb_device`), `LiveHeadlessBridge`, `MaestroCompiler`, base + Phase 2/3/5/6 tool registrations (`cmd/main.go` legacy minimal entrypoint) |
| `./engine/media` | `mp4` (ISO BMFF/`mvhd`/`tkhd` box parser), `codec` (`MediaCodec` capability probing), `oracle` (SHA-256 bit-exact & PSNR/SSIM/VMAF lossy oracles) |
| `./engine/network` | `mock`: Localhost configurable mock HTTP server, latency/packet-drop fault injection, request/response capture, traffic timeline |
| `./engine/plugins` | Versioned SDK (`10` `PluginKind` types), `Ed25519` manifest signing & verification, `SHA-256` entrypoint check, isolated subprocess JSON IPC with `limitedBuffer` (2 MB stdout / 64 KB stderr) |
| `./engine/profiler` | `jvm` (heap/GC/classloader/thread metrics), `device` (`adb`/Perfetto/ODPM `PowerMetric`/thermal probes), `crash` (clustering, normalization, R8 retrace), `timeline` (monotonic forensic event timeline) |
| `./engine/profiler_media` | `profiler_media`: Memory telemetry, Android 17 / 16 KB / Play compliance checks, text quarantine sanitizer, Python sidecar invocation |
| `./engine/render` | Host image hashing, layout rendering, and WebP/PNG encoding helpers |
| `./engine/reporting` | 22-field `UnifiedReport`, `Ed25519` + `SHA-256` sign/verify, exporters (`JSON`, `HTML`, `PDF 1.4`, `JUnit XML`, `SARIF v2.1.0`, forensic `.zip` with `CHECKSUMS.txt`), historical `AnalyticsStore` (`ComputeTrends`, `CompareReleases`) |
| `./engine/runtime` | Go legacy Dalvik test shim (`runtime_test.go`) + 230-file Java `nano_framework` (`android.*`, `androidx.*`, `j$.util.*`, `nanoandroid.*`) |
| `./engine/security` | `httpauth`, `pathcontainment`, `archive`, `policy`, `audit`, `worker_isolation`, red-team & Go fuzz tests |
| `./engine/testing` | `suite` (YAML/JSON/DSL runner, retries, flake classification), `orchestrator`, `differential` (11-dimension comparison & earliest divergence), `labs` (12 specialized labs / 23 scenarios), `mcphooks` |
| `./engine/ui` | `accessibility` (48dp touch target, WCAG contrast, text scale, RTL, talkback checks), `profile` (15 device/scenario profiles), `visual` (6 comparison modes, golden baseline approval store, heatmap & overlay PNGs) |
| `./plugins` | `plugins/ide`: Config generators for 7 IDE/MCP clients with XML/JSON escaping |

---

## 4. Binaries, Prerequisites & Build Commands

### Prerequisites

1. **Go:** `1.27.4+`
2. **Java JDK:** OpenJDK `25+` (`javac`, `java`, `jar`)
3. **Toolchain Artifacts (`tools/`):** Verify pinned JAR checksums before building:
   ```powershell
   .\scripts\verify_toolchain.ps1
   ```
   ```bash
   ./scripts/verify_toolchain.sh
   ```
4. **Android SDK `android.jar` & `dex-tools-v2.4`:** Place platform `android.jar` at `engine/runtime/android.jar` and `dex-tools-v2.4` at `tools/dex-tools-v2.4/`.
5. **Optional External Tools (for Tier 2/3 & Media Sidecars):** `adb`, `emulator`, `gradle`, `maestro`, `ffmpeg`/`ffprobe`, `gcloud`, `python` `3.12+`.

### Build `nano-framework.jar` & All Release Binaries (Windows PowerShell)

```powershell
# 1. Compile all 230 Java framework files & bundle ASM 9.11.1 into engine/runtime/nano-framework.jar
.\engine\runtime\rebuild_nano_framework.ps1

# 2. Build all 4 Go binaries into bin/
New-Item -ItemType Directory -Force bin | Out-Null
go build -trimpath -ldflags="-s -w" -o bin/nano.exe ./cmd/nano/main.go
go build -trimpath -ldflags="-s -w" -o bin/nano-studio.exe ./cmd/nano-studio/main.go
go build -trimpath -ldflags="-s -w" -o bin/nano-android-mcp.exe ./cmd/nano-mcp/main.go
Push-Location droidforge-mcp; go build -trimpath -ldflags="-s -w" -o ../bin/droidforge-mcp.exe ./cmd/droidforge-mcp; Pop-Location
```

*(Linux/macOS equivalent uses `./engine/runtime/rebuild_nano_framework.sh` and omits `.exe`.)*

---

## 5. Unified CLI (`nano`)

Entrypoint: `cmd/nano/main.go`. Supports `--json` structured output, workspace path containment (`cache/`, `tools/`, `fixtures/`, `downloads/`), and exit code `2` on policy violations (`BLOCKED_POLICY`) or usage errors.

### All Subcommands

| Category | Subcommands & Key Usage |
| :--- | :--- |
| **Workspace & Diagnostics** | `nano init [--dir=name]` · `nano doctor [--json]` · `nano worker` · `nano policy` · `nano cache <status\|clean>` |
| **Static APK / JAR / AAB Intel** | `nano inspect [--json] <artifact>` · `nano manifest <artifact>` · `nano dex <artifact>` · `nano resources <artifact>` · `nano compatibility <artifact>` · `nano security <artifact>` · `nano diff <base> <target>` |
| **Execution & UI** | `nano launch [--activity=auto] <artifact>` · `nano screenshot [--out=path.png] <artifact>` · `nano inspect-ui` / `nano a11y [--profile=id] <artifact>` · `nano profiles` |
| **Testing & Recording** | `nano test <suite.json>` · `nano run <suite.json>` · `nano replay <flow.yaml>` · `nano record <out.yaml>` · `nano lab <list\|run>` · `nano visual <approve\|compare\|list>` |
| **Profiling, Crash & Media** | `nano profile <artifact>` · `nano benchmark <artifact>` · `nano crash [--log=file]` · `nano media <inspect\|verify>` |
| **Devices & Gradle Build** | `nano devices` · `nano emulator` · `nano build [dir]` |
| **AI & Behavior Graph** | `nano graph <build\|impact\|query>` · `nano ai <understand\|explore\|generate-tests\|diagnose\|self-heal\|proposals\|approve\|memory>` |
| **Plugins, Adapters & Pools** | `nano plugin <list\|keygen\|sign\|verify\|install\|invoke>` · `nano adapter <list\|ide\|translate>` · `nano pool <discover\|health\|reserve\|shard>` |
| **Reporting & Distribution** | `nano export --format=markdown\|sarif\|json <artifact>` · `nano report [--verify=<file.json> \| --format=unified-json\|html\|pdf\|junit\|bundle] <artifact>` · `nano analytics [--base=v1 --target=v2]` · `nano sbom [--out=sbom.json]` |
| **MCP Server Mode** | `nano server` (starts full stdio JSON-RPC 2.0 MCP server with all hook packs attached) |

**Signed Report CLI Verification Example:**

```powershell
.\bin\nano.exe report --format=unified-json fixtures/apks/minimal_valid.apk | Out-File -Encoding utf8 cache\reporting\live_rep.json
.\bin\nano.exe report --verify=cache\reporting\live_rep.json
```
*(UTF-8 BOM `0xEF 0xBB 0xBF` is stripped automatically on `--verify`, `plugin sign`, and `plugin verify`.)*

---

## 6. MCP Servers Reference

Configure in `.mcp.json` or generate IDE-specific files via `nano adapter ide --client=<id>`:

```json
{
  "mcpServers": {
    "nano-android": {
      "command": "bin/nano-android-mcp",
      "args": ["--app", "com.prism.v"]
    }
  }
}
```

Flags on `nano-android-mcp` (`cmd/nano-mcp/main.go`): `--app`, `--log-level` (`DEBUG|INFO|NOTICE|WARNING|ERROR`), `--repeat`, `--mock`, `--version`, `--help`.

### A. `nano-android-mcp` Tools (73 Registered Tools)

- **Core / Backwards-Compatible Runtime Tools:**
  `nano_launch_app`, `nano_observe_screen`, `nano_dispatch_action`, `nano_step_time`, `nano_inspect_state`, `nano_freeze_test`, `nano_assert_ui`, `nano_get_session_flow`, `nano_clear_session`, `nano_doctor`
- **Group 1 — Project & APK Intelligence:**
  `nano_project_discover`, `nano_inspect_apk`, `nano_apk_inspect`, `nano_apk_manifest`, `nano_apk_dex_inspect`, `nano_apk_resource_inspect`, `nano_apk_native_inspect`, `nano_apk_security_audit`, `nano_apk_compatibility`, `nano_apk_compare`
- **Group 2 — Execution & Deterministic Runtime:**
  `nano_runtime_launch`, `nano_runtime_advance_time`, `nano_runtime_capabilities`
- **Group 3 — UI, Accessibility & Visual:**
  `nano_ui_observe`, `nano_ui_tap`, `nano_ui_long_press`, `nano_ui_type`, `nano_ui_swipe`, `nano_ui_back`, `nano_ui_assert`, `nano_ui_accessibility_audit`, `nano_ui_screenshot`, `nano_ui_compare`
- **Group 4 — Test Engineering, 12 Labs & Differential:**
  `nano_test_generate_regression`, `nano_test_get_flow`, `nano_test_clear`, `nano_lab_list`, `nano_lab_run`, `nano_test_differential`
- **Group 5 — Device & Pool Status:**
  `nano_device_list`, `nano_device_status`, `nano_pool_status`, `nano_pool_shard`
- **Group 6 — Profiler, Crash, Media & Compatibility DB:**
  `nano_profile_state`, `nano_profile_memory`, `nano_profile_cpu`, `nano_profile_threads`, `nano_profile_gc`, `nano_profile_performance`, `nano_timeline_inspect`, `nano_crash_analyze`, `nano_media_inspect`, `nano_compatibility_db`
- **Group 7 — Governance, Audit & Security:**
  `nano_policy_inspect`, `nano_security_verify`, `nano_audit_history`
- **Phase 5 — Behavior Graph & Autonomous AI (Budgeted & Approval-Gated):**
  `nano_graph_build`, `nano_graph_query`, `nano_graph_impact`, `nano_ai_understand`, `nano_ai_explore`, `nano_ai_generate_tests`, `nano_ai_diagnose`, `nano_ai_self_heal`, `nano_ai_coordinate`, `nano_ai_memory`, `nano_ai_approve`
- **Phase 6 — Plugins, Adapters, Reporting & Analytics:**
  `nano_plugin_list`, `nano_plugin_invoke`, `nano_adapters_list`, `nano_ide_config`, `nano_report_unified`, `nano_analytics_summary`

### B. `droidforge-mcp` Tools (26 Registered Tools — Live ADB, Gradle, Emulator & Hardware)

Entrypoint: `droidforge-mcp/cmd/droidforge-mcp`. Enforces 5-stage `policy.Guard` allowlisting, package-scope locks, nonced `<DF_UNTRUSTED_*>` output wrapping, and SHA-256 audit logging:

- **Build & Static Compliance:** `project_build`, `project_test_jvm`, `apk_inspect`, `apk_audit_compliance`
- **Devices & Emulators:** `device_list`, `emulator_boot_snapshot`, `device_reset_state`, `app_install_and_launch`, `device_wait_idle`, `device_press_system`, `device_clipboard_and_text`
- **UI & Observation:** `observe_screen`, `act_and_observe`, `app_state_inspect`, `logcat_read`, `crash_report_collect`
- **Test Freezing & AppCards:** `session_freeze_to_maestro`, `test_run_maestro`, `session_export_appcard`
- **Telemetry, Simulation & Governance:** `perf_capture_trace`, `power_thermal_snapshot`, `media_push_and_verify`, `sim_inject_sms_or_call`, `sim_post_notification`, `sim_set_permissions`, `approval_request_status`

---

## 7. Nano-Android Studio (`cmd/nano-studio`) & Shared HTTP/WebSocket API

Start Studio (binds `127.0.0.1:8088` by default, prints one-time bootstrap URL `?token=<csprng_hex>`, fails closed on non-loopback `-host` unless `--allow-remote-auth`, `--tls-cert`, `--tls-key`, and non-empty token are supplied):

```powershell
.\bin\nano-studio.exe -host 127.0.0.1 -port 8088
```

### 19 Workstation Workspaces (`studio/frontend/`)

`Dashboard` · `Project Explorer` · `APK Inspector` · `Runtime Studio` · `Live Device` · `UI Inspector` · `Layout Explorer` · `Visual Lab` · `Test Lab (12 Labs)` · `Differential Lab` · `AI Lab (Governance)` · `Profiler & Timeline` · `Network Inspector` · `Database Inspector` · `Security Center` · `Device Manager` · `Build Center` · `Reports` · `Settings`

Interactive UX includes a docked `1080x2400` live framebuffer viewport (tap, pointer drag-to-swipe, wheel scroll, AST bounding-box overlays, coordinate readout, Back/Type buttons), **Search Everywhere (`Ctrl+K`)**, refresh (`F5`), dark/light theme toggle, compact density mode, and an interactive bottom command drawer (`health`, `devices`, `labs`, `policies`, `workers`, `git`).

### HTTP REST & WebSocket Endpoints (`engine/api` & `cmd/nano-studio`)

All endpoints require `Authorization: Bearer <token>` (or `HttpOnly` session cookie) plus `Host` / `Origin` loopback enforcement (and explicit `Origin` check on WebSocket upgrade):

- **Core Studio:** `GET /`, `/static/*`, `/api/apps`, `/api/observe`, `/api/screen.png`, `/api/inspect`, `/api/layouts`, `/api/devices`, `/api/files`, `/api/export_maestro`, `/api/security`; `POST /api/upload`, `/api/launch`, `/api/action`, `/api/step`, `/api/preview_layout`, `/api/stage_file`, `/api/mode`
- **Platform `/api/v1/*` & WS:**
  - `GET /api/v1/health`, `/api/v1/projects`, `/api/v1/apks`, `/api/v1/apks/{id}/inspect`, `/api/v1/apks/{id}/compatibility`
  - `GET/POST /api/v1/executions`, `/api/v1/executions/{id}`, `/api/v1/executions/{id}/events`, `/api/v1/executions/{id}/snapshot`, `/api/v1/executions/{id}/restore`, `/api/v1/executions/{id}/process_death`, `/api/v1/executions/{id}/breakpoint`
  - `GET/POST /api/v1/tests`, `/api/v1/tests/{id}/run`
  - `GET /api/v1/reports`, `/api/v1/reports/{id}?format=html|pdf|junit|sarif|zip`
  - `GET/POST /api/v1/visual/baselines`, `/api/v1/visual/approve`, `/api/v1/visual/compare`
  - `GET /api/v1/devices`, `/api/v1/workers`, `/api/v1/policies`, `/api/v1/git`, `WS /api/v1/ws`
- **Phase 5 AI & Behavior Graph:**
  - `POST /api/v1/graph/build`, `/api/v1/graph/query` (GET/POST), `/api/v1/graph/impact`
  - `POST /api/v1/ai/understand`, `/api/v1/ai/explore`, `/api/v1/ai/generate_tests`, `/api/v1/ai/coordinate`, `/api/v1/ai/diagnose`, `/api/v1/ai/self_heal`, `GET /api/v1/ai/proposals`, `POST /api/v1/ai/proposals/approve`, `GET/POST /api/v1/ai/memory`
- **Phase 6 Extensibility, Pools & Signed Reporting:**
  - `GET /api/v1/plugins`, `POST /api/v1/plugins/install`, `POST /api/v1/plugins/invoke`
  - `GET /api/v1/adapters` (`?client=<ide>`)
  - `GET /api/v1/pools`, `POST /api/v1/pools/reserve`, `POST /api/v1/pools/release`, `POST /api/v1/pools/shard`
  - `POST /api/v1/reports/unified`, `POST /api/v1/reports/verify`, `GET /api/v1/analytics`

---

## 8. Specialized Engines Reference

- **12 Test Laboratories (`engine/testing/labs`):** `crash`, `permission`, `network`, `storage`, `visual`, `media`, `accessibility`, `compatibility`, `stress`, `security`, `differential`, `performance`.
- **23 Covered Stress Scenarios:** `low_memory`, `low_storage`, `slow_cpu`, `slow_network`, `disconnected_network`, `lossy_network`, `battery_saver`, `permission_denied`, `process_kill`, `background_restriction`, `live_app_update`, `db_corruption`, `clock_change`, `timezone_change`, `locale_change`, `font_scale_change`, `orientation_change`, `window_resize`, `app_switching`, `notification_interruption`, `concurrent_app_activity`, `repeated_launch_kill`, `soak_stability`.
- **15 Reusable Device & Scenario Profiles (`engine/ui/profile`):** `clean_install`, `existing_app_data`, `no_network`, `slow_network`, `permission_denied`, `low_storage`, `low_memory`, `battery_saver`, `dark_mode`, `large_font`, `small_screen`, `landscape`, `rtl_arabic`, `api_26`, `api_37`.
- **11 Differential Comparison Dimensions (`engine/testing/differential`):** `activity_lifecycle`, `navigation_stack`, `ui_hierarchy`, `visible_texts`, `widget_properties`, `screenshots`, `sqlite_state`, `preferences_datastore`, `network_requests`, `exceptions`, `callback_ordering` — isolated per target via `SandboxRootOverride`.
- **6 Visual Regression Modes (`engine/ui/visual`):** `pixel_exact`, `tolerant_pixel`, `structural`, `semantic`, `text`, `hybrid`, generating Side-by-Side, Difference Heatmap, and Overlay Blend PNG artifacts gated by human approval (`ApprovedBy`).
- **10 Plugin SDK Kinds (`engine/plugins`):** `api_shim`, `apk_analyzer`, `resource_parser`, `rendering_engine`, `test_reporter`, `ai_agent`, `device_provider`, `benchmark_engine`, `security_scanner`, `media_analyzer` — `Ed25519` + `SHA-256` verified and executed in scrubbed subprocesses with bounded buffers.
- **7 IDE Config Generators (`plugins/ide`):** `android_studio`, `intellij_idea`, `vscode`, `cursor`, `antigravity`, `claude_code`, `generic_mcp`.
- **12 Ecosystem Framework Adapters (`adapters/frameworks`):** `maestro`, `androidx_test`, `junit4`, `junit5`, `espresso`, `uiautomator`, `robolectric`, `gradle`, `adb`, `perfetto`, `ffmpeg`, `bundletool`.
- **35 Java Micro-Framework Packages (`engine/runtime/nano_framework/src`):**
  `android.{animation,app,appwidget,content,content.pm,content.res,database,database.sqlite,graphics,graphics.drawable,graphics.drawable.shapes,location,media,net,net.ssl,os,provider,text,text.format,text.style,util,view,view.animation,widget,window}`, `androidx.{appcompat.widget,cardview.widget,constraintlayout.widget,core.content,datastore.preferences.core}`, `j$.util{,.function,.stream}`, `nanoandroid.{compat,host}`.

---

## 9. Full Verification Suite

Run every automated gate locally (Windows PowerShell):

```powershell
# 1. Pinned toolchain SHA-256 check
.\scripts\verify_toolchain.ps1

# 2. Rebuild nano-framework.jar & run JUnit 5 security + conformance suite (11 tests)
.\engine\runtime\rebuild_nano_framework.ps1
New-Item -ItemType Directory -Force build\test-classes | Out-Null
javac -cp "engine\runtime\nano-framework.jar;engine\runtime\android.jar;tools\libs\sqlite-jdbc-3.54.1.0.jar;tools\libs\junit-platform-console-standalone-1.11.4.jar" `
  -d build\test-classes `
  tests\security\java\Phase0SecurityAndSQLiteTest.java `
  tests\conformance\java\Phase1RuntimeConformanceTest.java `
  tests\conformance\java\Phase3TelemetryAndCompatDbTest.java
java --enable-native-access=ALL-UNNAMED -jar tools\libs\junit-platform-console-standalone-1.11.4.jar `
  execute --class-path "build\test-classes;engine\runtime\nano-framework.jar;engine\runtime\android.jar;tools\libs\sqlite-jdbc-3.54.1.0.jar" `
  --scan-class-path

# 3. Go race tests & go vet across all 22 modules
$mods = @(".", "adapters", "droidforge-mcp", "engine/api", "engine/apk", "engine/ast_input", "engine/backend", "engine/compatibility", "engine/intelligence", "engine/mcp_compiler", "engine/media", "engine/network", "engine/plugins", "engine/profiler", "engine/profiler_media", "engine/render", "engine/reporting", "engine/runtime", "engine/security", "engine/testing", "engine/ui", "plugins")
foreach ($m in $mods) {
    Push-Location $m
    go test -race -count=1 ./...
    if ($LASTEXITCODE -ne 0) { Pop-Location; exit $LASTEXITCODE }
    go vet ./...
    if ($LASTEXITCODE -ne 0) { Pop-Location; exit $LASTEXITCODE }
    Pop-Location
}

# 4. Python sidecar tests
Push-Location droidforge-mcp\sidecar-python; python -m unittest -v test_sidecar.py; Pop-Location
Push-Location engine\profiler_media; python -m unittest -v test_media_sidecar.py; Pop-Location

# 5. Live OpenJDK micro-runtime verification
go run ./cmd/live-runtime-verify/main.go
```

---

## 10. License & Security Policy

- License: [LICENSE](LICENSE)
- Security & Threat Model: [SECURITY.md](SECURITY.md) & [docs/specs/SECURITY_V1.md](docs/specs/SECURITY_V1.md)
- Architecture & Specs: [docs/specs/](docs/specs/)
