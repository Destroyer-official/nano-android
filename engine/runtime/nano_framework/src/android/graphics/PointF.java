package android.graphics;

public class PointF {
    public float x;
    public float y;

    public PointF() {}

    public PointF(float x, float y) {
        this.x = x;
        this.y = y;
    }

    public PointF(Point p) {
        if (p != null) {
            this.x = p.x;
            this.y = p.y;
        }
    }

    public PointF(PointF p) {
        if (p != null) {
            this.x = p.x;
            this.y = p.y;
        }
    }

    public void set(float x, float y) {
        this.x = x;
        this.y = y;
    }

    public void set(PointF p) {
        if (p != null) {
            this.x = p.x;
            this.y = p.y;
        }
    }

    public void negate() {
        x = -x;
        y = -y;
    }

    public void offset(float dx, float dy) {
        x += dx;
        y += dy;
    }

    public boolean equals(float x, float y) {
        return this.x == x && this.y == y;
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (o == null || getClass() != o.getClass()) return false;
        PointF pointF = (PointF) o;
        if (Float.compare(pointF.x, x) != 0) return false;
        return Float.compare(pointF.y, y) == 0;
    }

    @Override
    public int hashCode() {
        int result = (x != +0.0f ? Float.floatToIntBits(x) : 0);
        result = 31 * result + (y != +0.0f ? Float.floatToIntBits(y) : 0);
        return result;
    }

    @Override
    public String toString() {
        return "PointF(" + x + ", " + y + ")";
    }

    public float length() {
        return length(x, y);
    }

    public static float length(float x, float y) {
        return (float) Math.hypot(x, y);
    }
}
