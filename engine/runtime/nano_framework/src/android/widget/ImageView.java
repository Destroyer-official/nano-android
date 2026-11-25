package android.widget;

import android.content.Context;
import android.graphics.drawable.Drawable;
import android.view.View;

public class ImageView extends View {
    public enum ScaleType {
        MATRIX, FIT_XY, FIT_START, FIT_CENTER, FIT_END, CENTER, CENTER_CROP, CENTER_INSIDE
    }

    private Drawable drawable;
    private ScaleType scaleType = ScaleType.FIT_CENTER;

    public ImageView() { super(); }
    public ImageView(Context context) { super(context); }
    public ImageView(Context context, android.util.AttributeSet attrs) { super(context, attrs); }
    public ImageView(Context context, android.util.AttributeSet attrs, int defStyleAttr) { super(context, attrs, defStyleAttr); }
    public ImageView(Context context, android.util.AttributeSet attrs, int defStyleAttr, int defStyleRes) { super(context, attrs, defStyleAttr, defStyleRes); }

    public void setImageDrawable(Drawable d) { this.drawable = d; }
    public Drawable getDrawable() {
        return drawable;
    }
    public void setImageResource(int resId) {
        Drawable d = getResources().getDrawable(resId);
        if (d != null) this.drawable = d;
    }

    @Override
    protected void onMeasure(int widthMeasureSpec, int heightMeasureSpec) {
        int intrinsicW = 72 + getPaddingLeft() + getPaddingRight();
        int intrinsicH = 72 + getPaddingTop() + getPaddingBottom();
        if (drawable instanceof android.graphics.drawable.BitmapDrawable) {
            android.graphics.Bitmap bmp = ((android.graphics.drawable.BitmapDrawable) drawable).getBitmap();
            if (bmp != null) {
                intrinsicW = bmp.getWidth() + getPaddingLeft() + getPaddingRight();
                intrinsicH = bmp.getHeight() + getPaddingTop() + getPaddingBottom();
            }
        }
        setMeasuredDimension(
            MeasureSpec.resolveSize(intrinsicW, widthMeasureSpec),
            MeasureSpec.resolveSize(intrinsicH, heightMeasureSpec)
        );
    }

    public void setScaleType(ScaleType scaleType) { this.scaleType = scaleType; }
    public ScaleType getScaleType() { return scaleType; }

    public void setColorFilter(int color) {}
    public void clearColorFilter() {}

    private android.content.res.ColorStateList imageTintList;
    private android.graphics.PorterDuff.Mode imageTintMode;

    public void setImageMatrix(android.graphics.Matrix matrix) {}
    public void setColorFilter(int color, android.graphics.PorterDuff.Mode mode) {}
    public void setColorFilter(android.graphics.ColorFilter cf) {}
    public android.graphics.Matrix getImageMatrix() { return new android.graphics.Matrix(); }
    public void setImageBitmap(android.graphics.Bitmap bm) {}
    public void setAdjustViewBounds(boolean adjustViewBounds) {}
    public boolean getAdjustViewBounds() { return false; }
    public void setCropToPadding(boolean cropToPadding) {}
    public boolean getCropToPadding() { return false; }
    public void setImageLevel(int level) {}
    public int getImageLevel() { return 0; }

    public void setImageTintList(android.content.res.ColorStateList tint) { this.imageTintList = tint; }
    public android.content.res.ColorStateList getImageTintList() { return imageTintList; }
    public void setImageTintMode(android.graphics.PorterDuff.Mode tintMode) { this.imageTintMode = tintMode; }
    public android.graphics.PorterDuff.Mode getImageTintMode() { return imageTintMode; }
}
