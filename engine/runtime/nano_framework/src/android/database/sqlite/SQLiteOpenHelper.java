package android.database.sqlite;

import android.content.Context;
import android.database.DatabaseErrorHandler;

public abstract class SQLiteOpenHelper implements AutoCloseable {
    private final Context mContext;
    private final String mName;
    private final int mNewVersion;
    private SQLiteDatabase mDatabase;
    private boolean mIsInitializing;

    public SQLiteOpenHelper(Context context, String name, SQLiteDatabase.CursorFactory factory, int version) {
        this(context, name, factory, version, null);
    }

    public SQLiteOpenHelper(Context context, String name, SQLiteDatabase.CursorFactory factory, int version, DatabaseErrorHandler errorHandler) {
        this.mContext = context;
        this.mName = name;
        this.mNewVersion = version;
    }

    public String getDatabaseName() {
        return mName;
    }

    public void setWriteAheadLoggingEnabled(boolean enabled) {}

    public synchronized SQLiteDatabase getWritableDatabase() {
        return getDatabaseLocked(true);
    }

    public synchronized SQLiteDatabase getReadableDatabase() {
        return getDatabaseLocked(false);
    }

    private synchronized SQLiteDatabase getDatabaseLocked(boolean writable) {
        if (mDatabase != null && mDatabase.isOpen()) {
            return mDatabase;
        }
        if (mIsInitializing) {
            throw new IllegalStateException("getDatabase called recursively");
        }
        try {
            mIsInitializing = true;
            String dbPath = (mContext != null && mName != null && !mName.trim().isEmpty())
                    ? mContext.getDatabasePath(mName).getAbsolutePath()
                    : (mName != null ? mName : ":memory:");
            mDatabase = SQLiteDatabase.openOrCreateDatabase(dbPath, null);
            onConfigure(mDatabase);
            int version = mDatabase.getVersion();
            if (version == 0) {
                onCreate(mDatabase);
                mDatabase.setVersion(mNewVersion);
            } else if (version != mNewVersion) {
                if (version < mNewVersion) {
                    onUpgrade(mDatabase, version, mNewVersion);
                } else {
                    onDowngrade(mDatabase, version, mNewVersion);
                }
                mDatabase.setVersion(mNewVersion);
            }
            onOpen(mDatabase);
            return mDatabase;
        } finally {
            mIsInitializing = false;
        }
    }

    @Override
    public synchronized void close() {
        if (mDatabase != null && mDatabase.isOpen()) {
            mDatabase.close();
            mDatabase = null;
        }
    }

    public abstract void onCreate(SQLiteDatabase db);
    public abstract void onUpgrade(SQLiteDatabase db, int oldVersion, int newVersion);
    public void onDowngrade(SQLiteDatabase db, int oldVersion, int newVersion) {}
    public void onOpen(SQLiteDatabase db) {}
    public void onConfigure(SQLiteDatabase db) {}
}
