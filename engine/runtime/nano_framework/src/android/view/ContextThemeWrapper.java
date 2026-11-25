package android.view;

import android.content.Context;
import android.content.ContextWrapper;
import android.content.res.Configuration;
import android.content.res.Resources;

/**
 * Lightweight ContextThemeWrapper matching Android SDK hierarchy so AppCompat,
 * MaterialComponents dialogs/menus, and Activity themes execute without hitting android.jar stubs.
 */
public class ContextThemeWrapper extends ContextWrapper {
    private int mThemeResource;
    private Resources.Theme mTheme;

    public ContextThemeWrapper() {
        super(null);
    }

    public ContextThemeWrapper(Context base) {
        super(base);
    }

    public ContextThemeWrapper(Context base, int themeResId) {
        super(base);
        this.mThemeResource = themeResId;
    }

    public ContextThemeWrapper(Context base, Resources.Theme theme) {
        super(base);
        this.mTheme = theme;
    }

    public void applyOverrideConfiguration(Configuration overrideConfiguration) {}

    public int getThemeResId() {
        return mThemeResource;
    }

    @Override
    public void setTheme(int resid) {
        this.mThemeResource = resid;
        if (mTheme != null) {
            mTheme.applyStyle(resid, true);
        }
    }

    @Override
    public Resources.Theme getTheme() {
        if (mTheme == null) {
            mTheme = getResources().newTheme();
            if (mThemeResource != 0) {
                mTheme.applyStyle(mThemeResource, true);
            }
        }
        return mTheme;
    }
}
