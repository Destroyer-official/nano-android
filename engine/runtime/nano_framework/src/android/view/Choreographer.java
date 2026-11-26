package android.view;

import android.os.Handler;
import android.os.Looper;
import android.os.SystemClock;

/**
 * Deterministic Choreographer advancing FrameCallback hooks strictly with virtual SystemClock time.
 */
public final class Choreographer {
    public interface FrameCallback {
        void doFrame(long frameTimeNanos);
    }

    private static final Choreographer MAIN_INSTANCE = new Choreographer(Looper.getMainLooper());
    private final Handler handler;

    private Choreographer(Looper looper) {
        this.handler = new Handler(looper != null ? looper : Looper.getMainLooper());
    }

    public static Choreographer getInstance() {
        return MAIN_INSTANCE;
    }

    public void postFrameCallback(FrameCallback callback) {
        postFrameCallbackDelayed(callback, 16L);
    }

    public void postFrameCallbackDelayed(FrameCallback callback, long delayMillis) {
        if (callback == null) return;
        handler.postDelayed(() -> callback.doFrame(SystemClock.uptimeMillis() * 1_000_000L), Math.max(1L, delayMillis));
    }

    public void removeFrameCallback(FrameCallback callback) {
        handler.removeCallbacksAndMessages(callback);
    }
}
