package android.view;

public interface WindowManager extends ViewManager {
    public static class LayoutParams extends ViewGroup.LayoutParams {
        public static final int FLAG_DRAWS_SYSTEM_BAR_BACKGROUNDS = 0x80000000;
        public static final int FLAG_TRANSLUCENT_STATUS = 0x04000000;
        public static final int FLAG_TRANSLUCENT_NAVIGATION = 0x08000000;
        public static final int FLAG_FULLSCREEN = 0x00000400;

        public int flags;

        public LayoutParams() { super(0, 0); }
        public LayoutParams(int width, int height) { super(width, height); }
    }

    public Display getDefaultDisplay();
}
