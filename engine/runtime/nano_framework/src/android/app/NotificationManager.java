package android.app;

import java.util.*;
import java.util.concurrent.ConcurrentHashMap;
import nanoandroid.compat.ApiSupportLevel;
import nanoandroid.compat.NanoApiStatus;

@NanoApiStatus(
    level = ApiSupportLevel.PARTIAL,
    fallback = "android_art",
    notes = "In-process NotificationManager recording channels and posted notifications for assertions"
)
public class NotificationManager {
    public static final int IMPORTANCE_LOW = 2;
    public static final int IMPORTANCE_DEFAULT = 3;
    public static final int IMPORTANCE_HIGH = 4;

    public static final class PostedNotification {
        public final String tag;
        public final int id;
        public final Object notification;
        public final long postedAtMs;
        public PostedNotification(String tag, int id, Object notification, long postedAtMs) {
            this.tag = tag;
            this.id = id;
            this.notification = notification;
            this.postedAtMs = postedAtMs;
        }
    }

    private final Map<String, Object> channels = new ConcurrentHashMap<>();
    private final Map<String, PostedNotification> active = new ConcurrentHashMap<>();

    public void createNotificationChannel(Object channel) {
        if (channel != null) channels.put(String.valueOf(channel), channel);
    }

    public void notify(int id, Object notification) {
        notify(null, id, notification);
    }

    public void notify(String tag, int id, Object notification) {
        String key = (tag != null ? tag : "") + ":" + id;
        active.put(key, new PostedNotification(tag, id, notification, android.os.SystemClock.uptimeMillis()));
        LifecycleEventTracer.record("NOTIFICATION_POSTED", key, String.valueOf(notification));
    }

    public void cancel(int id) {
        cancel(null, id);
    }

    public void cancel(String tag, int id) {
        active.remove((tag != null ? tag : "") + ":" + id);
    }

    public void cancelAll() {
        active.clear();
    }

    public List<PostedNotification> getActiveNotificationsList() {
        return new ArrayList<>(active.values());
    }

    public boolean areNotificationsEnabled() { return true; }
}
