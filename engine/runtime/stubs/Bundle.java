package android.os;

import java.util.*;

public class Bundle {
    private final Map<String, Object> map = new HashMap<>();

    public Bundle() {}

    public Bundle(Bundle other) {
        if (other != null) {
            this.map.putAll(other.map);
        }
    }

    public synchronized void putString(String key, String value) {
        map.put(key, value);
    }

    public synchronized String getString(String key) {
        Object val = map.get(key);
        return val != null ? String.valueOf(val) : null;
    }

    public synchronized String getString(String key, String defaultValue) {
        Object val = map.get(key);
        return val != null ? String.valueOf(val) : defaultValue;
    }

    public synchronized void putInt(String key, int value) {
        map.put(key, value);
    }

    public synchronized int getInt(String key, int defaultValue) {
        Object val = map.get(key);
        if (val instanceof Number) {
            return ((Number) val).intValue();
        }
        return defaultValue;
    }

    public synchronized void putBoolean(String key, boolean value) {
        map.put(key, value);
    }

    public synchronized boolean getBoolean(String key, boolean defaultValue) {
        Object val = map.get(key);
        if (val instanceof Boolean) {
            return (Boolean) val;
        }
        return defaultValue;
    }

    public synchronized void putAll(Map<String, Object> values) {
        if (values != null) {
            map.putAll(values);
        }
    }

    public synchronized boolean containsKey(String key) {
        return map.containsKey(key);
    }

    public synchronized Set<String> keySet() {
        return new HashSet<>(map.keySet());
    }

    public synchronized int size() {
        return map.size();
    }

    public synchronized void clear() {
        map.clear();
    }
}
