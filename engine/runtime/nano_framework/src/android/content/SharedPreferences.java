package android.content;

import java.io.*;
import java.nio.charset.StandardCharsets;
import java.util.*;

public interface SharedPreferences {
    interface OnSharedPreferenceChangeListener {
        void onSharedPreferenceChanged(SharedPreferences sharedPreferences, String key);
    }

    interface Editor {
        Editor putString(String key, String value);
        Editor putStringSet(String key, Set<String> values);
        Editor putInt(String key, int value);
        Editor putLong(String key, long value);
        Editor putFloat(String key, float value);
        Editor putBoolean(String key, boolean value);
        Editor remove(String key);
        Editor clear();
        boolean commit();
        void apply();
    }

    Map<String, ?> getAll();
    String getString(String key, String defValue);
    Set<String> getStringSet(String key, Set<String> defValues);
    int getInt(String key, int defValue);
    long getLong(String key, long defValue);
    float getFloat(String key, float defValue);
    boolean getBoolean(String key, boolean defValue);
    boolean contains(String key);
    Editor edit();
    void registerOnSharedPreferenceChangeListener(OnSharedPreferenceChangeListener listener);
    void unregisterOnSharedPreferenceChangeListener(OnSharedPreferenceChangeListener listener);

    class MemorySharedPreferences implements SharedPreferences {
        private final Map<String, Object> data = new HashMap<>();
        private final List<OnSharedPreferenceChangeListener> listeners = new ArrayList<>();
        private final File storageFile;

        public MemorySharedPreferences() {
            this(null);
        }

        public MemorySharedPreferences(File storageFile) {
            this.storageFile = storageFile;
            loadFromDisk();
        }

        private synchronized void loadFromDisk() {
            if (storageFile == null || !storageFile.exists()) return;
            Properties props = new Properties();
            try (InputStreamReader r = new InputStreamReader(new FileInputStream(storageFile), StandardCharsets.UTF_8)) {
                props.load(r);
                for (String k : props.stringPropertyNames()) {
                    String raw = props.getProperty(k);
                    if (raw == null || raw.length() < 2 || raw.charAt(1) != ':') {
                        data.put(k, raw);
                        continue;
                    }
                    char type = raw.charAt(0);
                    String val = raw.substring(2);
                    switch (type) {
                        case 'S': data.put(k, val); break;
                        case 'I': data.put(k, Integer.parseInt(val)); break;
                        case 'L': data.put(k, Long.parseLong(val)); break;
                        case 'F': data.put(k, Float.parseFloat(val)); break;
                        case 'B': data.put(k, Boolean.parseBoolean(val)); break;
                        case 'Z': {
                            Set<String> set = new LinkedHashSet<>();
                            if (!val.isEmpty()) {
                                for (String part : val.split("\u001F", -1)) {
                                    set.add(new String(Base64.getDecoder().decode(part), StandardCharsets.UTF_8));
                                }
                            }
                            data.put(k, set);
                            break;
                        }
                        default: data.put(k, val); break;
                    }
                }
            } catch (Throwable ignored) {}
        }

        private synchronized void saveToDisk() {
            if (storageFile == null) return;
            try {
                if (storageFile.getParentFile() != null && !storageFile.getParentFile().exists()) {
                    storageFile.getParentFile().mkdirs();
                }
                Properties props = new Properties();
                for (Map.Entry<String, Object> e : data.entrySet()) {
                    Object v = e.getValue();
                    if (v instanceof Integer) props.setProperty(e.getKey(), "I:" + v);
                    else if (v instanceof Long) props.setProperty(e.getKey(), "L:" + v);
                    else if (v instanceof Float) props.setProperty(e.getKey(), "F:" + v);
                    else if (v instanceof Boolean) props.setProperty(e.getKey(), "B:" + v);
                    else if (v instanceof Set<?>) {
                        StringBuilder sb = new StringBuilder();
                        boolean first = true;
                        for (Object item : (Set<?>) v) {
                            if (!first) sb.append('\u001F');
                            sb.append(Base64.getEncoder().encodeToString(String.valueOf(item).getBytes(StandardCharsets.UTF_8)));
                            first = false;
                        }
                        props.setProperty(e.getKey(), "Z:" + sb.toString());
                    } else if (v != null) {
                        props.setProperty(e.getKey(), "S:" + v.toString());
                    }
                }
                try (OutputStreamWriter w = new OutputStreamWriter(new FileOutputStream(storageFile), StandardCharsets.UTF_8)) {
                    props.store(w, "Nano-Android SharedPreferences");
                }
            } catch (Throwable ignored) {}
        }

        @Override public synchronized Map<String, ?> getAll() { return new HashMap<>(data); }
        @Override public synchronized String getString(String key, String defValue) { Object v = data.get(key); return v instanceof String ? (String) v : (v != null ? v.toString() : defValue); }
        @SuppressWarnings("unchecked")
        @Override public synchronized Set<String> getStringSet(String key, Set<String> defValues) { Object v = data.get(key); return v instanceof Set ? new LinkedHashSet<>((Set<String>) v) : defValues; }
        @Override public synchronized int getInt(String key, int defValue) { Object v = data.get(key); return v instanceof Number ? ((Number) v).intValue() : defValue; }
        @Override public synchronized long getLong(String key, long defValue) { Object v = data.get(key); return v instanceof Number ? ((Number) v).longValue() : defValue; }
        @Override public synchronized float getFloat(String key, float defValue) { Object v = data.get(key); return v instanceof Number ? ((Number) v).floatValue() : defValue; }
        @Override public synchronized boolean getBoolean(String key, boolean defValue) { Object v = data.get(key); return v instanceof Boolean ? (Boolean) v : defValue; }
        @Override public synchronized boolean contains(String key) { return data.containsKey(key); }

        @Override
        public synchronized void registerOnSharedPreferenceChangeListener(OnSharedPreferenceChangeListener listener) {
            if (listener != null && !listeners.contains(listener)) listeners.add(listener);
        }

        @Override
        public synchronized void unregisterOnSharedPreferenceChangeListener(OnSharedPreferenceChangeListener listener) {
            listeners.remove(listener);
        }

        @Override
        public Editor edit() {
            return new Editor() {
                private final Map<String, Object> staged = new HashMap<>();
                private final Set<String> removed = new HashSet<>();
                private boolean cleared = false;

                @Override public Editor putString(String k, String v) { staged.put(k, v); return this; }
                @Override public Editor putStringSet(String k, Set<String> v) { staged.put(k, v != null ? new LinkedHashSet<>(v) : null); return this; }
                @Override public Editor putInt(String k, int v) { staged.put(k, v); return this; }
                @Override public Editor putLong(String k, long v) { staged.put(k, v); return this; }
                @Override public Editor putFloat(String k, float v) { staged.put(k, v); return this; }
                @Override public Editor putBoolean(String k, boolean v) { staged.put(k, v); return this; }
                @Override public Editor remove(String k) { removed.add(k); staged.remove(k); return this; }
                @Override public Editor clear() { cleared = true; staged.clear(); removed.clear(); return this; }

                @Override
                public boolean commit() {
                    List<String> changedKeys = new ArrayList<>();
                    synchronized (MemorySharedPreferences.this) {
                        if (cleared) {
                            changedKeys.addAll(data.keySet());
                            data.clear();
                        }
                        for (String r : removed) {
                            if (data.remove(r) != null) changedKeys.add(r);
                        }
                        for (Map.Entry<String, Object> e : staged.entrySet()) {
                            if (e.getValue() == null) data.remove(e.getKey());
                            else data.put(e.getKey(), e.getValue());
                            changedKeys.add(e.getKey());
                        }
                        saveToDisk();
                    }
                    for (OnSharedPreferenceChangeListener l : new ArrayList<>(listeners)) {
                        for (String k : changedKeys) {
                            l.onSharedPreferenceChanged(MemorySharedPreferences.this, k);
                        }
                    }
                    return true;
                }

                @Override public void apply() { commit(); }
            };
        }
    }
}
