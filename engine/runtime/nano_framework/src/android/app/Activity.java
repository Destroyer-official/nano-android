package android.app;

import android.content.BroadcastReceiver;
import android.content.ComponentCallbacks2;
import android.content.Context;
import android.content.ContextWrapper;
import android.content.Intent;
import android.content.IntentFilter;
import android.content.SharedPreferences;
import android.content.pm.PackageManager;
import android.content.res.Configuration;
import android.content.res.Resources;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.view.ContextThemeWrapper;
import android.view.View;
import android.view.ViewGroup;
import android.view.Window;
import android.widget.FrameLayout;

public class Activity extends ContextThemeWrapper implements Window.Callback, ComponentCallbacks2 {
    public static volatile Application currentApplication;

    @Override public void onLowMemory() {}
    @Override public void onTrimMemory(int level) {}
    public enum State {
        UNINITIALIZED, CREATED, STARTED, RESUMED, PAUSED, STOPPED, DESTROYED
    }

    private Context baseContext;
    private Intent intent;
    private Application application = currentApplication != null ? currentApplication : new Application();
    private State state = State.UNINITIALIZED;
    private boolean isFinishing = false;
    private View contentView;
    private int themeResId = 0;
    private final Window window = new Window();
    private final Handler mainHandler = new Handler(Looper.getMainLooper());

    public Activity() {
        super(null);
        this.window.setCallback(this);
    }

    public Application getApplication() {
        return application;
    }

    public void setApplication(Application app) {
        if (app != null) {
            this.application = app;
        }
    }

    @Override
    public Context getApplicationContext() {
        return application != null ? application : super.getApplicationContext();
    }

    public android.content.ComponentName getComponentName() {
        return new android.content.ComponentName(getPackageName(), getClass().getName());
    }

    public String getLocalClassName() {
        return getClass().getName();
    }

    public Object getLastNonConfigurationInstance() {
        return null;
    }

    public Object onRetainNonConfigurationInstance() {
        return null;
    }

    public void onConfigurationChanged(android.content.res.Configuration newConfig) {}

    public void invalidateOptionsMenu() {}

    protected void onSaveInstanceState(Bundle outState) {}

    protected void onRestoreInstanceState(Bundle savedInstanceState) {}

    public void onUserInteraction() {}

    public void onUserLeaveHint() {}

    public void onWindowFocusChanged(boolean hasFocus) {}

    public void onAttachedToWindow() {}

    public void onDetachedFromWindow() {}

    public boolean isDestroyed() {
        return state == State.DESTROYED;
    }

    public synchronized void attach(Context baseContext, Intent intent) {
        this.baseContext = baseContext;
        if (baseContext != null) {
            attachBaseContext(baseContext);
            setPackageName(baseContext.getPackageName());
        }
        this.intent = (intent != null) ? intent : new Intent();
        if (window.getDecorView() == null) {
            window.setDecorView(new FrameLayout(this));
        }
        window.setCallback(this);
    }

    protected void onCreate(Bundle savedInstanceState) {
        this.state = State.CREATED;
    }

    protected void onStart() {
        this.state = State.STARTED;
    }

    protected void onResume() {
        this.state = State.RESUMED;
    }

    protected void onPause() {
        this.state = State.PAUSED;
    }

    protected void onStop() {
        this.state = State.STOPPED;
    }

    protected void onDestroy() {
        this.state = State.DESTROYED;
    }

    public synchronized void setTheme(int resid) {
        this.themeResId = resid;
    }

    public synchronized int getThemeResId() {
        return this.themeResId;
    }

    public Resources.Theme getTheme() {
        return getResources().newTheme();
    }

    public synchronized void setContentView(int layoutResId) {
        View inflated = getLayoutInflater().inflate(layoutResId, null, false);
        if (inflated != null) {
            if (inflated.getId() == 0) {
                inflated.setId(layoutResId);
            }
            setContentView(inflated);
        } else {
            FrameLayout root = new FrameLayout(this);
            root.setId(layoutResId);
            setContentView(root);
        }
    }

    public synchronized void setContentView(View view) {
        this.contentView = view;
        window.setDecorView(view);
    }

    public synchronized View getContentView() {
        if (this.contentView != null) return this.contentView;
        if (window != null) {
            View v = window.findViewById(16908290);
            if (v instanceof ViewGroup && ((ViewGroup) v).getChildCount() > 0) {
                return ((ViewGroup) v).getChildAt(0);
            }
            if (window.getDecorView() != null) {
                return window.getDecorView();
            }
        }
        return null;
    }

    public synchronized View findViewById(int id) {
        if (contentView != null) {
            return contentView.findViewById(id);
        }
        return null;
    }

    public Window getWindow() {
        return window;
    }

    public ActionBar getActionBar() {
        return null;
    }

    public boolean requestWindowFeature(int featureId) {
        return true;
    }

    private CharSequence title = "App";
    public CharSequence getTitle() { return title; }
    public void setTitle(CharSequence title) { this.title = title; }
    public void setTitle(int titleId) { this.title = getString(titleId); }

    public synchronized Intent getIntent() {
        return this.intent;
    }

    public synchronized void setIntent(Intent newIntent) {
        this.intent = newIntent;
    }

    public void runOnUiThread(Runnable action) {
        if (Thread.currentThread() == Looper.getMainLooper().getThread()) {
            action.run();
        } else {
            mainHandler.post(action);
        }
    }

    public static final int RESULT_CANCELED = 0;
    public static final int RESULT_OK = -1;
    public static final int RESULT_FIRST_USER = 1;

    private int resultCode = RESULT_CANCELED;
    private Intent resultData = null;

    public synchronized void setResult(int resultCode) {
        this.resultCode = resultCode;
    }

    public synchronized void setResult(int resultCode, Intent data) {
        this.resultCode = resultCode;
        this.resultData = data;
    }

    public synchronized int getResultCode() { return resultCode; }
    public synchronized Intent getResultData() { return resultData; }

    public synchronized void finish() {
        this.isFinishing = true;
        if (nanoandroid.host.NanoClassLoader.onFinishActivityHook != null) {
            nanoandroid.host.NanoClassLoader.onFinishActivityHook.accept(this);
        }
    }

    public synchronized void finishAndRemoveTask() {
        finish();
    }

    public void startActivityForResult(Intent intent, int requestCode) {
        if (intent != null && nanoandroid.host.NanoClassLoader.onStartActivityForResultHook != null) {
            nanoandroid.host.NanoClassLoader.onStartActivityForResultHook.accept(intent, requestCode);
        } else {
            startActivity(intent);
        }
    }

    public void startActivityForResult(Intent intent, int requestCode, Bundle options) {
        startActivityForResult(intent, requestCode);
    }

    protected void onActivityResult(int requestCode, int resultCode, Intent data) {}

    public final void dispatchActivityResult(int requestCode, int resultCode, Intent data) {
        onActivityResult(requestCode, resultCode, data);
    }

    public synchronized boolean isFinishing() {
        return this.isFinishing;
    }

    public synchronized State getState() {
        return this.state;
    }

    @Override
    public int checkSelfPermission(String permission) {
        return PackageManager.PERMISSION_GRANTED;
    }

    public void requestPermissions(String[] permissions, int requestCode) {
        int[] results = new int[permissions.length];
        for (int i = 0; i < permissions.length; i++) {
            results[i] = PackageManager.PERMISSION_GRANTED;
        }
        onRequestPermissionsResult(requestCode, permissions, results);
    }

    public void onRequestPermissionsResult(int requestCode, String[] permissions, int[] grantResults) {}

    public void overridePendingTransition(int enterAnim, int exitAnim) {}

    public android.view.LayoutInflater getLayoutInflater() {
        return android.view.LayoutInflater.from(this);
    }

    public android.view.MenuInflater getMenuInflater() {
        return new android.view.MenuInflater(this);
    }

    public boolean onCreateOptionsMenu(android.view.Menu menu) {
        return true;
    }

    public boolean onOptionsItemSelected(android.view.MenuItem item) {
        return false;
    }

    public boolean dispatchKeyEvent(android.view.KeyEvent event) { return false; }
    public boolean dispatchTouchEvent(android.view.MotionEvent event) { return false; }
    public View onCreatePanelView(int featureId) { return null; }
    public boolean onCreatePanelMenu(int featureId, android.view.Menu menu) { return false; }
    public boolean onPreparePanel(int featureId, View view, android.view.Menu menu) { return false; }
    public boolean onMenuOpened(int featureId, android.view.Menu menu) { return false; }
    public boolean onMenuItemSelected(int featureId, android.view.MenuItem item) { return false; }
    public void onWindowAttributesChanged(android.view.WindowManager.LayoutParams attrs) {}
    public void onContentChanged() {}

    public void registerActivityLifecycleCallbacks(Application.ActivityLifecycleCallbacks callback) {}
    public void unregisterActivityLifecycleCallbacks(Application.ActivityLifecycleCallbacks callback) {}

    private final java.util.List<Fragment> addedFragments = new java.util.ArrayList<>();
    private final java.util.Map<String, Fragment> tagFragments = new java.util.HashMap<>();
    private final java.util.Map<Integer, Fragment> idFragments = new java.util.HashMap<>();

    private final FragmentManager fragmentManager = new FragmentManager() {
        private void attachFragment(int containerViewId, Fragment f, String tag, boolean replace) {
            if (f == null) return;
            try {
                f.setActivity(Activity.this);
                f.onAttach(Activity.this);
                f.onCreate(null);
                View container = (containerViewId != 0) ? findViewById(containerViewId) : getContentView();
                ViewGroup vg = (container instanceof ViewGroup) ? (ViewGroup) container : null;
                if (replace && vg != null) {
                    vg.removeAllViews();
                }
                View fv = f.onCreateView(getLayoutInflater(), vg, null);
                if (fv != null) {
                    f.setView(fv);
                    if (vg != null && fv.getParent() == null) {
                        vg.addView(fv);
                    } else if (getContentView() == null) {
                        setContentView(fv);
                    }
                    f.onViewCreated(fv, null);
                }
                f.onStart();
                f.onResume();
                addedFragments.add(f);
                if (tag != null) tagFragments.put(tag, f);
                if (containerViewId != 0) idFragments.put(containerViewId, f);
            } catch (Throwable ignored) {}
        }

        @Override
        public FragmentTransaction beginTransaction() {
            return new FragmentTransaction() {
                private final java.util.List<Runnable> ops = new java.util.ArrayList<>();
                @Override public FragmentTransaction add(Fragment f, String tag) { ops.add(() -> attachFragment(0, f, tag, false)); return this; }
                @Override public FragmentTransaction add(int id, Fragment f) { ops.add(() -> attachFragment(id, f, null, false)); return this; }
                @Override public FragmentTransaction add(int id, Fragment f, String tag) { ops.add(() -> attachFragment(id, f, tag, false)); return this; }
                @Override public FragmentTransaction replace(int id, Fragment f) { ops.add(() -> attachFragment(id, f, null, true)); return this; }
                @Override public FragmentTransaction remove(Fragment f) { ops.add(() -> addedFragments.remove(f)); return this; }
                @Override public int commit() { ops.forEach(Runnable::run); return 0; }
                @Override public int commitAllowingStateLoss() { ops.forEach(Runnable::run); return 0; }
                @Override public void commitNow() { ops.forEach(Runnable::run); }
                @Override public void commitNowAllowingStateLoss() { ops.forEach(Runnable::run); }
            };
        }
        @Override public Fragment findFragmentById(int id) { return idFragments.get(id); }
        @Override public Fragment findFragmentByTag(String tag) { return tagFragments.get(tag); }
        @Override public boolean executePendingTransactions() { return true; }
    };

    public FragmentManager getFragmentManager() {
        return fragmentManager;
    }

    public void setTaskDescription(ActivityManager.TaskDescription taskDescription) {}

    public void onBackPressed() {
        finish();
    }

    public android.window.OnBackInvokedDispatcher getOnBackInvokedDispatcher() {
        return null;
    }
}
