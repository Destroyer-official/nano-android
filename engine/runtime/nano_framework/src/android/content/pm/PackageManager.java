package android.content.pm;

import android.content.ComponentName;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;

public class PackageManager {
    public static final int PERMISSION_GRANTED = 0;
    public static final int PERMISSION_DENIED = -1;
    public static final int GET_META_DATA = 128;
    public static final int GET_SIGNATURES = 64;

    private static final Set<String> SUPPORTED_FEATURES = ConcurrentHashMap.newKeySet();
    static {
        SUPPORTED_FEATURES.add("android.hardware.touchscreen");
        SUPPORTED_FEATURES.add("android.hardware.screen.portrait");
        SUPPORTED_FEATURES.add("android.hardware.screen.landscape");
        SUPPORTED_FEATURES.add("android.software.activities_on_secondary_displays");
    }

    public static void setSystemFeature(String name, boolean supported) {
        if (name == null) return;
        if (supported) SUPPORTED_FEATURES.add(name);
        else SUPPORTED_FEATURES.remove(name);
    }

    public static class NameNotFoundException extends Exception {
        public NameNotFoundException() {
            super();
        }
        public NameNotFoundException(String name) {
            super(name);
        }
    }

    public PackageInfo getPackageInfo(String packageName, int flags) throws NameNotFoundException {
        if (packageName != null && !packageName.isEmpty()) {
            PackageInfo pi = new PackageInfo();
            pi.packageName = packageName;
            if (pi.applicationInfo != null) {
                pi.applicationInfo.packageName = packageName;
            }
            return pi;
        }
        throw new NameNotFoundException("Package not installed: " + packageName);
    }

    public ApplicationInfo getApplicationInfo(String packageName, int flags) throws NameNotFoundException {
        if (packageName != null && !packageName.isEmpty()) {
            ApplicationInfo ai = new ApplicationInfo();
            ai.packageName = packageName;
            return ai;
        }
        throw new NameNotFoundException("Application not installed: " + packageName);
    }

    public ActivityInfo getActivityInfo(ComponentName component, int flags) {
        ActivityInfo ai = new ActivityInfo();
        ai.packageName = (component != null) ? component.getPackageName() : "";
        ai.name = (component != null) ? component.getClassName() : "";
        return ai;
    }

    public ServiceInfo getServiceInfo(ComponentName component, int flags) throws NameNotFoundException {
        throw new NameNotFoundException("Service not registered: " + (component != null ? component.getClassName() : "null"));
    }

    public ProviderInfo getProviderInfo(ComponentName component, int flags) throws NameNotFoundException {
        throw new NameNotFoundException("Provider not registered: " + (component != null ? component.getClassName() : "null"));
    }

    public boolean hasSystemFeature(String name) {
        return name != null && SUPPORTED_FEATURES.contains(name);
    }
}
