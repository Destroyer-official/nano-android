package android.os;

public class Handler {
    private final Looper looper;

    public Handler() {
        this(Looper.myLooper());
    }

    public Handler(Looper looper) {
        this.looper = (looper != null) ? looper : Looper.getMainLooper();
    }

    public static Handler createAsync(Looper looper) {
        return new Handler(looper);
    }

    public static Handler createAsync(Looper looper, Callback callback) {
        return new Handler(looper);
    }

    public interface Callback {
        boolean handleMessage(Message msg);
    }

    public boolean post(Runnable r) {
        return postDelayed(r, 0);
    }

    public boolean postDelayed(Runnable r, long delayMillis) {
        Message msg = Message.obtain(this, r);
        return sendMessageDelayed(msg, delayMillis);
    }

    public boolean sendMessage(Message msg) {
        return sendMessageDelayed(msg, 0);
    }

    public boolean sendMessageDelayed(Message msg, long delayMillis) {
        msg.target = this;
        long targetTime = SystemClock.uptimeMillis() + delayMillis;
        return looper.getQueue().enqueueMessage(msg, targetTime);
    }

    public void dispatchMessage(Message msg) {
        if (msg.callback != null) {
            msg.callback.run();
        } else {
            handleMessage(msg);
        }
    }

    public void handleMessage(Message msg) {}

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
