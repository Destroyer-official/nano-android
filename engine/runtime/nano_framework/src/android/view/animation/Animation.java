package android.view.animation;

import android.content.Context;
import android.util.AttributeSet;

public abstract class Animation implements Cloneable {
    public static final int INFINITE = -1;
    public static final int RESTART = 1;
    public static final int REVERSE = 2;

    public interface AnimationListener {
        void onAnimationStart(Animation animation);
        void onAnimationEnd(Animation animation);
        void onAnimationRepeat(Animation animation);
    }

    private Interpolator interpolator;
    private long duration;
    private AnimationListener listener;

    public Animation() {}
    public Animation(Context context, AttributeSet attrs) {}

    public void setInterpolator(Interpolator i) { this.interpolator = i; }
    public Interpolator getInterpolator() { return interpolator; }
    public void setDuration(long durationMillis) { this.duration = durationMillis; }
    public long getDuration() { return duration; }
    public void setAnimationListener(AnimationListener listener) { this.listener = listener; }
    public void cancel() {}
    public void reset() {}
    public void start() {}
}
