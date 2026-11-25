package android.widget;

import android.content.Context;
import android.graphics.Canvas;

public class EdgeEffect {
    public EdgeEffect(Context context) {}

    public void setSize(int width, int height) {}
    public boolean isFinished() { return true; }
    public void finish() {}
    public void onPull(float deltaDistance) {}
    public void onPull(float deltaDistance, float displacement) {}
    public void onRelease() {}
    public void onAbsorb(int velocity) {}
    public boolean draw(Canvas canvas) { return false; }
    public int getMaxHeight() { return 0; }
    public float onPullDistance(float deltaDistance, float displacement) { return deltaDistance; }
    public float getDistance() { return 0.0f; }
}
