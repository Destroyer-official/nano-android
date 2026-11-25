package android.media;

public class MediaCodecInfo {
    public String getName() { return "c2.android.avc.encoder"; }
    public boolean isEncoder() { return true; }
    public String[] getSupportedTypes() { return new String[]{"video/avc", "video/hevc"}; }
}
