package android.text;

import java.lang.reflect.Array;

public class SpannableString implements CharSequence, Spannable {
    private final String text;

    public SpannableString(CharSequence source) {
        this.text = source != null ? source.toString() : "";
    }

    public static SpannableString valueOf(CharSequence source) {
        if (source instanceof SpannableString) {
            return (SpannableString) source;
        }
        return new SpannableString(source);
    }

    @Override
    public int length() { return text.length(); }
    @Override
    public char charAt(int index) { return text.charAt(index); }
    @Override
    public CharSequence subSequence(int start, int end) { return text.subSequence(start, end); }
    @Override
    public String toString() { return text; }

    @Override public void setSpan(Object what, int start, int end, int flags) {}
    @Override public void removeSpan(Object what) {}
    @Override public void removeSpan(Object what, int flags) {}
    @Override
    @SuppressWarnings("unchecked")
    public <T> T[] getSpans(int start, int end, Class<T> type) {
        return (T[]) Array.newInstance(type, 0);
    }
    @Override public int getSpanStart(Object tag) { return -1; }
    @Override public int getSpanEnd(Object tag) { return -1; }
    @Override public int getSpanFlags(Object tag) { return 0; }
    @Override public int nextSpanTransition(int start, int limit, Class type) { return limit; }
}
