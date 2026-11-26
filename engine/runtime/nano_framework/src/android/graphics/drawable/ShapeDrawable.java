package android.graphics.drawable;

import android.graphics.Canvas;
import android.graphics.Paint;
import android.graphics.Rect;
import android.graphics.drawable.shapes.Shape;

public class ShapeDrawable extends Drawable {
    private final Paint paint = new Paint();
    private Shape shape;
    private int intrinsicWidth = -1;
    private int intrinsicHeight = -1;
    private Rect padding;

    public ShapeDrawable() {}

    public ShapeDrawable(Shape s) {
        this.shape = s;
    }

    public Shape getShape() {
        return shape;
    }

    public void setShape(Shape s) {
        this.shape = s;
        updateShape();
    }

    public Paint getPaint() {
        return paint;
    }

    public void setIntrinsicWidth(int width) {
        this.intrinsicWidth = width;
    }

    public void setIntrinsicHeight(int height) {
        this.intrinsicHeight = height;
    }

    @Override
    public int getIntrinsicWidth() {
        return intrinsicWidth;
    }

    @Override
    public int getIntrinsicHeight() {
        return intrinsicHeight;
    }

    public void setPadding(int left, int top, int right, int bottom) {
        if ((left | top | right | bottom) == 0) {
            padding = null;
        } else {
            if (padding == null) padding = new Rect();
            padding.set(left, top, right, bottom);
        }
    }

    public void setPadding(Rect padding) {
        if (padding == null) {
            this.padding = null;
        } else {
            if (this.padding == null) this.padding = new Rect();
            this.padding.set(padding);
        }
    }

    @Override
    public boolean getPadding(Rect padding) {
        if (this.padding != null && padding != null) {
            padding.set(this.padding);
            return true;
        }
        return super.getPadding(padding);
    }

    @Override
    protected void onBoundsChange(Rect bounds) {
        super.onBoundsChange(bounds);
        updateShape();
    }

    private void updateShape() {
        if (shape != null) {
            Rect r = getBounds();
            shape.resize(r.width(), r.height());
        }
    }

    @Override
    public void setAlpha(int alpha) {
        paint.setAlpha(alpha);
    }

    @Override
    public void draw(Canvas canvas) {
        if (shape != null && canvas != null) {
            Rect r = getBounds();
            canvas.save();
            canvas.translate(r.left, r.top);
            shape.draw(canvas, paint);
            canvas.restore();
        }
    }
}
