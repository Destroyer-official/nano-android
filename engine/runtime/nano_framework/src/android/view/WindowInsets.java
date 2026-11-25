package android.view;

public class WindowInsets {
    public static final WindowInsets CONSUMED = new WindowInsets();

    public int getSystemWindowInsetLeft() { return 0; }
    public int getSystemWindowInsetTop() { return 0; }
    public int getSystemWindowInsetRight() { return 0; }
    public int getSystemWindowInsetBottom() { return 0; }
    public boolean isConsumed() { return false; }
    public WindowInsets consumeSystemWindowInsets() { return this; }
}
