package android.os;

public class Message {
    public int what;
    public int arg1;
    public int arg2;
    public Object obj;
    public long when;
    public Handler target;
    public Runnable callback;

    public Message() {}

    public static Message obtain() {
        return new Message();
    }

    public static Message obtain(Handler h, Runnable callback) {
        Message m = new Message();
        m.target = h;
        m.callback = callback;
        return m;
    }

    public static Message obtain(Handler h, int what, Object obj) {
        Message m = new Message();
        m.target = h;
        m.what = what;
        m.obj = obj;
        return m;
    }

    public void sendToTarget() {
        if (target != null) {
            target.sendMessage(this);
        }
    }
}
