package android.view;

import android.animation.Animator;
import android.animation.TimeInterpolator;
import android.animation.ValueAnimator;

public class ViewPropertyAnimator {
    private final View view;
    private long duration = 300;
    private long startDelay = 0;
    private TimeInterpolator interpolator;
    private Animator.AnimatorListener listener;
    private ValueAnimator.AnimatorUpdateListener updateListener;

    public ViewPropertyAnimator(View view) {
        this.view = view;
    }

    public ViewPropertyAnimator setDuration(long duration) { this.duration = duration; return this; }
    public long getDuration() { return duration; }
    public long getStartDelay() { return startDelay; }
    public ViewPropertyAnimator setStartDelay(long startDelay) { this.startDelay = startDelay; return this; }
    public ViewPropertyAnimator setInterpolator(TimeInterpolator interpolator) { this.interpolator = interpolator; return this; }
    public TimeInterpolator getInterpolator() { return interpolator; }
    public ViewPropertyAnimator setListener(Animator.AnimatorListener listener) { this.listener = listener; return this; }
    public ViewPropertyAnimator setUpdateListener(ValueAnimator.AnimatorUpdateListener updateListener) { this.updateListener = updateListener; return this; }
    public void start() {}
    public void cancel() {}
    public ViewPropertyAnimator x(float value) { return this; }
    public ViewPropertyAnimator xBy(float value) { return this; }
    public ViewPropertyAnimator y(float value) { return this; }
    public ViewPropertyAnimator yBy(float value) { return this; }
    public ViewPropertyAnimator z(float value) { return this; }
    public ViewPropertyAnimator zBy(float value) { return this; }
    public ViewPropertyAnimator rotation(float value) { if (view != null) view.setRotation(value); return this; }
    public ViewPropertyAnimator rotationBy(float value) { return this; }
    public ViewPropertyAnimator rotationX(float value) { return this; }
    public ViewPropertyAnimator rotationXBy(float value) { return this; }
    public ViewPropertyAnimator rotationY(float value) { return this; }
    public ViewPropertyAnimator rotationYBy(float value) { return this; }
    public ViewPropertyAnimator translationX(float value) { if (view != null) view.setTranslationX(value); return this; }
    public ViewPropertyAnimator translationXBy(float value) { return this; }
    public ViewPropertyAnimator translationY(float value) { if (view != null) view.setTranslationY(value); return this; }
    public ViewPropertyAnimator translationYBy(float value) { return this; }
    public ViewPropertyAnimator translationZ(float value) { if (view != null) view.setTranslationZ(value); return this; }
    public ViewPropertyAnimator translationZBy(float value) { return this; }
    public ViewPropertyAnimator scaleX(float value) { if (view != null) view.setScaleX(value); return this; }
    public ViewPropertyAnimator scaleXBy(float value) { return this; }
    public ViewPropertyAnimator scaleY(float value) { if (view != null) view.setScaleY(value); return this; }
    public ViewPropertyAnimator scaleYBy(float value) { return this; }
    public ViewPropertyAnimator alpha(float value) { if (view != null) view.setAlpha(value); return this; }
    public ViewPropertyAnimator alphaBy(float value) { return this; }
    public ViewPropertyAnimator withLayer() { return this; }
    public ViewPropertyAnimator withStartAction(Runnable runnable) { return this; }
    public ViewPropertyAnimator withEndAction(Runnable runnable) {
        if (runnable != null) runnable.run();
        return this;
    }
}
