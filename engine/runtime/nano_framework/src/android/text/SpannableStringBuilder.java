package android.text;

import java.lang.reflect.Array;

public class SpannableStringBuilder implements CharSequence, Appendable, Editable, Spannable {
    private final StringBuilder builder;
    private InputFilter[] filters = new InputFilter[0];

    public SpannableStringBuilder() {
        this.builder = new StringBuilder();
    }

    public SpannableStringBuilder(CharSequence text) {
        this.builder = new StringBuilder(text != null ? text.toString() : "");
    }

    public SpannableStringBuilder(CharSequence text, int start, int end) {
        this.builder = new StringBuilder(text != null ? text.subSequence(start, end).toString() : "");
    }

    public static SpannableStringBuilder valueOf(CharSequence source) {
        if (source instanceof SpannableStringBuilder) {
            return (SpannableStringBuilder) source;
        }
        return new SpannableStringBuilder(source);
    }

    @Override
    public int length() {
        return builder.length();
    }

    @Override
    public char charAt(int index) {
        return builder.charAt(index);
    }

    @Override
    public CharSequence subSequence(int start, int end) {
        return builder.subSequence(start, end);
    }

    @Override
    public String toString() {
        return builder.toString();
    }

    @Override
    public SpannableStringBuilder append(CharSequence text) {
        if (text != null) builder.append(text);
        return this;
    }

    @Override
    public SpannableStringBuilder append(CharSequence text, int start, int end) {
        if (text != null) builder.append(text, start, end);
        return this;
    }

    @Override
    public SpannableStringBuilder append(char text) {
        builder.append(text);
        return this;
    }

    @Override
    public SpannableStringBuilder replace(int st, int en, CharSequence source, int start, int end) {
        if (source != null) {
            builder.replace(st, en, source.subSequence(start, end).toString());
        } else {
            builder.delete(st, en);
        }
        return this;
    }

    @Override
    public SpannableStringBuilder replace(int st, int en, CharSequence text) {
        if (text != null) {
            builder.replace(st, en, text.toString());
        } else {
            builder.delete(st, en);
        }
        return this;
    }

    @Override
    public SpannableStringBuilder insert(int where, CharSequence text, int start, int end) {
        if (text != null) {
            builder.insert(where, text.subSequence(start, end).toString());
        }
        return this;
    }

    @Override
    public SpannableStringBuilder insert(int where, CharSequence text) {
        if (text != null) {
            builder.insert(where, text.toString());
        }
        return this;
    }

    @Override
    public SpannableStringBuilder delete(int st, int en) {
        builder.delete(st, en);
        return this;
    }

    @Override
    public void clear() {
        builder.setLength(0);
    }

    @Override
    public void clearSpans() {}

    @Override
    public void setSpan(Object what, int start, int end, int flags) {}

    @Override
    public void removeSpan(Object what) {}

    @Override
    public void removeSpan(Object what, int flags) {}

    @Override
    @SuppressWarnings("unchecked")
    public <T> T[] getSpans(int start, int end, Class<T> type) {
        return (T[]) Array.newInstance(type, 0);
    }

    @Override
    public int getSpanStart(Object tag) { return -1; }

    @Override
    public int getSpanEnd(Object tag) { return -1; }

    @Override
    public int getSpanFlags(Object tag) { return 0; }

    @Override
    public int nextSpanTransition(int start, int limit, Class type) { return limit; }

    @Override
    public void setFilters(InputFilter[] filters) {
        this.filters = filters != null ? filters : new InputFilter[0];
    }

    @Override
    public InputFilter[] getFilters() { return filters; }
}
