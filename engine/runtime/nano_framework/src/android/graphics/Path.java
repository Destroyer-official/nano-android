package android.graphics;

public class Path {
    public enum Direction { CW, CCW }

    private float minX = Float.POSITIVE_INFINITY, minY = Float.POSITIVE_INFINITY;
    private float maxX = Float.NEGATIVE_INFINITY, maxY = Float.NEGATIVE_INFINITY;

    public Path() {}
    public Path(Path src) {
        if (src != null) {
            minX = src.minX; minY = src.minY; maxX = src.maxX; maxY = src.maxY;
        }
    }

    private void record(float x, float y) {
        if (x < minX) minX = x;
        if (x > maxX) maxX = x;
        if (y < minY) minY = y;
        if (y > maxY) maxY = y;
    }

    public void reset() {
        minX = Float.POSITIVE_INFINITY; minY = Float.POSITIVE_INFINITY;
        maxX = Float.NEGATIVE_INFINITY; maxY = Float.NEGATIVE_INFINITY;
    }

    public void rewind() { reset(); }
    public boolean isEmpty() { return minX > maxX || minY > maxY; }
    public void moveTo(float x, float y) { record(x, y); }
    public void lineTo(float x, float y) { record(x, y); }
    public void quadTo(float x1, float y1, float x2, float y2) { record(x1, y1); record(x2, y2); }
    public void cubicTo(float x1, float y1, float x2, float y2, float x3, float y3) { record(x1, y1); record(x2, y2); record(x3, y3); }
    public void close() {}

    public void addRect(float left, float top, float right, float bottom, Direction dir) {
        record(left, top);
        record(right, bottom);
    }

    public void addRect(RectF rect, Direction dir) {
        if (rect != null) addRect(rect.left, rect.top, rect.right, rect.bottom, dir);
    }

    public void addRoundRect(RectF rect, float rx, float ry, Direction dir) {
        addRect(rect, dir);
    }

    public void addRoundRect(RectF rect, float[] radii, Direction dir) {
        addRect(rect, dir);
    }

    public void addCircle(float x, float y, float radius, Direction dir) {
        record(x - radius, y - radius);
        record(x + radius, y + radius);
    }

    public void computeBounds(RectF bounds, boolean exact) {
        if (bounds == null) return;
        if (isEmpty()) {
            bounds.left = 0f; bounds.top = 0f; bounds.right = 0f; bounds.bottom = 0f;
        } else {
            bounds.left = minX; bounds.top = minY; bounds.right = maxX; bounds.bottom = maxY;
        }
    }
}
