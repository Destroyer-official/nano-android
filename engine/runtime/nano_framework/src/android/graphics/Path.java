package android.graphics;

public class Path {
    public enum Direction { CW, CCW }
    public enum Op { DIFFERENCE, INTERSECT, UNION, XOR, REVERSE_DIFFERENCE }
    public enum FillType { WINDING, EVEN_ODD, INVERSE_WINDING, INVERSE_EVEN_ODD }

    private float minX = Float.POSITIVE_INFINITY, minY = Float.POSITIVE_INFINITY;
    private float maxX = Float.NEGATIVE_INFINITY, maxY = Float.NEGATIVE_INFINITY;
    private float lastX = 0f, lastY = 0f;
    private FillType fillType = FillType.WINDING;

    public Path() {}
    public Path(Path src) {
        set(src);
    }

    private void record(float x, float y) {
        if (x < minX) minX = x;
        if (x > maxX) maxX = x;
        if (y < minY) minY = y;
        if (y > maxY) maxY = y;
    }

    public void set(Path src) {
        if (src == null) {
            reset();
            return;
        }
        if (this == src) return;
        minX = src.minX;
        minY = src.minY;
        maxX = src.maxX;
        maxY = src.maxY;
        lastX = src.lastX;
        lastY = src.lastY;
        fillType = src.fillType;
    }

    public void setFillType(FillType ft) {
        if (ft != null) {
            this.fillType = ft;
        }
    }

    public FillType getFillType() {
        return fillType;
    }

    public void reset() {
        minX = Float.POSITIVE_INFINITY; minY = Float.POSITIVE_INFINITY;
        maxX = Float.NEGATIVE_INFINITY; maxY = Float.NEGATIVE_INFINITY;
        lastX = 0f;
        lastY = 0f;
        fillType = FillType.WINDING;
    }

    public void rewind() {
        minX = Float.POSITIVE_INFINITY; minY = Float.POSITIVE_INFINITY;
        maxX = Float.NEGATIVE_INFINITY; maxY = Float.NEGATIVE_INFINITY;
        lastX = 0f;
        lastY = 0f;
    }

    public boolean isEmpty() { return minX > maxX || minY > maxY; }
    public void moveTo(float x, float y) { record(x, y); lastX = x; lastY = y; }
    public void rMoveTo(float dx, float dy) { moveTo(lastX + dx, lastY + dy); }
    public void lineTo(float x, float y) { record(x, y); lastX = x; lastY = y; }
    public void rLineTo(float dx, float dy) { lineTo(lastX + dx, lastY + dy); }
    public void quadTo(float x1, float y1, float x2, float y2) { record(x1, y1); record(x2, y2); lastX = x2; lastY = y2; }
    public void cubicTo(float x1, float y1, float x2, float y2, float x3, float y3) { record(x1, y1); record(x2, y2); record(x3, y3); lastX = x3; lastY = y3; }
    public void close() {}

    public void addRect(float left, float top, float right, float bottom, Direction dir) {
        record(left, top);
        record(right, bottom);
        lastX = left;
        lastY = top;
    }

    public void addRect(RectF rect, Direction dir) {
        if (rect != null) addRect(rect.left, rect.top, rect.right, rect.bottom, dir);
    }

    public void addOval(RectF oval, Direction dir) {
        if (oval != null) addRect(oval, dir);
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
        lastX = x + radius;
        lastY = y;
    }

    public void arcTo(RectF oval, float startAngle, float sweepAngle) {
        arcTo(oval, startAngle, sweepAngle, false);
    }

    public void arcTo(RectF oval, float startAngle, float sweepAngle, boolean forceMoveTo) {
        if (oval == null) return;
        float cx = oval.centerX();
        float cy = oval.centerY();
        float rx = oval.width() * 0.5f;
        float ry = oval.height() * 0.5f;

        double startRad = Math.toRadians(startAngle);
        double endAngle = startAngle + sweepAngle;
        double endRad = Math.toRadians(endAngle);

        record(cx + rx * (float) Math.cos(startRad), cy + ry * (float) Math.sin(startRad));
        float endX = cx + rx * (float) Math.cos(endRad);
        float endY = cy + ry * (float) Math.sin(endRad);
        record(endX, endY);

        double low = Math.min(startAngle, endAngle);
        double high = Math.max(startAngle, endAngle);
        int firstQuad = (int) Math.ceil(low / 90.0);
        int lastQuad = (int) Math.floor(high / 90.0);
        int limit = Math.min(lastQuad, firstQuad + 4);
        for (int q = firstQuad; q <= limit; q++) {
            double rad = Math.toRadians(q * 90.0);
            record(cx + rx * (float) Math.cos(rad), cy + ry * (float) Math.sin(rad));
        }

        lastX = endX;
        lastY = endY;
    }

    public void addArc(RectF oval, float startAngle, float sweepAngle) {
        arcTo(oval, startAngle, sweepAngle, true);
    }

    public void offset(float dx, float dy) {
        if (!isEmpty()) {
            minX += dx;
            maxX += dx;
            minY += dy;
            maxY += dy;
            lastX += dx;
            lastY += dy;
        }
    }

    public void offset(float dx, float dy, Path dst) {
        if (dst == null) {
            offset(dx, dy);
        } else {
            dst.set(this);
            dst.offset(dx, dy);
        }
    }

    public void transform(Matrix matrix) {
        if (matrix == null || isEmpty()) return;
        RectF bounds = new RectF();
        computeBounds(bounds, true);
        matrix.mapRect(bounds);
        minX = bounds.left;
        minY = bounds.top;
        maxX = bounds.right;
        maxY = bounds.bottom;
        float[] pt = new float[] { lastX, lastY };
        matrix.mapPoints(pt);
        lastX = pt[0];
        lastY = pt[1];
    }

    public void transform(Matrix matrix, Path dst) {
        if (dst == null) {
            transform(matrix);
        } else {
            dst.set(this);
            dst.transform(matrix);
        }
    }

    public boolean op(Path path, Op op) {
        return op(this, path, op);
    }

    public boolean op(Path path1, Path path2, Op op) {
        if (op == null) return false;
        boolean e1 = (path1 == null || path1.isEmpty());
        boolean e2 = (path2 == null || path2.isEmpty());

        float rMinX = Float.POSITIVE_INFINITY;
        float rMinY = Float.POSITIVE_INFINITY;
        float rMaxX = Float.NEGATIVE_INFINITY;
        float rMaxY = Float.NEGATIVE_INFINITY;

        switch (op) {
            case DIFFERENCE:
                if (!e1) {
                    boolean covered = !e2
                            && path2.minX <= path1.minX && path2.minY <= path1.minY
                            && path2.maxX >= path1.maxX && path2.maxY >= path1.maxY;
                    if (!covered) {
                        rMinX = path1.minX; rMinY = path1.minY;
                        rMaxX = path1.maxX; rMaxY = path1.maxY;
                    }
                }
                break;
            case REVERSE_DIFFERENCE:
                if (!e2) {
                    boolean covered = !e1
                            && path1.minX <= path2.minX && path1.minY <= path2.minY
                            && path1.maxX >= path2.maxX && path1.maxY >= path2.maxY;
                    if (!covered) {
                        rMinX = path2.minX; rMinY = path2.minY;
                        rMaxX = path2.maxX; rMaxY = path2.maxY;
                    }
                }
                break;
            case INTERSECT:
                if (!e1 && !e2) {
                    float ix1 = Math.max(path1.minX, path2.minX);
                    float iy1 = Math.max(path1.minY, path2.minY);
                    float ix2 = Math.min(path1.maxX, path2.maxX);
                    float iy2 = Math.min(path1.maxY, path2.maxY);
                    boolean degenerateTouch =
                            (ix1 == ix2 && path1.minX < path1.maxX && path2.minX < path2.maxX)
                            || (iy1 == iy2 && path1.minY < path1.maxY && path2.minY < path2.maxY);
                    if (ix1 <= ix2 && iy1 <= iy2 && !degenerateTouch) {
                        rMinX = ix1; rMinY = iy1;
                        rMaxX = ix2; rMaxY = iy2;
                    }
                }
                break;
            case UNION:
                if (!e1 && !e2) {
                    rMinX = Math.min(path1.minX, path2.minX);
                    rMinY = Math.min(path1.minY, path2.minY);
                    rMaxX = Math.max(path1.maxX, path2.maxX);
                    rMaxY = Math.max(path1.maxY, path2.maxY);
                } else if (!e1) {
                    rMinX = path1.minX; rMinY = path1.minY;
                    rMaxX = path1.maxX; rMaxY = path1.maxY;
                } else if (!e2) {
                    rMinX = path2.minX; rMinY = path2.minY;
                    rMaxX = path2.maxX; rMaxY = path2.maxY;
                }
                break;
            case XOR:
                if (!e1 && !e2) {
                    boolean identical = path1.minX == path2.minX && path1.minY == path2.minY
                            && path1.maxX == path2.maxX && path1.maxY == path2.maxY;
                    if (!identical) {
                        rMinX = Math.min(path1.minX, path2.minX);
                        rMinY = Math.min(path1.minY, path2.minY);
                        rMaxX = Math.max(path1.maxX, path2.maxX);
                        rMaxY = Math.max(path1.maxY, path2.maxY);
                    }
                } else if (!e1) {
                    rMinX = path1.minX; rMinY = path1.minY;
                    rMaxX = path1.maxX; rMaxY = path1.maxY;
                } else if (!e2) {
                    rMinX = path2.minX; rMinY = path2.minY;
                    rMaxX = path2.maxX; rMaxY = path2.maxY;
                }
                break;
        }

        minX = rMinX;
        minY = rMinY;
        maxX = rMaxX;
        maxY = rMaxY;
        return !isEmpty();
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
