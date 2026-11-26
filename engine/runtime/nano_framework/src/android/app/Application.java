package android.app;

import android.content.ComponentCallbacks2;
import android.content.Context;
import android.content.ContextWrapper;
import android.content.res.Configuration;
import android.os.Bundle;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;
import nanoandroid.compat.ApiSupportLevel;
import nanoandroid.compat.NanoApiStatus;

@NanoApiStatus(
    level = ApiSupportLevel.FULLY_IMPLEMENTED,
    notes = "Full Application lifecycle, ActivityLifecycleCallbacks dispatch, and configuration/memory callbacks"
)
public class Application extends ContextWrapper implements ComponentCallbacks2 {
    public interface ActivityLifecycleCallbacks {
        void onActivityCreated(Activity activity, Bundle savedInstanceState);
        void onActivityStarted(Activity activity);
        void onActivityResumed(Activity activity);
        void onActivityPaused(Activity activity);
        void onActivityStopped(Activity activity);
        void onActivitySaveInstanceState(Activity activity, Bundle outState);
        void onActivityDestroyed(Activity activity);
    }

    private final List<ActivityLifecycleCallbacks> activityCallbacks = new CopyOnWriteArrayList<>();
    private final List<ComponentCallbacks2> componentCallbacks = new CopyOnWriteArrayList<>();

    public Application() { super(null); }

    public static String getProcessName() {
        return Activity.currentApplication != null && Activity.currentApplication.getPackageName() != null
                ? Activity.currentApplication.getPackageName()
                : "nano.android.app";
    }

    @Override
    public void attachBaseContext(Context base) {
        super.attachBaseContext(base);
    }

    @Override
    public Context getApplicationContext() {
        return this;
    }

    public void onCreate() {
        LifecycleEventTracer.record("APPLICATION_CREATE", getClass().getName(), getPackageName());
    }

    public void onTerminate() {
        LifecycleEventTracer.record("APPLICATION_TERMINATE", getClass().getName(), getPackageName());
    }

    @Override
    public void onConfigurationChanged(Configuration newConfig) {
        LifecycleEventTracer.record("APPLICATION_CONFIG_CHANGED", getClass().getName(), "");
        for (ComponentCallbacks2 cb : componentCallbacks) {
            cb.onConfigurationChanged(newConfig);
        }
    }

    @Override
    public void onLowMemory() {
        for (ComponentCallbacks2 cb : componentCallbacks) {
            cb.onLowMemory();
        }
    }

    @Override
    public void onTrimMemory(int level) {
        LifecycleEventTracer.record("APPLICATION_TRIM_MEMORY", getClass().getName(), "level=" + level);
        for (ComponentCallbacks2 cb : componentCallbacks) {
            cb.onTrimMemory(level);
        }
    }

    public void registerActivityLifecycleCallbacks(ActivityLifecycleCallbacks callback) {
        if (callback != null && !activityCallbacks.contains(callback)) {
            activityCallbacks.add(callback);
        }
    }

    public void unregisterActivityLifecycleCallbacks(ActivityLifecycleCallbacks callback) {
        activityCallbacks.remove(callback);
    }

    public void registerComponentCallbacks(ComponentCallbacks2 callback) {
        if (callback != null && !componentCallbacks.contains(callback)) {
            componentCallbacks.add(callback);
        }
    }

    public void unregisterComponentCallbacks(ComponentCallbacks2 callback) {
        componentCallbacks.remove(callback);
    }

    public void dispatchActivityCreated(Activity activity, Bundle savedInstanceState) {
        for (ActivityLifecycleCallbacks cb : activityCallbacks) {
            cb.onActivityCreated(activity, savedInstanceState);
        }
    }

    public void dispatchActivityStarted(Activity activity) {
        for (ActivityLifecycleCallbacks cb : activityCallbacks) {
            cb.onActivityStarted(activity);
        }
    }

    public void dispatchActivityResumed(Activity activity) {
        for (ActivityLifecycleCallbacks cb : activityCallbacks) {
            cb.onActivityResumed(activity);
        }
    }

    public void dispatchActivityPaused(Activity activity) {
        for (ActivityLifecycleCallbacks cb : activityCallbacks) {
            cb.onActivityPaused(activity);
        }
    }

    public void dispatchActivityStopped(Activity activity) {
        for (ActivityLifecycleCallbacks cb : activityCallbacks) {
            cb.onActivityStopped(activity);
        }
    }

    public void dispatchActivitySaveInstanceState(Activity activity, Bundle outState) {
        for (ActivityLifecycleCallbacks cb : activityCallbacks) {
            cb.onActivitySaveInstanceState(activity, outState);
        }
    }

    public void dispatchActivityDestroyed(Activity activity) {
        for (ActivityLifecycleCallbacks cb : activityCallbacks) {
            cb.onActivityDestroyed(activity);
        }
    }
}
