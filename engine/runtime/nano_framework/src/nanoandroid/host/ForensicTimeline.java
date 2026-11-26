package nanoandroid.host;

import android.os.SystemClock;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicLong;

/**
 * Monotonic correlated forensic timeline capturing key Android micro-runtime
 * execution events (APK_LAUNCHED, ACTIVITY_CREATED, VIEW_INFLATED,
 * ACTION_DISPATCHED, CALLBACK_EXECUTED, DB_MUTATION, EXCEPTION_RAISED,
 * SCREENSHOT_CAPTURED, etc.).
 */
public final class ForensicTimeline {
    public static final class TimelineEvent {
        public final long seq;
        public final long timestampNs;
        public final long virtualUptimeMs;
        public final String eventType;
        public final String source;
        public final String backend;
        public final String correlationId;
        public final String details;

        public TimelineEvent(long seq, long timestampNs, long virtualUptimeMs, String eventType,
                String source, String backend, String correlationId, String details) {
            this.seq = seq;
            this.timestampNs = timestampNs;
            this.virtualUptimeMs = virtualUptimeMs;
            this.eventType = eventType != null ? eventType : "";
            this.source = source != null ? source : "";
            this.backend = backend != null ? backend : "nano_jvm";
            this.correlationId = correlationId != null ? correlationId : "";
            this.details = details != null ? details : "";
        }
    }

    private static final AtomicLong SEQ = new AtomicLong(0);
    private static final List<TimelineEvent> EVENTS = new CopyOnWriteArrayList<>();
    private static volatile String activeCorrelationId = "session-0";

    private ForensicTimeline() {}

    public static void clear(String correlationId) {
        EVENTS.clear();
        SEQ.set(0);
        if (correlationId != null && !correlationId.isEmpty()) {
            activeCorrelationId = correlationId;
        }
    }

    public static void record(String eventType, String source, String details) {
        record(eventType, source, "nano_jvm", activeCorrelationId, details);
    }

    public static void record(String eventType, String source, String backend, String correlationId, String details) {
        if (eventType == null || eventType.isEmpty()) return;
        EVENTS.add(new TimelineEvent(
                SEQ.incrementAndGet(),
                System.nanoTime(),
                SystemClock.uptimeMillis(),
                eventType,
                source,
                backend,
                correlationId != null && !correlationId.isEmpty() ? correlationId : activeCorrelationId,
                details));
    }

    public static List<TimelineEvent> getEvents() {
        return Collections.unmodifiableList(new ArrayList<>(EVENTS));
    }

    public static int size() {
        return EVENTS.size();
    }

    private static String escapeJson(String s) {
        if (s == null) return "";
        return s.replace("\\", "\\\\").replace("\"", "\\\"").replace("\n", "\\n")
                .replace("\r", "\\r").replace("\t", "\\t");
    }

    public static String toJson() {
        StringBuilder sb = new StringBuilder();
        sb.append("{\"status\":\"ok\",\"count\":").append(EVENTS.size()).append(",\"events\":[");
        boolean first = true;
        for (TimelineEvent ev : EVENTS) {
            if (!first) sb.append(",");
            first = false;
            sb.append("{\"seq\":").append(ev.seq)
                    .append(",\"timestamp_ns\":").append(ev.timestampNs)
                    .append(",\"virtual_uptime_ms\":").append(ev.virtualUptimeMs)
                    .append(",\"event_type\":\"").append(escapeJson(ev.eventType)).append("\"")
                    .append(",\"source\":\"").append(escapeJson(ev.source)).append("\"")
                    .append(",\"backend\":\"").append(escapeJson(ev.backend)).append("\"")
                    .append(",\"correlation_id\":\"").append(escapeJson(ev.correlationId)).append("\"")
                    .append(",\"details\":\"").append(escapeJson(ev.details)).append("\"}");
        }
        sb.append("]}");
        return sb.toString();
    }
}
