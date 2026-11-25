package android.os;

public class Build {
    public static final String MODEL = "Pixel 8 Pro (Nano-Android)";
    public static final String MANUFACTURER = "Google";
    public static final String BRAND = "google";
    public static final String DEVICE = "husky";
    public static final String BOARD = "shiba";
    public static final String HARDWARE = "tensor";
    public static final String FINGERPRINT = "google/husky/husky:14/UQ1A.240105.004/11202678:user/release-keys";
    public static final String[] SUPPORTED_ABIS = new String[]{"arm64-v8a", "armeabi-v7a", "x86_64"};
    public static final String[] SUPPORTED_32_BIT_ABIS = new String[]{"armeabi-v7a"};
    public static final String[] SUPPORTED_64_BIT_ABIS = new String[]{"arm64-v8a", "x86_64"};
    public static final String CPU_ABI = "arm64-v8a";
    public static final String CPU_ABI2 = "armeabi-v7a";
    public static final String SOC_MANUFACTURER = "Google";
    public static final String SOC_MODEL = "Tensor G3";

    public static class VERSION {
        public static final int SDK_INT = 35; // Android 15 / 16 preview
        public static final String RELEASE = "15";
        public static final String CODENAME = "VanillaIceCream";
    }

    public static class VERSION_CODES {
        public static final int BASE = 1;
        public static final int LOLLIPOP = 21;
        public static final int M = 23;
        public static final int N = 24;
        public static final int O = 26;
        public static final int P = 28;
        public static final int Q = 29;
        public static final int R = 30;
        public static final int S = 31;
        public static final int TIRAMISU = 33;
        public static final int UPSIDE_DOWN_CAKE = 34;
        public static final int VANILLA_ICE_CREAM = 35;
    }
}
