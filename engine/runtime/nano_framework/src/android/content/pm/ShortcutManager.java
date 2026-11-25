package android.content.pm;

import java.util.ArrayList;
import java.util.List;

public class ShortcutManager {
    private final List<ShortcutInfo> shortcuts = new ArrayList<>();

    public boolean setDynamicShortcuts(List<ShortcutInfo> shortcutInfoList) {
        shortcuts.clear();
        if (shortcutInfoList != null) {
            shortcuts.addAll(shortcutInfoList);
        }
        return true;
    }

    public List<ShortcutInfo> getDynamicShortcuts() {
        return new ArrayList<>(shortcuts);
    }
}
