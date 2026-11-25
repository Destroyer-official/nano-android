package android.os;

public class StatFs {
    public StatFs(String path) {}
    public long getAvailableBytes() { return 64L * 1024 * 1024 * 1024; }
    public long getTotalBytes() { return 128L * 1024 * 1024 * 1024; }
    public long getAvailableBlocksLong() { return 16 * 1024 * 1024; }
    public long getBlockSizeLong() { return 4096; }
}
