package android.content;

import java.util.ArrayList;
import java.util.List;
import nanoandroid.compat.ApiSupportLevel;
import nanoandroid.compat.NanoApiStatus;

@NanoApiStatus(
    level = ApiSupportLevel.PARTIAL,
    fallback = "android_art",
    notes = "In-process clipboard manager supporting ClipData get/set and change listeners"
)
public class ClipboardManager {
    public interface OnPrimaryClipChangedListener {
        void onPrimaryClipChanged();
    }

    private ClipData primaryClip;
    private final List<OnPrimaryClipChangedListener> listeners = new ArrayList<>();

    public synchronized void setPrimaryClip(ClipData clip) {
        this.primaryClip = clip;
        for (OnPrimaryClipChangedListener l : new ArrayList<>(listeners)) {
            l.onPrimaryClipChanged();
        }
    }

    public synchronized ClipData getPrimaryClip() {
        return primaryClip;
    }

    public synchronized boolean hasPrimaryClip() {
        return primaryClip != null;
    }

    public synchronized void clearPrimaryClip() {
        this.primaryClip = null;
    }

    public synchronized void addPrimaryClipChangedListener(OnPrimaryClipChangedListener listener) {
        if (listener != null && !listeners.contains(listener)) listeners.add(listener);
    }

    public synchronized void removePrimaryClipChangedListener(OnPrimaryClipChangedListener listener) {
        listeners.remove(listener);
    }
}
