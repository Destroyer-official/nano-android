package nanoandroid.runtime;

import android.app.Activity;
import android.content.Context;
import android.content.Intent;
import android.os.Bundle;
import java.io.*;
import java.util.HashMap;
import java.util.Map;

/**
 * NanoHost is an OpenJDK 25 headless CLI and IPC host runner for Dalvik / Android classes.
 */
public class NanoHost {
    private final DexClassLoader classLoader;
    private final NanoLooper looper;
    private Activity activeActivity;

    public NanoHost() {
        this.classLoader = new DexClassLoader();
        this.looper = new NanoLooper(0);
    }

    public static class LaunchResult {
        public String activityName;
        public String state;
        public long durationNanos;
        public boolean success;
        public String error;
    }

    public LaunchResult launch(String apkPath, String activityName, Map<String, String> extras) {
        LaunchResult result = new LaunchResult();
        long start = System.nanoTime();

        try {
            if (apkPath != null && !apkPath.isEmpty()) {
                File apkFile = new File(apkPath);
                if (apkFile.exists()) {
                    classLoader.loadFromApk(apkFile);
                }
            }

            DexClassLoader.DexClassEntry classEntry = classLoader.resolveClass(activityName);
            String targetName = (classEntry != null) ? classEntry.dottedName : activityName;
            result.activityName = targetName;

            Context ctx = new Context("com.example.app");
            Intent intent = new Intent(Intent.ACTION_MAIN);
            intent.addCategory(Intent.CATEGORY_LAUNCHER);
            if (extras != null) {
                for (Map.Entry<String, String> entry : extras.entrySet()) {
                    intent.putExtra(entry.getKey(), entry.getValue());
                }
            }

            Activity activity = new Activity();
            activity.attach(ctx, intent);

            // Lifecycle transitions
            Bundle savedState = new Bundle();
            activity.getClass().getDeclaredMethod("onCreate", Bundle.class).setAccessible(true);
            activity.getClass().getDeclaredMethod("onStart").setAccessible(true);
            activity.getClass().getDeclaredMethod("onResume").setAccessible(true);

            // Trigger methods
            classLoader.invokeMethod(classEntry, "onCreate", activity, savedState);
            classLoader.invokeMethod(classEntry, "onStart", activity);
            classLoader.invokeMethod(classEntry, "onResume", activity);

            this.activeActivity = activity;
            result.state = "RESUMED";
            result.success = true;
        } catch (Exception e) {
            result.success = false;
            result.error = e.getMessage();
        }

        result.durationNanos = System.nanoTime() - start;
        return result;
    }

    public int stepLooper(long deltaMs) {
        return looper.step(deltaMs);
    }

    public static void main(String[] args) {
        NanoHost host = new NanoHost();
        String apk = (args.length > 0) ? args[0] : "";
        String activity = (args.length > 1) ? args[1] : "MainActivity";

        LaunchResult res = host.launch(apk, activity, new HashMap<>());
        System.out.printf("{\"activity\":\"%s\",\"state\":\"%s\",\"duration_ms\":%.2f,\"success\":%b}\n",
                res.activityName, res.state, res.durationNanos / 1_000_000.0, res.success);
    }
}
