package android.util;

public class TypedValue {
    public static final int COMPLEX_UNIT_PX = 0;
    public static final int COMPLEX_UNIT_DIP = 1;
    public static final int COMPLEX_UNIT_SP = 2;
    public static final int COMPLEX_UNIT_PT = 3;
    public static final int COMPLEX_UNIT_IN = 4;
    public static final int COMPLEX_UNIT_MM = 5;

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

    public static float applyDimension(int unit, float value, DisplayMetrics metrics) {
        switch (unit) {
            case COMPLEX_UNIT_PX:
                return value;
            case COMPLEX_UNIT_DIP:
                return value * (metrics != null ? metrics.density : 2.625f);
            case COMPLEX_UNIT_SP:
                return value * (metrics != null ? metrics.scaledDensity : 2.625f);
            default:
                return value;
        }
    }
}
