package android.content;

import android.app.LifecycleEventTracer;
import android.database.Cursor;
import android.net.Uri;
import nanoandroid.compat.ApiSupportLevel;
import nanoandroid.compat.NanoApiStatus;

@NanoApiStatus(
    level = ApiSupportLevel.PARTIAL,
    fallback = "android_art",
    notes = "Local ContentProvider boot-before-Application and authority query/insert/update/delete routing"
)
public abstract class ContentProvider {
    private Context context;
    private String authority;

    public final void attachInfo(Context context, String authority) {
        this.context = context;
        this.authority = authority;
    }

    public final Context getContext() {
        return context;
    }

    public final String getAuthority() {
        return authority;
    }

    public final boolean performCreate() {
        LifecycleEventTracer.record("CONTENT_PROVIDER_CREATE", getClass().getName(), authority != null ? authority : "");
        return onCreate();
    }

    public abstract boolean onCreate();
    public abstract Cursor query(Uri uri, String[] projection, String selection, String[] selectionArgs, String sortOrder);
    public abstract String getType(Uri uri);
    public abstract Uri insert(Uri uri, ContentValues values);
    public abstract int delete(Uri uri, String selection, String[] selectionArgs);
    public abstract int update(Uri uri, ContentValues values, String selection, String[] selectionArgs);
}
