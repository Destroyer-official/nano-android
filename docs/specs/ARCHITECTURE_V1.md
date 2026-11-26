# Nano-Android Architecture Specification v1

- **Status:** Active P0 Baseline · **Date:** 2026-11-26
- **Workspace:** `d:\code\Main_projects\android_mcp` (`go.work` Go `1.27.4`, OpenJDK `25.0.5`)

## 1. Execution Tiers & Result Semantics
Every execution returns an explicit canonical status (`NANO_SIMULATION_PASS`, `ROBOLECTRIC_PASS`, `ART_EMULATOR_PASS`, `PHYSICAL_DEVICE_PASS`, `FAIL`, `UNSUPPORTED`, `INCONCLUSIVE`, `BLOCKED_POLICY`, `INFRASTRUCTURE_ERROR`) plus `execution_backend`, `framework_version`, `api_profile`, and `reproduced_behaviors[]`.

| Tier | Backend | Scope | Isolation Boundary |
| :--- | :--- | :--- | :--- |
| Tier 0 | `static` | APK/AAB/APKS/DEX/AXML/ELF/OWASP static analysis | Pre-validated archive reader + worker pool |
| Tier 1 | `nano_jvm` | Deterministic headless JVM execution (`nano_framework`) | OS-isolated `nano-worker` (Job Object / namespaces, no network, contained sandbox) |
| Tier 1B | `robolectric` | Independent Robolectric + Compose UI / Roborazzi JVM tests | Gradle isolated test JVM |
| Tier 2 | `android_art` | Local headless AVD / Redroid / Cuttlefish over ADB + `scrcpy-server` | Hypervisor VM + package-scope lock |
| Tier 3 | `physical_device` | USB / TLS-wireless real phones (MediaCodec, ODPM, thermal, OEM) | Package-scope lock + human approval gate |

## 2. Module Boundaries
- `engine/security/` — Shared Go security kernel (`policy`, `audit`, `archive`, `pathcontainment`, `worker_isolation`, `httpauth`, `redaction`).
- `engine/apk/` — Ingestion, manifest, DEX, resources, native ELF, signing, compatibility pre-scan.
- `engine/runtime/` — `nano_framework` Java runtime, `NanoClassLoader` (ASM bytecode instrumentation), deterministic `Looper`/`SystemClock`, real SQLite (`sqlite-jdbc` / strict transaction engine), contained storage.
- `engine/mcp_compiler/` & `droidforge-mcp/` — Unified MCP tool registry + DroidForge ADB/Gradle/compliance/vision layer sharing `engine/security/`.
- `cmd/nano-studio/` — Authenticated HTTP/WebSocket workstation gateway delegating to isolated workers.
