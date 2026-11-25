package android.graphics.drawable;

import android.graphics.Canvas;
import android.graphics.Rect;
import android.graphics.PorterDuff;
import android.content.res.ColorStateList;

public abstract class Drawable {
    private Rect bounds = new Rect();
    private int level = 0;

    public int getLevel() { return level; }
    public boolean setLevel(int level) {
        if (this.level != level) {
            this.level = level;
            return true;
        }
        return false;
    }

    public void setBounds(int left, int top, int right, int bottom) {
        this.bounds = new Rect(left, top, right, bottom);
    }

    public void setBounds(Rect bounds) {
        this.bounds = bounds;
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
