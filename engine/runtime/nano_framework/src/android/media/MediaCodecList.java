package android.media;

public class MediaCodecList {
    public static final int REGULAR_CODECS = 0;
    public static final int ALL_CODECS = 1;

    public MediaCodecList(int kind) {}
    public MediaCodecInfo[] getCodecInfos() {
        return new MediaCodecInfo[]{new MediaCodecInfo()};
    }
}
