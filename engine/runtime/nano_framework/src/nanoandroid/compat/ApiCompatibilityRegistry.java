package nanoandroid.compat;

import java.io.PrintWriter;
import java.io.StringWriter;
import java.util.*;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CopyOnWriteArrayList;

public final class ApiCompatibilityRegistry {

    public static final class ApiEntry {
        public final String api;
        public final ApiSupportLevel level;
        public final int sinceApi;
        public final int untilApi;
        public final String fallback;
        public final String notes;

        public ApiEntry(String api, ApiSupportLevel level, int sinceApi, int untilApi, String fallback, String notes) {
            this.api = api;
            this.level = level;
            this.sinceApi = sinceApi;
            this.untilApi = untilApi;
            this.fallback = fallback;
            this.notes = notes;
        }
    }

    public static final class CompatibilityFault {
        public final String api;
        public final String callerMethod;
        public final String stackTrace;
        public final String reason;
        public final String availableFallback;
        public final String testValidity; // "VALID_WITH_WARNING" | "INVALIDATED"
        public final long timestampMs;

        public CompatibilityFault(String api, String callerMethod, String stackTrace, String reason,
                                  String availableFallback, String testValidity) {
            this.api = api;
            this.callerMethod = callerMethod;
            this.stackTrace = stackTrace;
            this.reason = reason;
            this.availableFallback = availableFallback;
            this.testValidity = testValidity;
            this.timestampMs = System.currentTimeMillis();
        }
    }

    private static final Map<String, ApiEntry> REGISTRY = new ConcurrentHashMap<>();
    private static final List<CompatibilityFault> SESSION_FAULTS = new CopyOnWriteArrayList<>();

    static {
        // Core baseline compatibility matrix across the 17 Android framework subsystems
        register("android.app.Application", ApiSupportLevel.FULLY_IMPLEMENTED, 26, 37, "", "Lifecycle + ActivityLifecycleCallbacks");
        register("android.app.Activity", ApiSupportLevel.FULLY_IMPLEMENTED, 26, 37, "", "Lifecycle, stack, results, savedInstanceState, recreate");
        register("android.app.Service", ApiSupportLevel.PARTIAL, 26, 37, "android_art", "Local start/bind/foreground simulation");
        register("android.content.Context", ApiSupportLevel.BEHAVIORAL_DIFF, 26, 37, "", "Canonical sandbox containment + stateful permission simulation");
        register("android.content.ContentResolver", ApiSupportLevel.BEHAVIORAL_DIFF, 26, 37, "android_art", "Contained File/MediaStore + local ContentProvider authority routing");
        register("android.content.ContentProvider", ApiSupportLevel.PARTIAL, 26, 37, "android_art", "In-process authority registration & CRUD");
        register("android.content.BroadcastReceiver", ApiSupportLevel.PARTIAL, 26, 37, "android_art", "In-process dynamic/manifest broadcast dispatch");
        register("android.content.pm.PackageManager", ApiSupportLevel.PARTIAL, 26, 37, "android_art", "Profile-driven hasSystemFeature + manifest metadata");
        register("android.content.res.Resources", ApiSupportLevel.PARTIAL, 26, 37, "android_art", "Binary resources.arsc + AXML decoding");
        register("android.database.sqlite.SQLiteDatabase", ApiSupportLevel.FULLY_IMPLEMENTED, 26, 37, "", "Real SQLite 3.54.1 via sqlite-jdbc with ACID rollback & savepoints");
        register("android.graphics.Canvas", ApiSupportLevel.BEHAVIORAL_DIFF, 26, 37, "android_art", "Java2D host rasterization");
        register("android.net.Uri", ApiSupportLevel.BEHAVIORAL_DIFF, 26, 37, "android_art", "Contained sandbox URI resolution");
        register("android.os.Looper", ApiSupportLevel.FULLY_IMPLEMENTED, 26, 37, "", "Deterministic virtual MessageQueue + SystemClock");
        register("android.os.SystemClock", ApiSupportLevel.FULLY_IMPLEMENTED, 26, 37, "", "Deterministic virtual uptime/elapsedRealtime");
        register("android.view.View", ApiSupportLevel.PARTIAL, 26, 37, "android_art", "Two-pass measure/layout + semantic AST");
        register("android.widget.RecyclerView", ApiSupportLevel.PARTIAL, 26, 37, "android_art", "Adapter binding & layout integration");
        register("android.animation.ValueAnimator", ApiSupportLevel.BEHAVIORAL_DIFF, 26, 37, "", "Virtual Choreographer/clock progression");
        register("android.media.MediaMetadataRetriever", ApiSupportLevel.PARTIAL, 26, 37, "physical_device", "ISOBMFF/MP4 atom inspection; codecs require hardware/ART");
        register("android.media.MediaCodec", ApiSupportLevel.REQUIRES_HARDWARE, 26, 37, "physical_device", "Hardware video/audio codec silicon requires physical_device");
        register("androidx.compose.ui.platform.ComposeView", ApiSupportLevel.REQUIRES_ART, 26, 37, "robolectric_or_android_art", "Use Robolectric ComposeTestRule or ART device");
    }

    private ApiCompatibilityRegistry() {}

    public static void register(String api, ApiSupportLevel level, int sinceApi, int untilApi, String fallback, String notes) {
        if (api == null || api.isEmpty()) return;
        REGISTRY.put(api, new ApiEntry(api, level, sinceApi, untilApi, fallback, notes));
    }

    public static void registerAnnotatedClass(Class<?> cls) {
        if (cls == null) return;
        NanoApiStatus ann = cls.getAnnotation(NanoApiStatus.class);
        if (ann != null) {
            register(cls.getName(), ann.level(), ann.sinceApi(), ann.untilApi(), ann.fallback(), ann.notes());
        }
    }

    public static ApiEntry lookup(String api) {
        return REGISTRY.get(api);
    }

    public static Map<String, ApiEntry> snapshotRegistry() {
        return new TreeMap<>(REGISTRY);
    }

    public static void clearSessionFaults() {
        SESSION_FAULTS.clear();
    }

    public static void recordFault(String api, String reason, String fallbackBackend, boolean invalidatesResult) {
        StackTraceElement[] trace = Thread.currentThread().getStackTrace();
        String caller = "unknown";
        for (StackTraceElement el : trace) {
            String cn = el.getClassName();
            if (!cn.startsWith("java.lang.") && !cn.startsWith("nanoandroid.compat.")) {
                caller = cn + "." + el.getMethodName() + ":" + el.getLineNumber();
                break;
            }
        }
        StringWriter sw = new StringWriter();
        new Throwable(reason).printStackTrace(new PrintWriter(sw));
        String validity = invalidatesResult ? "INVALIDATED" : "VALID_WITH_WARNING";
        SESSION_FAULTS.add(new CompatibilityFault(api, caller, sw.toString(), reason, fallbackBackend, validity));
    }

    public static List<CompatibilityFault> getSessionFaults() {
        return Collections.unmodifiableList(new ArrayList<>(SESSION_FAULTS));
    }

    public static boolean hasInvalidatingFault() {
        for (CompatibilityFault f : SESSION_FAULTS) {
            if ("INVALIDATED".equals(f.testValidity)) {
                return true;
            }
        }
        return false;
    }

    public static String exportRegistryJson() {
        Map<String, ApiEntry> snap = snapshotRegistry();
        StringBuilder sb = new StringBuilder();
        sb.append("{\n  \"schema_version\": 1,\n  \"api_count\": ").append(snap.size()).append(",\n  \"entries\": [\n");
        int i = 0;
        for (ApiEntry e : snap.values()) {
            if (i++ > 0) sb.append(",\n");
            sb.append("    {\"api\":\"").append(esc(e.api)).append("\"")
              .append(",\"level\":\"").append(e.level.name()).append("\"")
              .append(",\"since_api\":").append(e.sinceApi)
              .append(",\"until_api\":").append(e.untilApi)
              .append(",\"fallback\":\"").append(esc(e.fallback)).append("\"")
              .append(",\"notes\":\"").append(esc(e.notes)).append("\"}");
        }
        sb.append("\n  ]\n}\n");
        return sb.toString();
    }

    public static void writeRegistryJson(java.io.File outFile) throws java.io.IOException {
        if (outFile == null) return;
        if (outFile.getParentFile() != null) outFile.getParentFile().mkdirs();
        try (java.io.OutputStreamWriter w = new java.io.OutputStreamWriter(
                new java.io.FileOutputStream(outFile), java.nio.charset.StandardCharsets.UTF_8)) {
            w.write(exportRegistryJson());
        }
    }

    private static String esc(String s) {
        if (s == null) return "";
        return s.replace("\\", "\\\\").replace("\"", "\\\"");
    }

    public static void main(String[] args) throws Exception {
        registerAnnotatedClass(android.app.Application.class);
        registerAnnotatedClass(android.app.Activity.class);
        registerAnnotatedClass(android.app.Service.class);
        registerAnnotatedClass(android.app.AlarmManager.class);
        registerAnnotatedClass(android.content.ContentProvider.class);
        registerAnnotatedClass(android.content.ContentResolver.class);
        registerAnnotatedClass(android.provider.MediaStore.class);
        registerAnnotatedClass(android.provider.DocumentsContract.class);
        registerAnnotatedClass(nanoandroid.host.NanoCoroutineMainBridge.class);
        String out = (args != null && args.length > 0) ? args[0] : "engine/compatibility/compatibility_registry.json";
        writeRegistryJson(new java.io.File(out));
        System.out.println("Wrote " + snapshotRegistry().size() + " API compatibility contracts to " + out);
    }
}
