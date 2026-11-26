package android.graphics.drawable.shapes;

import android.graphics.Canvas;
import android.graphics.Paint;

public class OvalShape extends RectShape {
    public OvalShape() {}

    @Override
    public void draw(Canvas canvas, Paint paint) {
        if (canvas != null) {
            canvas.drawOval(rect(), paint);
        }
    }
}
