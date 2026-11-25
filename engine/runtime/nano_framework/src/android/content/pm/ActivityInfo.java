package android.content.pm;

public class ActivityInfo {
    public static final int SCREEN_ORIENTATION_UNSPECIFIED = -1;
    public static final int SCREEN_ORIENTATION_PORTRAIT = 1;
    public static final int SCREEN_ORIENTATION_LANDSCAPE = 0;

    public static final int CONFIG_ORIENTATION = 0x0080;
    public static final int CONFIG_KEYBOARD = 0x0010;
    public static final int CONFIG_KEYBOARD_HIDDEN = 0x0020;
    public static final int CONFIG_SCREEN_SIZE = 0x0400;
    public static final int CONFIG_SMALLEST_SCREEN_SIZE = 0x0800;
    public static final int CONFIG_UI_MODE = 0x0200;
    public static final int CONFIG_SCREEN_LAYOUT = 0x0100;
    public static final int CONFIG_FONT_SCALE = 0x40000000;
    public static final int CONFIG_DENSITY = 0x1000;
    public static final int CONFIG_LOCALE = 0x0004;

    public String name;
    public String packageName;
    public String parentActivityName;
    public String processName;
    public String permission;
    public String targetActivity;
    public String taskAffinity;
    public android.os.Bundle metaData = new android.os.Bundle();
    public int launchMode;
    public int theme;
    public int screenOrientation = SCREEN_ORIENTATION_PORTRAIT;
    public int flags;
    public int configChanges = 0;
    public int softInputMode = 0;

    public ActivityInfo() {}
}
