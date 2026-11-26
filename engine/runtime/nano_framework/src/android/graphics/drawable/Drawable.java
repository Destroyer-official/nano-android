package android.graphics.drawable;

import android.graphics.Canvas;
import android.graphics.Rect;
import android.graphics.PorterDuff;
import android.content.res.ColorStateList;

public abstract class Drawable {
    private Rect bounds = new Rect();
    private int level = 0;
    private int[] stateSet = new int[0];
    private Callback callback;

    public interface Callback {
        void invalidateDrawable(Drawable who);
        void scheduleDrawable(Drawable who, Runnable what, long when);
        void unscheduleDrawable(Drawable who, Runnable what);
    }

    public void setCallback(Callback cb) {
        this.callback = cb;
    }

    public Callback getCallback() {
        return callback;
    }

    public int[] getState() {
        return stateSet;
    }

    public boolean setState(int[] stateSet) {
        this.stateSet = stateSet != null ? stateSet : new int[0];
        return onStateChange(this.stateSet);
    }

    public boolean isStateful() {
        return false;
    }

    protected boolean onStateChange(int[] state) {
        return false;
    }

    protected boolean onLevelChange(int level) {
        return false;
    }

    protected void onBoundsChange(Rect bounds) {}

    public void jumpToCurrentState() {}

    public int getMinimumWidth() {
        int w = getIntrinsicWidth();
        return Math.max(w, 0);
    }

    public int getMinimumHeight() {
        int h = getIntrinsicHeight();
        return Math.max(h, 0);
    }

    public boolean getPadding(Rect padding) {
        if (padding != null) {
            padding.set(0, 0, 0, 0);
        }
        return false;
    }

    public Rect copyBounds() {
        return new Rect(bounds);
    }

    public void copyBounds(Rect dst) {
        if (dst != null) {
            dst.set(bounds);
        }
    }

    public int getLevel() { return level; }
    public boolean setLevel(int level) {
        if (this.level != level) {
            this.level = level;
            onLevelChange(level);
            return true;
        }
        return false;
    }

    public void setBounds(int left, int top, int right, int bottom) {
        this.bounds = new Rect(left, top, right, bottom);
        onBoundsChange(this.bounds);
    }

    public void setBounds(Rect bounds) {
        this.bounds = bounds != null ? new Rect(bounds) : new Rect();
        onBoundsChange(this.bounds);
    }

    public Rect getBounds() {
        return bounds;
    }

    public void setAlpha(int alpha) {}
    public void setColorFilter(Object colorFilter) {}
    public void setColorFilter(int color, PorterDuff.Mode mode) {}
    public void clearColorFilter() {}
    public android.graphics.ColorFilter getColorFilter() { return null; }
    public void draw(Canvas canvas) {}

    public void invalidateSelf() {}
    public Drawable mutate() { return this; }
    public void setTint(int tintColor) {}
    public void setTintList(ColorStateList tint) {}
    public void setTintMode(PorterDuff.Mode tintMode) {}
    public int getIntrinsicWidth() { return -1; }
    public int getIntrinsicHeight() { return -1; }
    public boolean setVisible(boolean visible, boolean restart) { return false; }
    public boolean isVisible() { return true; }
    public int getOpacity() { return 0; }
    public ConstantState getConstantState() { return null; }
    public static abstract class ConstantState {
        public abstract Drawable newDrawable();
    }
}
