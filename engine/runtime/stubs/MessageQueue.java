package android.os;

import java.util.Comparator;
import java.util.PriorityQueue;
import java.util.concurrent.atomic.AtomicLong;

public class MessageQueue {
    private static final class QueuedMessage {
        final Message message;
        final long when;
        final long sequence;

        QueuedMessage(Message message, long when, long sequence) {
            this.message = message;
            this.when = when;
            this.sequence = sequence;
        }
    }

    private final PriorityQueue<QueuedMessage> pq = new PriorityQueue<>(
        Comparator.comparingLong((QueuedMessage qm) -> qm.when)
                  .thenComparingLong(qm -> qm.sequence)
    );
    private final AtomicLong seqGen = new AtomicLong(0);
    private boolean isQuitting = false;

    public MessageQueue() {}

    public synchronized boolean enqueueMessage(Message msg, long when) {
        if (isQuitting) {
            return false;
        }
        msg.when = when;
        pq.add(new QueuedMessage(msg, when, seqGen.incrementAndGet()));
        return true;
    }

    public synchronized Message pollNextDueMessage(long maxUptimeMillis) {
        QueuedMessage peek = pq.peek();
        if (peek != null && peek.when <= maxUptimeMillis) {
            pq.poll();
            return peek.message;
        }
        return null;
    }

    public synchronized void removeMessages(Handler h, int what, Object obj) {
        pq.removeIf(qm -> qm.message.target == h && qm.message.what == what && (obj == null || qm.message.obj == obj));
    }

    public synchronized void removeCallbacks(Handler h, Runnable r) {
        pq.removeIf(qm -> qm.message.target == h && (r == null || qm.message.callback == r));
    }

    public synchronized int size() {
        return pq.size();
    }

    public synchronized void quit() {
        isQuitting = true;
        pq.clear();
    }
}
