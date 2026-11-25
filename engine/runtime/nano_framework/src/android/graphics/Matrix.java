package android.graphics;

public class Matrix {
    public enum ScaleToFit { FILL, START, CENTER, END }

    public Matrix() {}
    public boolean postScale(float sx, float sy) { return true; }
    public boolean postScale(float sx, float sy, float px, float py) { return true; }
    public boolean postTranslate(float dx, float dy) { return true; }
    public boolean postRotate(float degrees) { return true; }
    public boolean postRotate(float degrees, float px, float py) { return true; }
    public boolean setRectToRect(RectF src, RectF dst, ScaleToFit stf) { return true; }
    public void reset() {}
}
