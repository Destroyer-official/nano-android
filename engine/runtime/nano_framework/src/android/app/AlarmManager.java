package android.app;

import android.content.Context;
import android.content.Intent;
import android.os.Handler;
import android.os.Looper;
import android.os.SystemClock;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import nanoandroid.compat.ApiSupportLevel;
import nanoandroid.compat.NanoApiStatus;

@NanoApiStatus(
    level = ApiSupportLevel.PARTIAL,
    fallback = "android_art",
    notes = "Deterministic virtual-clock AlarmManager scheduled via main Looper; cross-process PendingIntent alarms require ART"
)
public class AlarmManager {
    public static final int RTC_WAKEUP = 0;
    public static final int RTC = 1;
    public static final int ELAPSED_REALTIME_WAKEUP = 2;
    public static final int ELAPSED_REALTIME = 3;

    public interface OnAlarmListener {
        void onAlarm();
    }

    private final Context context;
    private final Handler handler = new Handler(Looper.getMainLooper());
    private final Map<OnAlarmListener, Runnable> scheduled = new ConcurrentHashMap<>();

    public AlarmManager() {
        this(null);
    }

    public AlarmManager(Context context) {
        this.context = context;
    }

    public void set(int type, long triggerAtMillis, String tag, OnAlarmListener listener, Handler targetHandler) {
        setExact(type, triggerAtMillis, tag, listener, targetHandler);
    }

    public void setExact(int type, long triggerAtMillis, String tag, OnAlarmListener listener, Handler targetHandler) {
        if (listener == null) return;
        Handler h = targetHandler != null ? targetHandler : handler;
        long now = (type == RTC || type == RTC_WAKEUP) ? SystemClock.currentTimeMillis() : SystemClock.elapsedRealtime();
        long delay = Math.max(0L, triggerAtMillis - now);
        Runnable task = () -> {
            LifecycleEventTracer.record("ALARM_TRIGGERED", tag != null ? tag : "alarm", "type=" + type);
            listener.onAlarm();
        };
        scheduled.put(listener, task);
        h.postDelayed(task, delay);
    }

    public void setRepeating(int type, long triggerAtMillis, long intervalMillis, Intent broadcastAction) {
        if (context == null || broadcastAction == null) return;
        long now = (type == RTC || type == RTC_WAKEUP) ? SystemClock.currentTimeMillis() : SystemClock.elapsedRealtime();
        long delay = Math.max(0L, triggerAtMillis - now);
        handler.postDelayed(new Runnable() {
            @Override
            public void run() {
                context.sendBroadcast(broadcastAction);
                if (intervalMillis > 0) {
                    handler.postDelayed(this, intervalMillis);
                }
            }
        }, delay);
    }

    public void cancel(OnAlarmListener listener) {
        if (listener == null) return;
        Runnable r = scheduled.remove(listener);
        if (r != null) {
            handler.removeCallbacksAndMessages(r);
        }
    }
}
