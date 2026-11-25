package android.content;

import android.os.Bundle;
import java.util.*;

public class Intent {
    public static final String ACTION_MAIN = "android.intent.action.MAIN";
    public static final String CATEGORY_LAUNCHER = "android.intent.category.LAUNCHER";

    private String action;
    private final Set<String> categories = new HashSet<>();
    private final Bundle extras = new Bundle();
    private String component;

    public Intent() {}

    public Intent(String action) {
        this.action = action;
    }

    public synchronized Intent setAction(String action) {
        this.action = action;
        return this;
    }

    public synchronized String getAction() {
        return this.action;
    }

    public synchronized Intent addCategory(String category) {
        this.categories.add(category);
        return this;
    }

    public synchronized boolean hasCategory(String category) {
        return this.categories.contains(category);
    }

    public synchronized Set<String> getCategories() {
        return new HashSet<>(this.categories);
    }

    public synchronized Intent putExtra(String name, String value) {
        this.extras.putString(name, value);
        return this;
    }

    public synchronized Intent putExtra(String name, int value) {
        this.extras.putInt(name, value);
        return this;
    }

    public synchronized Intent putExtra(String name, boolean value) {
        this.extras.putBoolean(name, value);
        return this;
    }

    public synchronized Bundle getExtras() {
        return this.extras;
    }

    public synchronized String getStringExtra(String name) {
        return this.extras.getString(name);
    }

    public synchronized int getIntExtra(String name, int defaultValue) {
        return this.extras.getInt(name, defaultValue);
    }

    public synchronized boolean getBooleanExtra(String name, boolean defaultValue) {
        return this.extras.getBoolean(name, defaultValue);
    }

    public synchronized void setComponent(String component) {
        this.component = component;
    }

    public synchronized String getComponent() {
        return this.component;
    }
}
