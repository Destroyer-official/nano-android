package android.widget;

import android.content.Context;
import android.view.View;
import android.view.ViewGroup;

public class ScrollView extends FrameLayout {
    private int scrollX = 0;
    private int scrollY = 0;
    private boolean fillViewport = false;

    public ScrollView() { super(); }
    public ScrollView(Context context) { super(context); }
    public ScrollView(Context context, android.util.AttributeSet attrs) { super(context, attrs); }

    public void setFillViewport(boolean fillViewport) { this.fillViewport = fillViewport; }
    public boolean isFillViewport() { return fillViewport; }

    public void smoothScrollTo(int x, int y) {
        scrollTo(x, y);
    }

    public void scrollTo(int x, int y) {
        int maxY = 0;
        if (getChildCount() > 0) {
            View c = getChildAt(0);
            if (c != null) maxY = Math.max(0, c.getMeasuredHeight() - getHeight());
        }
        this.scrollX = Math.max(0, x);
        this.scrollY = Math.max(0, Math.min(y, maxY));
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
        int hSize = MeasureSpec.getSize(heightMeasureSpec);

        for (int i = 0; i < getChildCount(); i++) {
            View child = getChildAt(i);
            if (child == null || child.getVisibility() == View.GONE) continue;
            ViewGroup.LayoutParams lp = child.getLayoutParams();
            if (lp == null) lp = new FrameLayout.LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.WRAP_CONTENT);
            int mH = (lp instanceof MarginLayoutParams) ? ((MarginLayoutParams) lp).leftMargin + ((MarginLayoutParams) lp).rightMargin : 0;
            int cw = getChildMeasureSpec(widthMeasureSpec, padH + mH, lp.width);
            int ch = MeasureSpec.makeMeasureSpec(0, MeasureSpec.UNSPECIFIED);
            child.measure(cw, ch);
            if (fillViewport && child.getMeasuredHeight() < Math.max(0, hSize - padV)) {
                child.measure(cw, MeasureSpec.makeMeasureSpec(Math.max(0, hSize - padV), MeasureSpec.EXACTLY));
            }
        }

        setMeasuredDimension(
            MeasureSpec.getSize(widthMeasureSpec),
            MeasureSpec.resolveSize(hSize, heightMeasureSpec)
        );
    }
}
