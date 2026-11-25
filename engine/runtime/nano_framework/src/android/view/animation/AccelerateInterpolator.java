package android.view.animation;

import android.content.Context;
import android.util.AttributeSet;

public class AccelerateInterpolator extends BaseInterpolator {
    private float factor = 1.0f;
    public AccelerateInterpolator() {}
    public AccelerateInterpolator(float factor) { this.factor = factor; }
    public AccelerateInterpolator(Context context, AttributeSet attrs) {}
    @Override
    public float getInterpolation(float input) {
        if (factor == 1.0f) {
            return input * input;
        } else {
            return (float) Math.pow(input, 2 * factor);
        }
    }
}
