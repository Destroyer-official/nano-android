package android.content.res;

import java.util.HashMap;
import java.util.Map;

public class Resources {
    private final Map<String, String> strings = new HashMap<>();

    public Resources() {}

    public synchronized void setString(String name, String value) {
        strings.put(name, value);
    }

    public synchronized String getString(String name) {
        return strings.get(name);
    }
}
