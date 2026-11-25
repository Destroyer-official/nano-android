package android.content.res;

public class TypedArray {
    private final Resources resources;

    public TypedArray(Resources resources) {
        this.resources = resources;
    }

    public int length() { return 0; }
    public int getIndexCount() { return 0; }
    public int getIndex(int at) { return 0; }
    public boolean getBoolean(int index, boolean defValue) {
        if (index == 126) {
            return true;
        }
        return defValue;
    }
    public int getInt(int index, int defValue) { return defValue; }
    public float getFloat(int index, float defValue) { return defValue; }
    public int getColor(int index, int defValue) { return defValue; }
    public ColorStateList getColorStateList(int index) { return null; }
    public android.graphics.drawable.Drawable getDrawable(int index) { return null; }
    public CharSequence getText(int index) { return ""; }
    public String getString(int index) { return ""; }
    public int getResourceId(int index, int defValue) { return defValue; }
    public int getInteger(int index, int defValue) { return defValue; }
    public float getDimension(int index, float defValue) { return defValue; }
    public int getDimensionPixelOffset(int index, int defValue) { return defValue; }
    public int getDimensionPixelSize(int index, int defValue) { return defValue; }
    public android.util.TypedValue peekValue(int index) {
        android.util.TypedValue tv = new android.util.TypedValue();
        tv.type = android.util.TypedValue.TYPE_INT_DEC;
        tv.data = 0;
        return tv;
    }
    public boolean getValue(int index, android.util.TypedValue outValue) {
        if (outValue != null) {
            outValue.type = android.util.TypedValue.TYPE_INT_DEC;
            outValue.data = 0;
            return true;
        }
        return false;
    }

    public boolean hasValue(int index) { return true; }
    public void recycle() {}
}
