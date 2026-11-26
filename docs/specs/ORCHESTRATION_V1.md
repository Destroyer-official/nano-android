# Nano-Android Test Orchestration Specification v1

- **Status:** Active Baseline · **Date:** 2026-11-26

## 1. Entry Points & Shared Governance
All entry points (`nano` CLI, `nano-studio` HTTP/WS, `nano-android-mcp`, `droidforge-mcp`) route through a unified security policy gate (`engine/security/policy`) and immutable hash-chained audit ledger (`engine/security/audit`).

## 2. Job Lifecycle & Adaptive Routing
1. `INGEST` -> Archive validation (`engine/security/archive`) + SHA-256 provenance.
2. `POLICY_CHECK` -> AuthN/AuthZ, path containment, package-scope lock, human approval check.
3. `ROUTE` -> Explicit `--mode` override or Adaptive Router (`nano_jvm` vs `robolectric` vs `android_art` vs `physical_device`).
4. `ISOLATE & EXECUTE` -> Isolated worker pool (`engine/security/worker_isolation`) with CPU/RAM/disk/timeout quotas.
5. `REPORT` -> Explicit canonical status, environment disclosure, nonce-fenced untrusted outputs, signed JSON/JUnit/SARIF/Maestro artifacts.
