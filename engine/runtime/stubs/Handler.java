package android.os;

public class Handler {
    public interface Callback {
        boolean handleMessage(Message msg);
    }

    private final Looper mLooper;
    private final MessageQueue mQueue;
    private final Callback mCallback;

    public Handler() {
        this(Looper.myLooper() != null ? Looper.myLooper() : Looper.getMainLooper(), null);
    }

    public Handler(Looper looper) {
        this(looper, null);
    }

    public Handler(Looper looper, Callback callback) {
        this.mLooper = looper;
        this.mQueue = looper != null ? looper.getQueue() : new MessageQueue();
        this.mCallback = callback;
    }

    public Looper getLooper() {
        return mLooper;
    }

    public final boolean post(Runnable r) {
        return postDelayed(r, 0);
    }

    public final boolean postDelayed(Runnable r, long delayMillis) {
        long uptime = SystemClock.uptimeMillis() + delayMillis;
        return postAtTime(r, uptime);
    }

    public final boolean postAtTime(Runnable r, long uptimeMillis) {
        Message msg = Message.obtain(this, r);
        return mQueue.enqueueMessage(msg, uptimeMillis);
    }

    public final boolean sendMessage(Message msg) {
        return sendMessageDelayed(msg, 0);
    }

    public final boolean sendMessageDelayed(Message msg, long delayMillis) {
        long uptime = SystemClock.uptimeMillis() + delayMillis;
        return sendMessageAtTime(msg, uptime);
    }

    public boolean sendMessageAtTime(Message msg, long uptimeMillis) {
        msg.target = this;
        return mQueue.enqueueMessage(msg, uptimeMillis);
    }

    public final void removeCallbacks(Runnable r) {
        mQueue.removeCallbacks(this, r);
    }

    public final void removeMessages(int what) {
        mQueue.removeMessages(this, what, null);
    }

    public void dispatchMessage(Message msg) {
        if (msg.callback != null) {
            msg.callback.run();
            return;
        }
        if (mCallback != null && mCallback.handleMessage(msg)) {
            return;
        }
        handleMessage(msg);
    }

    public void handleMessage(Message msg) {
        // Subclasses override
    }
}
