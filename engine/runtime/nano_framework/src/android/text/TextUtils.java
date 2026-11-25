package android.text;

public class TextUtils {
    public enum TruncateAt {
        START, MIDDLE, END, MARQUEE
    }

    public static boolean isEmpty(CharSequence str) {
        return str == null || str.length() == 0;
    }
}
