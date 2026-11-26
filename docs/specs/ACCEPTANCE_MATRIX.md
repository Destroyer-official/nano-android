# Nano-Android Incremental Migration Plan & Acceptance Matrix

- **Status:** Active Baseline · **Date:** 2026-11-26

## Incremental Migration Rules
1. Keep `go.work` green at every commit; introduce `engine/security/` first as a shared Go module imported by `cmd/nano-studio`, `engine/mcp_compiler`, and `droidforge-mcp`.
2. Preserve backwards compatibility for existing MCP tool names (`nano_launch_app`, `nano_observe_screen`, `nano_dispatch_action`, etc.) while adding strict security enforcement underneath.
3. Gate Phase 0 completion on 100% passing red-team security regression tests before expanding Phase 1 runtime APIs.

## Phase 0 Implementation Acceptance Matrix
| Deliverable | Owner Module | Verification Gate |
| :--- | :--- | :--- |
| Studio Auth, Origin/CSRF & Path Containment | `engine/security/httpauth`, `pathcontainment`, `cmd/nano-studio` | Unauthenticated POST `401`, untrusted Origin/Host `403`, path traversal `400/403` |
| Archive Bomb & Zip-Slip Protection | `engine/security/archive` | Deflate bomb, symlink entry, `../` zip-slip, Janus polyglot rejected |
| OS Worker Isolation (Job Object / Namespaces) | `engine/security/worker_isolation`, `engine/mcp_compiler/headless_bridge.go` | Worker process killed on timeout/close, stripped env, isolated temp root |
| Unified Policy, Redaction & Hash-Chained Audit | `engine/security/policy`, `engine/security/audit`, `engine/mcp_compiler` | All MCP calls audited, secrets redacted, metacharacters & scope escapes blocked |
| Java Containment, Permissions & SQLite Rollback | `engine/runtime/nano_framework` | Java security & SQLite transaction rollback tests pass in CI |
