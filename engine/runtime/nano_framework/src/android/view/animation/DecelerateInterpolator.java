package android.view.animation;

import android.content.Context;
import android.util.AttributeSet;

public class DecelerateInterpolator extends BaseInterpolator {
    private float factor = 1.0f;
    public DecelerateInterpolator() {}
    public DecelerateInterpolator(float factor) { this.factor = factor; }
    public DecelerateInterpolator(Context context, AttributeSet attrs) {}
    @Override
    public float getInterpolation(float input) {
        if (factor == 1.0f) {
            return 1.0f - (1.0f - input) * (1.0f - input);
        } else {
            return (float)(1.0f - Math.pow((1.0f - input), 2 * factor));
        }
    }
}
