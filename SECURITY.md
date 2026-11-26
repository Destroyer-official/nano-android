# Security Policy — Nano-Android & DroidForge

## Threat Model & Trust Boundaries

Nano-Android executes untrusted Android APK/JAR bytecode, parses untrusted binary archives (`AndroidManifest.xml`, `resources.arsc`, `classes*.dex`, images), exposes local HTTP/WebSocket interfaces (`nano-studio`), and bridges AI agent tool calls (`nano-android-mcp` and `droidforge-mcp`).

### Mandatory P0 Security Controls
1. **Loopback & Authenticated Transport:**
   - `nano-studio` binds to `127.0.0.1` by default and requires CSPRNG per-boot bearer authentication (or `HttpOnly; SameSite=Strict` session cookie) for state-mutating APIs.
   - Non-loopback binding (`0.0.0.0`) fails closed unless explicit auth and TLS/remote authorization flags are provided.
   - `Host` and `Origin` headers are validated against strict allowlists to prevent DNS rebinding and CSRF.
2. **OS-Enforced Worker Isolation (`engine/security/worker_isolation`):**
   - Untrusted JVM workers and archive translators execute inside isolated child processes governed by OS primitives:
     - **Windows:** Job Objects (`KILL_ON_CLOSE`, active process cap, memory ceiling, CPU rate limit) + restricted environment + scoped sandbox paths.
     - **Linux:** Unprivileged namespaces / `bwrap` / `nsjail` + cgroups v2 memory/CPU/PID ceilings + deny-by-default network (`--unshare-net`).
   - Hostile third-party APKs should additionally be executed in a disposable VM/container (`--isolation-profile=hostile`).
3. **Canonical Path & Symlink Containment:**
   - All host filesystem inputs (`/api/launch`, `/api/stage_file`, MCP file arguments) and in-runtime Java file APIs (`Context.openFileInput`, `openFileOutput`, `getDatabasePath`, `ContentResolver`) enforce canonical path resolution inside explicit root boundaries.
4. **Archive Bomb & Resource Exhaustion Protection:**
   - All `.apk`, `.jar`, `.aab`, and `.apks` inputs undergo pre-ingestion checks for compressed size, entry count, per-entry uncompressed size, cumulative uncompressed size, compression ratio, zip-slip traversal, and symlink entries.
5. **Unified Policy & Prompt-Injection Quarantine:**
   - Device/APK-derived strings (UI text, SMS, notifications, logcat) are stripped of zero-width/bidirectional characters and wrapped in cryptographic-nonce fences (`<DF_UNTRUSTED_{nonce}>`).
   - All state-changing tool and API calls pass through unified policy validation and are recorded in a SHA-256 hash-chained, secret-redacted audit ledger.

## Reporting a Vulnerability
Do not open public issues for unpatched security vulnerabilities. Report privately to the repository maintainers with reproduction steps, affected commit SHA, and impact scope.
