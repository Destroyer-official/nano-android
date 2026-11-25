package android.view;

import android.content.Context;
import android.util.AttributeSet;
import android.widget.FrameLayout;
import nanoandroid.host.ApkResourceTable;
import nanoandroid.host.BinaryXmlInflater;

public abstract class LayoutInflater {
    public interface Factory {
        View onCreateView(String name, Context context, AttributeSet attrs);
    }

    public interface Factory2 extends Factory {
        View onCreateView(View parent, String name, Context context, AttributeSet attrs);
    }

    private final Context context;
    private Factory factory;
    private Factory2 factory2;

    protected LayoutInflater(Context context) {
        this.context = context;
    }

    public static LayoutInflater from(Context context) {
        return new LayoutInflater(context) {
            @Override
            public View inflate(int resource, ViewGroup root, boolean attachToRoot) {
                ApkResourceTable table = (context != null && context.getResources() != null)
                        ? context.getResources().getApkTable() : null;
                View v = BinaryXmlInflater.inflateLayout(resource, context, root, attachToRoot, table);
                if (v != null) {
                    return v;
                }
                v = new FrameLayout(context);
                v.setId(resource);
                if (root != null && attachToRoot) {
                    root.addView(v);
                    return root;
                }
                return v;
            }
        };
    }

    public Context getContext() {
        return context;
    }

    public Factory getFactory() {
        return factory;
    }

    public Factory2 getFactory2() {
        return factory2;
    }

    public void setFactory(Factory factory) {
        this.factory = factory;
    }

    public void setFactory2(Factory2 factory) {
        this.factory = factory;
        this.factory2 = factory;
    }

    public LayoutInflater cloneInContext(Context newContext) {
        return LayoutInflater.from(newContext);
    }

    public View inflate(int resource, ViewGroup root) {
        return inflate(resource, root, root != null);
    }

    public abstract View inflate(int resource, ViewGroup root, boolean attachToRoot);
}
