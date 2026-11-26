# Nano-Android Runtime Compatibility Specification v1

- **Status:** Active Baseline · **Date:** 2026-11-26

## 1. Compatibility Classification Taxonomy
Every `nano_framework` class and method carries one explicit classification:
- `FULLY_IMPLEMENTED`: Behavioral contract verified equivalent to Android ART (API 26–37).
- `BEHAVIORAL_DIFF`: Functional with documented differences (e.g., Java2D typography metrics, simulated network fixtures).
- `PARTIAL`: Subset of methods/flags implemented; unsupported paths raise explicit diagnostics.
- `STUBBED`: Deterministic placeholder; **must never silently report `NANO_SIMULATION_PASS` when depended upon**.
- `UNSUPPORTED` / `REQUIRES_ART` / `REQUIRES_HARDWARE`: Immediately flags `CompatibilityFault` with exact API, caller, stack trace, fallback backend (`android_art` or `physical_device`), and test validity (`INVALIDATED` vs `DEGRADED_WITH_WARNING`).

## 2. Core Subsystem Behavioral Contracts
- `android.content.Context`: Canonical sandbox path containment (`files/`, `cache/`, `databases/`, `external_files/`), stateful per-package `PermissionStateRegistry` (`PERMISSION_GRANTED` / `PERMISSION_DENIED`), explicit broadcast dispatch (`RECEIVER_EXPORTED`/`NOT_EXPORTED`).
- `android.database.sqlite.SQLiteDatabase`: Real ACID transaction semantics (`beginTransaction` -> `setTransactionSuccessful` -> `endTransaction` commits; missing `setTransactionSuccessful` rolls back; nested `SAVEPOINT` support; constraints & FKs).
- `android.app.Activity`: Lifecycle state machine (`CREATED` -> `STARTED` -> `RESUMED` -> `PAUSED` -> `STOPPED` -> `DESTROYED`), navigation stack, `onActivityResult`, `onSaveInstanceState`.
- `android.os.Looper` & `SystemClock`: Virtual deterministic clock (`advanceUptimeMillis`), ordered `MessageQueue` execution.
