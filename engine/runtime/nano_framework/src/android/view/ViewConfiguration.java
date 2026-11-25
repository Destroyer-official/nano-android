package android.view;

import android.content.Context;

public class ViewConfiguration {
    private static final ViewConfiguration sInstance = new ViewConfiguration();

    public ViewConfiguration() {}

    public static ViewConfiguration get(Context context) {
        return sInstance;
    }

    public static int getScrollBarSize() { return 10; }
    public int getScaledScrollBarSize() { return 10; }
    public static int getScrollBarFadeDuration() { return 250; }
    public static int getScrollDefaultDelay() { return 300; }
    public static int getFadingEdgeLength() { return 12; }
    public int getScaledFadingEdgeLength() { return 12; }
    public static int getPressedStateDuration() { return 64; }
    public static int getLongPressTimeout() { return 400; }
    public static int getMultiPressTimeout() { return 300; }
    public static int getKeyRepeatTimeout() { return 50; }
    public static int getKeyRepeatDelay() { return 50; }
    public static int getTapTimeout() { return 100; }
    public static int getJumpTapTimeout() { return 500; }
    public static int getDoubleTapTimeout() { return 300; }
    public static int getEdgeSlop() { return 12; }
    public int getScaledEdgeSlop() { return 12; }
    public static int getTouchSlop() { return 8; }
    public int getScaledTouchSlop() { return 8; }
    public int getScaledHoverSlop() { return 4; }
    public int getScaledPagingTouchSlop() { return 16; }
    public int getScaledDoubleTapSlop() { return 100; }
    public static int getWindowTouchSlop() { return 16; }
    public int getScaledWindowTouchSlop() { return 16; }
    public static int getMinimumFlingVelocity() { return 50; }
    public int getScaledMinimumFlingVelocity() { return 50; }
    public static int getMaximumFlingVelocity() { return 8000; }
    public int getScaledMaximumFlingVelocity() { return 8000; }
    public float getScaledHorizontalScrollFactor() { return 64.0f; }
    public float getScaledVerticalScrollFactor() { return 64.0f; }
    public static int getMaximumDrawingCacheSize() { return 1920 * 1080 * 4; }
    public int getScaledMaximumDrawingCacheSize() { return 1920 * 1080 * 4; }
    public int getScaledOverscrollDistance() { return 0; }
    public int getScaledOverflingDistance() { return 6; }
    public static long getZoomControlsTimeout() { return 3000; }
    public static long getGlobalActionKeyTimeout() { return 500; }
    public static float getScrollFriction() { return 0.015f; }
    public static long getDefaultActionModeHideDuration() { return 2000; }
    public static float getAmbiguousGestureMultiplier() { return 2.0f; }
    public float getScaledAmbiguousGestureMultiplier() { return 2.0f; }
    public boolean hasPermanentMenuKey() { return false; }
    public boolean shouldShowMenuShortcutsWhenKeyboardPresent() { return false; }
    public int getScaledMinimumScalingSpan() { return 200; }
}
