package android.view;

import android.content.Context;
import android.util.AttributeSet;
import android.widget.FrameLayout;
import android.widget.TextView;
import java.lang.reflect.Constructor;
import java.util.ArrayList;
import java.util.List;

public class ViewGroup extends View implements ViewParent {
    public static class LayoutParams {
        public static final int MATCH_PARENT = -1;
        public static final int WRAP_CONTENT = -2;
        public int width;
        public int height;

        public LayoutParams() { this(WRAP_CONTENT, WRAP_CONTENT); }
        public LayoutParams(int width, int height) {
            this.width = width;
            this.height = height;
        }
        public LayoutParams(Context c, android.util.AttributeSet attrs) { this(WRAP_CONTENT, WRAP_CONTENT); }
        public LayoutParams(LayoutParams source) {
            this(source != null ? source.width : WRAP_CONTENT, source != null ? source.height : WRAP_CONTENT);
        }
    }

    public static class MarginLayoutParams extends LayoutParams {
        public int leftMargin = 0;
        public int topMargin = 0;
        public int rightMargin = 0;
        public int bottomMargin = 0;

        public MarginLayoutParams() { super(WRAP_CONTENT, WRAP_CONTENT); }
        public MarginLayoutParams(int width, int height) {
            super(width, height);
        }
        public MarginLayoutParams(Context c, android.util.AttributeSet attrs) { super(WRAP_CONTENT, WRAP_CONTENT); }
        public MarginLayoutParams(LayoutParams source) {
            super(source != null ? source.width : WRAP_CONTENT, source != null ? source.height : WRAP_CONTENT);
        }
        public MarginLayoutParams(MarginLayoutParams source) {
            super(source != null ? source.width : WRAP_CONTENT, source != null ? source.height : WRAP_CONTENT);
            if (source != null) {
                this.leftMargin = source.leftMargin;
                this.topMargin = source.topMargin;
                this.rightMargin = source.rightMargin;
                this.bottomMargin = source.bottomMargin;
            }
        }

        public void setMargins(int left, int top, int right, int bottom) {
            this.leftMargin = left;
            this.topMargin = top;
            this.rightMargin = right;
            this.bottomMargin = bottom;
        }

        public void setMarginStart(int start) { this.leftMargin = start; }
        public void setMarginEnd(int end) { this.rightMargin = end; }
        public int getMarginStart() { return leftMargin; }
        public int getMarginEnd() { return rightMargin; }
    }

    public boolean checkLayoutParams(ViewGroup.LayoutParams p) {
        return p != null;
    }

    public LayoutParams generateLayoutParams(android.util.AttributeSet attrs) {
        return new LayoutParams(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT);
    }

    protected LayoutParams generateLayoutParams(ViewGroup.LayoutParams p) {
        return p != null ? p : new LayoutParams(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT);
    }

    protected LayoutParams generateDefaultLayoutParams() {
        return new LayoutParams(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT);
    }

    private List<View> children = new ArrayList<>();

    private synchronized List<View> ensureChildren() {
        if (children == null) {
            children = new ArrayList<>();
        }
        return children;
    }

    public ViewGroup() { super(); }
    public ViewGroup(Context context) { super(context); }
    public ViewGroup(Context context, AttributeSet attrs) { super(context, attrs); }
    public ViewGroup(Context context, AttributeSet attrs, int defStyleAttr) { super(context, attrs, defStyleAttr); }

    public void requestDisallowInterceptTouchEvent(boolean disallowIntercept) {}

    private static boolean isAncestor(View potentialAncestor, View view) {
        if (potentialAncestor == null || view == null) return false;
        ViewParent p = view.getParent();
        while (p != null) {
            if (p == potentialAncestor) return true;
            if (p instanceof View) {
                p = ((View) p).getParent();
            } else {
                break;
            }
        }
        return false;
    }

    private void ensureValidChildLayoutParams(View child) {
        if (child == null) return;
        LayoutParams lp = child.getLayoutParams();
        if (lp == null) {
            try { lp = generateDefaultLayoutParams(); } catch (Throwable ignored) {}
        } else if (!checkLayoutParams(lp)) {
            try {
                LayoutParams converted = generateLayoutParams(lp);
                if (converted != null) lp = converted;
            } catch (Throwable ignored) {}
        }
        if (lp != null) {
            child.setLayoutParams(lp);
        }
    }

    public synchronized void addView(View child) {
        if (child != null && child != this && !isAncestor(child, this)) {
            if (child.getParent() instanceof ViewGroup) {
                ((ViewGroup) child.getParent()).removeView(child);
            }
            ensureValidChildLayoutParams(child);
            child.setParent(this);
            List<View> list = ensureChildren();
            if (!list.contains(child)) {
                list.add(child);
            }
        }
    }

    public synchronized void addView(View child, LayoutParams params) {
        if (child != null) {
            child.setLayoutParams(params);
            addView(child);
        }
    }

    public synchronized void addView(View child, int index) {
        if (child != null && child != this && !isAncestor(child, this)) {
            if (child.getParent() instanceof ViewGroup) {
                ((ViewGroup) child.getParent()).removeView(child);
            }
            ensureValidChildLayoutParams(child);
            child.setParent(this);
            List<View> list = ensureChildren();
            if (!list.contains(child)) {
                if (index >= 0 && index <= list.size()) {
                    list.add(index, child);
                } else {
                    list.add(child);
                }
            }
        }
    }

    public synchronized void removeView(View child) {
        if (child != null) {
            ensureChildren().remove(child);
            child.setParent(null);
        }
    }

    public synchronized void removeViewAt(int index) {
        List<View> list = ensureChildren();
        if (index >= 0 && index < list.size()) {
            View child = list.remove(index);
            if (child != null) {
                child.setParent(null);
            }
        }
    }

    public synchronized void removeAllViews() {
        for (View c : ensureChildren()) {
            if (c != null) {
                c.setParent(null);
            }
        }
        ensureChildren().clear();
    }

    public synchronized int getChildCount() {
        return ensureChildren().size();
    }

    public synchronized View getChildAt(int index) {
        List<View> list = ensureChildren();
        if (index >= 0 && index < list.size()) {
            return list.get(index);
        }
        return null;
    }

    @Override
    public synchronized View findViewById(int id) {
        if (getId() == id && id != 0) {
            return this;
        }
        for (View child : ensureChildren()) {
            View found = child.findViewById(id);
            if (found != null) {
                return found;
            }
        }
        if (id == 16908290) { // android.R.id.content
            Context ctx = getContext();
            FrameLayout fl = new FrameLayout(ctx);
            fl.setId(id);
            addView(fl);
            return fl;
        }
        return null;
    }

    protected int getChildMeasureSpec(int spec, int padding, int childDimension) {
        int specMode = View.MeasureSpec.getMode(spec);
        int specSize = View.MeasureSpec.getSize(spec);
        int size = Math.max(0, specSize - padding);

        int resultSize = 0;
        int resultMode = 0;
        switch (specMode) {
            case View.MeasureSpec.EXACTLY:
                if (childDimension >= 0) {
                    resultSize = childDimension;
                    resultMode = View.MeasureSpec.EXACTLY;
                } else if (childDimension == LayoutParams.MATCH_PARENT) {
                    resultSize = size;
                    resultMode = View.MeasureSpec.EXACTLY;
                } else {
                    resultSize = size;
                    resultMode = View.MeasureSpec.AT_MOST;
                }
                break;
            case View.MeasureSpec.AT_MOST:
                if (childDimension >= 0) {
                    resultSize = childDimension;
                    resultMode = View.MeasureSpec.EXACTLY;
                } else {
                    resultSize = size;
                    resultMode = View.MeasureSpec.AT_MOST;
                }
                break;
            case View.MeasureSpec.UNSPECIFIED:
            default:
                if (childDimension >= 0) {
                    resultSize = childDimension;
                    resultMode = View.MeasureSpec.EXACTLY;
                } else {
                    resultSize = 0;
                    resultMode = View.MeasureSpec.UNSPECIFIED;
                }
                break;
        }
        return View.MeasureSpec.makeMeasureSpec(resultSize, resultMode);
    }

    @Override
    protected void onMeasure(int widthMeasureSpec, int heightMeasureSpec) {
        int maxW = 0;
        int maxH = 0;
        int padH = getPaddingLeft() + getPaddingRight();
        int padV = getPaddingTop() + getPaddingBottom();

        for (View child : ensureChildren()) {
            if (child == null || child.getVisibility() == View.GONE) continue;
            LayoutParams lp = child.getLayoutParams();
            if (lp == null) lp = new LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.WRAP_CONTENT);
            int ml = (lp instanceof MarginLayoutParams) ? ((MarginLayoutParams) lp).leftMargin + ((MarginLayoutParams) lp).rightMargin : 0;
            int mt = (lp instanceof MarginLayoutParams) ? ((MarginLayoutParams) lp).topMargin + ((MarginLayoutParams) lp).bottomMargin : 0;

            int cw = getChildMeasureSpec(widthMeasureSpec, padH + ml, lp.width);
            int ch = getChildMeasureSpec(heightMeasureSpec, padV + mt, lp.height);
            child.measure(cw, ch);
            maxW = Math.max(maxW, child.getMeasuredWidth() + ml);
            maxH = Math.max(maxH, child.getMeasuredHeight() + mt);
        }
        if (View.MeasureSpec.getMode(widthMeasureSpec) == View.MeasureSpec.EXACTLY) {
            maxW = Math.max(maxW, View.MeasureSpec.getSize(widthMeasureSpec) - padH);
        }
        if (View.MeasureSpec.getMode(heightMeasureSpec) == View.MeasureSpec.EXACTLY) {
            maxH = Math.max(maxH, View.MeasureSpec.getSize(heightMeasureSpec) - padV);
        }
        setMeasuredDimension(
            View.MeasureSpec.resolveSize(maxW + padH, widthMeasureSpec),
            View.MeasureSpec.resolveSize(maxH + padV, heightMeasureSpec)
        );
    }

    @Override
    protected void onLayout(boolean changed, int left, int top, int right, int bottom) {
        int pL = getPaddingLeft();
        int pT = getPaddingTop();
        for (View child : ensureChildren()) {
            if (child == null || child.getVisibility() == View.GONE) continue;
            LayoutParams lp = child.getLayoutParams();
            int mL = (lp instanceof MarginLayoutParams) ? ((MarginLayoutParams) lp).leftMargin : 0;
            int mT = (lp instanceof MarginLayoutParams) ? ((MarginLayoutParams) lp).topMargin : 0;
            int cL = left + pL + mL;
            int cT = top + pT + mT;
            child.layout(cL, cT, cL + child.getMeasuredWidth(), cT + child.getMeasuredHeight());
        }
    }

    public void bringChildToFront(View child) {
        if (child != null) {
            List<View> list = ensureChildren();
            if (list.remove(child)) {
                list.add(child);
            }
        }
    }

    @Override
    public boolean isLayoutRequested() {
        return false;
    }

    public static final int FOCUS_BEFORE_DESCENDANTS = 0x20000;
    public static final int FOCUS_AFTER_DESCENDANTS = 0x40000;
    public static final int FOCUS_BLOCK_DESCENDANTS = 0x60000;
    private int descendantFocusability = FOCUS_BEFORE_DESCENDANTS;

    public void setDescendantFocusability(int focusability) {
        this.descendantFocusability = focusability;
    }

    public int getDescendantFocusability() {
        return descendantFocusability;
    }

    public void setClipChildren(boolean clipChildren) {}
    public void setClipToPadding(boolean clipToPadding) {}
    public boolean getClipChildren() { return true; }
    public boolean getClipToPadding() { return true; }
}
