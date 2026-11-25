package android.database;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

public class MatrixCursor implements Cursor {
    private final String[] columnNames;
    private final Map<String, Integer> columnNameToIndex = new HashMap<>();
    private final List<Object[]> rows = new ArrayList<>();
    private int position = -1;
    private boolean closed = false;

    public MatrixCursor(String[] columnNames) {
        this.columnNames = columnNames != null ? columnNames : new String[0];
        for (int i = 0; i < this.columnNames.length; i++) {
            columnNameToIndex.put(this.columnNames[i], i);
        }
    }

    public void addRow(Object[] columnValues) {
        Object[] row = new Object[columnNames.length];
        if (columnValues != null) {
            System.arraycopy(columnValues, 0, row, 0, Math.min(columnValues.length, row.length));
        }
        rows.add(row);
    }

    public RowBuilder newRow() {
        Object[] row = new Object[columnNames.length];
        rows.add(row);
        return new RowBuilder(row);
    }

    public class RowBuilder {
        private final Object[] row;
        private int index = 0;

        RowBuilder(Object[] row) {
            this.row = row;
        }

        public RowBuilder add(Object value) {
            if (index < row.length) {
                row[index++] = value;
            }
            return this;
        }

        public RowBuilder add(String columnName, Object value) {
            Integer col = columnNameToIndex.get(columnName);
            if (col != null && col < row.length) {
                row[col] = value;
            }
            return this;
        }
    }

    private Object get(int columnIndex) {
        if (columnIndex < 0 || columnIndex >= columnNames.length) {
            throw new IndexOutOfBoundsException("Requested column: " + columnIndex + ", # of columns: " + columnNames.length);
        }
        if (position < 0) {
            throw new IllegalStateException("Before first row.");
        }
        if (position >= rows.size()) {
            throw new IllegalStateException("After last row.");
        }
        return rows.get(position)[columnIndex];
    }

    @Override public int getCount() { return rows.size(); }
    @Override public int getPosition() { return position; }

    @Override
    public boolean move(int offset) {
        return moveToPosition(position + offset);
    }

    @Override
    public boolean moveToPosition(int targetPosition) {
        int count = rows.size();
        if (targetPosition >= count) {
            position = count;
            return false;
        }
        if (targetPosition < 0) {
            position = -1;
            return false;
        }
        position = targetPosition;
        return true;
    }

    @Override public boolean moveToFirst() { return moveToPosition(0); }
    @Override public boolean moveToLast() { return moveToPosition(rows.size() - 1); }
    @Override public boolean moveToNext() { return moveToPosition(position + 1); }
    @Override public boolean moveToPrevious() { return moveToPosition(position - 1); }
    @Override public boolean isFirst() { return position == 0 && rows.size() > 0; }
    @Override public boolean isLast() { return position == rows.size() - 1 && rows.size() > 0; }
    @Override public boolean isBeforeFirst() { return rows.isEmpty() || position < 0; }
    @Override public boolean isAfterLast() { return rows.isEmpty() || position >= rows.size(); }

    @Override
    public int getColumnIndex(String columnName) {
        Integer i = columnNameToIndex.get(columnName);
        return i != null ? i : -1;
    }

    @Override
    public int getColumnIndexOrThrow(String columnName) {
        int i = getColumnIndex(columnName);
        if (i < 0) throw new IllegalArgumentException("Column not found: " + columnName);
        return i;
    }

    @Override public String getColumnName(int columnIndex) { return columnNames[columnIndex]; }
    @Override public String[] getColumnNames() { return columnNames.clone(); }
    @Override public int getColumnCount() { return columnNames.length; }

    @Override
    public byte[] getBlob(int columnIndex) {
        Object val = get(columnIndex);
        return val instanceof byte[] ? (byte[]) val : new byte[0];
    }

    @Override
    public String getString(int columnIndex) {
        Object val = get(columnIndex);
        return val != null ? val.toString() : null;
    }

    @Override
    public short getShort(int columnIndex) {
        Number val = (Number) get(columnIndex);
        return val != null ? val.shortValue() : 0;
    }

    @Override
    public int getInt(int columnIndex) {
        Object val = get(columnIndex);
        if (val instanceof Number) return ((Number) val).intValue();
        if (val instanceof String) {
            try { return Integer.parseInt((String) val); } catch (NumberFormatException ignored) {}
        }
        return 0;
    }

    @Override
    public long getLong(int columnIndex) {
        Object val = get(columnIndex);
        if (val instanceof Number) return ((Number) val).longValue();
        if (val instanceof String) {
            try { return Long.parseLong((String) val); } catch (NumberFormatException ignored) {}
        }
        return 0L;
    }

    @Override
    public float getFloat(int columnIndex) {
        Number val = (Number) get(columnIndex);
        return val != null ? val.floatValue() : 0.0f;
    }

    @Override
    public double getDouble(int columnIndex) {
        Number val = (Number) get(columnIndex);
        return val != null ? val.doubleValue() : 0.0;
    }

    @Override
    public int getType(int columnIndex) {
        Object val = get(columnIndex);
        if (val == null) return FIELD_TYPE_NULL;
        if (val instanceof byte[]) return FIELD_TYPE_BLOB;
        if (val instanceof Float || val instanceof Double) return FIELD_TYPE_FLOAT;
        if (val instanceof Number) return FIELD_TYPE_INTEGER;
        return FIELD_TYPE_STRING;
    }

    @Override
    public boolean isNull(int columnIndex) {
        return get(columnIndex) == null;
    }

    @Override
    public void close() {
        closed = true;
    }

    @Override
    public boolean isClosed() {
        return closed;
    }
}
