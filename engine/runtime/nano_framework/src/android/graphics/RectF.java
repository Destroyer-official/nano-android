package android.graphics;

public class RectF {
    public float left, top, right, bottom;

    public RectF() {}

    public RectF(float l, float t, float r, float b) {
        left = l;
        top = t;
        right = r;
        bottom = b;
    }

    public RectF(Rect r) {
        if (r != null) {
            left = r.left;
            top = r.top;
            right = r.right;
            bottom = r.bottom;
        }
    }

    public RectF(RectF r) {
        if (r != null) {
            left = r.left;
            top = r.top;
            right = r.right;
            bottom = r.bottom;
        }
    }

    public float width() { return right - left; }
    public float height() { return bottom - top; }

    public void set(float l, float t, float r, float b) {
        left = l;
        top = t;
        right = r;
        bottom = b;
    }

    public void set(RectF src) {
        if (src != null) {
            set(src.left, src.top, src.right, src.bottom);
        }
    }

    public void set(Rect src) {
        if (src != null) {
            set(src.left, src.top, src.right, src.bottom);
        }
    }

    public void setEmpty() {
        left = right = top = bottom = 0f;
    }

    public boolean isEmpty() {
        return left >= right || top >= bottom;
    }

    public float centerX() {
        return (left + right) * 0.5f;
    }

    public float centerY() {
        return (top + bottom) * 0.5f;
    }

    public void offset(float dx, float dy) {
        left += dx;
        top += dy;
        right += dx;
        bottom += dy;
    }

    public void offsetTo(float nx, float ny) {
        right += nx - left;
        bottom += ny - top;
        left = nx;
        top = ny;
    }

    public void inset(float dx, float dy) {
        left += dx;
        top += dy;
        right -= dx;
        bottom -= dy;
    }

    public boolean contains(float x, float y) {
        return left < right && top < bottom && x >= left && x < right && y >= top && y < bottom;
    }

    public boolean contains(float l, float t, float r, float b) {
        return this.left < this.right && this.top < this.bottom
                && this.left <= l && this.top <= t && this.right >= r && this.bottom >= b;
    }

    public boolean contains(RectF r) {
        return r != null && contains(r.left, r.top, r.right, r.bottom);
    }

    public boolean intersect(float l, float t, float r, float b) {
        if (this.left < r && l < this.right && this.top < b && t < this.bottom) {
            if (this.left < l) this.left = l;
            if (this.top < t) this.top = t;
            if (this.right > r) this.right = r;
            if (this.bottom > b) this.bottom = b;
            return true;
        }
        return false;
    }

    public boolean intersect(RectF r) {
        return r != null && intersect(r.left, r.top, r.right, r.bottom);
    }

    public void union(float l, float t, float r, float b) {
        if (l < r && t < b) {
            if (this.left < this.right && this.top < this.bottom) {
                if (this.left > l) this.left = l;
                if (this.top > t) this.top = t;
                if (this.right < r) this.right = r;
                if (this.bottom < b) this.bottom = b;
            } else {
                this.left = l;
                this.top = t;
                this.right = r;
                this.bottom = b;
            }
        }
    }

    public void union(RectF r) {
        if (r != null) {
            union(r.left, r.top, r.right, r.bottom);
        }
    }

    public void round(Rect dst) {
        if (dst != null) {
            dst.set(Math.round(left), Math.round(top), Math.round(right), Math.round(bottom));
        }
    }

    public void roundOut(Rect dst) {
        if (dst != null) {
            dst.set((int) Math.floor(left), (int) Math.floor(top),
                    (int) Math.ceil(right), (int) Math.ceil(bottom));
        }
    }

    public void sort() {
        if (left > right) {
            float temp = left;
            left = right;
            right = temp;
        }
        if (top > bottom) {
            float temp = top;
            top = bottom;
            bottom = temp;
        }
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (o == null || getClass() != o.getClass()) return false;
        RectF r = (RectF) o;
        return Float.compare(r.left, left) == 0
                && Float.compare(r.top, top) == 0
                && Float.compare(r.right, right) == 0
                && Float.compare(r.bottom, bottom) == 0;
    }

    @Override
    public int hashCode() {
        int result = (left != +0.0f ? Float.floatToIntBits(left) : 0);
        result = 31 * result + (top != +0.0f ? Float.floatToIntBits(top) : 0);
        result = 31 * result + (right != +0.0f ? Float.floatToIntBits(right) : 0);
        result = 31 * result + (bottom != +0.0f ? Float.floatToIntBits(bottom) : 0);
        return result;
    }

    @Override
    public String toString() {
        return "RectF(" + left + ", " + top + ", " + right + ", " + bottom + ")";
    }
}
