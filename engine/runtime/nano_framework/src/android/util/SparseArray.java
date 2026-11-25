package android.util;

import java.util.ArrayList;
import java.util.Collections;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

public class SparseArray<E> implements Cloneable {
    private final Map<Integer, E> map = new HashMap<>();

    public SparseArray() {}
    public SparseArray(int initialCapacity) {}

    public SparseArray<E> clone() {
        SparseArray<E> clone = new SparseArray<>();
        clone.map.putAll(this.map);
        return clone;
    }

    public E get(int key) {
        return get(key, null);
    }

    public E get(int key, E valueIfKeyNotFound) {
        E val = map.get(key);
        return val != null ? val : valueIfKeyNotFound;
    }

    public void delete(int key) {
        map.remove(key);
    }

    public void remove(int key) {
        map.remove(key);
    }

    public void removeAt(int index) {
        List<Integer> keys = new ArrayList<>(map.keySet());
        Collections.sort(keys);
        if (index >= 0 && index < keys.size()) {
            map.remove(keys.get(index));
        }
    }

    public void put(int key, E value) {
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

    public E valueAt(int index) {
        return get(keyAt(index));
    }

    public void setValueAt(int index, E value) {
        put(keyAt(index), value);
    }

    public int indexOfKey(int key) {
        List<Integer> keys = new ArrayList<>(map.keySet());
        Collections.sort(keys);
        return keys.indexOf(key);
    }

    public int indexOfValue(E value) {
        List<Integer> keys = new ArrayList<>(map.keySet());
        Collections.sort(keys);
        for (int i = 0; i < keys.size(); i++) {
            E v = map.get(keys.get(i));
            if (v == value || (v != null && v.equals(value))) return i;
        }
        return -1;
    }

    public void clear() {
        map.clear();
    }

    public void append(int key, E value) {
        put(key, value);
    }

    @Override
    public String toString() {
        return map.toString();
    }
}
