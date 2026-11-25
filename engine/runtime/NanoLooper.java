package nanoandroid.runtime;

import android.os.Handler;
import android.os.Looper;
import android.os.Message;
import android.os.MessageQueue;
import android.os.SystemClock;
import java.util.ArrayList;
import java.util.List;
import java.util.function.Consumer;

/**
 * NanoLooper is a deterministic virtual Android looper that replaces wall-clock
 * sleeping with discrete virtual time progression.
 */
public class NanoLooper {
    private final Looper looper;
    private final MessageQueue queue;
    private final List<Consumer<Long>> frameCallbacks = new ArrayList<>();

    public NanoLooper() {
        this(0);
    }

    public NanoLooper(long initialUptimeMillis) {
        SystemClock.setUseVirtualTime(true);
        SystemClock.setUptimeMillis(initialUptimeMillis);
        this.looper = Looper.getMainLooper();
        this.queue = looper.getQueue();
    }

    public long getUptimeMillis() {
        return SystemClock.uptimeMillis();
    }

    public void setUptimeMillis(long virtualMillis) {
        SystemClock.setUptimeMillis(virtualMillis);
    }

    public boolean post(Runnable r) {
        return postDelayed(r, 0);
    }

    public boolean postDelayed(Runnable r, long delayMillis) {
        long targetUptime = SystemClock.uptimeMillis() + delayMillis;
        Message msg = Message.obtain(null, r);
        return queue.enqueueMessage(msg, targetUptime);
    }

    public synchronized void postFrameCallback(Consumer<Long> callback) {
        frameCallbacks.add(callback);
    }

    /**
     * Deterministically steps virtual time forward by deltaMs, executing all messages
     * scheduled within this window without sleeping.
     *
     * @param deltaMs milliseconds to advance virtual time
     * @return remaining pending messages count
     */
    public synchronized int step(long deltaMs) {
        if (deltaMs < 0) {
            throw new IllegalArgumentException("deltaMs cannot be negative: " + deltaMs);
        }

        long targetTime = SystemClock.uptimeMillis() + deltaMs;

        while (true) {
            Message dueMsg = queue.pollNextDueMessage(targetTime);
            if (dueMsg == null) {
                break;
            }

            // Advance virtual time to the message execution time
            if (dueMsg.when > SystemClock.uptimeMillis()) {
                SystemClock.setUptimeMillis(dueMsg.when);
            }

            // Execute message
            if (dueMsg.callback != null) {
                dueMsg.callback.run();
            } else if (dueMsg.target != null) {
                dueMsg.target.dispatchMessage(dueMsg);
            }
        }

        // Advance to full target time
        SystemClock.setUptimeMillis(targetTime);

        // Process animation frame callbacks
        if (!frameCallbacks.isEmpty()) {
            List<Consumer<Long>> dueCallbacks = new ArrayList<>(frameCallbacks);
            frameCallbacks.clear();
            long frameNanos = targetTime * 1_000_000L;
            for (Consumer<Long> cb : dueCallbacks) {
                cb.accept(frameNanos);
            }
        }

        return queue.size();
    }

    public int getPendingMessageCount() {
        return queue.size();
    }

    public Looper getLooper() {
        return looper;
    }
}
