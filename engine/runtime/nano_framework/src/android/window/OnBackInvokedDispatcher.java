package android.window;

public interface OnBackInvokedDispatcher {
    public static final int PRIORITY_DEFAULT = 0;
    public static final int PRIORITY_OVERLAY = 1000000;

    public void registerOnBackInvokedCallback(int priority, OnBackInvokedCallback callback);
    public void unregisterOnBackInvokedCallback(OnBackInvokedCallback callback);
}
