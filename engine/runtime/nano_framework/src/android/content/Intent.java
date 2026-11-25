package android.content;

import android.net.Uri;
import android.os.Bundle;
import android.os.Parcelable;
import java.io.Serializable;
import java.util.HashSet;
import java.util.Set;

public class Intent {
    public static final String ACTION_MAIN = "android.intent.action.MAIN";
    public static final String ACTION_VIEW = "android.intent.action.VIEW";
    public static final String ACTION_SEND = "android.intent.action.SEND";
    public static final String ACTION_CHOOSER = "android.intent.action.CHOOSER";
    public static final String ACTION_PICK = "android.intent.action.PICK";
    public static final String ACTION_GET_CONTENT = "android.intent.action.GET_CONTENT";
    public static final String ACTION_OPEN_DOCUMENT = "android.intent.action.OPEN_DOCUMENT";
    public static final String ACTION_OPEN_DOCUMENT_TREE = "android.intent.action.OPEN_DOCUMENT_TREE";
    public static final String ACTION_CREATE_DOCUMENT = "android.intent.action.CREATE_DOCUMENT";

    public static final String CATEGORY_LAUNCHER = "android.intent.category.LAUNCHER";
    public static final String CATEGORY_OPENABLE = "android.intent.category.OPENABLE";

    public static final String EXTRA_INTENT = "android.intent.extra.INTENT";
    public static final String EXTRA_TITLE = "android.intent.extra.TITLE";
    public static final String EXTRA_ALLOW_MULTIPLE = "android.intent.extra.ALLOW_MULTIPLE";
    public static final String EXTRA_MIME_TYPES = "android.intent.extra.MIME_TYPES";
    public static final String EXTRA_TEXT = "android.intent.extra.TEXT";
    public static final String EXTRA_STREAM = "android.intent.extra.STREAM";

    public static final int FLAG_ACTIVITY_NEW_TASK = 0x10000000;
    public static final int FLAG_ACTIVITY_CLEAR_TOP = 0x04000000;
    public static final int FLAG_ACTIVITY_SINGLE_TOP = 0x20000000;
    public static final int FLAG_GRANT_READ_URI_PERMISSION = 0x00000001;
    public static final int FLAG_GRANT_WRITE_URI_PERMISSION = 0x00000002;
    public static final int FLAG_GRANT_PERSISTABLE_URI_PERMISSION = 0x00000040;

    private int flags = 0;
    private String action;
    private Uri data;
    private String type;
    private ClipData clipData;
    private final Set<String> categories = new HashSet<>();
    private final Bundle extras = new Bundle();
    private ComponentName component;

    public Intent() {}
    public Intent(String action) { this.action = action; }
    public Intent(String action, Uri uri) { this.action = action; this.data = uri; }
    public Intent(Context packageContext, Class<?> cls) {
        String pkg = packageContext != null ? packageContext.getPackageName() : "com.example.app";
        this.component = new ComponentName(pkg, cls.getName());
    }

    public static Intent createChooser(Intent target, CharSequence title) {
        Intent chooser = new Intent(ACTION_CHOOSER);
        chooser.putExtra(EXTRA_INTENT, target);
        if (title != null) chooser.putExtra(EXTRA_TITLE, title.toString());
        return chooser;
    }

    public String getAction() { return action; }
    public Intent setAction(String action) { this.action = action; return this; }
    public Uri getData() { return data; }
    public Intent setData(Uri data) { this.data = data; return this; }
    public Intent setDataAndType(Uri data, String type) { this.data = data; this.type = type; return this; }
    public Intent setType(String type) { this.type = type; return this; }
    public String getType() { return type; }
    public ClipData getClipData() { return clipData; }
    public void setClipData(ClipData clip) { this.clipData = clip; }

    public Intent addCategory(String category) { categories.add(category); return this; }
    public boolean hasCategory(String category) { return categories.contains(category); }

    public Intent putExtra(String key, String value) { extras.putString(key, value); return this; }
    public Intent putExtra(String key, CharSequence value) { extras.putCharSequence(key, value); return this; }
    public Intent putExtra(String key, int value) { extras.putInt(key, value); return this; }
    public Intent putExtra(String key, long value) { extras.putLong(key, value); return this; }
    public Intent putExtra(String key, boolean value) { extras.putBoolean(key, value); return this; }
    public Intent putExtra(String key, double value) { extras.putDouble(key, value); return this; }
    public Intent putExtra(String key, String[] value) { extras.putStringArray(key, value); return this; }
    public Intent putExtra(String key, Bundle value) { extras.putBundle(key, value); return this; }
    public Intent putExtra(String key, Parcelable value) { extras.putParcelable(key, value); return this; }
    public Intent putExtra(String key, Serializable value) { extras.putSerializable(key, value); return this; }
    public Intent putExtra(String key, Object value) { extras.putParcelable(key, value); return this; }

    public String getStringExtra(String key) { return extras.getString(key); }
    public String[] getStringArrayExtra(String key) { return extras.getStringArray(key); }
    public int getIntExtra(String key, int def) { return extras.getInt(key, def); }
    public long getLongExtra(String key, long def) { return extras.getLong(key, def); }
    public boolean getBooleanExtra(String key, boolean def) { return extras.getBoolean(key, def); }
    public double getDoubleExtra(String key, double def) { return extras.getDouble(key, def); }
    @SuppressWarnings("unchecked")
    public <T> T getParcelableExtra(String key) {
        try { return (T) extras.getParcelable(key, Object.class); } catch (Throwable e) { return null; }
    }
    public Bundle getExtras() { return extras; }
    public void removeExtra(String name) { extras.remove(name); }
    public boolean hasExtra(String name) { return extras.containsKey(name); }
    public Intent putExtras(Bundle extras) { if (extras != null) this.extras.putAll(extras); return this; }
    public Intent putExtras(Intent src) { if (src != null) this.extras.putAll(src.extras); return this; }
    public int getFlags() { return flags; }
    public Intent setFlags(int flags) { this.flags = flags; return this; }
    public Intent addFlags(int flags) { this.flags |= flags; return this; }

    public ComponentName getComponent() { return component; }
    public Intent setComponent(ComponentName component) { this.component = component; return this; }
}
