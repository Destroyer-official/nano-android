---
name: nano-android
description: Autonomous zero-emulator Android APK testing, layout rendering, and Maestro test freezing using the Nano-Android headless micro-runtime.
---

# Nano-Android Playbook: 2027–2030+ Autonomous Android APK Testing

Use this skill when you need to test, run, inspect, or fix Android APKs without requiring physical devices, USB cables, or slow 15GB QEMU emulators.

---

## 1. Quick Reference: The 10 MCP Tools

| Tool | Purpose | Key Arguments |
| :--- | :--- | :--- |
| `nano_launch_app` | Boots real APK bytecode directly on OpenJDK 25 in < 300 ms. | `apk_path` (string), `activity` (string, optional) |
| `nano_observe_screen` | Extracts compact semantic AST (<200 tokens) with bounding boxes and text. | `include_screenshot` (bool, default false) |
| `nano_dispatch_action` | Dispatches real `click`, `type`, `swipe`, or `back` actions, executing developer listeners. | `action` ("click" / "type" / "back"), `node_id` or coordinates |
| `nano_step_time` | Advances virtual Looper & Handler message queues deterministically in 0 ms CPU wait. | `delta_ms` (integer, e.g. 300) |
| `nano_assert_ui` | Verifies visibility, text content, or accessibility state of any element. | `target` (string), `expected_text` (string) |
| `nano_freeze_test` | Compiles interactive test session into a versioned Maestro YAML regression test. | `test_name` (string), `repeat_count` (integer, default 3) |
| `nano_inspect_state` | Queries in-memory Dalvik heap, allocated objects, 16KB ELF compliance, and views. | `target` ("all", "heap", "memory", "view_tree") |
| `nano_get_session_flow` | Returns chronological trace of all executed actions and screen transitions. | None |
| `nano_clear_session` | Resets session state, flushes looper queues, and restarts sandbox. | None |
| `nano_doctor` | Diagnostic health check across runtime, AST engine, profiler, and ADB bridge. | None |

---

## 2. Standard 4-Step Autonomous Dev & Test Loop

### Step 1: Launch Application
```json
{
  "tool": "nano_launch_app",
  "arguments": {
    "apk_path": "release/prism-v-v1.2.3.apk",
    "activity": "com.prismv.ui.MainActivity"
  }
}
```
*Note:* If `activity` is omitted, Nano-Android automatically inspects `AndroidManifest.xml` and resolves the launcher activity.

### Step 2: Observe Semantic AST
```json
{
  "tool": "nano_observe_screen",
  "arguments": {
    "include_screenshot": false
  }
}
```
Always observe with `include_screenshot: false` first for maximum token efficiency (<200 tokens). Request screenshots only for visual rendering, Canvas views, or video verification.

### Step 3: Dispatch Action & Step Time
```json
{
  "tool": "nano_dispatch_action",
  "arguments": {
    "action": "click",
    "node_id": "Duo"
  }
}
```
Follow every click with:
```json
{
  "tool": "nano_step_time",
  "arguments": {
    "delta_ms": 300
  }
}
```
This flushes Looper message queues, coroutines, and transition animations deterministically without thread sleep.

### Step 4: Freeze Verified Regression Test
```json
{
  "tool": "nano_freeze_test",
  "arguments": {
    "test_name": "duo_deck_navigation_regression",
    "repeat_count": 3
  }
}
```
This saves an executable, deterministic Maestro YAML regression flow so the team and CI never pay token costs to re-verify the bug fix.
