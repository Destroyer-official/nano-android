package android.database.sqlite;

import android.database.Cursor;
import java.util.TreeMap;

public class SQLiteStatement extends SQLiteProgram {

    public SQLiteStatement(SQLiteDatabase db, String sql) {
        super(db, sql);
    }

    private Object[] orderedArgs() {
        if (bindings.isEmpty()) return null;
        TreeMap<Integer, Object> sorted = new TreeMap<>(bindings);
        int maxIdx = sorted.lastKey();
        Object[] args = new Object[Math.max(0, maxIdx)];
        for (int i = 1; i <= maxIdx; i++) {
            args[i - 1] = sorted.get(i);
        }
        return args;
    }

    private String[] orderedStringArgs() {
        Object[] raw = orderedArgs();
        if (raw == null) return null;
        String[] s = new String[raw.length];
        for (int i = 0; i < raw.length; i++) {
            s[i] = raw[i] != null ? String.valueOf(raw[i]) : null;
        }
        return s;
    }

    public void execute() {
        if (db != null) db.execSQL(sql, orderedArgs());
    }

    public int executeUpdateDelete() {
        return db != null ? db.execSQLUpdateDelete(sql, orderedArgs()) : 0;
    }

    public long executeInsert() {
        return db != null ? db.execSQLInsert(sql, orderedArgs()) : -1L;
    }

    public long simpleQueryForLong() {
        if (db == null || sql == null) return 0L;
        Cursor c = db.rawQuery(sql, orderedStringArgs());
        try {
            if (c != null && c.moveToFirst() && c.getColumnCount() > 0) {
                return c.getLong(0);
            }
        } finally {
            if (c != null) c.close();
        }
        return 0L;
    }

    public String simpleQueryForString() {
        if (db == null || sql == null) return "";
        Cursor c = db.rawQuery(sql, orderedStringArgs());
        try {
            if (c != null && c.moveToFirst() && c.getColumnCount() > 0) {
                String s = c.getString(0);
                return s != null ? s : "";
            }
        } finally {
            if (c != null) c.close();
        }
        return "";
    }
}
