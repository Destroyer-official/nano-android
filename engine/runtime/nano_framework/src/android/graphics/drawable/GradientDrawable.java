package android.graphics.drawable;

import android.graphics.Color;

public class GradientDrawable extends Drawable {
    public static final int RECTANGLE = 0;
    public static final int OVAL = 1;
    public static final int LINE = 2;
    public static final int RING = 3;

    public enum Orientation {
        TOP_BOTTOM, TR_BL, RIGHT_LEFT, BR_TL, BOTTOM_TOP, BL_TR, LEFT_RIGHT, TL_BR
    }

    private int shape = RECTANGLE;
    private int color = Color.TRANSPARENT;
    private Orientation orientation = Orientation.TOP_BOTTOM;
    private int[] colors = null;
    private float cornerRadius = 0;
    private float[] cornerRadii = null;
    private int strokeWidth = 0;
    private int strokeColor = Color.TRANSPARENT;

    public GradientDrawable() {}

    public GradientDrawable(Orientation orientation, int[] colors) {
        this.orientation = orientation != null ? orientation : Orientation.TOP_BOTTOM;
        this.colors = colors;
        if (colors != null && colors.length > 0) {
            this.color = colors[0];
        }
    }

    public void setShape(int shape) { this.shape = shape; }
    public int getShape() { return shape; }

    public void setColor(int argb) {
        this.color = argb;
        this.colors = null;
    }
    public int getColor() { return color; }

    public void setColors(int[] colors) {
        this.colors = colors;
        if (colors != null && colors.length > 0) {
            this.color = colors[0];
        }
    }
    public int[] getColors() { return colors; }

    public void setOrientation(Orientation orientation) {
        this.orientation = orientation != null ? orientation : Orientation.TOP_BOTTOM;
    }
    public Orientation getOrientation() { return orientation; }

    public void setCornerRadius(float radiusPx) { this.cornerRadius = radiusPx; }
    public float getCornerRadius() { return cornerRadius; }

    public void setCornerRadii(float[] radii) { this.cornerRadii = radii; }
    public float[] getCornerRadii() { return cornerRadii; }

    public void setStroke(int widthPx, int colorArgb) {
        this.strokeWidth = widthPx;
        this.strokeColor = colorArgb;
    }
    public int getStrokeWidth() { return strokeWidth; }
    public int getStrokeColor() { return strokeColor; }
}
