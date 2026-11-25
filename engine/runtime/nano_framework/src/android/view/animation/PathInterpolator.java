package android.view.animation;

import android.content.Context;
import android.util.AttributeSet;

public class PathInterpolator extends BaseInterpolator {
    public PathInterpolator() {}
    public PathInterpolator(float controlX, float controlY) {}
    public PathInterpolator(float controlX1, float controlY1, float controlX2, float controlY2) {}
    public PathInterpolator(Context context, AttributeSet attrs) {}
    @Override
    public float getInterpolation(float t) { return t; }
}
