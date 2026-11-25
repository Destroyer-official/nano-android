package android.database.sqlite;

import android.content.ContentValues;
import android.database.Cursor;
import android.database.DatabaseErrorHandler;
import android.database.MatrixCursor;
import java.io.*;
import java.util.*;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicLong;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

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

    private final String path;
    private int version = 0;
    private boolean inTransaction = false;
    private boolean transactionSuccessful = false;
    private boolean isOpen = true;

    private final Map<String, List<ColumnDef>> tables = new ConcurrentHashMap<>();
    private final Map<String, List<IndexDef>> tableIndices = new ConcurrentHashMap<>();
    private final Map<String, IndexDef> indicesByName = new ConcurrentHashMap<>();
    private final Map<String, List<Map<String, Object>>> tableRows = new ConcurrentHashMap<>();
    private final Map<String, AtomicLong> nextRowIds = new ConcurrentHashMap<>();

    private static final Map<String, SQLiteDatabase> activeDatabases = new ConcurrentHashMap<>();

    private SQLiteDatabase(String path) {
        this.path = (path != null && !path.trim().isEmpty()) ? path : ":memory:";
        loadFromDisk();
    }

    public static SQLiteDatabase openOrCreateDatabase(String path, CursorFactory factory) {
        return openOrCreateDatabase(path, factory, null);
    }

    public static SQLiteDatabase openOrCreateDatabase(String path, CursorFactory factory, DatabaseErrorHandler errorHandler) {
        String key = (path != null && !path.trim().isEmpty()) ? path : ":memory:";
        return activeDatabases.computeIfAbsent(key, SQLiteDatabase::new);
    }

    public static SQLiteDatabase openDatabase(String path, CursorFactory factory, int flags) {
        return openOrCreateDatabase(path, factory, null);
    }

    public static SQLiteDatabase createInMemory(CursorFactory factory) {
        return new SQLiteDatabase(":memory:");
    }

    public String getPath() { return path; }
    public synchronized int getVersion() { return version; }
    public synchronized void setVersion(int version) { this.version = version; saveToDisk(); }
    public boolean isOpen() { return isOpen; }
    public boolean isReadOnly() { return false; }
    public boolean inTransaction() { return inTransaction; }

    public void beginTransaction() { inTransaction = true; transactionSuccessful = false; }
    public void beginTransactionNonExclusive() { beginTransaction(); }
    public void setTransactionSuccessful() { transactionSuccessful = true; }
    public void endTransaction() { inTransaction = false; saveToDisk(); }
    public void setMaxSqlCacheSize(int cacheSize) {}
    public boolean enableWriteAheadLogging() { return true; }
    public void disableWriteAheadLogging() {}

    @SuppressWarnings("unchecked")
    private synchronized void loadFromDisk() {
        if (":memory:".equals(path)) return;
        File f = new File(path);
        if (!f.exists() || f.length() == 0) return;
        try (ObjectInputStream ois = new ObjectInputStream(new FileInputStream(f))) {
            this.version = ois.readInt();
            Map<String, List<ColumnDef>> t = (Map<String, List<ColumnDef>>) ois.readObject();
            Map<String, List<Map<String, Object>>> r = (Map<String, List<Map<String, Object>>>) ois.readObject();
            Map<String, Long> ids = (Map<String, Long>) ois.readObject();
            if (t != null) tables.putAll(t);
            if (r != null) tableRows.putAll(r);
            if (ids != null) {
                for (Map.Entry<String, Long> e : ids.entrySet()) {
                    nextRowIds.put(e.getKey(), new AtomicLong(e.getValue()));
                }
            }
        } catch (Throwable ignored) {}
    }

    private synchronized void saveToDisk() {
        if (":memory:".equals(path) || inTransaction) return;
        File f = new File(path);
        try {
            if (f.getParentFile() != null && !f.getParentFile().exists()) {
                f.getParentFile().mkdirs();
            }
            try (ObjectOutputStream oos = new ObjectOutputStream(new FileOutputStream(f))) {
                oos.writeInt(version);
                oos.writeObject(new HashMap<>(tables));
                Map<String, List<Map<String, Object>>> rowsCopy = new HashMap<>();
                for (Map.Entry<String, List<Map<String, Object>>> e : tableRows.entrySet()) {
                    rowsCopy.put(e.getKey(), new ArrayList<>(e.getValue()));
                }
                oos.writeObject(rowsCopy);
                Map<String, Long> idsCopy = new HashMap<>();
                for (Map.Entry<String, AtomicLong> e : nextRowIds.entrySet()) {
                    idsCopy.put(e.getKey(), e.getValue().get());
                }
                oos.writeObject(idsCopy);
            }
        } catch (Throwable ignored) {}
    }

    public void execSQL(String sql) {
        execSQL(sql, null);
    }

    public void execSQL(String sql, Object[] bindArgs) {
        execSQLWithResult(sql, bindArgs);
    }

    public long execSQLInsert(String sql, Object[] bindArgs) {
        return execSQLWithResult(sql, bindArgs);
    }

    public int execSQLUpdateDelete(String sql, Object[] bindArgs) {
        return (int) execSQLWithResult(sql, bindArgs);
    }

    private synchronized long execSQLWithResult(String sql, Object[] bindArgs) {
        if (sql == null) return 0;
        String cleanSql = sql.trim().replaceAll("\\s+", " ");
        String lower = cleanSql.toLowerCase();

        if (lower.startsWith("create table")) {
            parseCreateTable(cleanSql);
            saveToDisk();
            return 0;
        }
        if (lower.startsWith("create unique index") || lower.startsWith("create index")) {
            parseCreateIndex(cleanSql);
            return 0;
        }
        if (lower.startsWith("drop table")) {
            String[] parts = cleanSql.split("\\s+");
            String tbl = cleanIdentifier(parts[parts.length - 1]);
            tables.remove(tbl);
            tableRows.remove(tbl);
            saveToDisk();
            return 0;
        }
        if (lower.startsWith("alter table")) {
            parseAlterTable(cleanSql);
            saveToDisk();
            return 0;
        }
        if (lower.startsWith("insert")) {
            return parseInsertSQL(cleanSql, bindArgs);
        }
        if (lower.startsWith("update ")) {
            return parseUpdateSQL(cleanSql, bindArgs);
        }
        if (lower.startsWith("delete from ")) {
            return parseDeleteSQL(cleanSql, bindArgs);
        }
        return 0;
    }

    private static String cleanIdentifier(String raw) {
        if (raw == null) return "";
        return raw.trim().replace("`", "").replace("'", "").replace("\"", "").replace(";", "").toLowerCase();
    }

    private void parseCreateTable(String sql) {
        try {
            int openParen = sql.indexOf('(');
            int closeParen = sql.lastIndexOf(')');
            if (openParen == -1 || closeParen == -1 || closeParen <= openParen) return;

            String tableHeader = sql.substring(0, openParen).trim();
            String[] parts = tableHeader.split("\\s+");
            String tbl = cleanIdentifier(parts[parts.length - 1]);

            String body = sql.substring(openParen + 1, closeParen).trim();
            String[] colDefs = body.split(",(?=(?:[^']*'[^']*')*[^']*$)");
            List<ColumnDef> colList = new ArrayList<>();
            int cid = 0;

            for (String colStr : colDefs) {
                String trimmed = colStr.trim();
                if (trimmed.isEmpty()) continue;
                String upper = trimmed.toUpperCase();
                if (upper.startsWith("PRIMARY KEY") || upper.startsWith("FOREIGN KEY") || upper.startsWith("CONSTRAINT") || upper.startsWith("UNIQUE") || upper.startsWith("CHECK")) {
                    continue;
                }
                String[] tokens = trimmed.split("\\s+");
                if (tokens.length >= 1) {
                    String colName = tokens[0].replace("`", "").replace("'", "").replace("\"", "");
                    String colType = tokens.length >= 2 ? tokens[1].toUpperCase() : "TEXT";
                    boolean notNull = upper.contains("NOT NULL");
                    int pk = upper.contains("PRIMARY KEY") ? 1 : 0;
                    colList.add(new ColumnDef(cid++, colName, colType, notNull, null, pk));
                }
            }
            tables.putIfAbsent(tbl, colList);
            tableRows.computeIfAbsent(tbl, k -> Collections.synchronizedList(new ArrayList<>()));
        } catch (Throwable ignored) {}
    }

    private void parseAlterTable(String sql) {
        Matcher m = Pattern.compile("(?i)alter\\s+table\\s+([`\"'\\w]+)\\s+add\\s+(?:column\\s+)?([`\"'\\w]+)\\s*(\\w*)").matcher(sql);
        if (m.find()) {
            String tbl = cleanIdentifier(m.group(1));
            String col = m.group(2).replace("`", "").replace("\"", "").replace("'", "");
            String type = m.group(3) != null && !m.group(3).isEmpty() ? m.group(3).toUpperCase() : "TEXT";
            List<ColumnDef> cols = tables.computeIfAbsent(tbl, k -> new ArrayList<>());
            for (ColumnDef c : cols) {
                if (c.name.equalsIgnoreCase(col)) return;
            }
            cols.add(new ColumnDef(cols.size(), col, type, false, null, 0));
        }
    }

    private void parseCreateIndex(String sql) {
        try {
            boolean unique = sql.toLowerCase().contains("create unique index");
            int onIdx = sql.toLowerCase().indexOf(" on ");
            if (onIdx == -1) return;
            int openParen = sql.indexOf('(', onIdx);
            int closeParen = sql.lastIndexOf(')');
            if (openParen == -1 || closeParen == -1) return;
            String beforeOn = sql.substring(0, onIdx).trim();
            String[] beforeParts = beforeOn.split("\\s+");
            String indexName = beforeParts[beforeParts.length - 1].replace("`", "").replace("'", "").replace("\"", "");
            String afterOn = sql.substring(onIdx + 4, openParen).trim();
            String tableName = afterOn.replace("`", "").replace("'", "").replace("\"", "");
            String colsStr = sql.substring(openParen + 1, closeParen).trim();
            IndexDef idx = new IndexDef(indexName, tableName, unique);
            for (String col : colsStr.split(",")) {
                String cleanCol = col.trim().replace("`", "").replace("'", "").replace("\"", "");
                if (!cleanCol.isEmpty()) idx.columns.add(cleanCol);
            }
            tableIndices.computeIfAbsent(tableName.toLowerCase(), k -> new ArrayList<>()).add(idx);
            indicesByName.put(indexName.toLowerCase(), idx);
        } catch (Throwable ignored) {}
    }

    private long parseInsertSQL(String sql, Object[] bindArgs) {
        Matcher m = Pattern.compile("(?i)insert\\s+(?:or\\s+(\\w+)\\s+)?into\\s+([`\"'\\w]+)\\s*(?:\\(([^)]*)\\))?\\s*values\\s*\\((.+)\\)").matcher(sql);
        if (!m.find()) return -1L;
        String mode = m.group(1) != null ? m.group(1).toLowerCase() : "";
        String tbl = cleanIdentifier(m.group(2));
        String colsPart = m.group(3);
        String valsPart = m.group(4);

        List<String> colNames = new ArrayList<>();
        if (colsPart != null && !colsPart.trim().isEmpty()) {
            for (String c : colsPart.split(",")) {
                colNames.add(c.trim().replace("`", "").replace("\"", "").replace("'", ""));
            }
        } else if (tables.containsKey(tbl)) {
            for (ColumnDef cd : tables.get(tbl)) colNames.add(cd.name);
        }

        List<String> valTokens = splitSqlValues(valsPart);
        ContentValues cv = new ContentValues();
        int bindIdx = 0;
        for (int i = 0; i < valTokens.size() && i < colNames.size(); i++) {
            String tok = valTokens.get(i).trim();
            Object v;
            if ("?".equals(tok)) {
                v = (bindArgs != null && bindIdx < bindArgs.length) ? bindArgs[bindIdx++] : null;
            } else {
                v = parseLiteralToken(tok);
            }
            putObjectToContentValues(cv, colNames.get(i), v);
        }
        int conflict = "replace".equals(mode) ? CONFLICT_REPLACE : ("ignore".equals(mode) ? CONFLICT_IGNORE : CONFLICT_NONE);
        return insertWithOnConflict(tbl, null, cv, conflict);
    }

    private int parseUpdateSQL(String sql, Object[] bindArgs) {
        Matcher m = Pattern.compile("(?i)update\\s+([`\"'\\w]+)\\s+set\\s+(.+?)(?:\\s+where\\s+(.+))?$").matcher(sql);
        if (!m.find()) return 0;
        String tbl = cleanIdentifier(m.group(1));
        String setPart = m.group(2);
        String wherePart = m.group(3);

        List<String> assigns = splitSqlValues(setPart);
        ContentValues cv = new ContentValues();
        int bindIdx = 0;
        for (String a : assigns) {
            int eq = a.indexOf('=');
            if (eq <= 0) continue;
            String col = a.substring(0, eq).trim().replace("`", "").replace("\"", "").replace("'", "");
            String rhs = a.substring(eq + 1).trim();
            Object v = "?".equals(rhs)
                    ? ((bindArgs != null && bindIdx < bindArgs.length) ? bindArgs[bindIdx++] : null)
                    : parseLiteralToken(rhs);
            putObjectToContentValues(cv, col, v);
        }

        String[] whereArgs = null;
        if (bindArgs != null && bindIdx < bindArgs.length) {
            whereArgs = new String[bindArgs.length - bindIdx];
            for (int i = 0; i < whereArgs.length; i++) {
                Object b = bindArgs[bindIdx + i];
                whereArgs[i] = b != null ? String.valueOf(b) : null;
            }
        }
        return update(tbl, cv, wherePart, whereArgs);
    }

    private int parseDeleteSQL(String sql, Object[] bindArgs) {
        Matcher m = Pattern.compile("(?i)delete\\s+from\\s+([`\"'\\w]+)(?:\\s+where\\s+(.+))?$").matcher(sql);
        if (!m.find()) return 0;
        String tbl = cleanIdentifier(m.group(1));
        String wherePart = m.group(2);
        String[] whereArgs = null;
        if (bindArgs != null && bindArgs.length > 0) {
            whereArgs = new String[bindArgs.length];
            for (int i = 0; i < bindArgs.length; i++) {
                whereArgs[i] = bindArgs[i] != null ? String.valueOf(bindArgs[i]) : null;
            }
        }
        return delete(tbl, wherePart, whereArgs);
    }

    private static List<String> splitSqlValues(String s) {
        List<String> out = new ArrayList<>();
        if (s == null) return out;
        StringBuilder cur = new StringBuilder();
        boolean inQuote = false;
        int depth = 0;
        for (int i = 0; i < s.length(); i++) {
            char ch = s.charAt(i);
            if (ch == '\'' && (i == 0 || s.charAt(i - 1) != '\\')) {
                inQuote = !inQuote;
                cur.append(ch);
            } else if (!inQuote) {
                if (ch == '(') depth++;
                else if (ch == ')') depth = Math.max(0, depth - 1);
                if (ch == ',' && depth == 0) {
                    out.add(cur.toString());
                    cur.setLength(0);
                    continue;
                }
                cur.append(ch);
            } else {
                cur.append(ch);
            }
        }
        if (cur.length() > 0) out.add(cur.toString());
        return out;
    }

    private static Object parseLiteralToken(String tok) {
        if (tok == null || tok.equalsIgnoreCase("null")) return null;
        if (tok.startsWith("'") && tok.endsWith("'") && tok.length() >= 2) {
            return tok.substring(1, tok.length() - 1).replace("''", "'");
        }
        if (tok.equalsIgnoreCase("true")) return 1L;
        if (tok.equalsIgnoreCase("false")) return 0L;
        try {
            if (tok.contains(".")) return Double.parseDouble(tok);
            return Long.parseLong(tok);
        } catch (NumberFormatException e) {
            return tok;
        }
    }

    private static void putObjectToContentValues(ContentValues cv, String key, Object v) {
        if (v == null) cv.putNull(key);
        else if (v instanceof Integer) cv.put(key, (Integer) v);
        else if (v instanceof Long) cv.put(key, (Long) v);
        else if (v instanceof Double) cv.put(key, (Double) v);
        else if (v instanceof Float) cv.put(key, (Float) v);
        else if (v instanceof Boolean) cv.put(key, (Boolean) v);
        else if (v instanceof byte[]) cv.put(key, (byte[]) v);
        else cv.put(key, String.valueOf(v));
    }

    public Cursor rawQuery(String sql, String[] selectionArgs) {
        return rawQueryWithFactory(null, sql, selectionArgs, null);
    }

    public Cursor rawQueryWithFactory(CursorFactory factory, String sql, String[] selectionArgs, String editTable) {
        if (sql == null) {
            return new MatrixCursor(new String[]{"_id"});
        }
        String cleanSql = sql.trim().replaceAll("\\s+", " ");
        String lower = cleanSql.toLowerCase();

        if (lower.startsWith("pragma table_info")) {
            String targetTable = extractPragmaTarget(sql);
            MatrixCursor mc = new MatrixCursor(new String[]{"cid", "name", "type", "notnull", "dflt_value", "pk"});
            List<ColumnDef> cols = tables.get(targetTable.toLowerCase());
            if (cols != null) {
                for (ColumnDef c : cols) {
                    mc.addRow(new Object[]{c.cid, c.name, c.type, c.notNull ? 1 : 0, c.dfltValue, c.pk});
                }
            }
            return mc;
        }
        if (lower.startsWith("pragma index_list")) {
            String targetTable = extractPragmaTarget(sql);
            MatrixCursor mc = new MatrixCursor(new String[]{"seq", "name", "unique", "origin", "partial"});
            List<IndexDef> idxs = tableIndices.get(targetTable.toLowerCase());
            if (idxs != null) {
                int seq = 0;
                for (IndexDef id : idxs) mc.addRow(new Object[]{seq++, id.name, id.unique ? 1 : 0, "c", 0});
            }
            return mc;
        }
        if (lower.startsWith("pragma index_info")) {
            String targetIndex = extractPragmaTarget(sql);
            MatrixCursor mc = new MatrixCursor(new String[]{"seqno", "cid", "name"});
            IndexDef idx = indicesByName.get(targetIndex.toLowerCase());
            if (idx != null) {
                int seqno = 0;
                for (String colName : idx.columns) mc.addRow(new Object[]{seqno++, 0, colName});
            }
            return mc;
        }
        if (lower.startsWith("pragma foreign_key_list")) {
            return new MatrixCursor(new String[]{"id", "seq", "table", "from", "to", "on_update", "on_delete", "match"});
        }
        if (lower.contains("room_master_table")) {
            MatrixCursor mc = new MatrixCursor(new String[]{"identity_hash"});
            mc.addRow(new Object[]{"1d5b2fddcb258e43f2935820863057a9"});
            return mc;
        }

        Matcher selectM = Pattern.compile("(?i)select\\s+(.+?)\\s+from\\s+([`\"'\\w]+)(.*)$").matcher(cleanSql);
        if (selectM.find()) {
            String selCols = selectM.group(1).trim();
            String tbl = cleanIdentifier(selectM.group(2));
            String tail = selectM.group(3) != null ? selectM.group(3).trim() : "";

            String whereClause = null;
            String orderBy = null;
            String limit = null;

            String tailUpper = tail.toUpperCase();
            int limitIdx = tailUpper.indexOf(" LIMIT ");
            if (limitIdx >= 0) {
                limit = tail.substring(limitIdx + 7).trim().split("\\s+")[0];
                tail = tail.substring(0, limitIdx);
                tailUpper = tail.toUpperCase();
            }
            int orderIdx = tailUpper.indexOf(" ORDER BY ");
            if (orderIdx >= 0) {
                orderBy = tail.substring(orderIdx + 10).trim();
                tail = tail.substring(0, orderIdx);
                tailUpper = tail.toUpperCase();
            }
            int whereIdx = tailUpper.indexOf("WHERE ");
            if (whereIdx >= 0) {
                whereClause = tail.substring(whereIdx + 6).trim();
            }

            if (selCols.toLowerCase().contains("count(")) {
                List<Map<String, Object>> matched = filterRows(tbl, whereClause, selectionArgs, orderBy, null);
                MatrixCursor mc = new MatrixCursor(new String[]{selCols.contains(" ") ? selCols.substring(selCols.lastIndexOf(' ') + 1) : "count"});
                mc.addRow(new Object[]{(long) matched.size()});
                return mc;
            }

            String[] cols = null;
            if (!"*".equals(selCols)) {
                List<String> parsedCols = splitSqlValues(selCols);
                cols = new String[parsedCols.size()];
                for (int i = 0; i < parsedCols.size(); i++) {
                    String colExpr = parsedCols.get(i).trim();
                    int asIdx = colExpr.toLowerCase().indexOf(" as ");
                    if (asIdx > 0) colExpr = colExpr.substring(asIdx + 4).trim();
                    else if (colExpr.contains(".")) colExpr = colExpr.substring(colExpr.lastIndexOf('.') + 1);
                    cols[i] = colExpr.replace("`", "").replace("\"", "").replace("'", "");
                }
            }
            return query(tbl, cols, whereClause, selectionArgs, null, null, orderBy, limit);
        }

        return new MatrixCursor(new String[]{"_id", "id", "count", "value"});
    }

    private static String extractPragmaTarget(String sql) {
        int open = sql.indexOf('(');
        int close = sql.lastIndexOf(')');
        if (open != -1 && close != -1 && close > open) {
            return sql.substring(open + 1, close).trim().replace("`", "").replace("'", "").replace("\"", "");
        }
        String[] parts = sql.trim().split("\\s+");
        return parts[parts.length - 1].replace("`", "").replace("'", "").replace("\"", "");
    }

    public Cursor query(String table, String[] columns, String selection, String[] selectionArgs, String groupBy, String having, String orderBy) {
        return query(table, columns, selection, selectionArgs, groupBy, having, orderBy, null);
    }

    public synchronized Cursor query(String table, String[] columns, String selection, String[] selectionArgs, String groupBy, String having, String orderBy, String limit) {
        String tbl = cleanIdentifier(table);
        List<Map<String, Object>> matched = filterRows(tbl, selection, selectionArgs, orderBy, limit);

        String[] cols;
        if (columns != null && columns.length > 0) {
            cols = columns;
        } else if (tables.containsKey(tbl) && !tables.get(tbl).isEmpty()) {
            List<ColumnDef> cList = tables.get(tbl);
            cols = new String[cList.size()];
            for (int i = 0; i < cList.size(); i++) cols[i] = cList.get(i).name;
        } else if (!matched.isEmpty()) {
            cols = matched.get(0).keySet().toArray(new String[0]);
        } else {
            cols = new String[]{"_id", "id", "count", "value"};
        }

        MatrixCursor mc = new MatrixCursor(cols);
        for (Map<String, Object> row : matched) {
            Object[] vals = new Object[cols.length];
            for (int i = 0; i < cols.length; i++) {
                vals[i] = getCaseInsensitive(row, cols[i]);
            }
            mc.addRow(vals);
        }
        return mc;
    }

    private List<Map<String, Object>> filterRows(String tbl, String selection, String[] selectionArgs, String orderBy, String limit) {
        List<Map<String, Object>> rows = tableRows.getOrDefault(tbl, Collections.emptyList());
        List<Map<String, Object>> matched = new ArrayList<>();
        for (Map<String, Object> row : rows) {
            if (matchesWhere(row, selection, selectionArgs)) {
                matched.add(row);
            }
        }
        if (orderBy != null && !orderBy.trim().isEmpty()) {
            String[] parts = orderBy.trim().split("\\s+");
            final String sortCol = parts[0].replace("`", "").replace("\"", "").replace("'", "");
            final boolean desc = parts.length > 1 && "desc".equalsIgnoreCase(parts[1]);
            matched.sort((a, b) -> {
                Object va = getCaseInsensitive(a, sortCol);
                Object vb = getCaseInsensitive(b, sortCol);
                int cmp = compareValues(va, vb);
                return desc ? -cmp : cmp;
            });
        }
        if (limit != null && !limit.trim().isEmpty()) {
            try {
                int lim = Integer.parseInt(limit.trim().split(",")[0].trim());
                if (lim >= 0 && matched.size() > lim) {
                    matched = new ArrayList<>(matched.subList(0, lim));
                }
            } catch (NumberFormatException ignored) {}
        }
        return matched;
    }

    private static int compareValues(Object a, Object b) {
        if (a == b) return 0;
        if (a == null) return -1;
        if (b == null) return 1;
        if (a instanceof Number && b instanceof Number) {
            return Double.compare(((Number) a).doubleValue(), ((Number) b).doubleValue());
        }
        return String.valueOf(a).compareToIgnoreCase(String.valueOf(b));
    }

    private static Object getCaseInsensitive(Map<String, Object> row, String col) {
        if (row == null || col == null) return null;
        if (row.containsKey(col)) return row.get(col);
        for (Map.Entry<String, Object> e : row.entrySet()) {
            if (e.getKey() != null && e.getKey().equalsIgnoreCase(col)) return e.getValue();
        }
        return null;
    }

    private static void putCaseInsensitive(Map<String, Object> row, String col, Object val) {
        for (String existing : new ArrayList<>(row.keySet())) {
            if (existing.equalsIgnoreCase(col)) {
                row.put(existing, val);
                return;
            }
        }
        row.put(col, val);
    }

    private boolean matchesWhere(Map<String, Object> row, String selection, String[] selectionArgs) {
        if (selection == null || selection.trim().isEmpty()) return true;
        String[] clauses = selection.split("(?i)\\s+and\\s+");
        int argIdx = 0;
        for (String rawClause : clauses) {
            String clause = rawClause.trim();
            while (clause.startsWith("(") && clause.endsWith(")") && clause.length() >= 2) {
                clause = clause.substring(1, clause.length() - 1).trim();
            }
            if (clause.isEmpty()) continue;

            Matcher nullM = Pattern.compile("(?i)^([`\"'\\w.]+)\\s+is\\s+(not\\s+)?null$").matcher(clause);
            if (nullM.matches()) {
                String col = nullM.group(1).replace("`", "").replace("\"", "").replace("'", "");
                boolean isNotNull = nullM.group(2) != null;
                Object val = getCaseInsensitive(row, col);
                if (isNotNull ? (val == null) : (val != null)) return false;
                continue;
            }

            Matcher opM = Pattern.compile("(?i)^([`\"'\\w.]+)\\s*(=|!=|<>|<=|>=|<|>|\\s+like\\s+)\\s*(.+)$").matcher(clause);
            if (opM.matches()) {
                String col = opM.group(1).replace("`", "").replace("\"", "").replace("'", "");
                if (col.contains(".")) col = col.substring(col.lastIndexOf('.') + 1);
                String op = opM.group(2).trim().toLowerCase();
                String rhs = opM.group(3).trim();
                Object expected;
                if ("?".equals(rhs)) {
                    expected = (selectionArgs != null && argIdx < selectionArgs.length) ? selectionArgs[argIdx++] : null;
                } else {
                    expected = parseLiteralToken(rhs);
                }
                Object actual = getCaseInsensitive(row, col);
                if ("like".equals(op)) {
                    String aStr = actual != null ? String.valueOf(actual).toLowerCase() : "";
                    String eStr = expected != null ? String.valueOf(expected).toLowerCase().replace("%", ".*").replace("_", ".") : "";
                    if (!aStr.matches(eStr)) return false;
                } else {
                    int cmp = compareFlexible(actual, expected);
                    if ("=".equals(op) && cmp != 0) return false;
                    if (("!=".equals(op) || "<>".equals(op)) && cmp == 0) return false;
                    if ("<".equals(op) && cmp >= 0) return false;
                    if ("<=".equals(op) && cmp > 0) return false;
                    if (">".equals(op) && cmp <= 0) return false;
                    if (">=".equals(op) && cmp < 0) return false;
                }
            }
        }
        return true;
    }

    private static int compareFlexible(Object actual, Object expected) {
        if (actual == null && expected == null) return 0;
        if (actual == null) return -1;
        if (expected == null) return 1;
        try {
            double da = (actual instanceof Number) ? ((Number) actual).doubleValue() : Double.parseDouble(String.valueOf(actual));
            double db = (expected instanceof Number) ? ((Number) expected).doubleValue() : Double.parseDouble(String.valueOf(expected));
            return Double.compare(da, db);
        } catch (NumberFormatException ignored) {
            return String.valueOf(actual).equals(String.valueOf(expected)) ? 0 : String.valueOf(actual).compareTo(String.valueOf(expected));
        }
    }

    public long insert(String table, String nullColumnHack, ContentValues values) {
        return insertWithOnConflict(table, nullColumnHack, values, CONFLICT_NONE);
    }

    public long insertOrThrow(String table, String nullColumnHack, ContentValues values) {
        return insertWithOnConflict(table, nullColumnHack, values, CONFLICT_ABORT);
    }

    public long replace(String table, String nullColumnHack, ContentValues values) {
        return insertWithOnConflict(table, nullColumnHack, values, CONFLICT_REPLACE);
    }

    public synchronized long insertWithOnConflict(String table, String nullColumnHack, ContentValues values, int conflictAlgorithm) {
        if (table == null) return -1L;
        String tbl = cleanIdentifier(table);
        List<Map<String, Object>> rows = tableRows.computeIfAbsent(tbl, k -> Collections.synchronizedList(new ArrayList<>()));
        AtomicLong seq = nextRowIds.computeIfAbsent(tbl, k -> new AtomicLong(1L));

        Map<String, Object> newRow = new LinkedHashMap<>();
        long rowId = seq.getAndIncrement();
        if (values != null) {
            for (Map.Entry<String, Object> e : values.valueSet()) {
                newRow.put(e.getKey(), e.getValue());
            }
        }
        Object explicitId = getCaseInsensitive(newRow, "_id");
        if (explicitId == null) explicitId = getCaseInsensitive(newRow, "id");
        if (explicitId instanceof Number) {
            rowId = ((Number) explicitId).longValue();
            if (seq.get() <= rowId) seq.set(rowId + 1);
        }

        // Check primary key replacement/conflict
        String pkCol = findPrimaryKeyCol(tbl);
        if (pkCol != null && getCaseInsensitive(newRow, pkCol) != null) {
            Object pkVal = getCaseInsensitive(newRow, pkCol);
            for (int i = 0; i < rows.size(); i++) {
                if (compareFlexible(getCaseInsensitive(rows.get(i), pkCol), pkVal) == 0) {
                    if (conflictAlgorithm == CONFLICT_IGNORE) return -1L;
                    if (conflictAlgorithm == CONFLICT_REPLACE) {
                        rows.set(i, newRow);
                        saveToDisk();
                        return rowId;
                    }
                }
            }
        }
        if (getCaseInsensitive(newRow, "_id") == null && hasColumn(tbl, "_id")) {
            newRow.put("_id", rowId);
        }
        rows.add(newRow);
        saveToDisk();
        return rowId;
    }

    private String findPrimaryKeyCol(String tbl) {
        List<ColumnDef> cols = tables.get(tbl);
        if (cols != null) {
            for (ColumnDef c : cols) {
                if (c.pk == 1) return c.name;
            }
        }
        return null;
    }

    private boolean hasColumn(String tbl, String name) {
        List<ColumnDef> cols = tables.get(tbl);
        if (cols != null) {
            for (ColumnDef c : cols) {
                if (c.name.equalsIgnoreCase(name)) return true;
            }
        }
        return false;
    }

    public synchronized int update(String table, ContentValues values, String whereClause, String[] whereArgs) {
        if (table == null || values == null) return 0;
        String tbl = cleanIdentifier(table);
        List<Map<String, Object>> rows = tableRows.get(tbl);
        if (rows == null || rows.isEmpty()) return 0;
        int count = 0;
        for (Map<String, Object> row : rows) {
            if (matchesWhere(row, whereClause, whereArgs)) {
                for (Map.Entry<String, Object> e : values.valueSet()) {
                    putCaseInsensitive(row, e.getKey(), e.getValue());
                }
                count++;
            }
        }
        if (count > 0) saveToDisk();
        return count;
    }

    public synchronized int delete(String table, String whereClause, String[] whereArgs) {
        if (table == null) return 0;
        String tbl = cleanIdentifier(table);
        List<Map<String, Object>> rows = tableRows.get(tbl);
        if (rows == null || rows.isEmpty()) return 0;
        int count = 0;
        Iterator<Map<String, Object>> it = rows.iterator();
        while (it.hasNext()) {
            Map<String, Object> row = it.next();
            if (matchesWhere(row, whereClause, whereArgs)) {
                it.remove();
                count++;
            }
        }
        if (count > 0) saveToDisk();
        return count;
    }

    public SQLiteStatement compileStatement(String sql) {
        return new SQLiteStatement(this, sql);
    }

    @Override
    protected void onAllReferencesReleased() {
        isOpen = false;
        saveToDisk();
        if (path != null) {
            activeDatabases.remove(path);
        }
    }
}
