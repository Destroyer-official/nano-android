package android.widget;

import android.content.Context;
import android.util.AttributeSet;
import android.view.View;
import android.view.ViewGroup;

public class RelativeLayout extends ViewGroup {
    public static final int TRUE = -1;
    public static final int LEFT_OF = 0;
    public static final int RIGHT_OF = 1;
    public static final int ABOVE = 2;
    public static final int BELOW = 3;
    public static final int ALIGN_BASELINE = 4;
    public static final int ALIGN_LEFT = 5;
    public static final int ALIGN_TOP = 6;
    public static final int ALIGN_RIGHT = 7;
    public static final int ALIGN_BOTTOM = 8;
    public static final int ALIGN_PARENT_LEFT = 9;
    public static final int ALIGN_PARENT_TOP = 10;
    public static final int ALIGN_PARENT_RIGHT = 11;
    public static final int ALIGN_PARENT_BOTTOM = 12;
    public static final int CENTER_IN_PARENT = 13;
    public static final int CENTER_HORIZONTAL = 14;
    public static final int CENTER_VERTICAL = 15;
    public static final int START_OF = 16;
    public static final int END_OF = 17;
    public static final int ALIGN_START = 18;
    public static final int ALIGN_END = 19;
    public static final int ALIGN_PARENT_START = 20;
    public static final int ALIGN_PARENT_END = 21;

    public static class LayoutParams extends ViewGroup.MarginLayoutParams {
        private final int[] rules = new int[22];

        public LayoutParams(Context c, AttributeSet attrs) {
            super(c, attrs);
        }

        public LayoutParams(int w, int h) {
            super(w, h);
        }

        public LayoutParams(ViewGroup.LayoutParams source) {
            super(source);
        }

        public LayoutParams(ViewGroup.MarginLayoutParams source) {
            super(source);
        }

        public void addRule(int verb) {
            addRule(verb, TRUE);
        }

        public void addRule(int verb, int subject) {
            if (verb >= 0 && verb < rules.length) {
                rules[verb] = subject;
            }
        }

        public void removeRule(int verb) {
            if (verb >= 0 && verb < rules.length) {
                rules[verb] = 0;
            }
        }

        public int[] getRules() {
            return rules;
        }
    }

    public RelativeLayout() { super(); }
    public RelativeLayout(Context context) { super(context); }
    public RelativeLayout(Context context, AttributeSet attrs) { super(context, attrs); }
    public RelativeLayout(Context context, AttributeSet attrs, int defStyleAttr) { super(context, attrs, defStyleAttr); }

    private View findChildById(int id) {
        if (id == 0 || id == TRUE) return null;
        for (int i = 0; i < getChildCount(); i++) {
            View c = getChildAt(i);
            if (c != null && c.getId() == id) return c;
        }
        return null;
    }

    private int[] computePositions(int availW, int availH) {
        int count = getChildCount();
        int[] cL = new int[count];
        int[] cT = new int[count];
        int[] cR = new int[count];
        int[] cB = new int[count];
        boolean[] placed = new boolean[count];

        // Multi-pass iterative placement resolves forward & backward anchor dependencies cleanly
        for (int pass = 0; pass < 3; pass++) {
            for (int i = 0; i < count; i++) {
                View child = getChildAt(i);
                if (child == null || child.getVisibility() == View.GONE) continue;

                ViewGroup.LayoutParams rawLp = child.getLayoutParams();
                int mL = (rawLp instanceof MarginLayoutParams) ? ((MarginLayoutParams) rawLp).leftMargin : 0;
                int mT = (rawLp instanceof MarginLayoutParams) ? ((MarginLayoutParams) rawLp).topMargin : 0;
                int mR = (rawLp instanceof MarginLayoutParams) ? ((MarginLayoutParams) rawLp).rightMargin : 0;
                int mB = (rawLp instanceof MarginLayoutParams) ? ((MarginLayoutParams) rawLp).bottomMargin : 0;

                int cw = child.getMeasuredWidth();
                int ch = child.getMeasuredHeight();
                int l = mL;
                int t = mT;
                boolean hasL = false, hasR = false, hasT = false, hasB = false;
                int r = l + cw;
                int b = t + ch;

                if (rawLp instanceof LayoutParams) {
                    int[] rules = ((LayoutParams) rawLp).getRules();
                    if (rules[CENTER_IN_PARENT] == TRUE) {
                        l = Math.max(0, (availW - cw) / 2);
                        t = Math.max(0, (availH - ch) / 2);
                        hasL = true;
                        hasT = true;
                    }
                    if (rules[CENTER_HORIZONTAL] == TRUE) {
                        l = Math.max(0, (availW - cw) / 2);
                        hasL = true;
                    }
                    if (rules[CENTER_VERTICAL] == TRUE) {
                        t = Math.max(0, (availH - ch) / 2);
                        hasT = true;
                    }

                    int rightAnchor = rules[RIGHT_OF] != 0 ? rules[RIGHT_OF] : rules[END_OF];
                    if (rightAnchor > 0) {
                        int idx = indexOfChild(findChildById(rightAnchor));
                        if (idx >= 0 && placed[idx]) {
                            int ancMarginR = getChildMarginR(getChildAt(idx));
                            l = cR[idx] + ancMarginR + mL;
                            hasL = true;
                        }
                    }
                    int leftAnchor = rules[LEFT_OF] != 0 ? rules[LEFT_OF] : rules[START_OF];
                    if (leftAnchor > 0) {
                        int idx = indexOfChild(findChildById(leftAnchor));
                        if (idx >= 0 && placed[idx]) {
                            int ancMarginL = getChildMarginL(getChildAt(idx));
                            r = cL[idx] - ancMarginL - mR;
                            hasR = true;
                        }
                    }

                    if (rules[BELOW] > 0) {
                        int idx = indexOfChild(findChildById(rules[BELOW]));
                        if (idx >= 0 && placed[idx]) {
                            int ancMarginB = getChildMarginB(getChildAt(idx));
                            t = cB[idx] + ancMarginB + mT;
                            hasT = true;
                        }
                    }
                    if (rules[ABOVE] > 0) {
                        int idx = indexOfChild(findChildById(rules[ABOVE]));
                        if (idx >= 0 && placed[idx]) {
                            int ancMarginT = getChildMarginT(getChildAt(idx));
                            b = cT[idx] - ancMarginT - mB;
                            hasB = true;
                        }
                    }

                    int alLeft = rules[ALIGN_LEFT] != 0 ? rules[ALIGN_LEFT] : rules[ALIGN_START];
                    if (alLeft > 0) {
                        int idx = indexOfChild(findChildById(alLeft));
                        if (idx >= 0 && placed[idx]) {
                            l = cL[idx] + mL;
                            hasL = true;
                        }
                    }
                    int alRight = rules[ALIGN_RIGHT] != 0 ? rules[ALIGN_RIGHT] : rules[ALIGN_END];
                    if (alRight > 0) {
                        int idx = indexOfChild(findChildById(alRight));
                        if (idx >= 0 && placed[idx]) {
                            r = cR[idx] - mR;
                            hasR = true;
                        }
                    }
                    if (rules[ALIGN_TOP] > 0) {
                        int idx = indexOfChild(findChildById(rules[ALIGN_TOP]));
                        if (idx >= 0 && placed[idx]) {
                            t = cT[idx] + mT;
                            hasT = true;
                        }
                    }
                    if (rules[ALIGN_BOTTOM] > 0) {
                        int idx = indexOfChild(findChildById(rules[ALIGN_BOTTOM]));
                        if (idx >= 0 && placed[idx]) {
                            b = cB[idx] - mB;
                            hasB = true;
                        }
                    }

                    if (rules[ALIGN_PARENT_LEFT] == TRUE || rules[ALIGN_PARENT_START] == TRUE) {
                        l = mL;
                        hasL = true;
                    }
                    if (rules[ALIGN_PARENT_RIGHT] == TRUE || rules[ALIGN_PARENT_END] == TRUE) {
                        r = availW - mR;
                        hasR = true;
                    }
                    if (rules[ALIGN_PARENT_TOP] == TRUE) {
                        t = mT;
                        hasT = true;
                    }
                    if (rules[ALIGN_PARENT_BOTTOM] == TRUE) {
                        b = availH - mB;
                        hasB = true;
                    }
                }

                if (hasL && hasR) {
                    if (r < l) r = l + cw;
                } else if (hasR) {
                    l = Math.max(0, r - cw);
                } else {
                    r = l + cw;
                }

                if (hasT && hasB) {
                    if (b < t) b = t + ch;
                } else if (hasB) {
                    t = Math.max(0, b - ch);
                } else {
                    b = t + ch;
                }

                cL[i] = l;
                cT[i] = t;
                cR[i] = r;
                cB[i] = b;
                placed[i] = true;
            }
        }

        int[] packed = new int[count * 4];
        for (int i = 0; i < count; i++) {
            packed[i * 4] = cL[i];
            packed[i * 4 + 1] = cT[i];
            packed[i * 4 + 2] = cR[i];
            packed[i * 4 + 3] = cB[i];
        }
        return packed;
    }

    private int indexOfChild(View v) {
        if (v == null) return -1;
        for (int i = 0; i < getChildCount(); i++) {
            if (getChildAt(i) == v) return i;
        }
        return -1;
    }

    private static int getChildMarginL(View v) {
        return (v != null && v.getLayoutParams() instanceof MarginLayoutParams) ? ((MarginLayoutParams) v.getLayoutParams()).leftMargin : 0;
    }
    private static int getChildMarginT(View v) {
        return (v != null && v.getLayoutParams() instanceof MarginLayoutParams) ? ((MarginLayoutParams) v.getLayoutParams()).topMargin : 0;
    }
    private static int getChildMarginR(View v) {
        return (v != null && v.getLayoutParams() instanceof MarginLayoutParams) ? ((MarginLayoutParams) v.getLayoutParams()).rightMargin : 0;
    }
    private static int getChildMarginB(View v) {
        return (v != null && v.getLayoutParams() instanceof MarginLayoutParams) ? ((MarginLayoutParams) v.getLayoutParams()).bottomMargin : 0;
    }

    @Override
    protected void onMeasure(int widthMeasureSpec, int heightMeasureSpec) {
        int padH = getPaddingLeft() + getPaddingRight();
        int padV = getPaddingTop() + getPaddingBottom();
        int wMode = MeasureSpec.getMode(widthMeasureSpec);
        int wSize = MeasureSpec.getSize(widthMeasureSpec);
        int hMode = MeasureSpec.getMode(heightMeasureSpec);
        int hSize = MeasureSpec.getSize(heightMeasureSpec);

        for (int i = 0; i < getChildCount(); i++) {
            View child = getChildAt(i);
            if (child == null || child.getVisibility() == View.GONE) continue;
            ViewGroup.LayoutParams lp = child.getLayoutParams();
            if (lp == null) lp = new LayoutParams(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT);
            int ml = (lp instanceof MarginLayoutParams) ? ((MarginLayoutParams) lp).leftMargin + ((MarginLayoutParams) lp).rightMargin : 0;
            int mt = (lp instanceof MarginLayoutParams) ? ((MarginLayoutParams) lp).topMargin + ((MarginLayoutParams) lp).bottomMargin : 0;
            int cw = getChildMeasureSpec(widthMeasureSpec, padH + ml, lp.width);
            int ch = getChildMeasureSpec(heightMeasureSpec, padV + mt, lp.height);
            child.measure(cw, ch);
        }

        int estW = wMode == MeasureSpec.UNSPECIFIED ? 1080 : Math.max(0, wSize - padH);
        int estH = hMode == MeasureSpec.UNSPECIFIED ? 2264 : Math.max(0, hSize - padV);
        int[] packed = computePositions(estW, estH);

        int maxW = 0, maxH = 0;
        for (int i = 0; i < getChildCount(); i++) {
            View child = getChildAt(i);
            if (child == null || child.getVisibility() == View.GONE) continue;
            maxW = Math.max(maxW, packed[i * 4 + 2] + getChildMarginR(child));
            maxH = Math.max(maxH, packed[i * 4 + 3] + getChildMarginB(child));
        }
        if (wMode == MeasureSpec.EXACTLY) maxW = Math.max(maxW, estW);
        if (hMode == MeasureSpec.EXACTLY) maxH = Math.max(maxH, estH);

        setMeasuredDimension(
            MeasureSpec.resolveSize(maxW + padH, widthMeasureSpec),
            MeasureSpec.resolveSize(maxH + padV, heightMeasureSpec)
        );
    }

    @Override
    protected void onLayout(boolean changed, int left, int top, int right, int bottom) {
        int pL = getPaddingLeft();
        int pT = getPaddingTop();
        int pR = getPaddingRight();
        int pB = getPaddingBottom();
        int availW = Math.max(0, (right - left) - pL - pR);
        int availH = Math.max(0, (bottom - top) - pT - pB);

        int[] packed = computePositions(availW, availH);
        for (int i = 0; i < getChildCount(); i++) {
            View child = getChildAt(i);
            if (child == null || child.getVisibility() == View.GONE) continue;
            int cl = left + pL + packed[i * 4];
            int ct = top + pT + packed[i * 4 + 1];
            int cr = left + pL + packed[i * 4 + 2];
            int cb = top + pT + packed[i * 4 + 3];
            if (cr > cl || cb > ct) {
                if (cr - cl != child.getMeasuredWidth() || cb - ct != child.getMeasuredHeight()) {
                    child.measure(
                        MeasureSpec.makeMeasureSpec(Math.max(0, cr - cl), MeasureSpec.EXACTLY),
                        MeasureSpec.makeMeasureSpec(Math.max(0, cb - ct), MeasureSpec.EXACTLY)
                    );
                }
            }
            child.layout(cl, ct, cr, cb);
        }
    }
}
