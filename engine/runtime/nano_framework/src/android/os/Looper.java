package android.os;

public class Looper {
    private static final ThreadLocal<Looper> threadLocal = new ThreadLocal<>();
    private static final Looper mainLooper = new Looper(Thread.currentThread());

    private final MessageQueue queue = new MessageQueue();
    private final Thread thread;

    private Looper(Thread thread) {
        this.thread = thread;
    }

    public static Looper getMainLooper() {
        return mainLooper;
    }

    public static Looper myLooper() {
        Looper l = threadLocal.get();
        return l != null ? l : mainLooper;
    }

    public static void prepare() {
        if (threadLocal.get() != null) {
            throw new RuntimeException("Only one Looper may be created per thread");
        }
        threadLocal.set(new Looper(Thread.currentThread()));
    }

    public MessageQueue getQueue() {
        return queue;
    }

    public Thread getThread() {
        return thread;
    }
}
