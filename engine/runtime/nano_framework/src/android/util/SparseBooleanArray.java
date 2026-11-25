package android.util;

import java.util.ArrayList;
import java.util.Collections;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

public class SparseBooleanArray implements Cloneable {
    private final Map<Integer, Boolean> map = new HashMap<>();

    public SparseBooleanArray() {}
    public SparseBooleanArray(int initialCapacity) {}

    public SparseBooleanArray clone() {
        SparseBooleanArray clone = new SparseBooleanArray();
        clone.map.putAll(this.map);
        return clone;
    }

    public boolean get(int key) {
        return get(key, false);
    }

    public boolean get(int key, boolean valueIfKeyNotFound) {
        Boolean val = map.get(key);
        return val != null ? val : valueIfKeyNotFound;
    }

    public void delete(int key) {
        map.remove(key);
    }

    public void removeAt(int index) {
        List<Integer> keys = new ArrayList<>(map.keySet());
        Collections.sort(keys);
        if (index >= 0 && index < keys.size()) {
            map.remove(keys.get(index));
        }
    }

    public void put(int key, boolean value) {
        map.put(key, value);
    }

    public int size() {
        return map.size();
    }

    public int keyAt(int index) {
        List<Integer> keys = new ArrayList<>(map.keySet());
        Collections.sort(keys);
        return (index >= 0 && index < keys.size()) ? keys.get(index) : 0;
    }

    public boolean valueAt(int index) {
        return get(keyAt(index));
    }

    public void setValueAt(int index, boolean value) {
        put(keyAt(index), value);
    }

    public int indexOfKey(int key) {
        List<Integer> keys = new ArrayList<>(map.keySet());
        Collections.sort(keys);
        return keys.indexOf(key);
    }

    public int indexOfValue(boolean value) {
        List<Integer> keys = new ArrayList<>(map.keySet());
        Collections.sort(keys);
        for (int i = 0; i < keys.size(); i++) {
            if (map.get(keys.get(i)) == value) return i;
        }
        return -1;
    }

    public void clear() {
        map.clear();
    }

    public void append(int key, boolean value) {
        put(key, value);
    }

    @Override
    public int hashCode() {
        return map.hashCode();
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (o == null || getClass() != o.getClass()) return false;
        return map.equals(((SparseBooleanArray) o).map);
    }

    @Override
    public String toString() {
        return map.toString();
    }
}
