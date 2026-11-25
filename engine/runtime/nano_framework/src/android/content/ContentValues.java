package android.content;

import java.util.HashMap;
import java.util.Map;
import java.util.Set;

public class ContentValues {
    private final Map<String, Object> values = new HashMap<>();

    public ContentValues() {}
    public ContentValues(int size) {}
    public ContentValues(ContentValues from) {
        if (from != null) values.putAll(from.values);
    }

    public void put(String key, String value) { values.put(key, value); }
    public void put(String key, Byte value) { values.put(key, value); }
    public void put(String key, Short value) { values.put(key, value); }
    public void put(String key, Integer value) { values.put(key, value); }
    public void put(String key, Long value) { values.put(key, value); }
    public void put(String key, Float value) { values.put(key, value); }
    public void put(String key, Double value) { values.put(key, value); }
    public void put(String key, Boolean value) { values.put(key, value); }
    public void put(String key, byte[] value) { values.put(key, value); }
    public void putNull(String key) { values.put(key, null); }
    public void putAll(ContentValues other) { if (other != null) values.putAll(other.values); }

    public int size() { return values.size(); }
    public boolean isEmpty() { return values.isEmpty(); }
    public void remove(String key) { values.remove(key); }
    public void clear() { values.clear(); }
    public boolean containsKey(String key) { return values.containsKey(key); }
    public Object get(String key) { return values.get(key); }

    public String getAsString(String key) {
        Object v = values.get(key);
        return v != null ? v.toString() : null;
    }

    public Integer getAsInteger(String key) {
        Object v = values.get(key);
        if (v instanceof Number) return ((Number) v).intValue();
        try { return v != null ? Integer.valueOf(v.toString()) : null; } catch (Exception e) { return null; }
    }

    public Long getAsLong(String key) {
        Object v = values.get(key);
        if (v instanceof Number) return ((Number) v).longValue();
        try { return v != null ? Long.valueOf(v.toString()) : null; } catch (Exception e) { return null; }
    }

    public Double getAsDouble(String key) {
        Object v = values.get(key);
        if (v instanceof Number) return ((Number) v).doubleValue();
        try { return v != null ? Double.valueOf(v.toString()) : null; } catch (Exception e) { return null; }
    }

    public Float getAsFloat(String key) {
        Object v = values.get(key);
        if (v instanceof Number) return ((Number) v).floatValue();
        try { return v != null ? Float.valueOf(v.toString()) : null; } catch (Exception e) { return null; }
    }

    public Boolean getAsBoolean(String key) {
        Object v = values.get(key);
        if (v instanceof Boolean) return (Boolean) v;
        if (v instanceof Number) return ((Number) v).intValue() != 0;
        return v != null ? Boolean.valueOf(v.toString()) : null;
    }

    public Set<Map.Entry<String, Object>> valueSet() { return values.entrySet(); }
    public Set<String> keySet() { return values.keySet(); }
}
