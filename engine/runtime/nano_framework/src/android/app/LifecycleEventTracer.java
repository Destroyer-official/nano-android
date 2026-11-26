package android.app;

import android.os.SystemClock;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;

/**
 * Records deterministic ordered Android component lifecycle transitions across
 * ContentProvider, Application, Activity, Service, and BroadcastReceiver.
 */
public final class LifecycleEventTracer {
    public static final class Event {
        public final String eventType;
        public final String component;
        public final String details;
        public final long virtualUptimeMs;

        public Event(String eventType, String component, String details, long virtualUptimeMs) {
            this.eventType = eventType;
            this.component = component != null ? component : "";
            this.details = details != null ? details : "";
            this.virtualUptimeMs = virtualUptimeMs;
        }
    }

    private static final List<Event> EVENTS = new CopyOnWriteArrayList<>();

    private LifecycleEventTracer() {}

    public static void clear() {
        EVENTS.clear();
    }

    public static void record(String eventType, String component, String details) {
        if (eventType == null || eventType.isEmpty()) return;
        EVENTS.add(new Event(eventType, component, details, SystemClock.uptimeMillis()));
    }

    public static List<Event> getEvents() {
        return Collections.unmodifiableList(new ArrayList<>(EVENTS));
    }
}
