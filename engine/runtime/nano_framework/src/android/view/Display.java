package android.view;

import android.util.DisplayMetrics;

public class Display {
    public void getMetrics(DisplayMetrics outMetrics) {
        if (outMetrics != null) {
            outMetrics.widthPixels = 1080;
            outMetrics.heightPixels = 2400;
            outMetrics.density = 2.625f;
            outMetrics.densityDpi = 420;
        }
    }

    public int getRotation() {
        return Surface.ROTATION_0;
    }
}
