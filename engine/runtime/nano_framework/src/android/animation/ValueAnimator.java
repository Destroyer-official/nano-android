package android.animation;

public class ValueAnimator extends Animator {
    public static final int RESTART = 1;
    public static final int REVERSE = 2;
    public static final int INFINITE = -1;

    private int repeatCount = 0;
    private int repeatMode = RESTART;
    private long startDelay = 0;

    public ValueAnimator() {}

    public interface AnimatorUpdateListener {
        void onAnimationUpdate(ValueAnimator animation);
    }

    public static ValueAnimator ofFloat(float... values) {
        return new ValueAnimator();
    }
    public static ValueAnimator ofInt(int... values) {
        return new ValueAnimator();
    }
    public static ValueAnimator ofObject(TypeEvaluator evaluator, Object... values) {
        return new ValueAnimator();
    }

    public void setFloatValues(float... values) {}
    public void setIntValues(int... values) {}
    public void setObjectValues(Object... values) {}
    public void setEvaluator(TypeEvaluator value) {}

    public void setRepeatCount(int value) { this.repeatCount = value; }
    public int getRepeatCount() { return repeatCount; }
    public void setRepeatMode(int value) { this.repeatMode = value; }
    public int getRepeatMode() { return repeatMode; }
    public void setStartDelay(long startDelay) { this.startDelay = startDelay; }
    public long getStartDelay() { return startDelay; }

    public void addUpdateListener(AnimatorUpdateListener listener) {}
    public void removeUpdateListener(AnimatorUpdateListener listener) {}
    public void removeAllUpdateListeners() {}

    @Override public long getDuration() { return 300; }
    @Override public ValueAnimator setDuration(long duration) { return this; }
    @Override public void setInterpolator(TimeInterpolator value) {}
    @Override public boolean isRunning() { return false; }
    public Object getAnimatedValue() { return 1.0f; }
    public float getAnimatedFraction() { return 1.0f; }
}
