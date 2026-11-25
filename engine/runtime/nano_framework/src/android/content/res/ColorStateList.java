package android.content.res;

public class ColorStateList {
    private final int defaultColor;

    public ColorStateList(int defaultColor) {
        this.defaultColor = defaultColor;
    }

    public static ColorStateList valueOf(int color) {
        return new ColorStateList(color);
    }

    public int getDefaultColor() {
        return defaultColor;
    }

    public int getColorForState(int[] stateSet, int defaultColor) {
        return this.defaultColor;
    }

    public boolean isStateful() {
        return false;
    }
}
