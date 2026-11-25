package android.database.sqlite;

import java.util.HashMap;
import java.util.Map;

public abstract class SQLiteProgram extends SQLiteClosable {
    protected final SQLiteDatabase db;
    protected final String sql;
    protected final Map<Integer, Object> bindings = new HashMap<>();

    protected SQLiteProgram(SQLiteDatabase db, String sql) {
        this.db = db;
        this.sql = sql;
    }

    public void bindNull(int index) { bindings.put(index, null); }
    public void bindLong(int index, long value) { bindings.put(index, value); }
    public void bindDouble(int index, double value) { bindings.put(index, value); }
    public void bindString(int index, String value) { bindings.put(index, value); }
    public void bindBlob(int index, byte[] value) { bindings.put(index, value); }
    public void clearBindings() { bindings.clear(); }

    protected final SQLiteDatabase getDatabase() { return db; }
    protected final String getSql() { return sql; }

    @Override
    protected void onAllReferencesReleased() {
        bindings.clear();
    }
}
