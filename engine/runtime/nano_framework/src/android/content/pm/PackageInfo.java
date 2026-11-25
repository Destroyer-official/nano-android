package android.content.pm;

public class PackageInfo {
    public String packageName = "com.simplemobiletools.calendar";
    public String versionName = "1.2.3";
    public int versionCode = 123;
    public long firstInstallTime = System.currentTimeMillis();
    public long lastUpdateTime = System.currentTimeMillis();
    public ApplicationInfo applicationInfo = new ApplicationInfo();
    public Signature[] signatures = new Signature[0];
    public String[] requestedPermissions = new String[0];
    public int[] requestedPermissionsFlags = new int[0];
    public ActivityInfo[] activities = new ActivityInfo[0];

    public PackageInfo() {
        if (applicationInfo == null) {
            applicationInfo = new ApplicationInfo();
        }
        applicationInfo.packageName = packageName;
    }

    public long getLongVersionCode() {
        return versionCode;
    }
}
