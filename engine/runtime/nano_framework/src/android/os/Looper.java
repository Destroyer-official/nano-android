package android.os;

import nanoandroid.host.DeterministicRuntime;

public class Looper {
    private static final ThreadLocal<Looper> threadLocal = new ThreadLocal<>();
    private static final Looper mainLooper = new Looper(Thread.currentThread(), "main");

    private final MessageQueue queue = new MessageQueue();
    private final Thread thread;
    private final String name;
    private volatile boolean quitFlag = false;

    private Looper(Thread thread, String name) {
        this.thread = thread;
        this.name = name != null ? name : "looper";
    }

    public static Looper getMainLooper() {
        return mainLooper;
    }

    public static Looper myLooper() {
        Looper l = threadLocal.get();
        return l != null ? l : mainLooper;
    }

    public static void prepare() {
        if (threadLocal.get() != null) {
            throw new RuntimeException("Only one Looper may be created per thread");
        }
        threadLocal.set(new Looper(Thread.currentThread(), Thread.currentThread().getName()));
    }

    static Looper createNamedLooper(String name) {
        return new Looper(Thread.currentThread(), name);
    }

    public MessageQueue getQueue() {
        return queue;
    }

    public Thread getThread() {
        return thread;
    }

    public String getName() {
        return name;
    }

    public synchronized boolean quit(boolean safely) {
        if (this == mainLooper) return false;
        quitFlag = true;
        if (!safely) {
            queue.removeCallbacksAndMessages(null);
        }
        return true;
    }

    public boolean isQuit() {
        return quitFlag;
    }

    /**
     * Deterministically drains all messages due up to current virtual uptime,
     * throwing IllegalStateException if the infinite-loop watchdog limit is hit.
     */
    public int drainDueMessages() {
        return stepBy(0L);
    }

    /**
     * Advances virtual clock by deltaMs and deterministically drains all due messages
     * up to the infinite-loop watchdog limit.
     */
    public int stepBy(long deltaMs) {
        if (deltaMs > 0) {
            SystemClock.advanceUptimeMillis(deltaMs);
        }
        int limit = DeterministicRuntime.getMaxLooperTasksPerStep();
        int count = 0;
        while (!quitFlag) {
            long now = SystemClock.uptimeMillis();
            Message msg = queue.pollNextDueMessage(now);
            if (msg == null) break;
            if (++count > limit) {
                throw new IllegalStateException("Deterministic Looper watchdog triggered: exceeded "
                        + limit + " messages in a single step on looper '" + name + "'");
            }
            if (msg.target != null) {
                msg.target.dispatchMessage(msg);
            } else if (msg.callback != null) {
                msg.callback.run();
            }
        }
        return count;
    }

    /**
     * Runs until all currently queued messages up to any delayed threshold execute or watchdog triggers.
     */
    public int waitUntilIdle() {
        return drainDueMessages();
    }
}
