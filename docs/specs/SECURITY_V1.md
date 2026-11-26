# Nano-Android Security Specification v1 (P0 Mandatory)

- **Status:** Active Enforcement · **Date:** 2026-11-26

## 1. Trust Boundaries
1. **Untrusted Input Zone:** Any `.apk`, `.jar`, `.aab`, `.apks`, AXML, `resources.arsc`, HTTP request body, or device-originated string (UI node text, SMS, notification, logcat).
2. **Gateway Zone (`nano-studio`, `nano-android-mcp`, `droidforge-mcp`):** Must never execute untrusted bytecode or parse unvalidated archives in-process; enforces AuthN/AuthZ, `Host`/`Origin` checks, canonical path containment, 5-stage `policy.Guard`, and SHA-256 chained audit logging.
3. **Isolated Worker Zone (`nano-worker`):** Executes untrusted translation (`d2j-dex2jar`/`bundletool`) and `HeadlessAppHost` JVM inside OS-enforced containment.

## 2. Worker Sandbox Controls
| Control | Windows Implementation | Linux Implementation |
| :--- | :--- | :--- |
| Process Tree / Fork Bomb | Job Object `JOB_OBJECT_LIMIT_KILL_ON_CLOSE` + `ACTIVE_PROCESS <= 4` | PID namespace + `pids.max` cgroup v2 |
| Memory Ceiling | Job Object `JOB_OBJECT_LIMIT_JOB_MEMORY` + JVM `-Xmx` | cgroups v2 `memory.max` + JVM `-Xmx` (no low `RLIMIT_AS`) |
| CPU Budget | Job Object `CpuRateControl` + timeout kill | cgroups v2 `cpu.max` + timeout kill |
| Environment | Stripped env (`PATH` minimal, zero secrets/tokens) | Cleared env (`env -i`) |
| Filesystem | Per-worker root `cache/workers/<id>/`, read-only framework | Mount namespace (`bwrap`/`nsjail`), read-only root/JDK, writable worker sandbox only |
| Network | Deny by default (sandbox network policy / restricted token) | `--unshare-net` network namespace |

## 3. Archive & Parser Limits (Defaults)
- `MaxArchiveBytes`: 512 MiB
- `MaxEntries`: 50,000
- `MaxEntryUncompressedBytes`: 64 MiB
- `MaxTotalUncompressedBytes`: 1 GiB
- `MaxCompressionRatio`: 100:1 (checked for uncompressed > 1 MiB)
- `MaxClassCount`: 50,000 · `MaxTotalBytecodeBytes`: 256 MiB
- `MaxImageDimension`: 8192 x 8192 (67,108,864 pixels)
