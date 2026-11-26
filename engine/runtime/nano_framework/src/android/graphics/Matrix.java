package android.graphics;

public class Matrix {
    public static final int MSCALE_X = 0;
    public static final int MSKEW_X  = 1;
    public static final int MTRANS_X = 2;
    public static final int MSKEW_Y  = 3;
    public static final int MSCALE_Y = 4;
    public static final int MTRANS_Y = 5;
    public static final int MPERSP_0 = 6;
    public static final int MPERSP_1 = 7;
    public static final int MPERSP_2 = 8;

    public enum ScaleToFit { FILL, START, CENTER, END }

    final float[] m = new float[] {
        1f, 0f, 0f,
        0f, 1f, 0f,
        0f, 0f, 1f
    };

    public Matrix() {}

    public Matrix(Matrix src) {
        set(src);
    }

    public void reset() {
        m[0] = 1f; m[1] = 0f; m[2] = 0f;
        m[3] = 0f; m[4] = 1f; m[5] = 0f;
        m[6] = 0f; m[7] = 0f; m[8] = 1f;
    }

    public void set(Matrix src) {
        if (src == null) {
            reset();
        } else {
            System.arraycopy(src.m, 0, m, 0, 9);
        }
    }

    public boolean isIdentity() {
        return m[0] == 1f && m[1] == 0f && m[2] == 0f
                && m[3] == 0f && m[4] == 1f && m[5] == 0f
                && m[6] == 0f && m[7] == 0f && m[8] == 1f;
    }

    public void setTranslate(float dx, float dy) {
        m[0] = 1f; m[1] = 0f; m[2] = dx;
        m[3] = 0f; m[4] = 1f; m[5] = dy;
        m[6] = 0f; m[7] = 0f; m[8] = 1f;
    }

    public void setScale(float sx, float sy) {
        m[0] = sx; m[1] = 0f; m[2] = 0f;
        m[3] = 0f; m[4] = sy; m[5] = 0f;
        m[6] = 0f; m[7] = 0f; m[8] = 1f;
    }

    public void setScale(float sx, float sy, float px, float py) {
        m[0] = sx; m[1] = 0f; m[2] = px - sx * px;
        m[3] = 0f; m[4] = sy; m[5] = py - sy * py;
        m[6] = 0f; m[7] = 0f; m[8] = 1f;
    }

    public void setRotate(float degrees) {
        double rad = Math.toRadians(degrees);
        float s = (float) Math.sin(rad);
        float c = (float) Math.cos(rad);
        m[0] = c;  m[1] = -s; m[2] = 0f;
        m[3] = s;  m[4] = c;  m[5] = 0f;
        m[6] = 0f; m[7] = 0f; m[8] = 1f;
    }

    public void setRotate(float degrees, float px, float py) {
        double rad = Math.toRadians(degrees);
        float s = (float) Math.sin(rad);
        float c = (float) Math.cos(rad);
        m[0] = c;  m[1] = -s; m[2] = px - c * px + s * py;
        m[3] = s;  m[4] = c;  m[5] = py - s * px - c * py;
        m[6] = 0f; m[7] = 0f; m[8] = 1f;
    }

    public boolean setConcat(Matrix a, Matrix b) {
        if (a == null || b == null) return false;
        float[] am = a.m;
        float[] bm = b.m;
        float m0 = am[0] * bm[0] + am[1] * bm[3] + am[2] * bm[6];
        float m1 = am[0] * bm[1] + am[1] * bm[4] + am[2] * bm[7];
        float m2 = am[0] * bm[2] + am[1] * bm[5] + am[2] * bm[8];
        float m3 = am[3] * bm[0] + am[4] * bm[3] + am[5] * bm[6];
        float m4 = am[3] * bm[1] + am[4] * bm[4] + am[5] * bm[7];
        float m5 = am[3] * bm[2] + am[4] * bm[5] + am[5] * bm[8];
        float m6 = am[6] * bm[0] + am[7] * bm[3] + am[8] * bm[6];
        float m7 = am[6] * bm[1] + am[7] * bm[4] + am[8] * bm[7];
        float m8 = am[6] * bm[2] + am[7] * bm[5] + am[8] * bm[8];
        m[0] = m0; m[1] = m1; m[2] = m2;
        m[3] = m3; m[4] = m4; m[5] = m5;
        m[6] = m6; m[7] = m7; m[8] = m8;
        return true;
    }

    public boolean preConcat(Matrix other) {
        if (other == null) return false;
        return setConcat(this, other);
    }

    public boolean postConcat(Matrix other) {
        if (other == null) return false;
        return setConcat(other, this);
    }

    public boolean preTranslate(float dx, float dy) {
        Matrix t = new Matrix();
        t.setTranslate(dx, dy);
        return preConcat(t);
    }

    public boolean preScale(float sx, float sy) {
        Matrix s = new Matrix();
        s.setScale(sx, sy);
        return preConcat(s);
    }

    public boolean preScale(float sx, float sy, float px, float py) {
        Matrix s = new Matrix();
        s.setScale(sx, sy, px, py);
        return preConcat(s);
    }

    public boolean preRotate(float degrees) {
        Matrix r = new Matrix();
        r.setRotate(degrees);
        return preConcat(r);
    }

    public boolean preRotate(float degrees, float px, float py) {
        Matrix r = new Matrix();
        r.setRotate(degrees, px, py);
        return preConcat(r);
    }

    public boolean postTranslate(float dx, float dy) {
        Matrix t = new Matrix();
        t.setTranslate(dx, dy);
        return postConcat(t);
    }

    public boolean postScale(float sx, float sy) {
        Matrix s = new Matrix();
        s.setScale(sx, sy);
        return postConcat(s);
    }

    public boolean postScale(float sx, float sy, float px, float py) {
        Matrix s = new Matrix();
        s.setScale(sx, sy, px, py);
        return postConcat(s);
    }

    public boolean postRotate(float degrees) {
        Matrix r = new Matrix();
        r.setRotate(degrees);
        return postConcat(r);
    }

    public boolean postRotate(float degrees, float px, float py) {
        Matrix r = new Matrix();
        r.setRotate(degrees, px, py);
        return postConcat(r);
    }

    public boolean setRectToRect(RectF src, RectF dst, ScaleToFit stf) {
        if (src == null || dst == null) return false;
        if (src.isEmpty()) {
            reset();
            return false;
        }
        if (dst.isEmpty()) {
            m[0] = 0f; m[1] = 0f; m[2] = 0f;
            m[3] = 0f; m[4] = 0f; m[5] = 0f;
            m[6] = 0f; m[7] = 0f; m[8] = 1f;
            return true;
        }
        float sx = dst.width() / src.width();
        float sy = dst.height() / src.height();
        if (stf != ScaleToFit.FILL) {
            float s = Math.min(sx, sy);
            sx = s;
            sy = s;
        }
        float tx = dst.left - src.left * sx;
        float ty = dst.top - src.top * sy;
        if (stf == ScaleToFit.CENTER) {
            tx += (dst.width() - src.width() * sx) * 0.5f;
            ty += (dst.height() - src.height() * sy) * 0.5f;
        } else if (stf == ScaleToFit.END) {
            tx += dst.width() - src.width() * sx;
            ty += dst.height() - src.height() * sy;
        }
        m[0] = sx; m[1] = 0f; m[2] = tx;
        m[3] = 0f; m[4] = sy; m[5] = ty;
        m[6] = 0f; m[7] = 0f; m[8] = 1f;
        return true;
    }

    public void mapPoints(float[] pts) {
        mapPoints(pts, pts);
    }

    public void mapPoints(float[] dst, float[] src) {
        if (dst == null || src == null) return;
        int len = Math.min(dst.length, src.length);
        for (int i = 0; i + 1 < len; i += 2) {
            float x = src[i];
            float y = src[i + 1];
            float w = m[6] * x + m[7] * y + m[8];
            float invW = (w != 0f && w != 1f) ? (1f / w) : 1f;
            dst[i] = (m[0] * x + m[1] * y + m[2]) * invW;
            dst[i + 1] = (m[3] * x + m[4] * y + m[5]) * invW;
        }
    }

    public boolean rectStaysRect() {
        return m[6] == 0f && m[7] == 0f
                && ((m[1] == 0f && m[3] == 0f) || (m[0] == 0f && m[4] == 0f));
    }

    public boolean mapRect(RectF rect) {
        return mapRect(rect, rect);
    }

    public boolean mapRect(RectF dst, RectF src) {
        if (dst == null || src == null) return false;
        float[] corners = new float[] {
            src.left, src.top,
            src.right, src.top,
            src.right, src.bottom,
            src.left, src.bottom
        };
        mapPoints(corners);
        float minX = corners[0], maxX = corners[0];
        float minY = corners[1], maxY = corners[1];
        for (int i = 2; i < 8; i += 2) {
            if (corners[i] < minX) minX = corners[i];
            if (corners[i] > maxX) maxX = corners[i];
            if (corners[i + 1] < minY) minY = corners[i + 1];
            if (corners[i + 1] > maxY) maxY = corners[i + 1];
        }
        dst.set(minX, minY, maxX, maxY);
        return rectStaysRect();
    }

    public void getValues(float[] values) {
        if (values != null && values.length >= 9) {
            System.arraycopy(m, 0, values, 0, 9);
        }
    }

    public void setValues(float[] values) {
        if (values != null && values.length >= 9) {
            System.arraycopy(values, 0, m, 0, 9);
        }
    }
}
