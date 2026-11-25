package android.content;

import java.util.ArrayList;
import java.util.List;

public class IntentFilter {
    private final List<String> actions = new ArrayList<>();

    public IntentFilter() {}
    public IntentFilter(String action) {
        actions.add(action);
    }

    public void addAction(String action) {
        actions.add(action);
    }

    public boolean matchAction(String action) {
        return actions.contains(action);
    }
}
