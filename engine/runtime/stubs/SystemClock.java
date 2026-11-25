package android.os;

import java.util.concurrent.atomic.AtomicLong;

public class SystemClock {
    private static final AtomicLong sVirtualUptime = new AtomicLong(0);
    private static boolean sUseVirtualTime = true;

    private SystemClock() {}

    public static long uptimeMillis() {
        if (sUseVirtualTime) {
            return sVirtualUptime.get();
        }
        return java.lang.System.currentTimeMillis();
    }

    public static void setUptimeMillis(long virtualMillis) {
        sVirtualUptime.set(virtualMillis);
    }

    public static void advanceUptimeMillis(long deltaMillis) {
        sVirtualUptime.addAndGet(deltaMillis);
    }

    public static void setUseVirtualTime(boolean useVirtual) {
        sUseVirtualTime = useVirtual;
    }

    public static void sleep(long ms) {
        if (sUseVirtualTime) {
            advanceUptimeMillis(ms);
        } else {
            try {
                Thread.sleep(ms);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
            }
        }
    }
}
