package android.widget;

import android.content.Context;
import android.content.Intent;
import android.graphics.drawable.Drawable;
import android.view.Menu;
import android.view.MenuInflater;
import android.view.MenuItem;
import android.view.SubMenu;
import android.view.View;
import java.util.ArrayList;
import java.util.List;

public class PopupMenu {
    public interface OnMenuItemClickListener {
        boolean onMenuItemClick(MenuItem item);
    }

    public interface OnDismissListener {
        void onDismiss(PopupMenu menu);
    }

    private final Context context;
    private final Menu menu = new SimpleMenu();
    private OnMenuItemClickListener itemClickListener;
    private OnDismissListener dismissListener;

    public PopupMenu(Context context, View anchor) {
        this.context = context;
    }

    public Menu getMenu() { return menu; }
    public MenuInflater getMenuInflater() { return new MenuInflater(context); }
    public void inflate(int menuRes) { getMenuInflater().inflate(menuRes, menu); }
    public void setOnMenuItemClickListener(OnMenuItemClickListener listener) { this.itemClickListener = listener; }
    public void setOnDismissListener(OnDismissListener listener) { this.dismissListener = listener; }
    public void show() {}
    public void dismiss() {
        if (dismissListener != null) dismissListener.onDismiss(this);
    }

    public boolean dispatchItemClick(MenuItem item) {
        return itemClickListener != null && itemClickListener.onMenuItemClick(item);
    }

    private final class SimpleMenu implements Menu {
        private final List<MenuItem> items = new ArrayList<>();

        @Override public MenuItem add(CharSequence title) { return add(0, items.size() + 1, 0, title); }
        @Override public MenuItem add(int titleRes) { return add(0, titleRes, 0, titleRes); }
        @Override public MenuItem add(int groupId, int itemId, int order, CharSequence title) {
            SimpleMenuItem mi = new SimpleMenuItem(groupId, itemId, order, title);
            items.add(mi);
            return mi;
        }
        @Override public MenuItem add(int groupId, int itemId, int order, int titleRes) {
            String txt = context != null ? context.getString(titleRes) : ("Item " + titleRes);
            return add(groupId, itemId, order, txt);
        }
        @Override public SubMenu addSubMenu(CharSequence title) { return null; }
        @Override public SubMenu addSubMenu(int titleRes) { return null; }
        @Override public SubMenu addSubMenu(int groupId, int itemId, int order, CharSequence title) { return null; }
        @Override public SubMenu addSubMenu(int groupId, int itemId, int order, int titleRes) { return null; }
        @Override public void removeItem(int id) { items.removeIf(i -> i.getItemId() == id); }
        @Override public void removeGroup(int groupId) { items.removeIf(i -> i.getGroupId() == groupId); }
        @Override public void clear() { items.clear(); }
        @Override public void setGroupCheckable(int group, boolean checkable, boolean exclusive) {
            for (MenuItem i : items) if (i.getGroupId() == group) i.setCheckable(checkable);
        }
        @Override public void setGroupVisible(int group, boolean visible) {
            for (MenuItem i : items) if (i.getGroupId() == group) i.setVisible(visible);
        }
        @Override public void setGroupEnabled(int group, boolean enabled) {
            for (MenuItem i : items) if (i.getGroupId() == group) i.setEnabled(enabled);
        }
        @Override public boolean hasVisibleItems() {
            for (MenuItem i : items) if (i.isVisible()) return true;
            return false;
        }
        @Override public MenuItem findItem(int id) {
            for (MenuItem i : items) if (i.getItemId() == id) return i;
            return null;
        }
        @Override public int size() { return items.size(); }
        @Override public MenuItem getItem(int index) { return items.get(index); }
        @Override public void close() {}
    }

    private final class SimpleMenuItem implements MenuItem {
        private final int groupId, itemId, order;
        private CharSequence title, condensed;
        private Drawable icon;
        private Intent intent;
        private boolean visible = true, enabled = true, checkable = false, checked = false, expanded = false;
        private View actionView;

        SimpleMenuItem(int groupId, int itemId, int order, CharSequence title) {
            this.groupId = groupId;
            this.itemId = itemId;
            this.order = order;
            this.title = title;
        }
        @Override public int getItemId() { return itemId; }
        @Override public int getGroupId() { return groupId; }
        @Override public int getOrder() { return order; }
        @Override public MenuItem setTitle(CharSequence t) { this.title = t; return this; }
        @Override public MenuItem setTitle(int r) { this.title = context != null ? context.getString(r) : ""; return this; }
        @Override public CharSequence getTitle() { return title; }
        @Override public MenuItem setTitleCondensed(CharSequence t) { this.condensed = t; return this; }
        @Override public CharSequence getTitleCondensed() { return condensed; }
        @Override public MenuItem setIcon(Drawable i) { this.icon = i; return this; }
        @Override public MenuItem setIcon(int r) { return this; }
        @Override public Drawable getIcon() { return icon; }
        @Override public MenuItem setIntent(Intent i) { this.intent = i; return this; }
        @Override public Intent getIntent() { return intent; }
        @Override public MenuItem setVisible(boolean v) { this.visible = v; return this; }
        @Override public boolean isVisible() { return visible; }
        @Override public MenuItem setEnabled(boolean e) { this.enabled = e; return this; }
        @Override public boolean isEnabled() { return enabled; }
        @Override public MenuItem setCheckable(boolean c) { this.checkable = c; return this; }
        @Override public boolean isCheckable() { return checkable; }
        @Override public MenuItem setChecked(boolean c) { this.checked = c; return this; }
        @Override public boolean isChecked() { return checked; }
        @Override public void setShowAsAction(int a) {}
        @Override public MenuItem setShowAsActionFlags(int a) { return this; }
        @Override public MenuItem setActionView(View v) { this.actionView = v; return this; }
        @Override public MenuItem setActionView(int r) { return this; }
        @Override public View getActionView() { return actionView; }
        @Override public boolean expandActionView() { expanded = true; return true; }
        @Override public boolean collapseActionView() { expanded = false; return true; }
        @Override public boolean isActionViewExpanded() { return expanded; }
        @Override public MenuItem setOnMenuItemClickListener(OnMenuItemClickListener l) { return this; }
    }
}
