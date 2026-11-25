package android.widget;

import android.content.Context;
import android.view.animation.Interpolator;

public class Scroller {
    private int mStartX;
    private int mStartY;
    private int mFinalX;
    private int mFinalY;
    private int mCurrX;
    private int mCurrY;
    private boolean mFinished = true;

    public Scroller(Context context) {
        this(context, null);
    }

    public Scroller(Context context, Interpolator interpolator) {
        this(context, interpolator, false);
    }

    public Scroller(Context context, Interpolator interpolator, boolean flywheel) {}

    public final boolean isFinished() { return mFinished; }
    public final void forceFinished(boolean finished) { mFinished = finished; }
    public final int getDuration() { return 0; }
    public final int getCurrX() { return mCurrX; }
    public final int getCurrY() { return mCurrY; }
    public float getCurrVelocity() { return 0; }
    public final int getStartX() { return mStartX; }
    public final int getStartY() { return mStartY; }
    public final int getFinalX() { return mFinalX; }
    public final int getFinalY() { return mFinalY; }
    public void setFinalX(int newX) { mFinalX = newX; }
    public void setFinalY(int newY) { mFinalY = newY; }
    public boolean computeScrollOffset() { return false; }
    public void startScroll(int startX, int startY, int dx, int dy) { startScroll(startX, startY, dx, dy, 250); }
    public void startScroll(int startX, int startY, int dx, int dy, int duration) {
        mFinished = false;
        mStartX = startX;
        mStartY = startY;
        mFinalX = startX + dx;
        mFinalY = startY + dy;
    }
    public void fling(int startX, int startY, int velocityX, int velocityY, int minX, int maxX, int minY, int maxY) {
        mFinished = true;
    }
    public void abortAnimation() { mFinished = true; }
    public void extendDuration(int extend) {}
    public int timePassed() { return 0; }
}
