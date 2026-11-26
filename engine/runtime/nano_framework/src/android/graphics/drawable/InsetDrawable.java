package android.graphics.drawable;

import android.graphics.Canvas;
import android.graphics.Rect;

public class InsetDrawable extends Drawable {
    private Drawable drawable;
    private int insetLeft;
    private int insetTop;
    private int insetRight;
    private int insetBottom;

    public InsetDrawable(Drawable drawable, int inset) {
        this(drawable, inset, inset, inset, inset);
    }

    public InsetDrawable(Drawable drawable, float insetFraction) {
        this.drawable = drawable;
    }

    public InsetDrawable(Drawable drawable, int insetLeft, int insetTop, int insetRight, int insetBottom) {
        this.drawable = drawable;
        this.insetLeft = insetLeft;
        this.insetTop = insetTop;
        this.insetRight = insetRight;
        this.insetBottom = insetBottom;
    }

    public InsetDrawable(Drawable drawable, float insetLeftFraction, float insetTopFraction,
                         float insetRightFraction, float insetBottomFraction) {
        this.drawable = drawable;
    }

    public Drawable getDrawable() {
        return drawable;
    }

    public void setDrawable(Drawable drawable) {
        this.drawable = drawable;
    }

    @Override
    protected void onBoundsChange(Rect bounds) {
        if (drawable != null && bounds != null) {
            drawable.setBounds(
                bounds.left + insetLeft,
                bounds.top + insetTop,
                bounds.right - insetRight,
                bounds.bottom - insetBottom
            );
        }
    }

    @Override
    public boolean getPadding(Rect padding) {
        boolean pad = drawable != null && drawable.getPadding(padding);
        if (padding != null) {
            padding.left += insetLeft;
            padding.top += insetTop;
            padding.right += insetRight;
            padding.bottom += insetBottom;
        }
        return pad || (insetLeft | insetTop | insetRight | insetBottom) != 0;
    }

    @Override
    public int getIntrinsicWidth() {
        int w = drawable != null ? drawable.getIntrinsicWidth() : -1;
        return w < 0 ? -1 : w + insetLeft + insetRight;
    }

    @Override
    public int getIntrinsicHeight() {
        int h = drawable != null ? drawable.getIntrinsicHeight() : -1;
        return h < 0 ? -1 : h + insetTop + insetBottom;
    }

    @Override
    public void draw(Canvas canvas) {
        if (drawable != null) {
            drawable.draw(canvas);
        }
    }
}
