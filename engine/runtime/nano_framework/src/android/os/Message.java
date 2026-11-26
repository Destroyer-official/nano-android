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

    public static Message obtain(Handler h) {
        Message m = new Message();
        m.target = h;
        return m;
    }

    public static Message obtain(Handler h, int what) {
        Message m = new Message();
        m.target = h;
        m.what = what;
        return m;
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

    public static Message obtain(Handler h, int what, int arg1, int arg2) {
        Message m = new Message();
        m.target = h;
        m.what = what;
        m.arg1 = arg1;
        m.arg2 = arg2;
        return m;
    }

    public static Message obtain(Handler h, int what, int arg1, int arg2, Object obj) {
        Message m = new Message();
        m.target = h;
        m.what = what;
        m.arg1 = arg1;
        m.arg2 = arg2;
        m.obj = obj;
        return m;
    }

    public static Message obtain(Message orig) {
        Message m = new Message();
        if (orig != null) {
            m.what = orig.what;
            m.arg1 = orig.arg1;
            m.arg2 = orig.arg2;
            m.obj = orig.obj;
            m.target = orig.target;
            m.callback = orig.callback;
        }
        return m;
    }

    public Handler getTarget() { return target; }
    public void setTarget(Handler target) { this.target = target; }

    public void sendToTarget() {
        if (target != null) {
            target.sendMessage(this);
        }
    }
}
