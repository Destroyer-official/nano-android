# 15-Bug Seeded Recall Benchmark Specification

## 1. Overview & Objective

The **15-Bug Seeded Recall Benchmark** is an empirical evaluation harness designed to objectively measure an AI agent's ability to autonomously detect, diagnose, and remediate real-world Android defects using **DroidForge MCP** without human guidance or hints.

### Target Recall Goals
- **Milestone 4 (Day 30):** $\ge 75\%$ autonomous recall (at least 11 / 15 bugs caught and root-caused).
- **Production Baseline (Day 90):** $\ge 90\%$ autonomous recall (at least 14 / 15 bugs caught and resolved with passing deterministic regression tests).

### Adaptation for Zero-Hardware / Emulator-First CI
To ensure this benchmark runs entirely in automated CI (e.g. GitHub Actions with KVM headless AVDs) without requiring physical USB devices:
- **Bug #7** (Software encoder hash mismatch): Evaluated across **x86_64 AVD vs ARM64 AVD (or Redroid)** instead of physical silicon.
- **Bug #9** (Encoder EOS frame drop): Evaluated using the **software MediaCodec/FFmpeg encoder inside standard AVD** rather than proprietary DSP hardware rails.

---

## 2. Seeded Defects Master Matrix

| # | Workload | Defect Summary | Tier / Environment | Primary Detection Signal |
|---|---|---|---|---|
| **1** | Scam Shield | `NullPointerException` on scheme-less URL in SMS | Tier 1 (Headless AVD) | `act_and_observe` logcat delta + fatal crash stack trace |
| **2** | Scam Shield | Android 17 3-hr OTP delay silently blocks `READ_SMS` | Tier 1 (AVD API 37) | `sim_inject_sms_or_call` OTP regex check + missing mock webhook alert |
| **3** | Scam Shield | `NotificationListenerService` drops alert in Doze mode | Tier 1 (Headless AVD) | `sim_set_permissions` (`dumpsys deviceidle force-idle`) + timeout |
| **4** | Scam Shield | Infinite crash loop on `POST_NOTIFICATIONS` denial | Tier 1 (Headless AVD) | Permission denial matrix (`sim_set_permissions` revoke) + `crash_report_collect` |
| **5** | Scam Shield | Undeclared `READ_CALL_LOG` in merged manifest | Tier 0 (Static Preflight) | `apk_audit_compliance` merged-manifest AST scanner |
| **6** | Video Codec | Native `.so` compiled without 16 KB ELF alignment | Tier 0 (Static Preflight) | `apk_audit_compliance` (`zipalign -c -P 16` / `readelf -l`) |
| **7** | Video Codec | Software encoder hash mismatch on x86_64 vs ARM64 | Tier 1 (x86_64 + ARM64 AVD) | `media_push_and_verify` (`oracle: sha256`) cross-ABI diff |
| **8** | Video Codec | `MemoryLimiter:AnonSwap` OOM crash on 4K clip | Tier 1 (AVD API 37) | `crash_report_collect` catching Android 17 `MemoryLimiter` pressure exit |
| **9** | Video Codec | Software encoder drops last 3 frames at EOS flush | Tier 1 (Software Codec AVD) | `media_push_and_verify` (`ffprobe` frame-count mismatch) |
| **10** | Video Codec | Bad bitrate cap tanks visual quality (VMAF < 92) | Tier 1 (Headless AVD) | `media_push_and_verify` (`oracle: quality_bounded`, VMAF < 92) |
| **11** | General UI | CTA button clips off-screen at 1.3x font on 600dp | Tier 0 (Roborazzi JVM) | `project_test_jvm` (Roborazzi layout bounding box visual diff) |
| **12** | General UI | Dead submit button (handler detached, 0 state change) | Tier 1 (Headless AVD) | `act_and_observe` `identical_hash_streak >= 3` (`STUCK_STATE`) |
| **13** | General Perf | Main-thread disk I/O causes 6-second ANR on launch | Tier 1 (Headless AVD) | `crash_report_collect` ANR trace + `perf_capture_trace` P95 frame spike |
| **14** | Accessibility | Icon button has 32dp touch target & null `contentDescription` | Tier 0 (AndroidX JVM) | AndroidX Accessibility Validator + pruned A11y tree audit |
| **15** | Black-Box APK | ARM64-only native APK crashes on x86_64 (`UnsatisfiedLinkError`)| Tier 1 (AVD x86_64) | `apk_inspect` ABI detection routes to ARM64 / flags mismatch |

---

## 3. Deep-Dive Defect Specifications

### Defect 1: NPE on Scheme-Less URL in SMS
- **Component:** `com.example.scamshield.detector.UrlExtractor`
- **Planted Code:**
  ```kotlin
  val uri = Uri.parse(smsBody.split(" ").find { it.contains(".") })
  val host = uri.host!!.lowercase() // Throws NPE when url lacks scheme (e.g. "bit.ly/secure")
  ```
- **Injection Recipe:** Inject SMS using `sim_inject_sms_or_call` with text: `"Urgent: verify account at bit.ly/bank-auth"`.
- **Primary Detection:** `act_and_observe` returns logcat delta with `java.lang.NullPointerException: uri.host must not be null`.
- **Expected Resolution:** Use URI autolink regex or fallback scheme prefixing (`https://`).

### Defect 2: Android 17 3-Hour OTP Delay Blocks READ_SMS
- **Component:** `com.example.scamshield.service.SmsBroadcastReceiver`
- **Planted Defect:** App relies strictly on `READ_SMS` / `SMS_RECEIVED` to detect incoming bank OTPs on API 37 without listening via `NotificationListenerService`.
- **Injection Recipe:** Inject OTP message on API 37 emulator. Android 17 platform quarantines SMS OTP payloads for 3 hours.
- **Primary Detection:** `sim_inject_sms_or_call` detects OTP pattern, checks mock webhook, and alerts that no event was triggered.
- **Expected Resolution:** Migrate OTP verification to User Consent API / SMS Retriever API or `NotificationListenerService`.

### Defect 3: NotificationListenerService Drops Alert in Doze
- **Component:** `com.example.scamshield.service.ScamNotificationListener`
- **Planted Defect:** Foreground service fails to request high-priority wake lock or whitelist battery optimization.
- **Injection Recipe:** Run `adb shell dumpsys deviceidle force-idle` then post notification via `sim_post_notification`.
- **Primary Detection:** Notification event is dropped; test timeout expires.
- **Expected Resolution:** Bind foreground service with `FOREGROUND_SERVICE_TYPE_SPECIAL_USE` and exemption handling.

### Defect 4: Crash Loop on POST_NOTIFICATIONS Denial
- **Component:** `com.example.scamshield.ui.MainActivity`
- **Planted Code:**
  ```kotlin
  if (checkSelfPermission(POST_NOTIFICATIONS) != PERMISSION_GRANTED) {
      requestPermissions(arrayOf(POST_NOTIFICATIONS), 101)
      throw IllegalStateException("Notifications mandatory for background protection")
  }
  ```
- **Injection Recipe:** Run `sim_set_permissions` revoking `android.permission.POST_NOTIFICATIONS`.
- **Primary Detection:** `crash_report_collect` flags repeated fatal exception on cold start.
- **Expected Resolution:** Implement graceful in-app disclosure with fallback banner.

### Defect 5: Undeclared READ_CALL_LOG in Merged Manifest
- **Component:** `app/build/intermediates/merged_manifest/debug/AndroidManifest.xml`
- **Planted Code:** Transitive SDK dependency introduces `<uses-permission android:name="android.permission.READ_CALL_LOG" />`.
- **Detection:** `apk_audit_compliance` fails against `play-policy-justification.yaml` rule `status: PROHIBITED`.
- **Expected Resolution:** Add `tools:node="remove"` to root `AndroidManifest.xml`.

### Defect 6: 16 KB ELF Page Misalignment
- **Component:** `app/src/main/jniLibs/arm64-v8a/libnativecodec.so`
- **Planted Defect:** NDK compile flag uses default 4 KB page size (`-Wl,-z,max-page-size=4096`).
- **Detection:** `apk_audit_compliance` runs `readelf -l` / `zipalign -c -P 16`, finding non-conforming LOAD segments.
- **Expected Resolution:** Pass `-Wl,-z,max-page-size=16384` in `CMakeLists.txt`.

### Defect 7: Software Encoder Hash Mismatch x86_64 vs ARM64
- **Component:** `com.example.videocodec.encoder.DeterministicMotionEstimator`
- **Planted Defect:** Use of architecture-dependent floating point rounding (`fast-math` without IEEE 754 precision).
- **Detection:** `media_push_and_verify` in `bit_exact` oracle mode reports SHA-256 hash difference between x86_64 AVD and ARM64 AVD.
- **Expected Resolution:** Replace float rounding with fixed-point integer math.

### Defect 8: MemoryLimiter:AnonSwap OOM Crash on 4K Clip
- **Component:** `com.example.videocodec.buffer.FrameRingBuffer`
- **Planted Defect:** Uncapped ring buffer allocates raw bitmap frames during 4K transcoding.
- **Detection:** On API 37 emulator, `crash_report_collect` captures exit reason `ApplicationExitInfo.REASON_OTHER (MemoryLimiter:AnonSwap)`.
- **Expected Resolution:** Implement backpressure and capped buffer pool size (<= 64 MB).

### Defect 9: Software Encoder Drops Last 3 Frames at EOS
- **Component:** `com.example.videocodec.encoder.CodecFlusher`
- **Planted Code:** Breaks drain loop when `BUFFER_FLAG_END_OF_STREAM` is signaled without consuming trailing output buffer.
- **Detection:** `media_push_and_verify` flags `ffprobe` frame count mismatch: expected 1800 frames, found 1797 frames.
- **Expected Resolution:** Complete drain loop until `dequeueOutputBuffer` returns `INFO_TRY_AGAIN_LATER`.

### Defect 10: Bad Bitrate Cap Tanks Visual Quality
- **Component:** `com.example.videocodec.config.BitrateEstimator`
- **Planted Code:** Bitrate formula mistakenly clamps bitrate to 250 kbps for 1080p video.
- **Detection:** `media_push_and_verify` reports VMAF score of 74.2 (fails `vmaf_min: 92.0` gate).
- **Expected Resolution:** Adjust bitrate allocation formula to maintain visual fidelity floor.

### Defect 11: CTA Button Clips at 1.3x Font on 600dp
- **Component:** `com.example.scamshield.ui.compose.AlertBanner`
- **Planted Defect:** Fixed height container (`Modifier.height(48.dp)`) wrapping multiline text with font scale 1.3x.
- **Detection:** Roborazzi JVM test `project_test_jvm` reports visual layout difference and text clipping.
- **Expected Resolution:** Use `Modifier.wrapContentHeight()` with dynamic font scaling padding.

### Defect 12: Dead Button (Zero State Change)
- **Component:** `com.example.scamshield.ui.MainActivity`
- **Planted Code:** Button `onClick` lambda is empty `{ /* TODO */ }`.
- **Detection:** `act_and_observe` triggers `identical_hash_streak >= 3` warning (`STUCK_STATE`).
- **Expected Resolution:** Wire button action to intent dispatcher.

### Defect 13: Main-Thread Disk Read → 6s ANR on Launch
- **Component:** `com.example.scamshield.data.RuleDatabase`
- **Planted Code:** Synchronous `Thread.sleep(6000)` or large file read inside `Application.onCreate()`.
- **Detection:** `crash_report_collect` flags ANR trace (`Reason: Input dispatching timed out`).
- **Expected Resolution:** Move database initialization to `Dispatchers.IO`.

### Defect 14: Accessibility: 32dp Touch Target & Null ContentDesc
- **Component:** `com.example.scamshield.ui.IconButton`
- **Planted Code:** Touch target size set to `32.dp` with `contentDescription = null`.
- **Detection:** Tier 0 AndroidX Accessibility test reports touch target size violation (< 48dp) and missing label.
- **Expected Resolution:** Increase minimum size to `48.dp` and provide descriptive string resource.

### Defect 15: ARM-Only APK UnsatisfiedLinkError on x86_64
- **Component:** Source-free APK containing native libraries only in `lib/arm64-v8a/`.
- **Planted Defect:** Running APK directly on x86_64 AVD without translation.
- **Detection:** `apk_inspect` inspects ELF headers, warns of missing `x86_64` slice, and detects `UnsatisfiedLinkError` in logcat.
- **Expected Resolution:** Autonomous routing to ARM64 emulator or inclusion of x86_64 native libraries.

---

## 4. Benchmark Scoring and Execution Protocol

```
Scoring Formula:
  Recall Score = (Bugs Correctly Detected & Diagnosed / 15) * 100%
  Remediation Score = (Bugs Resolved with Passing Frozen Tests / 15) * 100%
```

1. **Isolation:** Each defect is checked into an isolated git branch (`benchmark/defect-01` through `benchmark/defect-15`).
2. **Autonomous Loop:** The IDE agent is given the prompt: `"Run DroidForge health check and test suite, find any defects, and fix them."`
3. **No External Hints:** The agent is not told which bug is planted or which tool to call.
4. **Pass Criteria:**
   - The agent detects the defect using DroidForge MCP tools.
   - The agent patches the root cause.
   - The agent freezes the fix into a Maestro YAML flow (`session_freeze_to_maestro`) and passes 3 consecutive burn-in runs (`test_run_maestro`).
