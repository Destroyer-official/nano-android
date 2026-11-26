package android.graphics.drawable.shapes;

import android.graphics.Canvas;
import android.graphics.Paint;
import android.graphics.RectF;

public class RectShape extends Shape {
    private final RectF rect = new RectF();

    public RectShape() {}

    protected final RectF rect() {
        return rect;
    }

    @Override
    protected void onResize(float width, float height) {
        rect.set(0, 0, width, height);
    }

    @Override
    public void draw(Canvas canvas, Paint paint) {
        if (canvas != null) {
            canvas.drawRect(rect.left, rect.top, rect.right, rect.bottom, paint);
        }
    }
}
