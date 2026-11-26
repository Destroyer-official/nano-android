package android.database.sqlite;

import android.content.ContentValues;
import android.database.Cursor;
import android.database.DatabaseErrorHandler;
import android.database.MatrixCursor;
import java.io.File;
import java.io.Serializable;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.ResultSetMetaData;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Deque;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/**
 * Real SQLite engine backed by org.xerial:sqlite-jdbc (SQLite 3.54.1).
 * Enforces ACID transaction semantics, automatic ROLLBACK on failed endTransaction(),
 * nested SAVEPOINTs, foreign keys, constraints, triggers, views, joins, and WAL mode.
 */
public class SQLiteDatabase extends SQLiteClosable {
    public static final int OPEN_READWRITE = 0x00000000;
    public static final int OPEN_READONLY = 0x00000001;
    public static final int CREATE_IF_NECESSARY = 0x10000000;
    public static final int CONFLICT_NONE = 0;
    public static final int CONFLICT_ROLLBACK = 1;
    public static final int CONFLICT_ABORT = 2;
    public static final int CONFLICT_FAIL = 3;
    public static final int CONFLICT_IGNORE = 4;
    public static final int CONFLICT_REPLACE = 5;

    public interface CursorFactory {
        Cursor newCursor(SQLiteDatabase db, Object masterQuery, String editTable, Object query);
    }

    public static class ColumnDef implements Serializable {
        public int cid;
        public String name;
        public String type;
        public boolean notNull;
        public String dfltValue;
        public int pk;

        public ColumnDef(int cid, String name, String type, boolean notNull, String dfltValue, int pk) {
            this.cid = cid;
            this.name = name;
            this.type = type;
            this.notNull = notNull;
            this.dfltValue = dfltValue;
            this.pk = pk;
        }
    }

    public static class IndexDef implements Serializable {
        public String name;
        public String table;
        public boolean unique;
        public List<String> columns = new ArrayList<>();

        public IndexDef(String name, String table, boolean unique) {
            this.name = name;
            this.table = table;
            this.unique = unique;
        }
    }

    private static final class TxFrame {
        final String savepointName; // null for outermost transaction
        boolean successful;
        TxFrame(String savepointName) {
            this.savepointName = savepointName;
        }
    }

    private final String path;
    private final Connection conn;
    private final boolean readOnly;
    private final Deque<TxFrame> txStack = new ArrayDeque<>();
    private int savepointCounter = 0;
    private boolean isOpen = true;

    private static final Map<String, SQLiteDatabase> activeDatabases = new ConcurrentHashMap<>();

    static {
        try {
            Class.forName("org.sqlite.JDBC");
        } catch (ClassNotFoundException e) {
            throw new IllegalStateException("sqlite-jdbc driver missing from runtime classpath", e);
        }
    }

    private SQLiteDatabase(String path, boolean readOnly) {
        this.path = (path != null && !path.trim().isEmpty()) ? path.trim() : ":memory:";
        this.readOnly = readOnly;
        try {
            String jdbcUrl;
            if (":memory:".equals(this.path)) {
                jdbcUrl = "jdbc:sqlite::memory:";
            } else {
                File f = new File(this.path);
                if (f.getParentFile() != null && !f.getParentFile().exists()) {
                    f.getParentFile().mkdirs();
                }
                jdbcUrl = "jdbc:sqlite:" + f.getAbsolutePath();
            }
            this.conn = DriverManager.getConnection(jdbcUrl);
            try (Statement st = this.conn.createStatement()) {
                st.execute("PRAGMA foreign_keys = ON;");
                st.execute("PRAGMA busy_timeout = 5000;");
                if (!":memory:".equals(this.path) && !readOnly) {
                    st.execute("PRAGMA journal_mode = WAL;");
                }
            }
        } catch (SQLException e) {
            throw new RuntimeException("Failed opening SQLite database at " + this.path + ": " + e.getMessage(), e);
        }
    }

    public static SQLiteDatabase openOrCreateDatabase(String path, CursorFactory factory) {
        return openOrCreateDatabase(path, factory, null);
    }

    public static SQLiteDatabase openOrCreateDatabase(String path, CursorFactory factory, DatabaseErrorHandler errorHandler) {
        String key = (path != null && !path.trim().isEmpty()) ? path.trim() : ":memory:";
        if (":memory:".equals(key)) {
            return new SQLiteDatabase(":memory:", false);
        }
        return activeDatabases.compute(key, (k, existing) -> {
            if (existing != null && existing.isOpen()) {
                return existing;
            }
            return new SQLiteDatabase(k, false);
        });
    }

    public static SQLiteDatabase openDatabase(String path, CursorFactory factory, int flags) {
        boolean ro = (flags & OPEN_READONLY) != 0;
        String key = (path != null && !path.trim().isEmpty()) ? path.trim() : ":memory:";
        if (":memory:".equals(key)) {
            return new SQLiteDatabase(":memory:", ro);
        }
        return activeDatabases.compute(key, (k, existing) -> {
            if (existing != null && existing.isOpen()) {
                return existing;
            }
            return new SQLiteDatabase(k, ro);
        });
    }

    public static SQLiteDatabase createInMemory(CursorFactory factory) {
        return new SQLiteDatabase(":memory:", false);
    }

    public String getPath() { return path; }
    public boolean isOpen() { return isOpen; }
    public boolean isReadOnly() { return readOnly; }
    public synchronized boolean inTransaction() { return !txStack.isEmpty(); }

    public synchronized int getVersion() {
        ensureOpen();
        try (Statement st = conn.createStatement(); ResultSet rs = st.executeQuery("PRAGMA user_version;")) {
            if (rs.next()) return rs.getInt(1);
            return 0;
        } catch (SQLException e) {
            throw new RuntimeException("getVersion failed: " + e.getMessage(), e);
        }
    }

    public synchronized void setVersion(int version) {
        ensureOpen();
        try (Statement st = conn.createStatement()) {
            st.execute("PRAGMA user_version = " + version + ";");
        } catch (SQLException e) {
            throw new RuntimeException("setVersion failed: " + e.getMessage(), e);
        }
    }

    public synchronized void beginTransaction() {
        ensureOpen();
        try {
            if (txStack.isEmpty()) {
                try (Statement st = conn.createStatement()) {
                    st.execute("BEGIN IMMEDIATE;");
                }
                txStack.push(new TxFrame(null));
            } else {
                String sp = "nano_sp_" + (++savepointCounter);
                try (Statement st = conn.createStatement()) {
                    st.execute("SAVEPOINT " + sp + ";");
                }
                txStack.push(new TxFrame(sp));
            }
        } catch (SQLException e) {
            throw new RuntimeException("beginTransaction failed: " + e.getMessage(), e);
        }
    }

    public void beginTransactionNonExclusive() {
        beginTransaction();
    }

    public synchronized void setTransactionSuccessful() {
        ensureOpen();
        if (txStack.isEmpty()) {
            throw new IllegalStateException("Cannot call setTransactionSuccessful outside of a transaction");
        }
        TxFrame top = txStack.peek();
        if (top.successful) {
            throw new IllegalStateException("setTransactionSuccessful already called for current transaction");
        }
        top.successful = true;
    }

    public synchronized void endTransaction() {
        ensureOpen();
        if (txStack.isEmpty()) {
            throw new IllegalStateException("No active transaction to end");
        }
        TxFrame top = txStack.pop();
        try (Statement st = conn.createStatement()) {
            if (top.savepointName == null) {
                if (top.successful) {
                    st.execute("COMMIT;");
                    nanoandroid.host.ForensicTimeline.record("DB_MUTATION", path, "COMMIT");
                } else {
                    st.execute("ROLLBACK;");
                    nanoandroid.host.ForensicTimeline.record("DB_MUTATION", path, "ROLLBACK");
                }
            } else {
                if (top.successful) {
                    st.execute("RELEASE SAVEPOINT " + top.savepointName + ";");
                } else {
                    st.execute("ROLLBACK TO SAVEPOINT " + top.savepointName + ";");
                    st.execute("RELEASE SAVEPOINT " + top.savepointName + ";");
                }
            }
        } catch (SQLException e) {
            throw new RuntimeException("endTransaction failed: " + e.getMessage(), e);
        }
    }

    public void setMaxSqlCacheSize(int cacheSize) {}

    public synchronized boolean enableWriteAheadLogging() {
        ensureOpen();
        if (":memory:".equals(path) || readOnly) return false;
        try (Statement st = conn.createStatement()) {
            st.execute("PRAGMA journal_mode = WAL;");
            return true;
        } catch (SQLException e) {
            return false;
        }
    }

    public synchronized void disableWriteAheadLogging() {
        ensureOpen();
        if (":memory:".equals(path) || readOnly) return;
        try (Statement st = conn.createStatement()) {
            st.execute("PRAGMA journal_mode = DELETE;");
        } catch (SQLException ignored) {}
    }

    public void execSQL(String sql) {
        execSQL(sql, null);
    }

    public synchronized void execSQL(String sql, Object[] bindArgs) {
        ensureOpen();
        if (sql == null || sql.trim().isEmpty()) return;
        try (PreparedStatement ps = conn.prepareStatement(sql)) {
            bindParameters(ps, bindArgs);
            ps.execute();
            nanoandroid.host.ForensicTimeline.record("DB_MUTATION", path, "execSQL:" + sql.trim().split("\\s+", 2)[0]);
        } catch (SQLException e) {
            throw new RuntimeException("SQLite execSQL failed [" + sql + "]: " + e.getMessage(), e);
        }
    }

    public synchronized long execSQLInsert(String sql, Object[] bindArgs) {
        ensureOpen();
        if (sql == null || sql.trim().isEmpty()) return -1L;
        try (PreparedStatement ps = conn.prepareStatement(sql)) {
            bindParameters(ps, bindArgs);
            ps.executeUpdate();
            return lastInsertRowId();
        } catch (SQLException e) {
            throw new RuntimeException("SQLite execSQLInsert failed: " + e.getMessage(), e);
        }
    }

    public synchronized int execSQLUpdateDelete(String sql, Object[] bindArgs) {
        ensureOpen();
        if (sql == null || sql.trim().isEmpty()) return 0;
        try (PreparedStatement ps = conn.prepareStatement(sql)) {
            bindParameters(ps, bindArgs);
            return ps.executeUpdate();
        } catch (SQLException e) {
            throw new RuntimeException("SQLite execSQLUpdateDelete failed: " + e.getMessage(), e);
        }
    }

    public Cursor rawQuery(String sql, String[] selectionArgs) {
        return rawQueryWithFactory(null, sql, selectionArgs, null);
    }

    public synchronized Cursor rawQueryWithFactory(CursorFactory factory, String sql, String[] selectionArgs, String editTable) {
        ensureOpen();
        if (sql == null || sql.trim().isEmpty()) {
            return new MatrixCursor(new String[]{"_id"});
        }
        try (PreparedStatement ps = conn.prepareStatement(sql)) {
            bindParameters(ps, selectionArgs);
            boolean hasResultSet = ps.execute();
            if (!hasResultSet) {
                return new MatrixCursor(new String[0]);
            }
            try (ResultSet rs = ps.getResultSet()) {
                return resultSetToCursor(rs);
            }
        } catch (SQLException e) {
            throw new RuntimeException("SQLite rawQuery failed [" + sql + "]: " + e.getMessage(), e);
        }
    }

    public Cursor query(String table, String[] columns, String selection, String[] selectionArgs, String groupBy, String having, String orderBy) {
        return query(table, columns, selection, selectionArgs, groupBy, having, orderBy, null);
    }

    public synchronized Cursor query(String table, String[] columns, String selection, String[] selectionArgs, String groupBy, String having, String orderBy, String limit) {
        ensureOpen();
        StringBuilder sb = new StringBuilder("SELECT ");
        if (columns != null && columns.length > 0) {
            for (int i = 0; i < columns.length; i++) {
                if (i > 0) sb.append(", ");
                sb.append(columns[i]);
            }
        } else {
            sb.append("*");
        }
        sb.append(" FROM ").append(quoteIdentifier(table));
        if (selection != null && !selection.trim().isEmpty()) {
            sb.append(" WHERE ").append(selection);
        }
        if (groupBy != null && !groupBy.trim().isEmpty()) {
            sb.append(" GROUP BY ").append(groupBy);
        }
        if (having != null && !having.trim().isEmpty()) {
            sb.append(" HAVING ").append(having);
        }
        if (orderBy != null && !orderBy.trim().isEmpty()) {
            sb.append(" ORDER BY ").append(orderBy);
        }
        if (limit != null && !limit.trim().isEmpty()) {
            sb.append(" LIMIT ").append(limit);
        }
        return rawQuery(sb.toString(), selectionArgs);
    }

    public long insert(String table, String nullColumnHack, ContentValues values) {
        try {
            return insertWithOnConflict(table, nullColumnHack, values, CONFLICT_NONE);
        } catch (RuntimeException e) {
            return -1L;
        }
    }

    public long insertOrThrow(String table, String nullColumnHack, ContentValues values) {
        return insertWithOnConflict(table, nullColumnHack, values, CONFLICT_ABORT);
    }

    public long replace(String table, String nullColumnHack, ContentValues values) {
        return insertWithOnConflict(table, nullColumnHack, values, CONFLICT_REPLACE);
    }

    public synchronized long insertWithOnConflict(String table, String nullColumnHack, ContentValues values, int conflictAlgorithm) {
        ensureOpen();
        if (table == null || table.trim().isEmpty()) return -1L;

        String conflictClause = conflictSqlClause(conflictAlgorithm);
        StringBuilder sql = new StringBuilder("INSERT ");
        if (!conflictClause.isEmpty()) {
            sql.append(conflictClause).append(" ");
        }
        sql.append("INTO ").append(quoteIdentifier(table)).append(" ");

        List<Object> bindVals = new ArrayList<>();
        if (values != null && values.size() > 0) {
            sql.append("(");
            StringBuilder placeholders = new StringBuilder();
            int idx = 0;
            for (Map.Entry<String, Object> e : values.valueSet()) {
                if (idx > 0) {
                    sql.append(", ");
                    placeholders.append(", ");
                }
                sql.append(quoteIdentifier(e.getKey()));
                placeholders.append("?");
                bindVals.add(e.getValue());
                idx++;
            }
            sql.append(") VALUES (").append(placeholders).append(")");
        } else if (nullColumnHack != null && !nullColumnHack.trim().isEmpty()) {
            sql.append("(").append(quoteIdentifier(nullColumnHack)).append(") VALUES (NULL)");
        } else {
            sql.append("DEFAULT VALUES");
        }

        try (PreparedStatement ps = conn.prepareStatement(sql.toString())) {
            bindParameters(ps, bindVals.toArray());
            int affected = ps.executeUpdate();
            if (affected == 0 && conflictAlgorithm == CONFLICT_IGNORE) {
                return -1L;
            }
            nanoandroid.host.ForensicTimeline.record("DB_MUTATION", path, "insert:" + table);
            return lastInsertRowId();
        } catch (SQLException e) {
            if (conflictAlgorithm == CONFLICT_IGNORE) {
                return -1L;
            }
            throw new RuntimeException("SQLite insert failed: " + e.getMessage(), e);
        }
    }

    public synchronized int update(String table, ContentValues values, String whereClause, String[] whereArgs) {
        ensureOpen();
        if (table == null || values == null || values.size() == 0) return 0;

        StringBuilder sql = new StringBuilder("UPDATE ").append(quoteIdentifier(table)).append(" SET ");
        List<Object> bindVals = new ArrayList<>();
        int idx = 0;
        for (Map.Entry<String, Object> e : values.valueSet()) {
            if (idx > 0) sql.append(", ");
            sql.append(quoteIdentifier(e.getKey())).append(" = ?");
            bindVals.add(e.getValue());
            idx++;
        }
        if (whereClause != null && !whereClause.trim().isEmpty()) {
            sql.append(" WHERE ").append(whereClause);
        }
        if (whereArgs != null) {
            for (String wa : whereArgs) {
                bindVals.add(wa);
            }
        }
        try (PreparedStatement ps = conn.prepareStatement(sql.toString())) {
            bindParameters(ps, bindVals.toArray());
            int rows = ps.executeUpdate();
            nanoandroid.host.ForensicTimeline.record("DB_MUTATION", path, "update:" + table + "=" + rows);
            return rows;
        } catch (SQLException e) {
            throw new RuntimeException("SQLite update failed: " + e.getMessage(), e);
        }
    }

    public synchronized int delete(String table, String whereClause, String[] whereArgs) {
        ensureOpen();
        if (table == null || table.trim().isEmpty()) return 0;
        StringBuilder sql = new StringBuilder("DELETE FROM ").append(quoteIdentifier(table));
        if (whereClause != null && !whereClause.trim().isEmpty()) {
            sql.append(" WHERE ").append(whereClause);
        }
        try (PreparedStatement ps = conn.prepareStatement(sql.toString())) {
            bindParameters(ps, whereArgs);
            int rows = ps.executeUpdate();
            nanoandroid.host.ForensicTimeline.record("DB_MUTATION", path, "delete:" + table + "=" + rows);
            return rows;
        } catch (SQLException e) {
            throw new RuntimeException("SQLite delete failed: " + e.getMessage(), e);
        }
    }

    public SQLiteStatement compileStatement(String sql) {
        return new SQLiteStatement(this, sql);
    }

    private long lastInsertRowId() throws SQLException {
        try (Statement st = conn.createStatement(); ResultSet rs = st.executeQuery("SELECT last_insert_rowid();")) {
            if (rs.next()) return rs.getLong(1);
            return -1L;
        }
    }

    private static MatrixCursor resultSetToCursor(ResultSet rs) throws SQLException {
        ResultSetMetaData meta = rs.getMetaData();
        int colCount = meta.getColumnCount();
        String[] cols = new String[colCount];
        for (int i = 0; i < colCount; i++) {
            cols[i] = meta.getColumnLabel(i + 1);
        }
        MatrixCursor mc = new MatrixCursor(cols);
        while (rs.next()) {
            Object[] row = new Object[colCount];
            for (int i = 0; i < colCount; i++) {
                row[i] = rs.getObject(i + 1);
            }
            mc.addRow(row);
        }
        return mc;
    }

    private static void bindParameters(PreparedStatement ps, Object[] args) throws SQLException {
        if (args == null) return;
        for (int i = 0; i < args.length; i++) {
            Object val = args[i];
            if (val == null) {
                ps.setNull(i + 1, java.sql.Types.NULL);
            } else if (val instanceof Boolean) {
                ps.setInt(i + 1, ((Boolean) val) ? 1 : 0);
            } else if (val instanceof byte[]) {
                ps.setBytes(i + 1, (byte[]) val);
            } else {
                ps.setObject(i + 1, val);
            }
        }
    }

    private static String conflictSqlClause(int algo) {
        switch (algo) {
            case CONFLICT_ROLLBACK: return "OR ROLLBACK";
            case CONFLICT_ABORT: return "OR ABORT";
            case CONFLICT_FAIL: return "OR FAIL";
            case CONFLICT_IGNORE: return "OR IGNORE";
            case CONFLICT_REPLACE: return "OR REPLACE";
            default: return "";
        }
    }

    private static String quoteIdentifier(String raw) {
        if (raw == null) return "\"\"";
        String clean = raw.trim().replace("`", "").replace("\"", "");
        return "\"" + clean + "\"";
    }

    private void ensureOpen() {
        if (!isOpen) {
            throw new IllegalStateException("SQLiteDatabase is closed: " + path);
        }
    }

    public synchronized String integrityCheck() {
        ensureOpen();
        try (Statement st = conn.createStatement(); ResultSet rs = st.executeQuery("PRAGMA integrity_check;")) {
            if (rs.next()) {
                return rs.getString(1);
            }
            return "error";
        } catch (SQLException e) {
            return "error: " + e.getMessage();
        }
    }

    /**
     * Snapshots all active open SQLite database table names and exact row counts for
     * differential behavioral verification.
     */
    public static Map<String, String> snapshotActiveTables() {
        Map<String, String> out = new java.util.TreeMap<>();
        for (Map.Entry<String, SQLiteDatabase> entry : activeDatabases.entrySet()) {
            SQLiteDatabase db = entry.getValue();
            if (db == null || !db.isOpen()) continue;
            String dbName = new File(entry.getKey()).getName();
            synchronized (db) {
                if (!db.isOpen()) continue;
                List<String> tables = new ArrayList<>();
                try (Statement st = db.conn.createStatement();
                     ResultSet rs = st.executeQuery("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name")) {
                    while (rs.next()) {
                        tables.add(rs.getString(1));
                    }
                } catch (SQLException ignored) {
                    continue;
                }
                for (String t : tables) {
                    try (Statement st2 = db.conn.createStatement();
                         ResultSet rs2 = st2.executeQuery("SELECT COUNT(*) FROM " + quoteIdentifier(t))) {
                        if (rs2.next()) {
                            out.put(dbName + "#" + t, "rows=" + rs2.getLong(1));
                        }
                    } catch (SQLException ignored) {}
                }
            }
        }
        return out;
    }

    @Override
    protected synchronized void onAllReferencesReleased() {
        isOpen = false;
        txStack.clear();
        try {
            if (conn != null && !conn.isClosed()) {
                conn.close();
            }
        } catch (SQLException ignored) {}
        if (path != null) {
            activeDatabases.remove(path);
        }
    }
}
