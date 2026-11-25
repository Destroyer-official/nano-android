package android.os;

public class Debug {
    public static class MemoryInfo {
        public int getTotalPss() { return 45 * 1024; }
    }
    public static void getMemoryInfo(MemoryInfo memoryInfo) {}
    public static long getNativeHeapAllocatedSize() { return 32L * 1024 * 1024; }
}
