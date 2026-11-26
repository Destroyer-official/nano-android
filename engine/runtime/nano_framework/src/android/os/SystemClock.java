package android.os;

import nanoandroid.host.DeterministicRuntime;

public class SystemClock {
    private static final long DEFAULT_EPOCH_MS = 1735689600000L; // 2025-01-01T00:00:00Z
    private static volatile long virtualUptime = 0;
    private static volatile boolean useVirtual = true;

    public static long getVirtualUptimeMillis() {
        return virtualUptime;
    }

    public static long uptimeMillis() {
        if (useVirtual) {
            return virtualUptime;
        }
        DeterministicRuntime.markNondeterministic("SystemClock.uptimeMillis", "Wall-clock mode enabled");
        return System.currentTimeMillis();
    }

    public static long elapsedRealtime() {
        return uptimeMillis();
    }

    public static long elapsedRealtimeNanos() {
        return uptimeMillis() * 1_000_000L;
    }

    public static long currentThreadTimeMillis() {
        return uptimeMillis();
    }

    public static long currentTimeMillis() {
        if (useVirtual) {
            return DEFAULT_EPOCH_MS + virtualUptime;
        }
        DeterministicRuntime.markNondeterministic("SystemClock.currentTimeMillis", "Wall-clock mode enabled");
        return System.currentTimeMillis();
    }

    public static void setUptimeMillis(long millis) {
        virtualUptime = Math.max(0L, millis);
    }

    public static void advanceUptimeMillis(long delta) {
        if (delta > 0) {
            virtualUptime += delta;
        }
    }

    public static void setUseVirtualTime(boolean enabled) {
        useVirtual = enabled;
        if (!enabled) {
            DeterministicRuntime.markNondeterministic("SystemClock.setUseVirtualTime", "Disabled virtual deterministic clock");
        }
    }

    public static boolean isUsingVirtualTime() {
        return useVirtual;
    }

    public static void sleep(long ms) {
        if (useVirtual) {
            advanceUptimeMillis(ms);
        } else {
            DeterministicRuntime.markNondeterministic("SystemClock.sleep", "Real Thread.sleep used");
            try { Thread.sleep(ms); } catch (InterruptedException ignored) {}
        }
    }
}
