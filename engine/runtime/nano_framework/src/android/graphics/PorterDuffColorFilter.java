package android.graphics;

public class PorterDuffColorFilter extends ColorFilter {
    private int color;
    private PorterDuff.Mode mode;

    public PorterDuffColorFilter(int color, PorterDuff.Mode mode) {
        this.color = color;
        this.mode = mode;
    }
}
