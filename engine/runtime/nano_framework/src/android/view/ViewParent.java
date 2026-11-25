package android.view;

public interface ViewParent {
    void requestLayout();
    boolean isLayoutRequested();
    void bringChildToFront(View child);
    void requestDisallowInterceptTouchEvent(boolean disallowIntercept);
    ViewParent getParent();
}
