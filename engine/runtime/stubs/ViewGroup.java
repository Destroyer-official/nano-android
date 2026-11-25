package android.view;

import android.content.Context;
import java.util.ArrayList;
import java.util.List;

public class ViewGroup extends View {
    private final List<View> children = new ArrayList<>();

    public ViewGroup() {
        super();
    }

    public ViewGroup(Context context) {
        super(context);
    }

    public synchronized void addView(View child) {
        if (child != null) {
            child.setParent(this);
            children.add(child);
        }
    }

    public synchronized void removeView(View child) {
        if (child != null) {
            child.setParent(null);
            children.remove(child);
        }
    }

    public synchronized int getChildCount() {
        return children.size();
    }

    public synchronized View getChildAt(int index) {
        if (index >= 0 && index < children.size()) {
            return children.get(index);
        }
        return null;
    }

    @Override
    public synchronized View findViewById(int id) {
        if (getId() == id) {
            return this;
        }
        for (View child : children) {
            View found = child.findViewById(id);
            if (found != null) {
                return found;
            }
        }
        return null;
    }
}
