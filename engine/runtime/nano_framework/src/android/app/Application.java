package android.app;

import android.content.ComponentCallbacks2;
import android.content.Context;
import android.content.ContextWrapper;
import android.content.res.Configuration;
import android.os.Bundle;

public class Application extends ContextWrapper implements ComponentCallbacks2 {
    @Override public void onConfigurationChanged(Configuration newConfig) {}
    @Override public void onLowMemory() {}
    @Override public void onTrimMemory(int level) {}
    public interface ActivityLifecycleCallbacks {
        void onActivityCreated(Activity activity, Bundle savedInstanceState);
        void onActivityStarted(Activity activity);
        void onActivityResumed(Activity activity);
        void onActivityPaused(Activity activity);
        void onActivityStopped(Activity activity);
        void onActivitySaveInstanceState(Activity activity, Bundle outState);
        void onActivityDestroyed(Activity activity);
    }

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

    public void onCreate() {}
    public void onTerminate() {}
    public void registerActivityLifecycleCallbacks(ActivityLifecycleCallbacks callback) {}
    public void unregisterActivityLifecycleCallbacks(ActivityLifecycleCallbacks callback) {}
}
