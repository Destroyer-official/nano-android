package android.os;

public class SystemClock {
    private static volatile long virtualUptime = 0;
    private static volatile boolean useVirtual = true;

    public static long uptimeMillis() {
        if (useVirtual) {
            return virtualUptime;
        }
        return System.currentTimeMillis();
    }

    public static long elapsedRealtime() {
        return uptimeMillis();
    }

    public static long elapsedRealtimeNanos() {
        return uptimeMillis() * 1_000_000L;
    }

    public static void setUptimeMillis(long millis) {
        virtualUptime = millis;
    }

    public static void advanceUptimeMillis(long delta) {
        virtualUptime += delta;
    }

    public static void setUseVirtualTime(boolean enabled) {
        useVirtual = enabled;
    }

    public static void sleep(long ms) {
        if (useVirtual) {
            virtualUptime += ms;
        } else {
            try { Thread.sleep(ms); } catch (InterruptedException ignored) {}
        }
    }
}
