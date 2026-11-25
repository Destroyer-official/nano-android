package android.content.res;

import java.util.Locale;

public class Configuration {
    public static final int ORIENTATION_UNDEFINED = 0;
    public static final int ORIENTATION_PORTRAIT = 1;
    public static final int ORIENTATION_LANDSCAPE = 2;

    public static final int UI_MODE_NIGHT_MASK = 48;
    public static final int UI_MODE_NIGHT_NO = 16;
    public static final int UI_MODE_NIGHT_YES = 32;
    public static final int UI_MODE_NIGHT_UNDEFINED = 0;

    public static final int SCREENLAYOUT_SIZE_MASK = 15;
    public static final int SCREENLAYOUT_SIZE_NORMAL = 2;

    public int orientation = ORIENTATION_PORTRAIT;
    public int uiMode = UI_MODE_NIGHT_NO;
    public int screenLayout = SCREENLAYOUT_SIZE_NORMAL;
    public int screenWidthDp = 360;
    public int screenHeightDp = 800;
    public int smallestScreenWidthDp = 360;
    public int densityDpi = 480;
    public float fontScale = 1.0f;
    public int mcc = 0;
    public int mnc = 0;
    public int touchscreen = 0;
    public int keyboard = 0;
    public int keyboardHidden = 0;
    public int hardKeyboardHidden = 0;
    public int navigation = 0;
    public int navigationHidden = 0;
    public int colorMode = 0;
    public Locale locale = Locale.US;
    private android.os.LocaleList locales = android.os.LocaleList.getDefault();

    public android.os.LocaleList getLocales() {
        return locales != null ? locales : android.os.LocaleList.getDefault();
    }

    public void setLocales(android.os.LocaleList locales) {
        this.locales = locales != null ? locales : android.os.LocaleList.getDefault();
    }

    public void setLocale(Locale loc) {
        this.locale = loc;
        this.locales = new android.os.LocaleList(loc);
    }

    public int getLayoutDirection() {
        return 0; // View.LAYOUT_DIRECTION_LTR
    }

    public void setLayoutDirection(Locale loc) {}

    public Configuration() {}

    public Configuration(Configuration init) {
        setTo(init);
    }

    public void setTo(Configuration o) {
        if (o == null) return;
        this.orientation = o.orientation;
        this.uiMode = o.uiMode;
        this.screenLayout = o.screenLayout;
        this.screenWidthDp = o.screenWidthDp;
        this.screenHeightDp = o.screenHeightDp;
        this.smallestScreenWidthDp = o.smallestScreenWidthDp;
        this.densityDpi = o.densityDpi;
        this.fontScale = o.fontScale;
        this.mcc = o.mcc;
        this.mnc = o.mnc;
        this.touchscreen = o.touchscreen;
        this.keyboard = o.keyboard;
        this.keyboardHidden = o.keyboardHidden;
        this.hardKeyboardHidden = o.hardKeyboardHidden;
        this.navigation = o.navigation;
        this.navigationHidden = o.navigationHidden;
        this.colorMode = o.colorMode;
        this.locale = o.locale;
        this.locales = o.locales;
    }

    public void setToDefaults() {
        this.orientation = ORIENTATION_UNDEFINED;
        this.uiMode = UI_MODE_NIGHT_UNDEFINED;
        this.screenLayout = SCREENLAYOUT_SIZE_NORMAL;
        this.fontScale = 1.0f;
    }

    public boolean equals(Configuration that) {
        if (that == null) return false;
        if (that == this) return true;
        return this.orientation == that.orientation
                && this.uiMode == that.uiMode
                && this.screenLayout == that.screenLayout
                && this.screenWidthDp == that.screenWidthDp
                && this.screenHeightDp == that.screenHeightDp
                && this.smallestScreenWidthDp == that.smallestScreenWidthDp
                && this.densityDpi == that.densityDpi
                && Float.compare(this.fontScale, that.fontScale) == 0
                && this.mcc == that.mcc
                && this.mnc == that.mnc
                && this.colorMode == that.colorMode
                && java.util.Objects.equals(this.locale, that.locale);
    }

    @Override
    public boolean equals(Object that) {
        return (that instanceof Configuration) && equals((Configuration) that);
    }

    @Override
    public int hashCode() {
        return java.util.Objects.hash(orientation, uiMode, screenLayout, screenWidthDp, screenHeightDp, densityDpi, fontScale, locale);
    }
}
