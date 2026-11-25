package android.util;

import java.util.LinkedHashMap;
import java.util.Map;

public class LruCache<K, V> {
    private final LinkedHashMap<K, V> map;
    private final int maxSize;

    public LruCache(int maxSize) {
        this.maxSize = maxSize;
        this.map = new LinkedHashMap<>(maxSize, 0.75f, true);
    }

    public synchronized V get(K key) {
        return map.get(key);
    }

    public synchronized V put(K key, V value) {
        return map.put(key, value);
    }

    public synchronized V remove(K key) {
        return map.remove(key);
    }

    public synchronized void evictAll() {
        map.clear();
    }

    public synchronized int size() {
        return map.size();
    }

    public synchronized int maxSize() {
        return maxSize;
    }
}
