// Package policy implements the deterministic 5-stage security guard:
// argument validation, device trust, package scope lock, budget/circuit breaker,
// and nonce fencing for untrusted device text.
// OWNER: Worker 2
package policy
