package android.util;

public class TypedValue {
    public static final int COMPLEX_UNIT_SHIFT = 0;
    public static final int COMPLEX_UNIT_MASK = 0xf;
    public static final int COMPLEX_UNIT_PX = 0;
    public static final int COMPLEX_UNIT_DIP = 1;
    public static final int COMPLEX_UNIT_SP = 2;
    public static final int COMPLEX_UNIT_PT = 3;
    public static final int COMPLEX_UNIT_IN = 4;
    public static final int COMPLEX_UNIT_MM = 5;

    public static final int COMPLEX_MANTISSA_SHIFT = 8;
    public static final int COMPLEX_MANTISSA_MASK = 0xffffff;
    public static final int COMPLEX_RADIX_SHIFT = 4;
    public static final int COMPLEX_RADIX_MASK = 0x3;
    private static final float MANTISSA_MULT = 1.0f / (1 << COMPLEX_MANTISSA_SHIFT);
    private static final float[] RADIX_MULTS = new float[]{
        1.0f * MANTISSA_MULT, 1.0f / (1 << 7) * MANTISSA_MULT,
        1.0f / (1 << 15) * MANTISSA_MULT, 1.0f / (1 << 23) * MANTISSA_MULT
    };

    public static final int TYPE_NULL = 0;
    public static final int TYPE_REFERENCE = 1;
    public static final int TYPE_ATTRIBUTE = 2;
    public static final int TYPE_STRING = 3;
    public static final int TYPE_FLOAT = 4;
    public static final int TYPE_DIMENSION = 5;
    public static final int TYPE_FRACTION = 6;
    public static final int TYPE_INT_DEC = 16;
    public static final int TYPE_INT_HEX = 17;
    public static final int TYPE_INT_BOOLEAN = 18;

    public int type;
    public int data;
    public int resourceId;
    public CharSequence string;

    public final float getFloat() {
        return Float.intBitsToFloat(data);
    }

    public float getDimension(DisplayMetrics metrics) {
        return complexToDimension(data, metrics);
    }

    public static float complexToFloat(int complex) {
        return (complex & (COMPLEX_MANTISSA_MASK << COMPLEX_MANTISSA_SHIFT))
                * RADIX_MULTS[(complex >> COMPLEX_RADIX_SHIFT) & COMPLEX_RADIX_MASK];
    }

    public static float complexToDimension(int data, DisplayMetrics metrics) {
        return applyDimension((data >> COMPLEX_UNIT_SHIFT) & COMPLEX_UNIT_MASK, complexToFloat(data), metrics);
    }

    public static int complexToDimensionPixelOffset(int data, DisplayMetrics metrics) {
        return (int) complexToDimension(data, metrics);
    }

    public static int complexToDimensionPixelSize(int data, DisplayMetrics metrics) {
        float value = complexToDimension(data, metrics);
        float f = (value >= 0) ? (value + 0.5f) : (value - 0.5f);
        int res = (int) f;
        if (res != 0) return res;
        if (value == 0) return 0;
        if (value > 0) return 1;
        return -1;
    }

    public static float applyDimension(int unit, float value, DisplayMetrics metrics) {
        DisplayMetrics m = metrics != null ? metrics : new DisplayMetrics();
        switch (unit) {
            case COMPLEX_UNIT_PX:
                return value;
            case COMPLEX_UNIT_DIP:
                return value * m.density;
            case COMPLEX_UNIT_SP:
                return value * m.scaledDensity;
            case COMPLEX_UNIT_PT:
                return value * m.xdpi * (1.0f / 72);
            case COMPLEX_UNIT_IN:
                return value * m.xdpi;
            case COMPLEX_UNIT_MM:
                return value * m.xdpi * (1.0f / 25.4f);
            default:
                return value;
        }
    }

    public final CharSequence coerceToString() {
        if (type == TYPE_STRING) return string;
        if (type == TYPE_FLOAT) return Float.toString(getFloat());
        if (type == TYPE_INT_BOOLEAN) return data != 0 ? "true" : "false";
        if (type >= TYPE_INT_DEC && type <= TYPE_INT_HEX) return Integer.toString(data);
        return string != null ? string : "";
    }
}
