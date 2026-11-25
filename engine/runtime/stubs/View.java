package android.view;

import android.content.Context;
import java.util.function.Consumer;

public class View {
    public static final int VISIBLE = 0;
    public static final int INVISIBLE = 4;
    public static final int GONE = 8;

    private int id;
    private int visibility = VISIBLE;
    private boolean enabled = true;
    private boolean clickable = true;
    private Object tag;
    private ViewGroup parent;
    private Context context;
    private Consumer<View> onClickListener;

    public View() {}

    public View(Context context) {
        this.context = context;
    }

    public synchronized int getId() {
        return id;
    }

    public synchronized void setId(int id) {
        this.id = id;
    }

    public synchronized int getVisibility() {
        return visibility;
    }

    public synchronized void setVisibility(int visibility) {
        this.visibility = visibility;
    }

    public synchronized boolean isEnabled() {
        return enabled;
    }

    public synchronized void setEnabled(boolean enabled) {
        this.enabled = enabled;
    }

    public synchronized void setOnClickListener(Consumer<View> listener) {
        this.onClickListener = listener;
        this.clickable = true;
    }

    public synchronized boolean performClick() {
        if (!enabled || visibility != VISIBLE || onClickListener == null) {
            return false;
        }
        onClickListener.accept(this);
        return true;
    }

    public synchronized Object getTag() {
        return tag;
    }

    public synchronized void setTag(Object tag) {
        this.tag = tag;
    }

    public synchronized ViewGroup getParent() {
        return parent;
    }

    synchronized void setParent(ViewGroup parent) {
        this.parent = parent;
    }

    public View findViewById(int id) {
        return this.id == id ? this : null;
    }
}
