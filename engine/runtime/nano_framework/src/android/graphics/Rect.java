package android.graphics;

public class Rect {
    public int left;
    public int top;
    public int right;
    public int bottom;

    public Rect() {}

    public Rect(int left, int top, int right, int bottom) {
        this.left = left;
        this.top = top;
        this.right = right;
        this.bottom = bottom;
    }

    public Rect(Rect r) {
        if (r != null) set(r.left, r.top, r.right, r.bottom);
    }

    public int width() { return right - left; }
    public int height() { return bottom - top; }

    public void set(int left, int top, int right, int bottom) {
        this.left = left;
        this.top = top;
        this.right = right;
        this.bottom = bottom;
    }

    public void set(Rect src) {
        if (src != null) set(src.left, src.top, src.right, src.bottom);
    }

    public boolean isEmpty() { return left >= right || top >= bottom; }
    public void setEmpty() { left = right = top = bottom = 0; }
    public void offset(int dx, int dy) { left += dx; right += dx; top += dy; bottom += dy; }

    public boolean contains(int x, int y) {
        return left < right && top < bottom && x >= left && x < right && y >= top && y < bottom;
    }

    public boolean contains(int left, int top, int right, int bottom) {
        return this.left < this.right && this.top < this.bottom
                && this.left <= left && this.top <= top && this.right >= right && this.bottom >= bottom;
    }

    public boolean contains(Rect r) {
        return r != null && contains(r.left, r.top, r.right, r.bottom);
    }

    public boolean intersect(Rect r) {
        if (r == null) return false;
        if (this.left < r.right && r.left < this.right && this.top < r.bottom && r.top < this.bottom) {
            if (this.left < r.left) this.left = r.left;
            if (this.top < r.top) this.top = r.top;
            if (this.right > r.right) this.right = r.right;
            if (this.bottom > r.bottom) this.bottom = r.bottom;
            return true;
        }
        return false;
    }

    public boolean intersects(int left, int top, int right, int bottom) {
        return this.left < right && left < this.right && this.top < bottom && top < this.bottom;
    }

    public static boolean intersects(Rect a, Rect b) {
        return a != null && b != null && a.intersects(b.left, b.top, b.right, b.bottom);
    }

    public void union(Rect r) {
        if (r == null || r.isEmpty()) return;
        if (isEmpty()) {
            set(r);
            return;
        }
        if (r.left < left) left = r.left;
        if (r.top < top) top = r.top;
        if (r.right > right) right = r.right;
        if (r.bottom > bottom) bottom = r.bottom;
    }
}
