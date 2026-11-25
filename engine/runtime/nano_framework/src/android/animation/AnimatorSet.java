package android.animation;

import java.util.ArrayList;

public class AnimatorSet extends Animator {
    public AnimatorSet() {}

    public class Builder {
        private Animator anim;
        Builder(Animator anim) { this.anim = anim; }
        public Builder with(Animator a) { return this; }
        public Builder before(Animator a) { return this; }
        public Builder after(Animator a) { return this; }
        public Builder after(long delay) { return this; }
    }

    public Builder play(Animator anim) { return new Builder(anim); }
    public void playTogether(Animator... items) {}
    public void playTogether(java.util.Collection<Animator> items) {}
    public void playSequentially(Animator... items) {}
    public void playSequentially(java.util.List<Animator> items) {}
    public ArrayList<Animator> getChildAnimations() { return new ArrayList<>(); }
    @Override public long getDuration() { return 300; }
    @Override public AnimatorSet setDuration(long duration) { return this; }
    @Override public void setInterpolator(TimeInterpolator value) {}
    @Override public boolean isRunning() { return false; }
}
