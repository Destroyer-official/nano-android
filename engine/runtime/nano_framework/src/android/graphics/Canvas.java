package android.graphics;

public class Canvas {
    private int width = 1080;
    private int height = 1920;

    public Canvas() {}
    public Canvas(Bitmap bitmap) {
        if (bitmap != null) {
            this.width = bitmap.getWidth();
            this.height = bitmap.getHeight();
        }
    }

    public int getWidth() { return width; }
    public int getHeight() { return height; }

    public int save() { return 1; }
    public void restore() {}
    public void restoreToCount(int saveCount) {}

    public void translate(float dx, float dy) {}
    public void scale(float sx, float sy) {}
    public void scale(float sx, float sy, float px, float py) {}
    public void rotate(float degrees) {}
    public void rotate(float degrees, float px, float py) {}

    public boolean clipRect(Rect rect) { return true; }
    public boolean clipRect(RectF rect) { return true; }
    public boolean clipRect(int left, int top, int right, int bottom) { return true; }
    public boolean clipRect(float left, float top, float right, float bottom) { return true; }

    public void drawColor(int color) {}
    public void drawRect(Rect r, Paint p) {}
    public void drawRect(float left, float top, float right, float bottom, Paint paint) {}
    public void drawRoundRect(RectF rect, float rx, float ry, Paint paint) {}
    public void drawRoundRect(float left, float top, float right, float bottom, float rx, float ry, Paint paint) {}
    public void drawCircle(float cx, float cy, float radius, Paint paint) {}
    public void drawOval(RectF oval, Paint paint) {}
    public void drawOval(float left, float top, float right, float bottom, Paint paint) {}
    public void drawArc(RectF oval, float startAngle, float sweepAngle, boolean useCenter, Paint paint) {}
    public void drawArc(float left, float top, float right, float bottom, float startAngle, float sweepAngle, boolean useCenter, Paint paint) {}
    public void drawLine(float startX, float startY, float stopX, float stopY, Paint paint) {}
    public void drawLines(float[] pts, Paint paint) {}
    public void drawPoint(float x, float y, Paint paint) {}
    public void drawPath(Path path, Paint paint) {}
    public void drawText(String text, float x, float y, Paint paint) {}
    public void drawText(CharSequence text, int start, int end, float x, float y, Paint paint) {}
    public void drawBitmap(Bitmap bitmap, float left, float top, Paint paint) {}
    public void drawBitmap(Bitmap bitmap, Rect src, Rect dst, Paint paint) {}
    public void drawBitmap(Bitmap bitmap, Rect src, RectF dst, Paint paint) {}
    public void drawBitmap(Bitmap bitmap, Matrix matrix, Paint paint) {}
}
