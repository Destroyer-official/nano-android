package nanoandroid.host;

import android.os.Handler;
import android.os.Looper;
import java.util.concurrent.Executor;
import nanoandroid.compat.ApiSupportLevel;
import nanoandroid.compat.NanoApiStatus;

/**
 * Deterministic kotlinx.coroutines Dispatchers.Main -> Nano main Looper bridge.
 */
@NanoApiStatus(
    level = ApiSupportLevel.BEHAVIORAL_DIFF,
    fallback = "android_art",
    notes = "Bridges main-dispatcher runnables deterministically onto Nano main Looper MessageQueue"
)
public final class NanoCoroutineMainBridge implements Executor {
    private static final NanoCoroutineMainBridge INSTANCE = new NanoCoroutineMainBridge();
    private final Handler mainHandler = new Handler(Looper.getMainLooper());

    private NanoCoroutineMainBridge() {}

    public static NanoCoroutineMainBridge getInstance() {
        return INSTANCE;
    }

    @Override
    public void execute(Runnable command) {
        if (command == null) return;
        mainHandler.post(command);
    }

    public void postDelayed(Runnable command, long delayMillis) {
        if (command == null) return;
        mainHandler.postDelayed(command, Math.max(0L, delayMillis));
    }

    public boolean isDispatchNeeded() {
        return Thread.currentThread() != Looper.getMainLooper().getThread();
    }
}
