package android.view;

import android.content.Context;
import android.content.res.Resources;
import android.graphics.Rect;
import android.graphics.drawable.ColorDrawable;
import android.graphics.drawable.Drawable;
import android.os.Handler;
import android.os.Looper;
import java.util.function.Consumer;

public class View {
    public static final int VISIBLE = 0;
    public static final int INVISIBLE = 4;
    public static final int GONE = 8;

    public static final int SYSTEM_UI_FLAG_VISIBLE = 0;
    public static final int SYSTEM_UI_FLAG_LOW_PROFILE = 1;
    public static final int SYSTEM_UI_FLAG_HIDE_NAVIGATION = 2;
    public static final int SYSTEM_UI_FLAG_FULLSCREEN = 4;
    public static final int SYSTEM_UI_FLAG_LAYOUT_STABLE = 256;
    public static final int SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION = 512;
    public static final int SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN = 1024;
    public static final int SYSTEM_UI_FLAG_IMMERSIVE = 2048;
    public static final int SYSTEM_UI_FLAG_IMMERSIVE_STICKY = 4096;
    public static final int SYSTEM_UI_FLAG_LIGHT_STATUS_BAR = 0x00002000;
    public static final int SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR = 0x00000010;

    private int systemUiVisibility = 0;

    public void setSystemUiVisibility(int visibility) {
        this.systemUiVisibility = visibility;
    }
    public int getSystemUiVisibility() {
        return systemUiVisibility;
    }

    public interface OnClickListener {
        void onClick(View v);
    }

    public interface OnTouchListener {
        boolean onTouch(View v, Object event);
    }

    public interface OnLongClickListener {
        boolean onLongClick(View v);
    }

    public interface OnLayoutChangeListener {
        void onLayoutChange(View v, int left, int top, int right, int bottom,
                            int oldLeft, int oldTop, int oldRight, int oldBottom);
    }

    public interface OnAttachStateChangeListener {
        void onViewAttachedToWindow(View v);
        void onViewDetachedFromWindow(View v);
    }

    public interface OnApplyWindowInsetsListener {
        WindowInsets onApplyWindowInsets(View v, WindowInsets insets);
    }

    public void setOnApplyWindowInsetsListener(OnApplyWindowInsetsListener listener) {}

    private int id = 0;
    private String resourceIdName = "";
    private int visibility = VISIBLE;
    private boolean enabled = true;
    private boolean clickable = false;
    private Object tag;
    private ViewParent parent;
    private Context context;
    private Drawable background;
    private ViewGroup.LayoutParams layoutParams;
    private OnClickListener onClickListener;
    private OnTouchListener onTouchListener;
    private String text = "";
    private Rect bounds = new Rect(0, 0, 0, 0);
    private boolean useSafeLayoutFallback = false;

    public boolean isUseSafeLayoutFallback() { return useSafeLayoutFallback; }
    public void setUseSafeLayoutFallback(boolean v) { this.useSafeLayoutFallback = v; }

    private int paddingLeft = 0;
    private int paddingTop = 0;
    private int paddingRight = 0;
    private int paddingBottom = 0;

    public View() {}
    public View(Context context) { this.context = context; }
    public View(Context context, android.util.AttributeSet attrs) { this.context = context; }
    public View(Context context, android.util.AttributeSet attrs, int defStyleAttr) { this.context = context; }
    public View(Context context, android.util.AttributeSet attrs, int defStyleAttr, int defStyleRes) { this.context = context; }

    public int getId() { return id; }
    public void setId(int id) {
        this.id = id;
    }

    public String getResourceIdName() {
        if (resourceIdName != null && !resourceIdName.isEmpty()) return resourceIdName;
        if (id != 0 && context != null && context.getResources() != null) {
            nanoandroid.host.ApkResourceTable t = context.getResources().getApkTable();
            if (t != null) {
                String n = t.getEntryName(id);
                if (n != null) {
                    this.resourceIdName = n;
                    return n;
                }
            }
        }
        return "";
    }
    public void setResourceIdName(String name) { this.resourceIdName = name != null ? name : ""; }

    public int getVisibility() { return visibility; }
    public void setVisibility(int visibility) { this.visibility = visibility; }

    public boolean isEnabled() { return enabled; }
    public void setEnabled(boolean enabled) { this.enabled = enabled; }

    public boolean isClickable() { return clickable; }
    public void clearAnimation() {}
    public void startAnimation(android.view.animation.Animation animation) {}
    public android.view.animation.Animation getAnimation() { return null; }
    public void setAnimation(android.view.animation.Animation animation) {}

    public void setSaveFromParentEnabled(boolean enabled) {}
    public boolean isSaveFromParentEnabled() { return true; }
    public void setSaveEnabled(boolean enabled) {}
    public boolean isSaveEnabled() { return true; }

    public void setClickable(boolean clickable) { this.clickable = clickable; }

    private boolean longClickable = false;
    private OnLongClickListener onLongClickListener;
    private boolean focusableInTouchMode = false;
    private OnFocusChangeListener onFocusChangeListener;

    public interface OnFocusChangeListener {
        void onFocusChange(View v, boolean hasFocus);
    }

    public boolean isLongClickable() { return longClickable; }
    public void setLongClickable(boolean longClickable) { this.longClickable = longClickable; }

    public void setOnLongClickListener(OnLongClickListener listener) {
        this.onLongClickListener = listener;
        this.longClickable = true;
    }

    public boolean performLongClick() {
        if (!enabled || visibility != VISIBLE || onLongClickListener == null) {
            return false;
        }
        return onLongClickListener.onLongClick(this);
    }

    public boolean isFocusableInTouchMode() { return focusableInTouchMode; }
    public void setFocusableInTouchMode(boolean focusableInTouchMode) { this.focusableInTouchMode = focusableInTouchMode; }

    public void setOnFocusChangeListener(OnFocusChangeListener listener) {
        this.onFocusChangeListener = listener;
    }
    public OnFocusChangeListener getOnFocusChangeListener() { return onFocusChangeListener; }

    public boolean hasFocus() { return isFocused(); }
    public void setWillNotDraw(boolean willNotDraw) {}
    public boolean willNotDraw() { return false; }

    public void setOnClickListener(OnClickListener listener) {
        this.onClickListener = listener;
        this.clickable = true;
    }

    public void setOnTouchListener(OnTouchListener listener) {
        this.onTouchListener = listener;
    }

    public boolean hasOnClickListeners() {
        return onClickListener != null;
    }

    public boolean dispatchTouchEvent(MotionEvent event) {
        if (event == null || !enabled || visibility != VISIBLE) return false;
        if (onTouchListener != null && onTouchListener.onTouch(this, event)) {
            return true;
        }
        return onTouchEvent(event);
    }

    public boolean onTouchEvent(MotionEvent event) {
        if (event == null || !enabled || visibility != VISIBLE) return false;
        if (event.getAction() == MotionEvent.ACTION_UP && clickable && onClickListener != null) {
            performClick();
            return true;
        }
        return false;
    }

    public boolean performClick() {
        if (!enabled || visibility != VISIBLE || onClickListener == null) {
            return false;
        }
        onClickListener.onClick(this);
        return true;
    }

    private final java.util.Map<Integer, Object> keyedTags = new java.util.HashMap<>();

    public boolean isLaidOut() { return true; }
    public boolean isAttachedToWindow() { return true; }
    public boolean isActivated() { return false; }
    public void setActivated(boolean activated) {}
    public boolean isFocused() { return false; }
    public View findFocus() { return null; }
    public boolean requestFocus() { return true; }
    public void clearFocus() {}
    public void postInvalidate() {}
    public void requestApplyInsets() {}

    public Object getTag() { return tag; }
    public void setTag(Object tag) { this.tag = tag; }
    public void setTag(int key, Object tag) { keyedTags.put(key, tag); }
    public Object getTag(int key) { return keyedTags.get(key); }

    public ViewParent getParent() { return parent; }
    public void setParent(ViewParent parent) { this.parent = parent; }

    public Context getContext() { return context; }
    public void setContext(Context context) { this.context = context; }
    public Resources getResources() { return context != null ? context.getResources() : new Resources(); }

    public void saveAttributeDataForStyleable(Context context, int[] styleable, android.util.AttributeSet set, android.content.res.TypedArray attrs, int defStyleAttr, int defStyleRes) {}

    public void setBackground(Drawable background) { this.background = background; }
    public void setBackgroundDrawable(Drawable background) { this.background = background; }
    public void setBackgroundColor(int color) { this.background = new ColorDrawable(color); }
    public Drawable getBackground() {
        return background;
    }

    private android.content.res.ColorStateList backgroundTintList;
    private android.graphics.PorterDuff.Mode backgroundTintMode;
    private float translationZ;
    private String transitionName;

    public void setBackgroundTintList(android.content.res.ColorStateList tint) { this.backgroundTintList = tint; }
    public android.content.res.ColorStateList getBackgroundTintList() { return backgroundTintList; }
    public void setBackgroundTintMode(android.graphics.PorterDuff.Mode tintMode) { this.backgroundTintMode = tintMode; }
    public android.graphics.PorterDuff.Mode getBackgroundTintMode() { return backgroundTintMode; }

    public void setTranslationZ(float translationZ) { this.translationZ = translationZ; }
    public float getTranslationZ() { return translationZ; }
    public void setTransitionName(String transitionName) { this.transitionName = transitionName; }
    public String getTransitionName() { return transitionName; }
    public void setOutlineProvider(Object provider) {}

    private CharSequence contentDescription = "";
    private CharSequence tooltipText = "";

    public void setContentDescription(CharSequence contentDescription) { this.contentDescription = contentDescription; }
    public CharSequence getContentDescription() { return contentDescription; }
    public void setTooltipText(CharSequence tooltipText) { this.tooltipText = tooltipText; }
    public CharSequence getTooltipText() { return tooltipText; }

    private Drawable foreground;
    public void setForeground(Drawable foreground) { this.foreground = foreground; }
    public Drawable getForeground() { return foreground; }

    public void setPadding(int left, int top, int right, int bottom) {
        this.paddingLeft = left;
        this.paddingTop = top;
        this.paddingRight = right;
        this.paddingBottom = bottom;
    }

    private float elevation = 0;
    private float alpha = 1.0f;
    private float translationX = 0;
    private float translationY = 0;
    private float scaleX = 1.0f;
    private float scaleY = 1.0f;
    private float rotation = 0;
    private boolean selected = false;
    private boolean focusable = false;

    public android.window.OnBackInvokedDispatcher findOnBackInvokedDispatcher() {
        return null;
    }

    public ViewPropertyAnimator animate() {
        return new ViewPropertyAnimator(this);
    }

    public void setElevation(float elevation) { this.elevation = elevation; }
    public float getElevation() { return elevation; }

    public void setAlpha(float alpha) { this.alpha = alpha; }
    public float getAlpha() { return alpha; }

    public void setTranslationX(float translationX) { this.translationX = translationX; }
    public float getTranslationX() { return translationX; }

    public void setTranslationY(float translationY) { this.translationY = translationY; }
    public float getTranslationY() { return translationY; }

    public void setScaleX(float scaleX) { this.scaleX = scaleX; }
    public float getScaleX() { return scaleX; }

    public void setScaleY(float scaleY) { this.scaleY = scaleY; }
    public float getScaleY() { return scaleY; }

    public void setRotation(float rotation) { this.rotation = rotation; }
    public float getRotation() { return rotation; }

    public void setSelected(boolean selected) { this.selected = selected; }
    public boolean isSelected() { return selected; }

    public static class AccessibilityDelegate {
        public AccessibilityDelegate() {}
        public void sendAccessibilityEvent(View host, int eventType) {}
        public boolean performAccessibilityAction(View host, int action, android.os.Bundle args) { return false; }
    }
    private AccessibilityDelegate accessibilityDelegate;
    public void setAccessibilityDelegate(AccessibilityDelegate delegate) { this.accessibilityDelegate = delegate; }
    public AccessibilityDelegate getAccessibilityDelegate() { return accessibilityDelegate; }

    public static final int IMPORTANT_FOR_ACCESSIBILITY_AUTO = 0;
    public static final int IMPORTANT_FOR_ACCESSIBILITY_YES = 1;
    public static final int IMPORTANT_FOR_ACCESSIBILITY_NO = 2;
    public static final int IMPORTANT_FOR_ACCESSIBILITY_NO_HIDE_DESCENDANTS = 4;
    private int importantForAccessibility = IMPORTANT_FOR_ACCESSIBILITY_AUTO;

    public int getImportantForAccessibility() { return importantForAccessibility; }
    public void setImportantForAccessibility(int mode) { this.importantForAccessibility = mode; }

    public void setFocusable(boolean focusable) { this.focusable = focusable; }
    public boolean isFocusable() { return focusable; }

    public void setHorizontalScrollBarEnabled(boolean enabled) {}
    public void setVerticalScrollBarEnabled(boolean enabled) {}
    public boolean isHorizontalScrollBarEnabled() { return false; }
    public boolean isVerticalScrollBarEnabled() { return false; }
    public void setOverScrollMode(int mode) {}
    public int getOverScrollMode() { return 0; }
    public void setClipToOutline(boolean clip) {}

    public void addOnLayoutChangeListener(OnLayoutChangeListener listener) {}
    public void removeOnLayoutChangeListener(OnLayoutChangeListener listener) {}
    public void addOnAttachStateChangeListener(OnAttachStateChangeListener listener) {}
    public void removeOnAttachStateChangeListener(OnAttachStateChangeListener listener) {}

    public void bringToFront() {
        if (parent != null) {
            parent.bringChildToFront(this);
        }
    }

    public boolean performHapticFeedback(int feedbackConstant) { return true; }
    public boolean performHapticFeedback(int feedbackConstant, int flags) { return true; }

    private Rect clipBounds;
    public void setClipBounds(Rect clipBounds) { this.clipBounds = clipBounds; }
    public Rect getClipBounds() { return clipBounds; }

    public void invalidate() {}
    public void requestLayout() {}

    public int getPaddingLeft() { return paddingLeft; }
    public int getPaddingTop() { return paddingTop; }
    public int getPaddingRight() { return paddingRight; }
    public int getPaddingBottom() { return paddingBottom; }

    public void setLayoutParams(ViewGroup.LayoutParams params) { this.layoutParams = params; }
    public ViewGroup.LayoutParams getLayoutParams() { return layoutParams; }

    public Rect getBounds() {
        if (bounds == null) bounds = new Rect(0, 0, 1080, 1920);
        return bounds;
    }
    public void setBounds(int left, int top, int right, int bottom) {
        this.bounds = new Rect(left, top, right, bottom);
    }
    public int getTop() { return getBounds().top; }
    public int getLeft() { return getBounds().left; }
    public int getRight() { return getBounds().right; }
    public int getBottom() { return getBounds().bottom; }
    public int getWidth() { return getBounds().width(); }
    public int getHeight() { return getBounds().height(); }

    public void offsetLeftAndRight(int offset) {
        Rect b = getBounds();
        this.bounds = new Rect(b.left + offset, b.top, b.right + offset, b.bottom);
    }

    public void offsetTopAndBottom(int offset) {
        Rect b = getBounds();
        this.bounds = new Rect(b.left, b.top + offset, b.right, b.bottom + offset);
    }

    public CharSequence getText() { return text; }
    public void setText(CharSequence text) { this.text = text != null ? text.toString() : ""; }

    public View findViewById(int id) {
        return this.id == id ? this : null;
    }

    public void postOnAnimation(Runnable action) { post(action); }
    public void postOnAnimationDelayed(Runnable action, long delayMillis) { postDelayed(action, delayMillis); }
    public boolean removeCallbacks(Runnable action) { return true; }

    public boolean post(Runnable r) {
        Handler h = new Handler(Looper.getMainLooper());
        return h.post(r);
    }

    public boolean postDelayed(Runnable r, long delayMillis) {
        Handler h = new Handler(Looper.getMainLooper());
        return h.postDelayed(r, delayMillis);
    }

    public static class MeasureSpec {
        private static final int MODE_SHIFT = 30;
        private static final int MODE_MASK = 0x3 << MODE_SHIFT;
        public static final int UNSPECIFIED = 0 << MODE_SHIFT;
        public static final int EXACTLY = 1 << MODE_SHIFT;
        public static final int AT_MOST = 2 << MODE_SHIFT;

        public static int makeMeasureSpec(int size, int mode) {
            return (size & ~MODE_MASK) | (mode & MODE_MASK);
        }

        public static int getMode(int measureSpec) {
            return (measureSpec & MODE_MASK);
        }

        public static int getSize(int measureSpec) {
            return (measureSpec & ~MODE_MASK);
        }

        public static int resolveSizeAndState(int size, int measureSpec, int childMeasuredState) {
            final int specMode = getMode(measureSpec);
            final int specSize = getSize(measureSpec);
            int result;
            switch (specMode) {
                case AT_MOST:
                    if (specSize < size) {
                        result = specSize | 0x01000000;
                    } else {
                        result = size;
                    }
                    break;
                case EXACTLY:
                    result = specSize;
                    break;
                case UNSPECIFIED:
                default:
                    result = size;
            }
            return result | (childMeasuredState & ~0xff000000);
        }

        public static int resolveSize(int size, int measureSpec) {
            return resolveSizeAndState(size, measureSpec, 0) & 0x00ffffff;
        }

        public static int combineMeasuredStates(int curState, int newState) {
            return curState | newState;
        }
    }

    protected int measuredWidth = 0;
    protected int measuredHeight = 0;

    public void measure(int widthMeasureSpec, int heightMeasureSpec) {
        if (useSafeLayoutFallback) {
            fallbackMeasure(widthMeasureSpec, heightMeasureSpec);
            return;
        }
        try {
            onMeasure(widthMeasureSpec, heightMeasureSpec);
        } catch (Throwable ignored) {
            useSafeLayoutFallback = true;
            fallbackMeasure(widthMeasureSpec, heightMeasureSpec);
        }
        String cn = getClass().getSimpleName();
        boolean isRootWrapper = cn.contains("Coordinator") || cn.contains("Constraint") || cn.contains("FitWindows") || cn.contains("ContentFrame") || cn.contains("SwipeRefresh");
        if (this instanceof ViewGroup && ((ViewGroup) this).getChildCount() > 0 &&
                ((measuredWidth <= 0 && measuredHeight <= 0) || (isRootWrapper && measuredWidth < 100 && MeasureSpec.getSize(widthMeasureSpec) >= 500))) {
            useSafeLayoutFallback = true;
            fallbackMeasure(widthMeasureSpec, heightMeasureSpec);
        }
    }

    private void fallbackMeasure(int widthMeasureSpec, int heightMeasureSpec) {
        if (this instanceof ViewGroup) {
            ViewGroup vg = (ViewGroup) this;
            String selfCn = getClass().getSimpleName().toLowerCase();
            boolean isStacked = (this instanceof android.widget.LinearLayout) || selfCn.contains("recycler") || selfCn.contains("fitwindows") || selfCn.contains("linear") || selfCn.contains("menu");
            int maxW = 0, totalH = 0;
            int padH = paddingLeft + paddingRight, padV = paddingTop + paddingBottom;
            int wMode = MeasureSpec.getMode(widthMeasureSpec);
            int wSize = MeasureSpec.getSize(widthMeasureSpec);
            int hMode = MeasureSpec.getMode(heightMeasureSpec);
            int hSize = MeasureSpec.getSize(heightMeasureSpec);
            for (int i = 0; i < vg.getChildCount(); i++) {
                View c = vg.getChildAt(i);
                if (c == null || c.getVisibility() == GONE) continue;
                int clpW = (c.getLayoutParams() != null) ? c.getLayoutParams().width : ViewGroup.LayoutParams.MATCH_PARENT;
                int clpH = (c.getLayoutParams() != null) ? c.getLayoutParams().height : ViewGroup.LayoutParams.WRAP_CONTENT;
                String cName = c.getClass().getSimpleName();
                if (cName.contains("Coordinator") || cName.contains("Constraint") || cName.contains("FitWindows") || cName.contains("ContentFrame") || cName.contains("Drawer")) {
                    clpW = ViewGroup.LayoutParams.MATCH_PARENT;
                    clpH = ViewGroup.LayoutParams.MATCH_PARENT;
                }
                int availH = isStacked ? Math.max(0, hSize - padV - totalH) : Math.max(0, hSize - padV);
                int cw = (wSize > 0 && clpW == ViewGroup.LayoutParams.MATCH_PARENT)
                        ? MeasureSpec.makeMeasureSpec(Math.max(0, wSize - padH), MeasureSpec.EXACTLY)
                        : (clpW >= 0 ? MeasureSpec.makeMeasureSpec(clpW, MeasureSpec.EXACTLY)
                        : MeasureSpec.makeMeasureSpec(Math.max(0, wSize - padH), MeasureSpec.AT_MOST));
                int ch = (hSize > 0 && clpH == ViewGroup.LayoutParams.MATCH_PARENT)
                        ? MeasureSpec.makeMeasureSpec(availH, MeasureSpec.EXACTLY)
                        : (clpH >= 0 ? MeasureSpec.makeMeasureSpec(clpH, MeasureSpec.EXACTLY)
                        : MeasureSpec.makeMeasureSpec(availH, MeasureSpec.AT_MOST));
                c.measure(cw, ch);
                maxW = Math.max(maxW, c.getMeasuredWidth());
                if (isStacked) {
                    totalH += c.getMeasuredHeight();
                } else {
                    totalH = Math.max(totalH, c.getMeasuredHeight());
                }
            }
            if (wMode == MeasureSpec.EXACTLY || wSize >= 500) {
                maxW = Math.max(maxW, wSize - padH);
            }
            if (hMode == MeasureSpec.EXACTLY) {
                totalH = Math.max(totalH, hSize - padV);
            }
            setMeasuredDimension(
                MeasureSpec.resolveSize(maxW + padH, widthMeasureSpec),
                MeasureSpec.resolveSize(totalH + padV, heightMeasureSpec)
            );
        } else {
            setMeasuredDimension(
                MeasureSpec.resolveSize(Math.max(48, paddingLeft + paddingRight), widthMeasureSpec),
                MeasureSpec.resolveSize(Math.max(48, paddingTop + paddingBottom), heightMeasureSpec)
            );
        }
    }

    protected void onMeasure(int widthMeasureSpec, int heightMeasureSpec) {
        setMeasuredDimension(getDefaultSize(getSuggestedMinimumWidth(), widthMeasureSpec),
                             getDefaultSize(getSuggestedMinimumHeight(), heightMeasureSpec));
    }

    protected final void setMeasuredDimension(int measuredWidth, int measuredHeight) {
        this.measuredWidth = measuredWidth;
        this.measuredHeight = measuredHeight;
    }

    public static int getDefaultSize(int size, int measureSpec) {
        int result = size;
        int specMode = MeasureSpec.getMode(measureSpec);
        int specSize = MeasureSpec.getSize(measureSpec);
        switch (specMode) {
            case MeasureSpec.UNSPECIFIED:
                result = size;
                break;
            case MeasureSpec.AT_MOST:
            case MeasureSpec.EXACTLY:
                result = specSize;
                break;
        }
        return result;
    }

    protected int getSuggestedMinimumWidth() { return 0; }
    protected int getSuggestedMinimumHeight() { return 0; }
    public final int getMeasuredWidth() { return measuredWidth; }
    public final int getMeasuredHeight() { return measuredHeight; }
    public final int getMeasuredWidthAndState() { return measuredWidth; }
    public final int getMeasuredHeightAndState() { return measuredHeight; }

    public void layout(int l, int t, int r, int b) {
        setBounds(l, t, r, b);
        if (useSafeLayoutFallback) {
            fallbackLayout(l, t, r, b);
            return;
        }
        try {
            onLayout(true, l, t, r, b);
        } catch (Throwable ignored) {
            useSafeLayoutFallback = true;
            fallbackLayout(l, t, r, b);
        }
    }

    private void fallbackLayout(int l, int t, int r, int b) {
        if (this instanceof ViewGroup) {
            ViewGroup vg = (ViewGroup) this;
            String selfCn = getClass().getSimpleName().toLowerCase();
            boolean isStacked = (this instanceof android.widget.LinearLayout) || selfCn.contains("recycler") || selfCn.contains("fitwindows") || selfCn.contains("linear") || selfCn.contains("menu");
            int curY = t + paddingTop;
            for (int i = 0; i < vg.getChildCount(); i++) {
                View c = vg.getChildAt(i);
                if (c == null || c.getVisibility() == GONE) continue;
                int cw = c.getMeasuredWidth();
                int ch = c.getMeasuredHeight();
                int cL = l + paddingLeft;
                int cT = isStacked ? curY : (t + paddingTop);
                String cName = c.getClass().getSimpleName().toLowerCase();
                if (!isStacked && cName.contains("floatingactionbutton")) {
                    cL = Math.max(l + paddingLeft, r - paddingRight - cw - 42);
                    cT = Math.max(t + paddingTop, b - paddingBottom - ch - 42);
                }
                c.layout(cL, cT, cL + cw, cT + ch);
                if (isStacked) {
                    curY = Math.max(curY, cT + ch);
                }
            }
        }
    }

    protected void onLayout(boolean changed, int left, int top, int right, int bottom) {}
}
