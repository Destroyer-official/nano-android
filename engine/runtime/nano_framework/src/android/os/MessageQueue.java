package android.os;

import java.util.PriorityQueue;
import java.util.Comparator;

public class MessageQueue {
    private final PriorityQueue<Message> queue = new PriorityQueue<>(Comparator.comparingLong(m -> m.when));

    public synchronized boolean enqueueMessage(Message msg, long when) {
        msg.when = when;
        return queue.offer(msg);
    }

    public synchronized Message pollNextDueMessage(long currentUptime) {
        Message head = queue.peek();
        if (head != null && head.when <= currentUptime) {
            return queue.poll();
        }
        return null;
    }

    public synchronized boolean removeCallbacksAndMessages(Object token) {
        return queue.removeIf(m -> m.obj == token || m.callback == token);
    }

    public synchronized int size() {
        return queue.size();
    }
}
