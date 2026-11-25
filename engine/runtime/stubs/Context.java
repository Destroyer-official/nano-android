package android.content;

import android.content.res.Resources;
import android.os.Looper;
import java.util.HashMap;
import java.util.Map;

public class Context {
    private String packageName = "com.example.app";
    private final Map<String, SharedPreferences> sharedPrefs = new HashMap<>();
    private final Resources resources = new Resources();
    private Looper mainLooper = Looper.getMainLooper();

    public Context() {}

    public Context(String packageName) {
        this.packageName = packageName;
    }

    public synchronized String getPackageName() {
        return packageName;
    }

    public synchronized void setPackageName(String packageName) {
        this.packageName = packageName;
    }

    public synchronized SharedPreferences getSharedPreferences(String name, int mode) {
        return sharedPrefs.computeIfAbsent(name, k -> new SharedPreferences.MemorySharedPreferences());
    }

    public synchronized Resources getResources() {
        return resources;
    }

    public synchronized Looper getMainLooper() {
        return mainLooper;
    }

    public synchronized void setMainLooper(Looper looper) {
        this.mainLooper = looper;
    }

    public void startActivity(Intent intent) {
        // Mock starter
    }
}
