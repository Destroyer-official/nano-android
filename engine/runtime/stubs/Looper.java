package android.os;

public class Looper {
    private static final ThreadLocal<Looper> sThreadLocal = new ThreadLocal<>();
    private static Looper sMainLooper;

    private final MessageQueue mQueue;
    private final Thread mThread;

    private Looper() {
        this.mQueue = new MessageQueue();
        this.mThread = Thread.currentThread();
    }

    public static synchronized void prepare() {
        if (sThreadLocal.get() != null) {
            throw new RuntimeException("Only one Looper may be created per thread");
        }
        Looper l = new Looper();
        sThreadLocal.set(l);
        if (sMainLooper == null) {
            sMainLooper = l;
        }
    }

    public static synchronized void prepareMainLooper() {
        prepare();
        sMainLooper = myLooper();
    }

    public static synchronized Looper getMainLooper() {
        if (sMainLooper == null) {
            prepareMainLooper();
        }
        return sMainLooper;
    }

    public static synchronized void setMainLooper(Looper looper) {
        sMainLooper = looper;
    }

    public static Looper myLooper() {
        return sThreadLocal.get();
    }

    public MessageQueue getQueue() {
        return mQueue;
    }

    public Thread getThread() {
        return mThread;
    }

    public void quit() {
        mQueue.quit();
    }
}
