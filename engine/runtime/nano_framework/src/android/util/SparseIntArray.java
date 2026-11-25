package android.util;

import java.util.ArrayList;
import java.util.Collections;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

public class SparseIntArray implements Cloneable {
    private final Map<Integer, Integer> map = new HashMap<>();

    public SparseIntArray() {}
    public SparseIntArray(int initialCapacity) {}

    public SparseIntArray clone() {
        SparseIntArray clone = new SparseIntArray();
        clone.map.putAll(this.map);
        return clone;
    }

    public int get(int key) {
        return get(key, 0);
    }

    public int get(int key, int valueIfKeyNotFound) {
        Integer val = map.get(key);
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

    public void put(int key, int value) {
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

    public int valueAt(int index) {
        return get(keyAt(index));
    }

    public void setValueAt(int index, int value) {
        put(keyAt(index), value);
    }

    public int indexOfKey(int key) {
        List<Integer> keys = new ArrayList<>(map.keySet());
        Collections.sort(keys);
        return keys.indexOf(key);
    }

    public int indexOfValue(int value) {
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

    public void append(int key, int value) {
        put(key, value);
    }

    @Override
    public String toString() {
        return map.toString();
    }
}
