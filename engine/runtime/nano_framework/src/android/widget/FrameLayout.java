package android.widget;

import android.content.Context;
import android.util.AttributeSet;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;

public class FrameLayout extends ViewGroup {
    public static class LayoutParams extends ViewGroup.MarginLayoutParams {
        public int gravity = -1;

        public LayoutParams(Context c, AttributeSet attrs) {
            super(c, attrs);
        }

        public LayoutParams(int width, int height) {
            super(width, height);
        }

        public LayoutParams(int width, int height, int gravity) {
            super(width, height);
            this.gravity = gravity;
        }
    }

    public FrameLayout() { super(); }
    public FrameLayout(Context context) { super(context); }
    public FrameLayout(Context context, AttributeSet attrs) { super(context, attrs); }
    public FrameLayout(Context context, AttributeSet attrs, int defStyleAttr) { super(context, attrs, defStyleAttr); }

    public void setClipBounds(android.graphics.Rect clipBounds) { super.setClipBounds(clipBounds); }

    @Override
    protected void onLayout(boolean changed, int left, int top, int right, int bottom) {
        int pL = getPaddingLeft();
        int pT = getPaddingTop();
        int pR = getPaddingRight();
        int pB = getPaddingBottom();
        int availW = Math.max(0, (right - left) - pL - pR);
        int availH = Math.max(0, (bottom - top) - pT - pB);

        for (int i = 0; i < getChildCount(); i++) {
            View child = getChildAt(i);
            if (child == null || child.getVisibility() == View.GONE) continue;

            ViewGroup.LayoutParams lp = child.getLayoutParams();
            int mL = (lp instanceof MarginLayoutParams) ? ((MarginLayoutParams) lp).leftMargin : 0;
            int mT = (lp instanceof MarginLayoutParams) ? ((MarginLayoutParams) lp).topMargin : 0;
            int mR = (lp instanceof MarginLayoutParams) ? ((MarginLayoutParams) lp).rightMargin : 0;
            int mB = (lp instanceof MarginLayoutParams) ? ((MarginLayoutParams) lp).bottomMargin : 0;

            int cw = child.getMeasuredWidth();
            int ch = child.getMeasuredHeight();
            int cL = left + pL + mL;
            int cT = top + pT + mT;

            int grav = (lp instanceof LayoutParams) ? ((LayoutParams) lp).gravity : -1;
            if (grav != -1) {
                if ((grav & Gravity.RIGHT) == Gravity.RIGHT) {
                    cL = right - pR - mR - cw;
                } else if ((grav & Gravity.CENTER_HORIZONTAL) == Gravity.CENTER_HORIZONTAL) {
                    cL = left + pL + Math.max(0, (availW - cw) / 2);
                }
                if ((grav & Gravity.BOTTOM) == Gravity.BOTTOM) {
                    cT = bottom - pB - mB - ch;
                } else if ((grav & Gravity.CENTER_VERTICAL) == Gravity.CENTER_VERTICAL) {
                    cT = top + pT + Math.max(0, (availH - ch) / 2);
                }
            }
            child.layout(cL, cT, cL + cw, cT + ch);
        }
    }
}
