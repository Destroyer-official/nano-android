package android.text;

public interface InputFilter {
    CharSequence filter(CharSequence source, int start, int end, Spanned dest, int dstart, int dend);

    public static class LengthFilter implements InputFilter {
        private final int mMax;
        public LengthFilter(int max) { mMax = max; }
        public int getMax() { return mMax; }
        @Override
        public CharSequence filter(CharSequence source, int start, int end, Spanned dest, int dstart, int dend) {
            return null;
        }
    }
}
