package android.graphics;

import android.os.Parcel;
import android.os.Parcelable;

public class Region implements Parcelable {
    public enum Op {
        DIFFERENCE(0),
        INTERSECT(1),
        UNION(2),
        XOR(3),
        REVERSE_DIFFERENCE(4),
        REPLACE(5);
        public final int nativeInt;
        Op(int nativeInt) { this.nativeInt = nativeInt; }
    }

    private final Rect bounds = new Rect();

    public Region() {}
    public Region(Region region) { if (region != null) bounds.set(region.bounds); }
    public Region(Rect r) { if (r != null) bounds.set(r); }
    public Region(int left, int top, int right, int bottom) { bounds.set(left, top, right, bottom); }

    public static final Creator<Region> CREATOR = new Creator<Region>() {
        @Override public Region createFromParcel(Parcel in) { return new Region(); }
        @Override public Region[] newArray(int size) { return new Region[size]; }
    };

    public void setEmpty() { bounds.setEmpty(); }
    public boolean set(Region region) { if (region != null) bounds.set(region.bounds); else bounds.setEmpty(); return !isEmpty(); }
    public boolean set(Rect r) { if (r != null) bounds.set(r); else bounds.setEmpty(); return !isEmpty(); }
    public boolean set(int left, int top, int right, int bottom) { bounds.set(left, top, right, bottom); return !isEmpty(); }

    public boolean setPath(Path path, Region clip) {
        if (path != null) {
            RectF rf = new RectF();
            path.computeBounds(rf, true);
            bounds.set((int) rf.left, (int) rf.top, (int) rf.right, (int) rf.bottom);
            if (clip != null && !clip.isEmpty()) {
                bounds.intersect(clip.bounds);
            }
        } else {
            bounds.setEmpty();
        }
        return !isEmpty();
    }

    public boolean isEmpty() { return bounds.isEmpty(); }
    public boolean isRect() { return !isEmpty(); }
    public boolean isComplex() { return false; }

    public Rect getBounds() { return new Rect(bounds); }
    public boolean getBounds(Rect r) { if (r == null) return false; r.set(bounds); return !isEmpty(); }

    public Path getBoundaryPath() {
        Path p = new Path();
        if (!isEmpty()) p.addRect(bounds.left, bounds.top, bounds.right, bounds.bottom, Path.Direction.CW);
        return p;
    }
    public boolean getBoundaryPath(Path path) {
        if (path == null) return false;
        path.reset();
        if (!isEmpty()) path.addRect(bounds.left, bounds.top, bounds.right, bounds.bottom, Path.Direction.CW);
        return !isEmpty();
    }

    public boolean contains(int x, int y) { return bounds.contains(x, y); }
    public boolean quickContains(Rect r) { return r != null && bounds.contains(r); }
    public boolean quickContains(int left, int top, int right, int bottom) { return bounds.contains(left, top, right, bottom); }
    public boolean quickReject(Rect r) { return r == null || !Rect.intersects(bounds, r); }
    public boolean quickReject(int left, int top, int right, int bottom) { return !bounds.intersects(left, top, right, bottom); }
    public boolean quickReject(Region rgn) { return rgn == null || !Rect.intersects(bounds, rgn.bounds); }

    public void translate(int dx, int dy) { bounds.offset(dx, dy); }
    public void translate(int dx, int dy, Region dst) {
        if (dst != null) {
            dst.set(bounds);
            dst.translate(dx, dy);
        }
    }

    public final boolean union(Rect r) {
        if (r != null) bounds.union(r);
        return !isEmpty();
    }

    public boolean op(Rect r, Op op) { return op(new Region(r), op); }
    public boolean op(int left, int top, int right, int bottom, Op op) { return op(new Region(left, top, right, bottom), op); }
    public boolean op(Region region, Op op) {
        if (region == null || op == null) return !isEmpty();
        switch (op) {
            case UNION: bounds.union(region.bounds); break;
            case INTERSECT: if (!bounds.intersect(region.bounds)) bounds.setEmpty(); break;
            case REPLACE: bounds.set(region.bounds); break;
            default: break;
        }
        return !isEmpty();
    }
    public boolean op(Rect rect, Region region, Op op) {
        bounds.set(rect != null ? rect : new Rect());
        return op(region, op);
    }
    public boolean op(Region region1, Region region2, Op op) {
        set(region1);
        return op(region2, op);
    }

    @Override public int describeContents() { return 0; }
    @Override public void writeToParcel(Parcel dest, int flags) {}
}
