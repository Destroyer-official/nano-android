package android.graphics;

public class Paint {
    public enum Style { FILL, STROKE, FILL_AND_STROKE }
    public enum Cap { BUTT, ROUND, SQUARE }
    public enum Join { MITER, ROUND, BEVEL }
    public enum Align { LEFT, CENTER, RIGHT }

    public static final int ANTI_ALIAS_FLAG = 1;

    private int color = Color.BLACK;
    private Style style = Style.FILL;
    private Cap cap = Cap.BUTT;
    private Join join = Join.MITER;
    private Align align = Align.LEFT;
    private float strokeWidth = 1.0f;
    private float textSize = 14.0f;
    private Typeface typeface = Typeface.DEFAULT;
    private boolean antiAlias = true;

    public Paint() {}
    public Paint(int flags) {
        this.antiAlias = (flags & ANTI_ALIAS_FLAG) != 0;
    }
    public Paint(Paint paint) {
        if (paint != null) {
            this.color = paint.color;
            this.style = paint.style;
            this.cap = paint.cap;
            this.join = paint.join;
            this.align = paint.align;
            this.strokeWidth = paint.strokeWidth;
            this.textSize = paint.textSize;
            this.typeface = paint.typeface;
            this.antiAlias = paint.antiAlias;
        }
    }

    public void setColor(int color) { this.color = color; }
    public int getColor() { return color; }
    public void setStyle(Style style) { this.style = style; }
    public Style getStyle() { return style; }
    public void setStrokeWidth(float width) { this.strokeWidth = width; }
    public float getStrokeWidth() { return strokeWidth; }
    public void setAntiAlias(boolean aa) { this.antiAlias = aa; }
    public boolean isAntiAlias() { return antiAlias; }

    public void setStrokeCap(Cap cap) { this.cap = cap; }
    public Cap getStrokeCap() { return cap; }
    public void setStrokeJoin(Join join) { this.join = join; }
    public Join getStrokeJoin() { return join; }

    public void setTextAlign(Align align) { this.align = align; }
    public Align getTextAlign() { return align; }

    public void setTextSize(float size) { this.textSize = size; }
    public float getTextSize() { return textSize; }

    public void setTypeface(Typeface tf) { this.typeface = tf; }
    public Typeface getTypeface() { return typeface; }

    public void setShader(Object shader) {}
    public void setColorFilter(Object filter) {}
    public ColorFilter setColorFilter(ColorFilter filter) { return filter; }
    public Xfermode setXfermode(Xfermode xfermode) { return xfermode; }
    public void setAlpha(int a) {}
    public void setFilterBitmap(boolean filter) {}

    public float measureText(String text) {
        return text != null ? text.length() * (textSize * 0.6f) : 0;
    }

    public void getTextBounds(String text, int start, int end, Rect bounds) {
        if (bounds != null) {
            int len = (text != null && end > start) ? end - start : 0;
            bounds.left = 0;
            bounds.top = 0;
            bounds.right = (int) (len * textSize * 0.6f);
            bounds.bottom = (int) textSize;
        }
    }
}
