package android.graphics.drawable.shapes;

import android.graphics.Canvas;
import android.graphics.Paint;

public abstract class Shape implements Cloneable {
    private float width;
    private float height;

    public final float getWidth() { return width; }
    public final float getHeight() { return height; }

    public abstract void draw(Canvas canvas, Paint paint);

    public final void resize(float width, float height) {
        if (width < 0) width = 0;
        if (height < 0) height = 0;
        if (this.width != width || this.height != height) {
            this.width = width;
            this.height = height;
            onResize(width, height);
        }
    }

    protected void onResize(float width, float height) {}

    public boolean hasAlpha() { return true; }

    @Override
    public Shape clone() throws CloneNotSupportedException {
        return (Shape) super.clone();
    }
}
