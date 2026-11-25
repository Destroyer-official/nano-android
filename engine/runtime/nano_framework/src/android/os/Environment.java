package android.os;

import java.io.File;

public class Environment {
    public static final String DIRECTORY_MOVIES = "Movies";
    public static final String DIRECTORY_DOWNLOADS = "Download";
    public static final String DIRECTORY_PICTURES = "Pictures";
    public static final String DIRECTORY_DCIM = "DCIM";
    public static final String DIRECTORY_DOCUMENTS = "Documents";
    public static final String DIRECTORY_MUSIC = "Music";
    public static final String MEDIA_MOUNTED = "mounted";

    public static File getExternalStorageDirectory() {
        String baseProp = System.getProperty("nano.sandbox.dir");
        File root = (baseProp != null && !baseProp.trim().isEmpty())
                ? new File(baseProp)
                : new File("cache", "sandbox");
        File dir = new File(root, "shared_storage");
        if (!dir.exists()) {
            dir.mkdirs();
        }
        return dir;
    }

    public static File getExternalStoragePublicDirectory(String type) {
        File dir = (type != null && !type.isEmpty())
                ? new File(getExternalStorageDirectory(), type)
                : getExternalStorageDirectory();
        if (!dir.exists()) {
            dir.mkdirs();
        }
        return dir;
    }

    public static String getExternalStorageState() {
        return MEDIA_MOUNTED;
    }

    public static boolean isExternalStorageEmulated() {
        return true;
    }

    public static boolean isExternalStorageLegacy() {
        return true;
    }
}
