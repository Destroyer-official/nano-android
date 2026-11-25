package android.view;

import android.content.Context;
import android.graphics.drawable.Drawable;
import android.util.AttributeSet;
import android.widget.FrameLayout;
import java.lang.reflect.Constructor;

public class Window {
    public static final int FEATURE_NO_TITLE = 1;
    public static final int FEATURE_ACTION_BAR = 8;
    public static final int FEATURE_ACTION_BAR_OVERLAY = 9;

    public interface Callback {
        boolean dispatchKeyEvent(android.view.KeyEvent event);
        boolean dispatchTouchEvent(android.view.MotionEvent event);
        View onCreatePanelView(int featureId);
        boolean onCreatePanelMenu(int featureId, Menu menu);
        boolean onPreparePanel(int featureId, View view, Menu menu);
        boolean onMenuOpened(int featureId, Menu menu);
        boolean onMenuItemSelected(int featureId, MenuItem item);
        void onWindowAttributesChanged(WindowManager.LayoutParams attrs);
        void onContentChanged();
        void onWindowFocusChanged(boolean hasFocus);
        void onAttachedToWindow();
        void onDetachedFromWindow();
    }

    private View decorView;
    private int statusBarColor;
    private int navigationBarColor;
    private Callback callback;
    private final WindowManager.LayoutParams attributes = new WindowManager.LayoutParams();

    public void setStatusBarColor(int color) { this.statusBarColor = color; }
    public int getStatusBarColor() { return statusBarColor; }
    public void setNavigationBarColor(int color) { this.navigationBarColor = color; }
    public int getNavigationBarColor() { return navigationBarColor; }

    public void setBackgroundDrawable(Drawable drawable) {}

    public void setDecorView(View view) { this.decorView = view; }
    public View getDecorView() { return decorView; }
    public View peekDecorView() { return decorView; }

    public void setContentView(View view) {
        if (decorView instanceof ViewGroup) {
            View content = findViewById(16908290); // R.id.content
            if (content instanceof ViewGroup) {
                ((ViewGroup) content).addView(view);
                return;
            }
            ((ViewGroup) decorView).addView(view);
        } else {
            setDecorView(view);
        }
    }

    public void setContentView(int layoutResId) {}
    public void setContentView(View view, ViewGroup.LayoutParams params) {
        setContentView(view);
    }

    public View findViewById(int id) {
        if (decorView != null) {
            View v = decorView.findViewById(id);
            if (v != null) return v;
        }

        Context ctx = (decorView != null) ? decorView.getContext() : null;
        try {
            ClassLoader cl = (ctx != null) ? ctx.getClass().getClassLoader() : getClass().getClassLoader();
            Class<?> clazz = cl.loadClass("androidx.appcompat.widget.ContentFrameLayout");
            Constructor<?> ctor = clazz.getConstructor(Context.class, AttributeSet.class);
            View cfl = (View) ctor.newInstance(ctx, null);
            cfl.setId(id);
            if (decorView instanceof ViewGroup) {
                ((ViewGroup) decorView).addView(cfl);
            }
            return cfl;
        } catch (Throwable t) {
            FrameLayout fl = new FrameLayout(ctx);
            fl.setId(id);
            if (decorView instanceof ViewGroup) {
                ((ViewGroup) decorView).addView(fl);
            }
            return fl;
        }
    }

    public Callback getCallback() { return callback; }
    public void setCallback(Callback callback) { this.callback = callback; }

    public boolean requestFeature(int featureId) { return true; }
    public boolean hasFeature(int featureId) { return false; }

    public void addFlags(int flags) { attributes.flags |= flags; }
    public void clearFlags(int flags) { attributes.flags &= ~flags; }
    public void setFlags(int flags, int mask) {
        attributes.flags = (attributes.flags & ~mask) | (flags & mask);
    }

    public WindowManager.LayoutParams getAttributes() { return attributes; }
    public void setAttributes(WindowManager.LayoutParams a) {
        if (a != null) {
            attributes.width = a.width;
            attributes.height = a.height;
            attributes.flags = a.flags;
        }
    }
    public void setLayout(int width, int height) {
        attributes.width = width;
        attributes.height = height;
    }
    public void setBackgroundDrawableResource(int resId) {}
}
