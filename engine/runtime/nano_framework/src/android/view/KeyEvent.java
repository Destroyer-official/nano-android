package android.view;

public class KeyEvent {
    public static final int ACTION_DOWN = 0;
    public static final int ACTION_UP = 1;
    public static final int KEYCODE_BACK = 4;
    public static final int KEYCODE_MENU = 82;
    public static final int KEYCODE_ENTER = 66;

    private int action;
    private int keyCode;

    public KeyEvent(int action, int keyCode) {
        this.action = action;
        this.keyCode = keyCode;
    }

    public int getAction() { return action; }
    public int getKeyCode() { return keyCode; }
}
