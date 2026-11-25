package android.content;

import android.content.pm.ApplicationInfo;
import android.content.pm.PackageManager;
import android.content.res.Resources;
import android.content.res.TypedArray;
import android.graphics.drawable.Drawable;
import android.os.Bundle;
import android.util.AttributeSet;
import java.io.File;

/**
 * Proxying Context implementation matching Android SDK ContextWrapper so Application
 * and Service subclasses resolve attachBaseContext/getBaseContext without hitting android.jar stubs.
 */
public class ContextWrapper extends Context {
    Context mBase;

    public ContextWrapper(Context base) {
        mBase = base;
    }

    protected void attachBaseContext(Context base) {
        if (mBase == null) {
            mBase = base;
        }
    }

    public Context getBaseContext() {
        return mBase;
    }

    @Override
    public String getPackageName() {
        return mBase != null ? mBase.getPackageName() : super.getPackageName();
    }

    @Override
    public void setPackageName(String packageName) {
        if (mBase != null) {
            mBase.setPackageName(packageName);
        } else {
            super.setPackageName(packageName);
        }
    }

    @Override
    public ClassLoader getClassLoader() {
        return mBase != null ? mBase.getClassLoader() : super.getClassLoader();
    }

    @Override
    public Resources getResources() {
        return mBase != null ? mBase.getResources() : super.getResources();
    }

    @Override
    public File[] getExternalFilesDirs(String type) {
        return mBase != null ? mBase.getExternalFilesDirs(type) : super.getExternalFilesDirs(type);
    }

    @Override
    public File getFilesDir() {
        return mBase != null ? mBase.getFilesDir() : super.getFilesDir();
    }

    @Override
    public File getCacheDir() {
        return mBase != null ? mBase.getCacheDir() : super.getCacheDir();
    }

    @Override
    public File getDatabasePath(String name) {
        return mBase != null ? mBase.getDatabasePath(name) : super.getDatabasePath(name);
    }

    @Override
    public File getExternalCacheDir() {
        return mBase != null ? mBase.getExternalCacheDir() : super.getExternalCacheDir();
    }

    @Override
    public File getDataDir() {
        return mBase != null ? mBase.getDataDir() : super.getDataDir();
    }

    @Override
    public String getString(int resId) {
        return mBase != null ? mBase.getString(resId) : super.getString(resId);
    }

    @Override
    public String getString(int resId, Object... formatArgs) {
        return mBase != null ? mBase.getString(resId, formatArgs) : super.getString(resId, formatArgs);
    }

    @Override
    public void setTheme(int resid) {
        if (mBase != null) {
            mBase.setTheme(resid);
        } else {
            super.setTheme(resid);
        }
    }

    @Override
    public Resources.Theme getTheme() {
        return mBase != null ? mBase.getTheme() : super.getTheme();
    }

    @Override
    public Context createConfigurationContext(android.content.res.Configuration overrideConfiguration) {
        return mBase != null ? mBase.createConfigurationContext(overrideConfiguration) : super.createConfigurationContext(overrideConfiguration);
    }

    @Override
    public SharedPreferences getSharedPreferences(String name, int mode) {
        return mBase != null ? mBase.getSharedPreferences(name, mode) : super.getSharedPreferences(name, mode);
    }

    @Override
    public Context getApplicationContext() {
        return mBase != null ? mBase.getApplicationContext() : super.getApplicationContext();
    }

    @Override
    public PackageManager getPackageManager() {
        return mBase != null ? mBase.getPackageManager() : super.getPackageManager();
    }

    @Override
    public ApplicationInfo getApplicationInfo() {
        return mBase != null ? mBase.getApplicationInfo() : super.getApplicationInfo();
    }

    @Override
    public Object getSystemService(String name) {
        return mBase != null ? mBase.getSystemService(name) : super.getSystemService(name);
    }

    @Override
    public <T> T getSystemService(Class<T> serviceClass) {
        return mBase != null ? mBase.getSystemService(serviceClass) : super.getSystemService(serviceClass);
    }

    @Override
    public int checkSelfPermission(String permission) {
        return mBase != null ? mBase.checkSelfPermission(permission) : super.checkSelfPermission(permission);
    }

    @Override
    public int checkPermission(String permission, int pid, int uid) {
        return mBase != null ? mBase.checkPermission(permission, pid, uid) : super.checkPermission(permission, pid, uid);
    }

    @Override
    public int checkCallingPermission(String permission) {
        return mBase != null ? mBase.checkCallingPermission(permission) : super.checkCallingPermission(permission);
    }

    @Override
    public int checkCallingOrSelfPermission(String permission) {
        return mBase != null ? mBase.checkCallingOrSelfPermission(permission) : super.checkCallingOrSelfPermission(permission);
    }

    @Override
    public TypedArray obtainStyledAttributes(int[] attrs) {
        return mBase != null ? mBase.obtainStyledAttributes(attrs) : super.obtainStyledAttributes(attrs);
    }

    @Override
    public TypedArray obtainStyledAttributes(int resid, int[] attrs) {
        return mBase != null ? mBase.obtainStyledAttributes(resid, attrs) : super.obtainStyledAttributes(resid, attrs);
    }

    @Override
    public TypedArray obtainStyledAttributes(AttributeSet set, int[] attrs) {
        return mBase != null ? mBase.obtainStyledAttributes(set, attrs) : super.obtainStyledAttributes(set, attrs);
    }

    @Override
    public TypedArray obtainStyledAttributes(AttributeSet set, int[] attrs, int defStyleAttr, int defStyleRes) {
        return mBase != null ? mBase.obtainStyledAttributes(set, attrs, defStyleAttr, defStyleRes) : super.obtainStyledAttributes(set, attrs, defStyleAttr, defStyleRes);
    }

    @Override
    public Intent registerReceiver(BroadcastReceiver receiver, IntentFilter filter) {
        return mBase != null ? mBase.registerReceiver(receiver, filter) : super.registerReceiver(receiver, filter);
    }

    @Override
    public Intent registerReceiver(BroadcastReceiver receiver, IntentFilter filter, int flags) {
        return mBase != null ? mBase.registerReceiver(receiver, filter, flags) : super.registerReceiver(receiver, filter, flags);
    }

    @Override
    public void unregisterReceiver(BroadcastReceiver receiver) {
        if (mBase != null) {
            mBase.unregisterReceiver(receiver);
        } else {
            super.unregisterReceiver(receiver);
        }
    }

    @Override
    public void sendBroadcast(Intent intent) {
        if (mBase != null) {
            mBase.sendBroadcast(intent);
        } else {
            super.sendBroadcast(intent);
        }
    }

    @Override
    public void startActivity(Intent intent) {
        if (mBase != null) {
            mBase.startActivity(intent);
        } else {
            super.startActivity(intent);
        }
    }

    @Override
    public Drawable getDrawable(int id) {
        return mBase != null ? mBase.getDrawable(id) : super.getDrawable(id);
    }

    @Override
    public ContentResolver getContentResolver() {
        return mBase != null ? mBase.getContentResolver() : super.getContentResolver();
    }

    @Override
    public File getExternalFilesDir(String type) {
        return mBase != null ? mBase.getExternalFilesDir(type) : super.getExternalFilesDir(type);
    }
}
