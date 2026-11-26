package android.content;

import android.content.pm.PackageManager;
import android.content.res.Resources;
import android.graphics.drawable.Drawable;
import android.os.Handler;
import android.os.Looper;
import java.io.File;
import java.io.IOException;
import java.nio.file.Path;
import java.util.Collection;
import java.util.HashMap;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;

public class Context {
    public static final int RECEIVER_EXPORTED = 2;
    public static final int RECEIVER_NOT_EXPORTED = 4;
    public static final int MODE_PRIVATE = 0;

    private String packageName = "com.example.app";
    private final Map<String, SharedPreferences> sharedPrefs = new HashMap<>();
    private static final Map<String, SharedPreferences> GLOBAL_SHARED_PREFS = new ConcurrentHashMap<>();
    private final Resources resources = new Resources();

    private static final class RegisteredReceiver {
        final IntentFilter filter;
        final boolean exported;

        RegisteredReceiver(IntentFilter filter, boolean exported) {
            this.filter = filter;
            this.exported = exported;
        }
    }

    private final Map<BroadcastReceiver, RegisteredReceiver> receivers = new HashMap<>();
    private static final Map<String, android.app.Service> ACTIVE_SERVICES = new ConcurrentHashMap<>();
    private static final Map<ServiceConnection, String> BOUND_CONNECTIONS = new ConcurrentHashMap<>();
    private static int nextServiceStartId = 1;
    private static volatile long storageQuotaBytes = 256L * 1024L * 1024L;
    private static volatile nanoandroid.host.ApkResourceTable globalApkTable;
    private static final Map<String, Map<String, Integer>> PERMISSION_STATES = new ConcurrentHashMap<>();
    private static final Set<String> RUNTIME_DANGEROUS_PERMISSIONS = Set.of(
            "android.permission.READ_SMS",
            "android.permission.RECEIVE_SMS",
            "android.permission.SEND_SMS",
            "android.permission.READ_CALL_LOG",
            "android.permission.WRITE_CALL_LOG",
            "android.permission.READ_CONTACTS",
            "android.permission.WRITE_CONTACTS",
            "android.permission.CAMERA",
            "android.permission.RECORD_AUDIO",
            "android.permission.ACCESS_FINE_LOCATION",
            "android.permission.ACCESS_COARSE_LOCATION",
            "android.permission.ACCESS_BACKGROUND_LOCATION",
            "android.permission.READ_EXTERNAL_STORAGE",
            "android.permission.WRITE_EXTERNAL_STORAGE",
            "android.permission.READ_MEDIA_VIDEO",
            "android.permission.READ_MEDIA_IMAGES",
            "android.permission.READ_MEDIA_AUDIO",
            "android.permission.POST_NOTIFICATIONS",
            "android.permission.ACCESS_LOCAL_NETWORK");

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

    public String getPackageName() {
        return packageName;
    }

    public void setPackageName(String packageName) {
        this.packageName = packageName;
    }

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
        File pkgDir = resolveContainedFile(root, safePkg);
        File target = (subPath != null && !subPath.isEmpty()) ? resolveContainedFile(pkgDir, subPath) : pkgDir;
        if (!target.exists()) {
            target.mkdirs();
        }
        return target;
    }

    private static File resolveContainedFile(File baseDir, String childPath) {
        if (childPath == null || childPath.trim().isEmpty()) {
            throw new SecurityException("Empty path is not allowed inside sandbox");
        }
        if (childPath.indexOf('\0') >= 0) {
            throw new SecurityException("Null byte in sandbox path");
        }
        File candidateInput = new File(childPath);
        if (candidateInput.isAbsolute()) {
            throw new SecurityException("Absolute paths are forbidden inside sandbox: " + childPath);
        }
        try {
            if (!baseDir.exists()) {
                baseDir.mkdirs();
            }
            Path canonBase = baseDir.getCanonicalFile().toPath().normalize();
            Path resolved = canonBase.resolve(childPath).normalize();
            if (!resolved.startsWith(canonBase)) {
                throw new SecurityException("Path traversal blocked: " + childPath);
            }
            File target = resolved.toFile();
            if (target.exists()) {
                Path canonTarget = target.getCanonicalFile().toPath().normalize();
                if (!canonTarget.startsWith(canonBase)) {
                    throw new SecurityException("Symlink escape blocked: " + childPath);
                }
            }
            return target;
        } catch (IOException e) {
            throw new SecurityException("Failed resolving contained sandbox path: " + childPath, e);
        }
    }

    public File[] getExternalFilesDirs(String type) {
        File dir = (type != null && !type.isEmpty())
                ? getNanoSandboxDir("external_files/" + type)
                : getNanoSandboxDir("external_files");
        return new File[] { dir };
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
        String safeName = (name == null || name.trim().isEmpty()) ? "app.db" : name.trim();
        return resolveContainedFile(dbDir, safeName);
    }

    public File getExternalCacheDir() {
        return getNanoSandboxDir("external_cache");
    }

    public File getDataDir() {
        return getNanoSandboxDir(null);
    }

    public java.io.FileInputStream openFileInput(String name) throws java.io.FileNotFoundException {
        File resolved = resolveContainedFile(getFilesDir(), name);
        nanoandroid.host.ForensicTimeline.record("FILE_OP", "openFileInput", name);
        return new java.io.FileInputStream(resolved);
    }

    public java.io.FileOutputStream openFileOutput(String name, int mode) throws java.io.FileNotFoundException {
        if (getSandboxStorageBytes() > storageQuotaBytes) {
            throw new SecurityException("App sandbox storage quota exceeded (" + getSandboxStorageBytes() + " > "
                    + storageQuotaBytes + ")");
        }
        File f = resolveContainedFile(getFilesDir(), name);
        if (f.getParentFile() != null)
            f.getParentFile().mkdirs();
        nanoandroid.host.ForensicTimeline.record("FILE_OP", "openFileOutput", name);
        return new java.io.FileOutputStream(f);
    }

    public boolean deleteDatabase(String name) {
        File f = getDatabasePath(name);
        boolean deleted = !f.exists() || f.delete();
        if (deleted) {
            nanoandroid.host.ForensicTimeline.record("DB_MUTATION", name != null ? name : "app.db", "deleteDatabase");
        }
        return deleted;
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

    public void setTheme(int resid) {
    }

    public Resources.Theme getTheme() {
        return resources.newTheme();
    }

    public Context createConfigurationContext(android.content.res.Configuration overrideConfiguration) {
        return this;
    }

    public SharedPreferences getSharedPreferences(String name, int mode) {
        String key = (name != null && !name.isEmpty()) ? name : "default";
        SharedPreferences sp = sharedPrefs.computeIfAbsent(key, k -> {
            String pkgPrefix = (packageName != null && !packageName.isEmpty()) ? packageName : "com.example.app";
            return GLOBAL_SHARED_PREFS.computeIfAbsent(pkgPrefix + "/" + k, gk -> {
                String safeName = k.replaceAll("[^a-zA-Z0-9._-]", "_") + ".properties";
                return new SharedPreferences.MemorySharedPreferences(new File(getNanoSandboxDir("shared_prefs"), safeName));
            });
        });
        return sp;
    }

    public static Map<String, String> snapshotSharedPreferences(String pkg) {
        Map<String, String> out = new java.util.TreeMap<>();
        String prefix = (pkg != null && !pkg.isEmpty()) ? (pkg + "/") : "";
        for (Map.Entry<String, SharedPreferences> entry : GLOBAL_SHARED_PREFS.entrySet()) {
            if (!prefix.isEmpty() && !entry.getKey().startsWith(prefix)) {
                continue;
            }
            String prefFile = entry.getKey();
            for (Map.Entry<String, ?> kv : entry.getValue().getAll().entrySet()) {
                out.put(prefFile + "#" + kv.getKey(), String.valueOf(kv.getValue()));
            }
        }
        return out;
    }

    public static void clearTrackedSharedPreferences() {
        GLOBAL_SHARED_PREFS.clear();
    }

    public static final String LAYOUT_INFLATER_SERVICE = "layout_inflater";
    public static final String POWER_SERVICE = "power";
    public static final String WINDOW_SERVICE = "window";
    public static final String ACTIVITY_SERVICE = "activity";
    public static final String AUDIO_SERVICE = "audio";
    public static final String VIBRATOR_SERVICE = "vibrator";
    public static final String NOTIFICATION_SERVICE = "notification";
    public static final String ALARM_SERVICE = "alarm";
    public static final String CLIPBOARD_SERVICE = "clipboard";
    public static final String CONNECTIVITY_SERVICE = "connectivity";
    public static final String LOCATION_SERVICE = "location";

    private static final android.app.NotificationManager NOTIFICATION_MANAGER = new android.app.NotificationManager();
    private static final ClipboardManager CLIPBOARD_MANAGER = new ClipboardManager();

    public Context getApplicationContext() {
        return this;
    }

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
        } else if (NOTIFICATION_SERVICE.equals(name)) {
            return NOTIFICATION_MANAGER;
        } else if (ALARM_SERVICE.equals(name)) {
            return new android.app.AlarmManager(this);
        } else if (CLIPBOARD_SERVICE.equals(name)) {
            return CLIPBOARD_MANAGER;
        } else if (CONNECTIVITY_SERVICE.equals(name)) {
            return new android.net.ConnectivityManager();
        } else if (LOCATION_SERVICE.equals(name)) {
            return new android.location.LocationManager();
        } else if ("shortcut".equals(name)) {
            return new android.content.pm.ShortcutManager();
        } else if ("jobscheduler".equals(name)) {
            return new android.app.job.JobScheduler() {
                @Override
                public int schedule(android.app.job.JobInfo job) {
                    return RESULT_SUCCESS;
                }

                @Override
                public int enqueue(android.app.job.JobInfo job, android.app.job.JobWorkItem work) {
                    return RESULT_SUCCESS;
                }

                @Override
                public void cancel(int jobId) {
                }

                @Override
                public void cancelAll() {
                }

                @Override
                public java.util.List<android.app.job.JobInfo> getAllPendingJobs() {
                    return new java.util.ArrayList<>();
                }

                @Override
                public android.app.job.JobInfo getPendingJob(int jobId) {
                    return null;
                }
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

    public static void setPermissionState(String pkg, String permission, int state) {
        if (pkg == null || permission == null)
            return;
        PERMISSION_STATES.computeIfAbsent(pkg, k -> new ConcurrentHashMap<>()).put(permission, state);
    }

    public static void seedManifestPermissions(String pkg, Collection<String> manifestPermissions) {
        if (pkg == null || manifestPermissions == null)
            return;
        Map<String, Integer> map = PERMISSION_STATES.computeIfAbsent(pkg, k -> new ConcurrentHashMap<>());
        for (String perm : manifestPermissions) {
            if (perm == null)
                continue;
            // Install-time normal permissions are granted; dangerous runtime permissions
            // default to DENIED until explicitly granted.
            int initial = RUNTIME_DANGEROUS_PERMISSIONS.contains(perm)
                    ? PackageManager.PERMISSION_DENIED
                    : PackageManager.PERMISSION_GRANTED;
            map.putIfAbsent(perm, initial);
        }
    }

    public static void resetPermissions(String pkg) {
        if (pkg != null) {
            PERMISSION_STATES.remove(pkg);
        }
    }

    public int checkSelfPermission(String permission) {
        if (permission == null || permission.trim().isEmpty()) {
            return PackageManager.PERMISSION_DENIED;
        }
        String pkg = (packageName != null && !packageName.isEmpty()) ? packageName : "com.example.app";
        Map<String, Integer> map = PERMISSION_STATES.get(pkg);
        if (map != null && map.containsKey(permission)) {
            return map.get(permission);
        }
        return PackageManager.PERMISSION_DENIED;
    }

    public int checkPermission(String permission, int pid, int uid) {
        return checkSelfPermission(permission);
    }

    public int checkCallingPermission(String permission) {
        return checkSelfPermission(permission);
    }

    public int checkCallingOrSelfPermission(String permission) {
        return checkSelfPermission(permission);
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

    public android.content.res.TypedArray obtainStyledAttributes(android.util.AttributeSet set, int[] attrs,
            int defStyleAttr, int defStyleRes) {
        return new android.content.res.TypedArray(resources);
    }

    public Intent registerReceiver(BroadcastReceiver receiver, IntentFilter filter) {
        return registerReceiver(receiver, filter, RECEIVER_NOT_EXPORTED);
    }

    public Intent registerReceiver(BroadcastReceiver receiver, IntentFilter filter, int flags) {
        if (receiver == null || filter == null)
            return null;
        boolean exported = (flags & RECEIVER_EXPORTED) != 0;
        receivers.put(receiver, new RegisteredReceiver(filter, exported));
        return null;
    }

    public void unregisterReceiver(BroadcastReceiver receiver) {
        receivers.remove(receiver);
    }

    public void sendBroadcast(Intent intent) {
        sendBroadcast(intent, false);
    }

    public void sendBroadcast(Intent intent, boolean fromExternalPackage) {
        if (intent == null)
            return;
        for (Map.Entry<BroadcastReceiver, RegisteredReceiver> entry : new java.util.ArrayList<>(receivers.entrySet())) {
            RegisteredReceiver reg = entry.getValue();
            if (fromExternalPackage && !reg.exported) {
                continue;
            }
            if (reg.filter.match(intent)) {
                android.app.LifecycleEventTracer.record("BROADCAST_RECEIVE", entry.getKey().getClass().getName(),
                        intent.getAction() != null ? intent.getAction() : "");
                entry.getKey().onReceive(this, intent);
            }
        }
    }

    public static void clearServices() {
        for (android.app.Service svc : new java.util.ArrayList<>(ACTIVE_SERVICES.values())) {
            try {
                svc.onDestroy();
            } catch (Throwable ignored) {
            }
        }
        ACTIVE_SERVICES.clear();
        BOUND_CONNECTIONS.clear();
        nextServiceStartId = 1;
    }

    public static Map<String, android.app.Service> getActiveServices() {
        return java.util.Collections.unmodifiableMap(new java.util.LinkedHashMap<>(ACTIVE_SERVICES));
    }

    public static void stopServiceInstance(android.app.Service svc, int startId) {
        if (svc == null)
            return;
        String cls = svc.getClass().getName();
        android.app.Service removed = ACTIVE_SERVICES.remove(cls);
        if (removed != null) {
            try {
                removed.onDestroy();
            } catch (Throwable ignored) {
            }
        }
    }

    private android.app.Service getOrCreateService(ComponentName cn) {
        if (cn == null || cn.getClassName() == null || cn.getClassName().isEmpty())
            return null;
        String clsName = cn.getClassName();
        android.app.Service existing = ACTIVE_SERVICES.get(clsName);
        if (existing != null)
            return existing;
        try {
            ClassLoader cl = getClassLoader();
            Class<?> cls = cl.loadClass(clsName);
            Object obj = cls.getDeclaredConstructor().newInstance();
            if (obj instanceof android.app.Service) {
                android.app.Service svc = (android.app.Service) obj;
                svc.attach(new Context(getPackageName()), android.app.Activity.currentApplication);
                svc.onCreate();
                ACTIVE_SERVICES.put(clsName, svc);
                return svc;
            }
        } catch (Throwable ignored) {
        }
        return null;
    }

    public ComponentName startService(Intent service) {
        if (service == null || service.getComponent() == null)
            return null;
        android.app.Service svc = getOrCreateService(service.getComponent());
        if (svc == null)
            return null;
        svc.onStartCommand(service, 0, nextServiceStartId++);
        return service.getComponent();
    }

    public boolean stopService(Intent service) {
        if (service == null || service.getComponent() == null)
            return false;
        android.app.Service svc = ACTIVE_SERVICES.remove(service.getComponent().getClassName());
        if (svc == null)
            return false;
        svc.onDestroy();
        return true;
    }

    public boolean bindService(Intent service, ServiceConnection conn, int flags) {
        if (service == null || conn == null || service.getComponent() == null)
            return false;
        android.app.Service svc = getOrCreateService(service.getComponent());
        if (svc == null)
            return false;
        android.os.IBinder binder = svc.onBind(service);
        BOUND_CONNECTIONS.put(conn, service.getComponent().getClassName());
        if (binder != null) {
            conn.onServiceConnected(service.getComponent(), binder);
        } else {
            conn.onNullBinding(service.getComponent());
        }
        return true;
    }

    public void unbindService(ServiceConnection conn) {
        if (conn == null)
            return;
        String cls = BOUND_CONNECTIONS.remove(conn);
        if (cls != null) {
            android.app.Service svc = ACTIVE_SERVICES.get(cls);
            if (svc != null) {
                svc.onUnbind(new Intent());
            }
            conn.onServiceDisconnected(new ComponentName(getPackageName(), cls));
        }
    }

    public static void setStorageQuotaBytes(long quotaBytes) {
        storageQuotaBytes = Math.max(1024L, quotaBytes);
    }

    public static long getStorageQuotaBytes() {
        return storageQuotaBytes;
    }

    public long getSandboxStorageBytes() {
        return computeDirectoryBytes(getDataDir());
    }

    private static long computeDirectoryBytes(File dir) {
        if (dir == null || !dir.exists())
            return 0L;
        if (dir.isFile())
            return dir.length();
        File[] children = dir.listFiles();
        if (children == null)
            return 0L;
        long total = 0L;
        for (File c : children) {
            total += computeDirectoryBytes(c);
        }
        return total;
    }

    public void startActivity(Intent intent) {
        if (intent == null)
            return;
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

    public ContentResolver getContentResolver() {
        return contentResolver;
    }
}
