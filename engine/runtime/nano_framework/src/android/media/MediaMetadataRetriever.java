package android.media;

import android.content.Context;
import android.net.Uri;
import java.io.File;
import java.io.RandomAccessFile;
import java.nio.charset.StandardCharsets;

public class MediaMetadataRetriever {
    public static final int METADATA_KEY_DURATION = 9;
    public static final int METADATA_KEY_VIDEO_WIDTH = 18;
    public static final int METADATA_KEY_VIDEO_HEIGHT = 19;
    public static final int METADATA_KEY_BITRATE = 20;
    public static final int METADATA_KEY_MIMETYPE = 12;
    public static final int METADATA_KEY_VIDEO_ROTATION = 24;

    private File sourceFile;
    private long durationMs = 15000L;
    private int width = 1920;
    private int height = 1080;
    private long bitrate = 3200000L;
    private String mimeType = "video/mp4";

    public MediaMetadataRetriever() {}

    public void setDataSource(String path) {
        if (path != null && !path.isEmpty()) {
            this.sourceFile = new File(path);
            probeFile();
        }
    }

    public void setDataSource(Context context, Uri uri) {
        this.sourceFile = Uri.resolveRegisteredFile(uri);
        probeFile();
    }

    public void setDataSource(java.io.FileDescriptor fd) {}

    private void probeFile() {
        if (sourceFile == null || !sourceFile.exists()) return;
        String name = sourceFile.getName().toLowerCase();
        if (name.endsWith(".mkv")) mimeType = "video/x-matroska";
        else if (name.endsWith(".webm")) mimeType = "video/webm";
        else if (name.endsWith(".mov")) mimeType = "video/quicktime";

        long len = sourceFile.length();
        // Lightweight ISOBMFF/MP4 box parser for mvhd (duration) & tkhd (width/height)
        try (RandomAccessFile raf = new RandomAccessFile(sourceFile, "r")) {
            scanBoxes(raf, 0L, Math.min(len, 2L * 1024L * 1024L));
        } catch (Throwable ignored) {}

        if (len > 0 && durationMs > 0) {
            bitrate = Math.max(128000L, (len * 8L * 1000L) / durationMs);
        }
    }

    private void scanBoxes(RandomAccessFile raf, long start, long end) throws Exception {
        long pos = start;
        byte[] hdr = new byte[8];
        while (pos + 8 <= end) {
            raf.seek(pos);
            if (raf.read(hdr) != 8) break;
            long size = ((hdr[0] & 0xFFL) << 24) | ((hdr[1] & 0xFFL) << 16) | ((hdr[2] & 0xFFL) << 8) | (hdr[3] & 0xFFL);
            if (size < 8 || pos + size > end) break;
            String type = new String(hdr, 4, 4, StandardCharsets.US_ASCII);
            if ("moov".equals(type) || "trak".equals(type) || "mdia".equals(type)) {
                scanBoxes(raf, pos + 8, pos + size);
            } else if ("mvhd".equals(type) && size >= 32) {
                raf.seek(pos + 8);
                int version = raf.readUnsignedByte();
                raf.skipBytes(3);
                if (version == 1 && size >= 44) {
                    raf.skipBytes(16);
                    long timescale = raf.readInt() & 0xFFFFFFFFL;
                    long dur = raf.readLong();
                    if (timescale > 0 && dur > 0) durationMs = Math.max(1000L, (dur * 1000L) / timescale);
                } else {
                    raf.skipBytes(8);
                    long timescale = raf.readInt() & 0xFFFFFFFFL;
                    long dur = raf.readInt() & 0xFFFFFFFFL;
                    if (timescale > 0 && dur > 0) durationMs = Math.max(1000L, (dur * 1000L) / timescale);
                }
            } else if ("tkhd".equals(type) && size >= 92) {
                raf.seek(pos + 8);
                int version = raf.readUnsignedByte();
                raf.skipBytes(3);
                int skip = (version == 1) ? 80 : 68;
                if (size >= 8 + 4 + skip + 8) {
                    raf.skipBytes(skip);
                    int wFixed = raf.readInt();
                    int hFixed = raf.readInt();
                    int w = (wFixed >>> 16) & 0xFFFF;
                    int h = (hFixed >>> 16) & 0xFFFF;
                    if (w > 64 && h > 64 && w <= 7680 && h <= 4320) {
                        this.width = w;
                        this.height = h;
                    }
                }
            }
            pos += size;
        }
    }

    public String extractMetadata(int keyCode) {
        switch (keyCode) {
            case METADATA_KEY_DURATION: return String.valueOf(durationMs);
            case METADATA_KEY_VIDEO_WIDTH: return String.valueOf(width);
            case METADATA_KEY_VIDEO_HEIGHT: return String.valueOf(height);
            case METADATA_KEY_BITRATE: return String.valueOf(bitrate);
            case METADATA_KEY_MIMETYPE: return mimeType;
            case METADATA_KEY_VIDEO_ROTATION: return "0";
            default: return null;
        }
    }

    public void release() {}
}
