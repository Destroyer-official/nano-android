package android.widget;

import android.content.Context;
import android.view.View;

public class ProgressBar extends View {
    private int max = 100;
    private int progress = 0;

    public ProgressBar() { super(); }
    public ProgressBar(Context context) { super(context); }
    public ProgressBar(Context context, android.util.AttributeSet attrs) { super(context, attrs); }
    public ProgressBar(Context context, android.util.AttributeSet attrs, int defStyleAttr) { super(context, attrs, defStyleAttr); }

    public void setIndeterminate(boolean ind) {}
    public boolean isIndeterminate() { return false; }
    public void setProgressDrawable(android.graphics.drawable.Drawable d) {}
    public void setIndeterminateDrawable(android.graphics.drawable.Drawable d) {}

    public synchronized void setMax(int max) { this.max = max; }
    public synchronized int getMax() { return max; }

    public synchronized void setProgress(int progress) { this.progress = progress; }
    public synchronized int getProgress() { return progress; }

    @Override
    protected void onMeasure(int widthMeasureSpec, int heightMeasureSpec) {
        int intrinsicW = 360 + getPaddingLeft() + getPaddingRight();
        int intrinsicH = ((this instanceof SeekBar) ? 68 : 24) + getPaddingTop() + getPaddingBottom();
        setMeasuredDimension(
            MeasureSpec.resolveSize(intrinsicW, widthMeasureSpec),
            MeasureSpec.resolveSize(intrinsicH, heightMeasureSpec)
        );
    }
}
