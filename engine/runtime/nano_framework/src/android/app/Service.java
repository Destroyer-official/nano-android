package android.app;

import android.content.Context;
import android.content.ContextWrapper;
import android.content.Intent;
import android.os.IBinder;
import nanoandroid.compat.ApiSupportLevel;
import nanoandroid.compat.NanoApiStatus;

@NanoApiStatus(
    level = ApiSupportLevel.PARTIAL,
    fallback = "android_art",
    notes = "In-process started, bound, and foreground service lifecycle; cross-process AIDL/remote processes require ART"
)
public abstract class Service extends ContextWrapper {
    public static final int START_STICKY_COMPATIBILITY = 0;
    public static final int START_STICKY = 1;
    public static final int START_NOT_STICKY = 2;
    public static final int START_REDELIVER_INTENT = 3;
    public static final int STOP_FOREGROUND_REMOVE = 1;
    public static final int STOP_FOREGROUND_DETACH = 2;

    private Application application;
    private boolean created = false;
    private boolean destroyed = false;
    private boolean foreground = false;
    private int foregroundNotificationId = 0;
    private Object foregroundNotification = null;
    private int lastStartId = 0;

    public Service() {
        super(null);
    }

    public final void attach(Context context, Application application) {
        attachBaseContext(context);
        this.application = application != null ? application : Activity.currentApplication;
    }

    public final Application getApplication() {
        return application;
    }

    public void onCreate() {
        this.created = true;
        LifecycleEventTracer.record("SERVICE_CREATE", getClass().getName(), "");
    }

    public int onStartCommand(Intent intent, int flags, int startId) {
        this.lastStartId = startId;
        String act = intent != null && intent.getAction() != null ? intent.getAction() : "";
        LifecycleEventTracer.record("SERVICE_START_COMMAND", getClass().getName(), "startId=" + startId + ";action=" + act);
        return START_STICKY;
    }

    public IBinder onBind(Intent intent) {
        String act = intent != null && intent.getAction() != null ? intent.getAction() : "";
        LifecycleEventTracer.record("SERVICE_BIND", getClass().getName(), act);
        return null;
    }

    public boolean onUnbind(Intent intent) {
        LifecycleEventTracer.record("SERVICE_UNBIND", getClass().getName(), "");
        return false;
    }

    public void onRebind(Intent intent) {
        LifecycleEventTracer.record("SERVICE_REBIND", getClass().getName(), "");
    }

    public void onDestroy() {
        this.destroyed = true;
        LifecycleEventTracer.record("SERVICE_DESTROY", getClass().getName(), "");
    }

    public final void startForeground(int id, Object notification) {
        this.foreground = true;
        this.foregroundNotificationId = id;
        this.foregroundNotification = notification;
        LifecycleEventTracer.record("SERVICE_START_FOREGROUND", getClass().getName(), "notificationId=" + id);
    }

    public final void stopForeground(boolean removeNotification) {
        this.foreground = false;
        if (removeNotification) {
            this.foregroundNotification = null;
            this.foregroundNotificationId = 0;
        }
        LifecycleEventTracer.record("SERVICE_STOP_FOREGROUND", getClass().getName(), "");
    }

    public final void stopForeground(int flags) {
        stopForeground((flags & STOP_FOREGROUND_REMOVE) != 0);
    }

    public final void stopSelf() {
        stopSelf(lastStartId);
    }

    public final void stopSelf(int startId) {
        Context.stopServiceInstance(this, startId);
    }

    public boolean isForeground() { return foreground; }
    public int getForegroundNotificationId() { return foregroundNotificationId; }
    public Object getForegroundNotification() { return foregroundNotification; }
    public boolean isCreated() { return created; }
    public boolean isDestroyed() { return destroyed; }
}
