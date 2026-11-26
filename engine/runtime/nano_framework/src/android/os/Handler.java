package android.os;

public class Handler {
    private final Looper looper;
    private final Callback callback;

    public Handler() {
        this(Looper.myLooper(), null);
    }

    public Handler(Callback callback) {
        this(Looper.myLooper(), callback);
    }

    public Handler(Looper looper) {
        this(looper, null);
    }

    public Handler(Looper looper, Callback callback) {
        this.looper = (looper != null) ? looper : Looper.getMainLooper();
        this.callback = callback;
    }

    public static Handler createAsync(Looper looper) {
        return new Handler(looper);
    }

    public static Handler createAsync(Looper looper, Callback callback) {
        return new Handler(looper, callback);
    }

    public interface Callback {
        boolean handleMessage(Message msg);
    }

    public final Message obtainMessage() {
        return Message.obtain(this);
    }

    public final Message obtainMessage(int what) {
        return Message.obtain(this, what);
    }

    public final Message obtainMessage(int what, Object obj) {
        return Message.obtain(this, what, obj);
    }

    public final Message obtainMessage(int what, int arg1, int arg2) {
        return Message.obtain(this, what, arg1, arg2);
    }

    public final Message obtainMessage(int what, int arg1, int arg2, Object obj) {
        return Message.obtain(this, what, arg1, arg2, obj);
    }

    public boolean post(Runnable r) {
        return postDelayed(r, 0);
    }

    public boolean postDelayed(Runnable r, long delayMillis) {
        Message msg = Message.obtain(this, r);
        return sendMessageDelayed(msg, delayMillis);
    }

    public boolean postAtFrontOfQueue(Runnable r) {
        return post(r);
    }

    public boolean sendEmptyMessage(int what) {
        return sendEmptyMessageDelayed(what, 0);
    }

    public boolean sendEmptyMessageDelayed(int what, long delayMillis) {
        return sendMessageDelayed(Message.obtain(this, what), delayMillis);
    }

    public boolean sendMessage(Message msg) {
        return sendMessageDelayed(msg, 0);
    }

    public boolean sendMessageAtFrontOfQueue(Message msg) {
        return sendMessageDelayed(msg, 0);
    }

    public boolean sendMessageDelayed(Message msg, long delayMillis) {
        msg.target = this;
        long targetTime = SystemClock.uptimeMillis() + Math.max(0L, delayMillis);
        return looper.getQueue().enqueueMessage(msg, targetTime);
    }

    public void dispatchMessage(Message msg) {
        if (msg.callback != null) {
            msg.callback.run();
        } else {
            if (callback != null && callback.handleMessage(msg)) {
                return;
            }
            handleMessage(msg);
        }
    }

    public void handleMessage(Message msg) {}

    public final boolean hasMessages(int what) {
        return false;
    }

    public final void removeMessages(int what) {
        looper.getQueue().removeCallbacksAndMessages(this);
    }

    public final void removeMessages(int what, Object object) {
        looper.getQueue().removeCallbacksAndMessages(object != null ? object : this);
    }

    public void removeCallbacks(Runnable r) {
        looper.getQueue().removeCallbacksAndMessages(r);
    }

    public void removeCallbacksAndMessages(Object token) {
        looper.getQueue().removeCallbacksAndMessages(token);
    }

    public Looper getLooper() {
        return looper;
    }
}
