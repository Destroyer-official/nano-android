package android.view;

import android.content.Context;

public class ScaleGestureDetector {
    public interface OnScaleGestureListener {
        boolean onScale(ScaleGestureDetector detector);
        boolean onScaleBegin(ScaleGestureDetector detector);
        void onScaleEnd(ScaleGestureDetector detector);
    }

    public static class SimpleOnScaleGestureListener implements OnScaleGestureListener {
        public boolean onScale(ScaleGestureDetector detector) { return false; }
        public boolean onScaleBegin(ScaleGestureDetector detector) { return true; }
        public void onScaleEnd(ScaleGestureDetector detector) {}
    }

    public ScaleGestureDetector(Context context, OnScaleGestureListener listener) {}
    public boolean onTouchEvent(Object event) { return true; }
    public float getScaleFactor() { return 1.0f; }
}
