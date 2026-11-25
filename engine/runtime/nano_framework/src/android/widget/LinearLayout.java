package android.widget;

import android.content.Context;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;

public class LinearLayout extends ViewGroup {
    public static final int HORIZONTAL = 0;
    public static final int VERTICAL = 1;

    public static class LayoutParams extends ViewGroup.MarginLayoutParams {
        public float weight = 0.0f;
        public int gravity = -1;

        public LayoutParams(Context c, android.util.AttributeSet attrs) {
            super(c, attrs);
        }

        public LayoutParams(int width, int height) {
            super(width, height);
        }

        public LayoutParams(int width, int height, float weight) {
            super(width, height);
            this.weight = weight;
        }

        public LayoutParams(ViewGroup.LayoutParams p) {
            super(p);
        }

        public LayoutParams(ViewGroup.MarginLayoutParams source) {
            super(source);
        }
    }

    @Override
    protected LayoutParams generateDefaultLayoutParams() {
        return new LayoutParams(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT);
    }

    @Override
    public LayoutParams generateLayoutParams(android.util.AttributeSet attrs) {
        return new LayoutParams(getContext(), attrs);
    }

    @Override
    protected LayoutParams generateLayoutParams(ViewGroup.LayoutParams lp) {
        if (lp instanceof LayoutParams) return (LayoutParams) lp;
        if (lp instanceof ViewGroup.MarginLayoutParams) return new LayoutParams((ViewGroup.MarginLayoutParams) lp);
        return new LayoutParams(lp != null ? lp : generateDefaultLayoutParams());
    }

    private int orientation = HORIZONTAL;
    private float weightSum = -1.0f;
    private int gravity = Gravity.TOP | Gravity.LEFT;

    public LinearLayout() { super(); }
    public LinearLayout(Context context) { super(context); }
    public LinearLayout(Context context, android.util.AttributeSet attrs) { super(context, attrs); }
    public LinearLayout(Context context, android.util.AttributeSet attrs, int defStyleAttr) { super(context, attrs, defStyleAttr); }

    public int getOrientation() { return orientation; }
    public void setOrientation(int orientation) { this.orientation = orientation; }

    public float getWeightSum() { return weightSum; }
    public void setWeightSum(float weightSum) { this.weightSum = weightSum; }

    public void setGravity(int gravity) { this.gravity = gravity; }
    public int getGravity() { return gravity; }

    private static float getWeight(View v) {
        ViewGroup.LayoutParams lp = v.getLayoutParams();
        return (lp instanceof LayoutParams) ? ((LayoutParams) lp).weight : 0f;
    }

    private static int getLpMarginH(ViewGroup.LayoutParams lp) {
        return (lp instanceof ViewGroup.MarginLayoutParams)
                ? ((ViewGroup.MarginLayoutParams) lp).leftMargin + ((ViewGroup.MarginLayoutParams) lp).rightMargin : 0;
    }

    private static int getLpMarginV(ViewGroup.LayoutParams lp) {
        return (lp instanceof ViewGroup.MarginLayoutParams)
                ? ((ViewGroup.MarginLayoutParams) lp).topMargin + ((ViewGroup.MarginLayoutParams) lp).bottomMargin : 0;
    }

    @Override
    protected void onMeasure(int widthMeasureSpec, int heightMeasureSpec) {
        if (orientation == VERTICAL) {
            measureVertical(widthMeasureSpec, heightMeasureSpec);
        } else {
            measureHorizontal(widthMeasureSpec, heightMeasureSpec);
        }
    }

    private void measureVertical(int widthMeasureSpec, int heightMeasureSpec) {
        int padH = getPaddingLeft() + getPaddingRight();
        int padV = getPaddingTop() + getPaddingBottom();
        int hMode = MeasureSpec.getMode(heightMeasureSpec);
        int hSize = MeasureSpec.getSize(heightMeasureSpec);

        int totalH = 0;
        int maxW = 0;
        float totalWeight = 0f;
        int count = getChildCount();

        for (int i = 0; i < count; i++) {
            View child = getChildAt(i);
            if (child == null || child.getVisibility() == View.GONE) continue;
            float w = getWeight(child);
            totalWeight += w;
            ViewGroup.LayoutParams lp = child.getLayoutParams();
            if (lp == null) lp = new LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
            int mH = getLpMarginH(lp);
            int mV = getLpMarginV(lp);

            if (w > 0 && (lp.height == 0 || lp.height == ViewGroup.LayoutParams.MATCH_PARENT) && hMode != MeasureSpec.UNSPECIFIED) {
                continue;
            }
            int cw = getChildMeasureSpec(widthMeasureSpec, padH + mH, lp.width);
            int ch;
            if (hMode == MeasureSpec.UNSPECIFIED) {
                ch = getChildMeasureSpec(heightMeasureSpec, padV + mV, lp.height);
            } else {
                int avail = Math.max(0, hSize - padV - totalH);
                int cSpec = MeasureSpec.makeMeasureSpec(avail, MeasureSpec.AT_MOST);
                ch = getChildMeasureSpec(cSpec, mV, lp.height);
            }
            child.measure(cw, ch);
            maxW = Math.max(maxW, child.getMeasuredWidth() + mH);
            totalH += child.getMeasuredHeight() + mV;
        }

        if (totalWeight > 0 && hMode != MeasureSpec.UNSPECIFIED) {
            int remaining = Math.max(0, hSize - padV - totalH);
            float denom = weightSum > 0 ? weightSum : totalWeight;
            for (int i = 0; i < count; i++) {
                View child = getChildAt(i);
                if (child == null || child.getVisibility() == View.GONE) continue;
                float w = getWeight(child);
                if (w <= 0) continue;
                ViewGroup.LayoutParams lp = child.getLayoutParams();
                int mH = getLpMarginH(lp);
                int mV = getLpMarginV(lp);
                int allocH = Math.max(0, Math.round(remaining * (w / denom)) - mV);
                int cw = getChildMeasureSpec(widthMeasureSpec, padH + mH, lp != null ? lp.width : LayoutParams.MATCH_PARENT);
                int ch = MeasureSpec.makeMeasureSpec(allocH, MeasureSpec.EXACTLY);
                child.measure(cw, ch);
                maxW = Math.max(maxW, child.getMeasuredWidth() + mH);
                totalH += child.getMeasuredHeight() + mV;
            }
        }

        setMeasuredDimension(
            MeasureSpec.resolveSize(maxW + padH, widthMeasureSpec),
            MeasureSpec.resolveSize(totalH + padV, heightMeasureSpec)
        );
    }

    private void measureHorizontal(int widthMeasureSpec, int heightMeasureSpec) {
        int padH = getPaddingLeft() + getPaddingRight();
        int padV = getPaddingTop() + getPaddingBottom();
        int wMode = MeasureSpec.getMode(widthMeasureSpec);
        int wSize = MeasureSpec.getSize(widthMeasureSpec);

        int totalW = 0;
        int maxH = 0;
        float totalWeight = 0f;
        int count = getChildCount();

        for (int i = 0; i < count; i++) {
            View child = getChildAt(i);
            if (child == null || child.getVisibility() == View.GONE) continue;
            float w = getWeight(child);
            totalWeight += w;
            ViewGroup.LayoutParams lp = child.getLayoutParams();
            if (lp == null) lp = new LayoutParams(ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
            int mH = getLpMarginH(lp);
            int mV = getLpMarginV(lp);

            if (w > 0 && (lp.width == 0 || lp.width == ViewGroup.LayoutParams.MATCH_PARENT) && wMode != MeasureSpec.UNSPECIFIED) {
                continue;
            }
            int cw;
            if (wMode == MeasureSpec.UNSPECIFIED) {
                cw = getChildMeasureSpec(widthMeasureSpec, padH + mH, lp.width);
            } else {
                int avail = Math.max(0, wSize - padH - totalW);
                int cSpec = MeasureSpec.makeMeasureSpec(avail, MeasureSpec.AT_MOST);
                cw = getChildMeasureSpec(cSpec, mH, lp.width);
            }
            int ch = getChildMeasureSpec(heightMeasureSpec, padV + mV, lp.height);
            child.measure(cw, ch);
            totalW += child.getMeasuredWidth() + mH;
            maxH = Math.max(maxH, child.getMeasuredHeight() + mV);
        }

        if (totalWeight > 0 && wMode != MeasureSpec.UNSPECIFIED) {
            int remaining = Math.max(0, wSize - padH - totalW);
            float denom = weightSum > 0 ? weightSum : totalWeight;
            for (int i = 0; i < count; i++) {
                View child = getChildAt(i);
                if (child == null || child.getVisibility() == View.GONE) continue;
                float w = getWeight(child);
                if (w <= 0) continue;
                ViewGroup.LayoutParams lp = child.getLayoutParams();
                int mH = getLpMarginH(lp);
                int mV = getLpMarginV(lp);
                int allocW = Math.max(0, Math.round(remaining * (w / denom)) - mH);
                int cw = MeasureSpec.makeMeasureSpec(allocW, MeasureSpec.EXACTLY);
                int ch = getChildMeasureSpec(heightMeasureSpec, padV + mV, lp != null ? lp.height : LayoutParams.WRAP_CONTENT);
                child.measure(cw, ch);
                totalW += child.getMeasuredWidth() + mH;
                maxH = Math.max(maxH, child.getMeasuredHeight() + mV);
            }
        }

        // Re-stretch MATCH_PARENT height children to match maxH
        int finalH = MeasureSpec.resolveSize(maxH + padV, heightMeasureSpec);
        for (int i = 0; i < count; i++) {
            View child = getChildAt(i);
            if (child == null || child.getVisibility() == View.GONE) continue;
            ViewGroup.LayoutParams lp = child.getLayoutParams();
            if (lp != null && lp.height == ViewGroup.LayoutParams.MATCH_PARENT) {
                int mV = getLpMarginV(lp);
                int targetH = Math.max(0, finalH - padV - mV);
                child.measure(
                    MeasureSpec.makeMeasureSpec(child.getMeasuredWidth(), MeasureSpec.EXACTLY),
                    MeasureSpec.makeMeasureSpec(targetH, MeasureSpec.EXACTLY)
                );
            }
        }

        setMeasuredDimension(
            MeasureSpec.resolveSize(totalW + padH, widthMeasureSpec),
            finalH
        );
    }

    @Override
    protected void onLayout(boolean changed, int left, int top, int right, int bottom) {
        int pL = getPaddingLeft();
        int pT = getPaddingTop();
        int pR = getPaddingRight();
        int pB = getPaddingBottom();
        int contentW = Math.max(0, (right - left) - pL - pR);
        int contentH = Math.max(0, (bottom - top) - pT - pB);
        int count = getChildCount();

        if (orientation == VERTICAL) {
            int curY = top + pT;
            for (int i = 0; i < count; i++) {
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
                int childGrav = (lp instanceof LayoutParams) ? ((LayoutParams) lp).gravity : -1;
                if (childGrav != -1) {
                    if ((childGrav & Gravity.CENTER_HORIZONTAL) == Gravity.CENTER_HORIZONTAL) {
                        cL = left + pL + Math.max(0, (contentW - cw) / 2);
                    } else if ((childGrav & Gravity.RIGHT) == Gravity.RIGHT) {
                        cL = right - pR - mR - cw;
                    }
                }
                int cT = curY + mT;
                child.layout(cL, cT, cL + cw, cT + ch);
                curY = cT + ch + mB;
            }
        } else {
            int curX = left + pL;
            for (int i = 0; i < count; i++) {
                View child = getChildAt(i);
                if (child == null || child.getVisibility() == View.GONE) continue;
                ViewGroup.LayoutParams lp = child.getLayoutParams();
                int mL = (lp instanceof MarginLayoutParams) ? ((MarginLayoutParams) lp).leftMargin : 0;
                int mT = (lp instanceof MarginLayoutParams) ? ((MarginLayoutParams) lp).topMargin : 0;
                int mR = (lp instanceof MarginLayoutParams) ? ((MarginLayoutParams) lp).rightMargin : 0;
                int mB = (lp instanceof MarginLayoutParams) ? ((MarginLayoutParams) lp).bottomMargin : 0;

                int cw = child.getMeasuredWidth();
                int ch = child.getMeasuredHeight();
                int cL = curX + mL;
                int cT = top + pT + mT;
                int effGrav = (lp instanceof LayoutParams && ((LayoutParams) lp).gravity != -1) ? ((LayoutParams) lp).gravity : gravity;
                if ((effGrav & Gravity.CENTER_VERTICAL) == Gravity.CENTER_VERTICAL) {
                    cT = top + pT + Math.max(0, (contentH - ch) / 2);
                } else if ((effGrav & Gravity.BOTTOM) == Gravity.BOTTOM) {
                    cT = bottom - pB - mB - ch;
                }
                child.layout(cL, cT, cL + cw, cT + ch);
                curX = cL + cw + mR;
            }
        }
    }
}
