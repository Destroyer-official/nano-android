package android.os;

import java.io.Serializable;
import java.util.ArrayList;

public class Bundle extends BaseBundle {
    public Bundle() {
        super();
    }

    public Bundle(Bundle other) {
        super();
        if (other != null) {
            this.map.putAll(other.map);
        }
    }

    public synchronized void putCharSequence(String key, CharSequence value) { map.put(key, value); }
    public synchronized void putBundle(String key, Bundle value) { map.put(key, value); }
    public synchronized void putSerializable(String key, Serializable value) { map.put(key, value); }
    public synchronized void putParcelable(String key, Parcelable value) { map.put(key, value); }
    public synchronized void putParcelable(String key, Object value) { map.put(key, value); }
    public synchronized void putStringArrayList(String key, ArrayList<String> value) { map.put(key, value); }
    public synchronized void putIntegerArrayList(String key, ArrayList<Integer> value) { map.put(key, value); }
    public synchronized void putAll(Bundle bundle) {
        if (bundle != null) {
            map.putAll(bundle.map);
        }
    }

    public synchronized CharSequence getCharSequence(String key) {
        Object val = map.get(key);
        return val instanceof CharSequence ? (CharSequence) val : null;
    }

    public synchronized Bundle getBundle(String key) {
        Object val = map.get(key);
        return val instanceof Bundle ? (Bundle) val : null;
    }

    public synchronized Serializable getSerializable(String key) {
        Object val = map.get(key);
        return val instanceof Serializable ? (Serializable) val : null;
    }

    public synchronized Parcelable getParcelable(String key) {
        Object val = map.get(key);
        return val instanceof Parcelable ? (Parcelable) val : null;
    }

    public synchronized <T> T getParcelable(String key, Class<T> clazz) {
        Object val = map.get(key);
        return clazz.isInstance(val) ? clazz.cast(val) : null;
    }

    @SuppressWarnings("unchecked")
    public synchronized ArrayList<String> getStringArrayList(String key) {
        Object val = map.get(key);
        return val instanceof ArrayList ? (ArrayList<String>) val : null;
    }

    @SuppressWarnings("unchecked")
    public synchronized ArrayList<Integer> getIntegerArrayList(String key) {
        Object val = map.get(key);
        return val instanceof ArrayList ? (ArrayList<Integer>) val : null;
    }
}
