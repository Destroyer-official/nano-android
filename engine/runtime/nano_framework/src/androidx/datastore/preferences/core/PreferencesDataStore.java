package androidx.datastore.preferences.core;

import android.content.Context;
import java.io.*;
import java.nio.charset.StandardCharsets;
import java.nio.file.AtomicMoveNotSupportedException;
import java.nio.file.Files;
import java.nio.file.StandardCopyOption;
import java.util.*;

/**
 * Atomic-write file-backed PreferencesDataStore adapter compatible with AndroidX DataStore semantics.
 */
public class PreferencesDataStore {
    private final File backingFile;
    private final Map<String, Object> state = new LinkedHashMap<>();

    public PreferencesDataStore(Context context, String name) {
        if (context == null || name == null || name.trim().isEmpty()) {
            throw new IllegalArgumentException("Context and store name required");
        }
        File dir = new File(context.getFilesDir(), "datastore");
        if (!dir.exists()) dir.mkdirs();
        String safe = name.replaceAll("[^a-zA-Z0-9._-]", "_") + ".preferences_pb";
        this.backingFile = new File(dir, safe);
        load();
    }

    private synchronized void load() {
        if (!backingFile.exists()) return;
        Properties props = new Properties();
        try (InputStreamReader r = new InputStreamReader(new FileInputStream(backingFile), StandardCharsets.UTF_8)) {
            props.load(r);
            for (String k : props.stringPropertyNames()) {
                String raw = props.getProperty(k);
                if (raw == null || raw.length() < 2 || raw.charAt(1) != ':') {
                    state.put(k, raw);
                    continue;
                }
                char t = raw.charAt(0);
                String v = raw.substring(2);
                switch (t) {
                    case 'I': state.put(k, Integer.parseInt(v)); break;
                    case 'L': state.put(k, Long.parseLong(v)); break;
                    case 'B': state.put(k, Boolean.parseBoolean(v)); break;
                    case 'D': state.put(k, Double.parseDouble(v)); break;
                    default: state.put(k, v); break;
                }
            }
        } catch (Throwable ignored) {}
    }

    public synchronized Map<String, Object> snapshot() {
        return Collections.unmodifiableMap(new LinkedHashMap<>(state));
    }

    public synchronized void updateData(java.util.function.Consumer<Map<String, Object>> mutator) throws IOException {
        Map<String, Object> copy = new LinkedHashMap<>(state);
        mutator.accept(copy);
        File tmp = new File(backingFile.getParentFile(), backingFile.getName() + ".tmp");
        Properties props = new Properties();
        for (Map.Entry<String, Object> e : copy.entrySet()) {
            Object v = e.getValue();
            if (v == null) continue;
            if (v instanceof Integer) props.setProperty(e.getKey(), "I:" + v);
            else if (v instanceof Long) props.setProperty(e.getKey(), "L:" + v);
            else if (v instanceof Boolean) props.setProperty(e.getKey(), "B:" + v);
            else if (v instanceof Double) props.setProperty(e.getKey(), "D:" + v);
            else props.setProperty(e.getKey(), "S:" + v.toString());
        }
        try (OutputStreamWriter w = new OutputStreamWriter(new FileOutputStream(tmp), StandardCharsets.UTF_8)) {
            props.store(w, "Nano-Android DataStore");
        }
        try {
            Files.move(tmp.toPath(), backingFile.toPath(), StandardCopyOption.REPLACE_EXISTING, StandardCopyOption.ATOMIC_MOVE);
        } catch (AtomicMoveNotSupportedException e) {
            Files.move(tmp.toPath(), backingFile.toPath(), StandardCopyOption.REPLACE_EXISTING);
        }
        state.clear();
        state.putAll(copy);
    }

    public synchronized String getString(String key, String defValue) {
        Object v = state.get(key);
        return v instanceof String ? (String) v : (v != null ? v.toString() : defValue);
    }

    public synchronized int getInt(String key, int defValue) {
        Object v = state.get(key);
        return v instanceof Number ? ((Number) v).intValue() : defValue;
    }

    public synchronized boolean getBoolean(String key, boolean defValue) {
        Object v = state.get(key);
        return v instanceof Boolean ? (Boolean) v : defValue;
    }
}
