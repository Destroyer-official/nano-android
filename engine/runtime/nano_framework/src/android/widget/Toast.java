package android.widget;

import android.content.Context;

public class Toast {
    public static final int LENGTH_SHORT = 0;
    public static final int LENGTH_LONG = 1;

    private static volatile String lastToastText = "";
    private static volatile long lastToastShownAtMs = 0L;
    private static volatile int lastToastDurationMs = 2000;

    private final CharSequence text;
    private final int duration;

    private Toast(CharSequence text, int duration) {
        this.text = text != null ? text : "";
        this.duration = duration;
    }

    public static Toast makeText(Context context, CharSequence text, int duration) {
        return new Toast(text, duration);
    }

    public static Toast makeText(Context context, int resId, int duration) {
        String s = context != null ? context.getString(resId) : ("res_" + resId);
        return new Toast(s, duration);
    }

    public void show() {
        lastToastText = text != null ? text.toString() : "";
        lastToastShownAtMs = System.currentTimeMillis();
        lastToastDurationMs = (duration == LENGTH_LONG) ? 3500 : 2000;
    }

    public void cancel() {
        lastToastText = "";
    }

    public void setGravity(int gravity, int xOffset, int yOffset) {}

    public static String getCurrentToastText() {
        if (lastToastText == null || lastToastText.isEmpty()) return "";
        if (System.currentTimeMillis() - lastToastShownAtMs > 5000L) {
            return "";
        }
        return lastToastText;
    }

    public static void clearToast() {
        lastToastText = "";
    }
}
