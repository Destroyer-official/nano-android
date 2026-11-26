# Nano-Android Conformance Test Specification v1

- **Status:** Active Baseline · **Date:** 2026-11-26

## 1. Dual-Engine Conformance Rule
Every implemented `nano_framework` subsystem (`Context` containment, permission state, `SQLiteDatabase` ACID rollback/savepoints, `Activity` lifecycle stack, `Looper` ordering, `Intent` resolution, `ContentResolver` containment) must have:
1. **Tier 1 Java Unit & Conformance Tests** executed in CI on every commit.
2. **Tier 2 ART Reference Equivalency Suite** verifying identical assertions on `android_art` (headless API 37 AVD) and emitting explicit differential reports on any behavioral divergence.

## 2. Canonical Test Result Statuses
`NANO_SIMULATION_PASS` | `ROBOLECTRIC_PASS` | `ART_EMULATOR_PASS` | `PHYSICAL_DEVICE_PASS` | `FAIL` | `UNSUPPORTED` | `INCONCLUSIVE` | `BLOCKED_POLICY` | `INFRASTRUCTURE_ERROR`.
