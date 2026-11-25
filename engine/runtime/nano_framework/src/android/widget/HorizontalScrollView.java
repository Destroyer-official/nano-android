package android.widget;

import android.content.Context;
import android.view.View;
import android.view.ViewGroup;

public class HorizontalScrollView extends FrameLayout {
    private int scrollX = 0;
    private int scrollY = 0;
    private boolean fillViewport = false;

    public HorizontalScrollView() { super(); }
    public HorizontalScrollView(Context context) { super(context); }
    public HorizontalScrollView(Context context, android.util.AttributeSet attrs) { super(context, attrs); }

    public void setFillViewport(boolean fillViewport) { this.fillViewport = fillViewport; }
    public boolean isFillViewport() { return fillViewport; }

    public void smoothScrollTo(int x, int y) { scrollTo(x, y); }
    public void scrollTo(int x, int y) {
        int maxX = 0;
        if (getChildCount() > 0) {
            View c = getChildAt(0);
            if (c != null) maxX = Math.max(0, c.getMeasuredWidth() - getWidth());
        }
        this.scrollX = Math.max(0, Math.min(x, maxX));
        this.scrollY = Math.max(0, y);
    }
    public void scrollBy(int dx, int dy) {
        scrollTo(scrollX + dx, scrollY + dy);
    }
    public int getScrollX() { return scrollX; }
    public int getScrollY() { return scrollY; }

    @Override
    protected void onMeasure(int widthMeasureSpec, int heightMeasureSpec) {
        int padH = getPaddingLeft() + getPaddingRight();
        int padV = getPaddingTop() + getPaddingBottom();
        int maxH = 0;

        for (int i = 0; i < getChildCount(); i++) {
            View child = getChildAt(i);
            if (child == null || child.getVisibility() == View.GONE) continue;
            ViewGroup.LayoutParams lp = child.getLayoutParams();
            if (lp == null) lp = new FrameLayout.LayoutParams(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT);
            int mV = (lp instanceof MarginLayoutParams) ? ((MarginLayoutParams) lp).topMargin + ((MarginLayoutParams) lp).bottomMargin : 0;
            int cw = MeasureSpec.makeMeasureSpec(0, MeasureSpec.UNSPECIFIED);
            int ch = getChildMeasureSpec(heightMeasureSpec, padV + mV, lp.height);
            child.measure(cw, ch);
            maxH = Math.max(maxH, child.getMeasuredHeight() + mV);
        }

        setMeasuredDimension(
            MeasureSpec.getSize(widthMeasureSpec),
            MeasureSpec.resolveSize(maxH + padV, heightMeasureSpec)
        );
    }
}
