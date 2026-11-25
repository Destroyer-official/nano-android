package android.content.pm;

public class ApplicationInfo {
    public static final int FLAG_SYSTEM = 1 << 0;
    public static final int FLAG_DEBUGGABLE = 1 << 1;

    public String name;
    public String packageName;
    public String className;
    public String sourceDir = "";
    public String dataDir = "/data/user/0";
    public android.os.Bundle metaData = new android.os.Bundle();
    public int flags = FLAG_DEBUGGABLE;
    public int targetSdkVersion = 35;
    public int minSdkVersion = 26;
    public int uid = 10000;

    public ApplicationInfo() {}
}
