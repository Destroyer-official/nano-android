package android.content;

import java.util.HashMap;
import java.util.HashSet;
import java.util.Map;
import java.util.Set;

public interface SharedPreferences {
    String getString(String key, String defValue);
    int getInt(String key, int defValue);
    boolean getBoolean(String key, boolean defValue);
    boolean contains(String key);
    Editor edit();

    interface Editor {
        Editor putString(String key, String value);
        Editor putInt(String key, int value);
        Editor putBoolean(String key, boolean value);
        Editor remove(String key);
        Editor clear();
        void apply();
        boolean commit();
    }

    class MemorySharedPreferences implements SharedPreferences {
        private final Map<String, Object> values = new HashMap<>();

        @Override
        public synchronized String getString(String key, String defValue) {
            Object val = values.get(key);
            return val != null ? String.valueOf(val) : defValue;
        }

        @Override
        public synchronized int getInt(String key, int defValue) {
            Object val = values.get(key);
            if (val instanceof Number) {
                return ((Number) val).intValue();
            }
            return defValue;
        }

        @Override
        public synchronized boolean getBoolean(String key, boolean defValue) {
            Object val = values.get(key);
            if (val instanceof Boolean) {
                return (Boolean) val;
            }
            return defValue;
        }

        @Override
        public synchronized boolean contains(String key) {
            return values.containsKey(key);
        }

        @Override
        public Editor edit() {
            return new MemoryEditor(this);
        }

        private static class MemoryEditor implements Editor {
            private final MemorySharedPreferences target;
            private final Map<String, Object> staged = new HashMap<>();
            private final Set<String> removes = new HashSet<>();
            private boolean clearAll = false;

            MemoryEditor(MemorySharedPreferences target) {
                this.target = target;
            }

            @Override
            public synchronized Editor putString(String key, String value) {
                staged.put(key, value);
                removes.remove(key);
                return this;
            }

            @Override
            public synchronized Editor putInt(String key, int value) {
                staged.put(key, value);
                removes.remove(key);
                return this;
            }

            @Override
            public synchronized Editor putBoolean(String key, boolean value) {
                staged.put(key, value);
                removes.remove(key);
                return this;
            }

            @Override
            public synchronized Editor remove(String key) {
                removes.add(key);
                staged.remove(key);
                return this;
            }

            @Override
            public synchronized Editor clear() {
                clearAll = true;
                return this;
            }

            @Override
            public void apply() {
                commit();
            }

            @Override
            public synchronized boolean commit() {
                synchronized (target) {
                    if (clearAll) {
                        target.values.clear();
                    }
                    for (String r : removes) {
                        target.values.remove(r);
                    }
                    target.values.putAll(staged);
                }
                staged.clear();
                removes.clear();
                clearAll = false;
                return true;
            }
        }
    }
}
