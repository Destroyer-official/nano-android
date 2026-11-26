package nanoandroid.host;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Locale;
import java.util.Random;
import java.util.TimeZone;
import java.util.concurrent.CopyOnWriteArrayList;

/**
 * Central deterministic runtime primitives: seeded PRNG, virtual clock snapshots,
 * controllable TimeZone/Locale test state, infinite-loop watchdog, and explicit
 * NONDETERMINISTIC boundary tracker.
 */
public final class DeterministicRuntime {
    public static final int DEFAULT_MAX_LOOPER_TASKS_PER_STEP = 10_000;
    private static final long DEFAULT_SEED = 0x4E414E4F5F534545L; // "NANO_SEE"
    private static final TimeZone HOST_DEFAULT_TZ = TimeZone.getDefault();
    private static final Locale HOST_DEFAULT_LOCALE = Locale.getDefault();

    public static final class NondeterministicBoundary {
        public final String api;
        public final String reason;
        public final long virtualUptimeMs;

        public NondeterministicBoundary(String api, String reason, long virtualUptimeMs) {
            this.api = api;
            this.reason = reason;
            this.virtualUptimeMs = virtualUptimeMs;
        }
    }

    private static volatile long currentSeed = DEFAULT_SEED;
    private static Random rng = new Random(DEFAULT_SEED);
    private static int maxLooperTasksPerStep = DEFAULT_MAX_LOOPER_TASKS_PER_STEP;
    private static final List<NondeterministicBoundary> BOUNDARIES = new CopyOnWriteArrayList<>();
    private static volatile TimeZone testTimeZone = TimeZone.getTimeZone("UTC");
    private static volatile Locale testLocale = Locale.US;

    private DeterministicRuntime() {}

    public static synchronized void reset(long seed) {
        currentSeed = seed;
        rng = new Random(seed);
        maxLooperTasksPerStep = DEFAULT_MAX_LOOPER_TASKS_PER_STEP;
        testTimeZone = TimeZone.getTimeZone("UTC");
        testLocale = Locale.US;
        BOUNDARIES.clear();
    }

    public static synchronized void reset() {
        reset(DEFAULT_SEED);
    }

    public static synchronized void restoreHostLocaleAndTimeZone() {
        TimeZone.setDefault(HOST_DEFAULT_TZ);
        Locale.setDefault(HOST_DEFAULT_LOCALE);
    }

    public static synchronized void setTestTimeZone(String zoneId) {
        if (zoneId == null || zoneId.trim().isEmpty()) return;
        testTimeZone = TimeZone.getTimeZone(zoneId.trim());
    }

    public static synchronized TimeZone getTestTimeZone() {
        return testTimeZone;
    }

    public static synchronized void setTestLocale(Locale locale) {
        if (locale != null) {
            testLocale = locale;
        }
    }

    public static synchronized Locale getTestLocale() {
        return testLocale;
    }

    public static long captureClockSnapshot() {
        return android.os.SystemClock.getVirtualUptimeMillis();
    }

    public static void restoreClockSnapshot(long virtualUptimeMs) {
        android.os.SystemClock.setUptimeMillis(virtualUptimeMs);
    }

    public static synchronized long getSeed() {
        return currentSeed;
    }

    public static synchronized Random getRandom() {
        return rng;
    }

    public static synchronized int nextInt(int bound) {
        return rng.nextInt(bound);
    }

    public static synchronized long nextLong() {
        return rng.nextLong();
    }

    public static synchronized void setMaxLooperTasksPerStep(int limit) {
        maxLooperTasksPerStep = Math.max(1, limit);
    }

    public static synchronized int getMaxLooperTasksPerStep() {
        return maxLooperTasksPerStep;
    }

    public static void markNondeterministic(String api, String reason) {
        BOUNDARIES.add(new NondeterministicBoundary(api, reason, android.os.SystemClock.getVirtualUptimeMillis()));
    }

    public static List<NondeterministicBoundary> getBoundaries() {
        return Collections.unmodifiableList(new ArrayList<>(BOUNDARIES));
    }
}
