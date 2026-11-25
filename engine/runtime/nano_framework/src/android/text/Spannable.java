package android.text;

public interface Spannable extends Spanned {
    void setSpan(Object what, int start, int end, int flags);
    void removeSpan(Object what);
    void removeSpan(Object what, int flags);
}
