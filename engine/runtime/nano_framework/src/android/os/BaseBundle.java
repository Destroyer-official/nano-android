package android.os;

import java.util.HashMap;
import java.util.Map;
import java.util.Set;

public class BaseBundle {
    final Map<String, Object> map = new HashMap<>();

    public BaseBundle() {}

    public synchronized void putString(String key, String value) { map.put(key, value); }
    public synchronized void putInt(String key, int value) { map.put(key, value); }
    public synchronized void putLong(String key, long value) { map.put(key, value); }
    public synchronized void putBoolean(String key, boolean value) { map.put(key, value); }
    public synchronized void putDouble(String key, double value) { map.put(key, value); }
    public synchronized void putFloat(String key, float value) { map.put(key, value); }
    public synchronized void putStringArray(String key, String[] value) { map.put(key, value); }

    public synchronized String[] getStringArray(String key) {
        Object val = map.get(key);
        return val instanceof String[] ? (String[]) val : null;
    }

    public synchronized Object get(String key) { return map.get(key); }

    public synchronized String getString(String key) { return getString(key, null); }
    public synchronized String getString(String key, String defaultValue) {
        Object val = map.get(key);
        return val instanceof String ? (String) val : defaultValue;
    }

    public synchronized int getInt(String key) { return getInt(key, 0); }
    public synchronized int getInt(String key, int defaultValue) {
        Object val = map.get(key);
        return val instanceof Number ? ((Number) val).intValue() : defaultValue;
    }

    public synchronized long getLong(String key) { return getLong(key, 0L); }
    public synchronized long getLong(String key, long defaultValue) {
        Object val = map.get(key);
        return val instanceof Number ? ((Number) val).longValue() : defaultValue;
    }

    public synchronized boolean getBoolean(String key) { return getBoolean(key, false); }
    public synchronized boolean getBoolean(String key, boolean defaultValue) {
        Object val = map.get(key);
        return val instanceof Boolean ? (Boolean) val : defaultValue;
    }

    public synchronized double getDouble(String key) { return getDouble(key, 0.0); }
    public synchronized double getDouble(String key, double defaultValue) {
        Object val = map.get(key);
        return val instanceof Number ? ((Number) val).doubleValue() : defaultValue;
    }

    public synchronized boolean containsKey(String key) { return map.containsKey(key); }
    public synchronized Set<String> keySet() { return map.keySet(); }
    public synchronized int size() { return map.size(); }
    public synchronized boolean isEmpty() { return map.isEmpty(); }
    public synchronized void remove(String key) { map.remove(key); }
    public synchronized void clear() { map.clear(); }
}
