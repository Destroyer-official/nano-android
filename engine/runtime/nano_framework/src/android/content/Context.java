package android.content;

import android.content.pm.PackageManager;
import android.content.res.Resources;
import android.graphics.drawable.Drawable;
import android.os.Handler;
import android.os.Looper;
import java.io.File;
import java.util.HashMap;
import java.util.Map;

public class Context {
    public static final int RECEIVER_EXPORTED = 2;
    public static final int RECEIVER_NOT_EXPORTED = 4;
    public static final int MODE_PRIVATE = 0;

    private String packageName = "com.example.app";
    private final Map<String, SharedPreferences> sharedPrefs = new HashMap<>();
    private final Resources resources = new Resources();
    private final Map<BroadcastReceiver, IntentFilter> receivers = new HashMap<>();
    private static volatile nanoandroid.host.ApkResourceTable globalApkTable;

    public Context() {
        if (globalApkTable != null) {
            this.resources.setApkTable(globalApkTable);
        }
    }
    public Context(String packageName) {
        this();
        this.packageName = packageName;
    }

    public static void setGlobalApkTable(nanoandroid.host.ApkResourceTable table) {
        globalApkTable = table;
    }

    public static nanoandroid.host.ApkResourceTable getGlobalApkTable() {
        return globalApkTable;
    }

    public String getPackageName() { return packageName; }
    public void setPackageName(String packageName) { this.packageName = packageName; }

    public ClassLoader getClassLoader() {
        ClassLoader cl = Thread.currentThread().getContextClassLoader();
        return cl != null ? cl : getClass().getClassLoader();
    }

    public Resources getResources() {
        if (resources.getApkTable() == null && globalApkTable != null) {
            resources.setApkTable(globalApkTable);
        }
        return resources;
    }

    public File getNanoSandboxDir(String subPath) {
        String pkg = (packageName != null && !packageName.trim().isEmpty()) ? packageName : "com.example.app";
        String safePkg = pkg.replaceAll("[^a-zA-Z0-9._-]", "_");
        String baseProp = System.getProperty("nano.sandbox.dir");
        File root = (baseProp != null && !baseProp.trim().isEmpty())
                ? new File(baseProp)
                : new File("cache", "sandbox");
        File pkgDir = new File(root, safePkg);
        File target = (subPath != null && !subPath.isEmpty()) ? new File(pkgDir, subPath) : pkgDir;
        if (!target.exists()) {
            target.mkdirs();
        }
        return target;
    }

    public File[] getExternalFilesDirs(String type) {
        File dir = (type != null && !type.isEmpty())
                ? getNanoSandboxDir("external_files/" + type)
                : getNanoSandboxDir("external_files");
        return new File[]{dir};
    }

    public File getExternalFilesDir(String type) {
        return getExternalFilesDirs(type)[0];
    }

    public File getFilesDir() {
        return getNanoSandboxDir("files");
    }

    public File getCacheDir() {
        return getNanoSandboxDir("cache");
    }

    public File getDatabasePath(String name) {
        File dbDir = getNanoSandboxDir("databases");
        String safeName = (name == null || name.trim().isEmpty()) ? "app.db" : new File(name).getName();
        return new File(dbDir, safeName);
    }

    public File getExternalCacheDir() {
        return getNanoSandboxDir("external_cache");
    }

    public File getDataDir() {
        return getNanoSandboxDir(null);
    }

    public java.io.FileInputStream openFileInput(String name) throws java.io.FileNotFoundException {
        return new java.io.FileInputStream(new File(getFilesDir(), name));
    }

    public java.io.FileOutputStream openFileOutput(String name, int mode) throws java.io.FileNotFoundException {
        File f = new File(getFilesDir(), name);
        if (f.getParentFile() != null) f.getParentFile().mkdirs();
        return new java.io.FileOutputStream(f);
    }

    public boolean deleteDatabase(String name) {
        File f = getDatabasePath(name);
        return !f.exists() || f.delete();
    }

    public CharSequence getText(int resId) {
        return getString(resId);
    }

    public String getString(int resId) {
        return resources.getString(resId);
    }

    public String getString(int resId, Object... formatArgs) {
        return resources.getString(resId, formatArgs);
    }

    public int getColor(int id) {
        return getResources().getColor(id);
    }

    public android.content.res.ColorStateList getColorStateList(int id) {
        return getResources().getColorStateList(id);
    }

    public void setTheme(int resid) {}
    public Resources.Theme getTheme() {
        return resources.newTheme();
    }

    public Context createConfigurationContext(android.content.res.Configuration overrideConfiguration) {
        return this;
    }

    public SharedPreferences getSharedPreferences(String name, int mode) {
        String key = (name != null && !name.isEmpty()) ? name : "default";
        return sharedPrefs.computeIfAbsent(key, k -> {
            String safeName = k.replaceAll("[^a-zA-Z0-9._-]", "_") + ".properties";
            return new SharedPreferences.MemorySharedPreferences(new File(getNanoSandboxDir("shared_prefs"), safeName));
        });
    }

    public static final String LAYOUT_INFLATER_SERVICE = "layout_inflater";
    public static final String POWER_SERVICE = "power";
    public static final String WINDOW_SERVICE = "window";
    public static final String ACTIVITY_SERVICE = "activity";
    public static final String AUDIO_SERVICE = "audio";
    public static final String VIBRATOR_SERVICE = "vibrator";

    public Context getApplicationContext() { return this; }

    public PackageManager getPackageManager() {
        return new PackageManager();
    }

    public android.content.pm.ApplicationInfo getApplicationInfo() {
        android.content.pm.ApplicationInfo ai = new android.content.pm.ApplicationInfo();
        ai.packageName = packageName;
        return ai;
    }

    public Object getSystemService(String name) {
        if (LAYOUT_INFLATER_SERVICE.equals(name)) {
            return android.view.LayoutInflater.from(this);
        } else if (POWER_SERVICE.equals(name)) {
            return new android.os.PowerManager();
        } else if (ACTIVITY_SERVICE.equals(name)) {
            return new android.app.ActivityManager();
        } else if (WINDOW_SERVICE.equals(name)) {
            return null;
        } else if ("shortcut".equals(name)) {
            return new android.content.pm.ShortcutManager();
        } else if ("jobscheduler".equals(name)) {
            return new android.app.job.JobScheduler() {
                @Override public int schedule(android.app.job.JobInfo job) { return RESULT_SUCCESS; }
                @Override public int enqueue(android.app.job.JobInfo job, android.app.job.JobWorkItem work) { return RESULT_SUCCESS; }
                @Override public void cancel(int jobId) {}
                @Override public void cancelAll() {}
                @Override public java.util.List<android.app.job.JobInfo> getAllPendingJobs() { return new java.util.ArrayList<>(); }
                @Override public android.app.job.JobInfo getPendingJob(int jobId) { return null; }
            };
        }
        return null;
    }

    @SuppressWarnings("unchecked")
    public <T> T getSystemService(Class<T> serviceClass) {
        if (serviceClass == android.content.pm.ShortcutManager.class) {
            return (T) new android.content.pm.ShortcutManager();
        } else if (serviceClass == android.app.ActivityManager.class) {
            return (T) new android.app.ActivityManager();
        } else if (serviceClass == android.os.PowerManager.class) {
            return (T) new android.os.PowerManager();
        } else if (serviceClass == android.view.LayoutInflater.class) {
            return (T) android.view.LayoutInflater.from(this);
        }
        return null;
    }

    public int checkSelfPermission(String permission) {
        // In headless testing, grant permissions by default so test suites can exercise features
        return PackageManager.PERMISSION_GRANTED;
    }
    public int checkPermission(String permission, int pid, int uid) {
        return PackageManager.PERMISSION_GRANTED;
    }
    public int checkCallingPermission(String permission) {
        return PackageManager.PERMISSION_GRANTED;
    }
    public int checkCallingOrSelfPermission(String permission) {
        return PackageManager.PERMISSION_GRANTED;
    }

    public android.content.res.TypedArray obtainStyledAttributes(int[] attrs) {
        return new android.content.res.TypedArray(resources);
    }
    public android.content.res.TypedArray obtainStyledAttributes(int resid, int[] attrs) {
        return new android.content.res.TypedArray(resources);
    }
    public android.content.res.TypedArray obtainStyledAttributes(android.util.AttributeSet set, int[] attrs) {
        return new android.content.res.TypedArray(resources);
    }
    public android.content.res.TypedArray obtainStyledAttributes(android.util.AttributeSet set, int[] attrs, int defStyleAttr, int defStyleRes) {
        return new android.content.res.TypedArray(resources);
    }

    public Intent registerReceiver(BroadcastReceiver receiver, IntentFilter filter) {
        receivers.put(receiver, filter);
        return null;
    }

    public Intent registerReceiver(BroadcastReceiver receiver, IntentFilter filter, int flags) {
        return registerReceiver(receiver, filter);
    }

    public void unregisterReceiver(BroadcastReceiver receiver) {
        receivers.remove(receiver);
    }

    public void sendBroadcast(Intent intent) {
        if (intent == null) return;
        for (Map.Entry<BroadcastReceiver, IntentFilter> entry : receivers.entrySet()) {
            if (entry.getValue().matchAction(intent.getAction())) {
                entry.getKey().onReceive(this, intent);
            }
        }
    }

    public void startActivity(Intent intent) {
        if (intent == null) return;
        if (nanoandroid.host.NanoClassLoader.onStartActivityForResultHook != null) {
            nanoandroid.host.NanoClassLoader.onStartActivityForResultHook.accept(intent, -1);
        } else if (nanoandroid.host.NanoClassLoader.onStartActivityHook != null) {
            nanoandroid.host.NanoClassLoader.onStartActivityHook.accept(intent);
        }
    }

    private final ContentResolver contentResolver = new ContentResolver();

    public Drawable getDrawable(int id) {
        return getResources().getDrawable(id);
    }

    public ContentResolver getContentResolver() { return contentResolver; }
}
