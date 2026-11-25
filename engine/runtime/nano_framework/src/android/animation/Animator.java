package android.animation;

import java.util.ArrayList;

public abstract class Animator implements Cloneable {
    public interface AnimatorListener {
        void onAnimationStart(Animator animation);
        void onAnimationEnd(Animator animation);
        void onAnimationCancel(Animator animation);
        void onAnimationRepeat(Animator animation);
    }

    public interface AnimatorPauseListener {
        void onAnimationPause(Animator animation);
        void onAnimationResume(Animator animation);
    }

    private final ArrayList<AnimatorListener> listeners = new ArrayList<>();

    public void addListener(AnimatorListener listener) { listeners.add(listener); }
    public void removeListener(AnimatorListener listener) { listeners.remove(listener); }
    public ArrayList<AnimatorListener> getListeners() { return listeners; }

    public abstract long getDuration();
    public abstract Animator setDuration(long duration);
    public abstract void setInterpolator(TimeInterpolator value);
    public TimeInterpolator getInterpolator() { return null; }
    public abstract boolean isRunning();
    public boolean isStarted() { return isRunning(); }
    public void start() {}
    public void cancel() {}
    public void end() {}
    public void pause() {}
    public void resume() {}
    public Animator clone() {
        try {
            return (Animator) super.clone();
        } catch (CloneNotSupportedException e) {
            throw new AssertionError();
        }
    }
}
