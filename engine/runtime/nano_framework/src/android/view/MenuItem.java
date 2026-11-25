package android.view;

import android.graphics.drawable.Drawable;

public interface MenuItem {
    int SHOW_AS_ACTION_NEVER = 0;
    int SHOW_AS_ACTION_IF_ROOM = 1;
    int SHOW_AS_ACTION_ALWAYS = 2;
    int SHOW_AS_ACTION_WITH_TEXT = 4;
    int SHOW_AS_ACTION_COLLAPSE_ACTION_VIEW = 8;

    int getItemId();
    int getGroupId();
    int getOrder();
    MenuItem setTitle(CharSequence title);
    MenuItem setTitle(int title);
    CharSequence getTitle();
    MenuItem setTitleCondensed(CharSequence title);
    CharSequence getTitleCondensed();
    MenuItem setIcon(Drawable icon);
    MenuItem setIcon(int iconRes);
    Drawable getIcon();
    MenuItem setIntent(android.content.Intent intent);
    android.content.Intent getIntent();
    MenuItem setVisible(boolean visible);
    boolean isVisible();
    MenuItem setEnabled(boolean enabled);
    boolean isEnabled();
    MenuItem setCheckable(boolean checkable);
    boolean isCheckable();
    MenuItem setChecked(boolean checked);
    boolean isChecked();
    void setShowAsAction(int actionEnum);
    MenuItem setShowAsActionFlags(int actionEnum);
    MenuItem setActionView(View view);
    MenuItem setActionView(int resId);
    View getActionView();
    boolean expandActionView();
    boolean collapseActionView();
    boolean isActionViewExpanded();
    MenuItem setOnMenuItemClickListener(OnMenuItemClickListener menuItemClickListener);

    interface OnMenuItemClickListener {
        boolean onMenuItemClick(MenuItem item);
    }
}
