package android.os;

/**
 * Thread with an attached deterministic Looper/MessageQueue matching Android HandlerThread.
 */
public class HandlerThread extends Thread {
    private Looper looper;
    private int priority = 0;

    public HandlerThread(String name) {
        super(name);
    }

    public HandlerThread(String name, int priority) {
        super(name);
        this.priority = priority;
    }

    protected void onLooperPrepared() {}

    @Override
    public synchronized void start() {
        ensureLooper();
    }

    private synchronized void ensureLooper() {
        if (looper == null) {
            looper = Looper.createNamedLooper(getName() + "-" + priority);
            onLooperPrepared();
        }
    }

    public Looper getLooper() {
        ensureLooper();
        return looper;
    }

    public boolean quit() {
        Looper l = getLooper();
        return l != null && l.quit(false);
    }

    public boolean quitSafely() {
        Looper l = getLooper();
        return l != null && l.quit(true);
    }
}
