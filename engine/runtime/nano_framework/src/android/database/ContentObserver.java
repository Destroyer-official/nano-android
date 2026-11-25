package android.database;

import android.net.Uri;
import android.os.Handler;

public abstract class ContentObserver {
    private Handler handler;

    public ContentObserver(Handler handler) {
        this.handler = handler;
    }

    public boolean deliverSelfNotifications() {
        return false;
    }

    public void onChange(boolean selfChange) {
    }

    public void onChange(boolean selfChange, Uri uri) {
        onChange(selfChange);
    }

    public void dispatchChange(boolean selfChange, Uri uri) {
        if (handler != null) {
            handler.post(() -> onChange(selfChange, uri));
        } else {
            onChange(selfChange, uri);
        }
    }
}
