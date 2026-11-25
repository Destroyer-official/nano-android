package android.content;

import android.net.Uri;
import android.database.Cursor;
import android.database.MatrixCursor;
import android.os.ParcelFileDescriptor;
import java.io.*;

public class ContentResolver {
    public Cursor query(Uri uri, String[] projection, String selection, String[] selectionArgs, String sortOrder) {
        File f = Uri.resolveRegisteredFile(uri);
        String name = (f != null) ? f.getName() : (uri != null ? uri.getLastPathSegment() : "file");
        long size = (f != null && f.exists()) ? f.length() : 0L;
        String mime = guessMimeType(name);
        String[] cols = (projection != null && projection.length > 0)
                ? projection
                : new String[]{"_display_name", "_size", "mime_type", "_data"};
        MatrixCursor mc = new MatrixCursor(cols);
        Object[] row = new Object[cols.length];
        for (int i = 0; i < cols.length; i++) {
            String c = cols[i];
            if ("_display_name".equals(c) || "title".equals(c)) row[i] = name;
            else if ("_size".equals(c)) row[i] = size;
            else if ("mime_type".equals(c)) row[i] = mime;
            else if ("_data".equals(c)) row[i] = (f != null ? f.getAbsolutePath() : uri.getPath());
            else if ("duration".equals(c)) row[i] = 15000L;
            else row[i] = "";
        }
        mc.addRow(row);
        return mc;
    }

    public InputStream openInputStream(Uri uri) throws FileNotFoundException {
        File f = Uri.resolveRegisteredFile(uri);
        if (f == null || !f.exists()) {
            throw new FileNotFoundException("Cannot open input stream for URI: " + uri);
        }
        return new FileInputStream(f);
    }

    public OutputStream openOutputStream(Uri uri) throws FileNotFoundException {
        File f = Uri.resolveRegisteredFile(uri);
        if (f == null && uri != null && uri.getPath() != null) {
            f = new File(uri.getPath());
        }
        if (f == null) {
            throw new FileNotFoundException("Cannot open output stream for URI: " + uri);
        }
        if (f.getParentFile() != null) f.getParentFile().mkdirs();
        return new FileOutputStream(f);
    }

    public ParcelFileDescriptor openFileDescriptor(Uri uri, String mode) throws FileNotFoundException {
        File f = Uri.resolveRegisteredFile(uri);
        int flags = (mode != null && mode.contains("w")) ? ParcelFileDescriptor.MODE_READ_WRITE : ParcelFileDescriptor.MODE_READ_ONLY;
        return ParcelFileDescriptor.open(f, flags);
    }

    public String getType(Uri uri) {
        File f = Uri.resolveRegisteredFile(uri);
        String name = (f != null) ? f.getName() : (uri != null ? uri.getLastPathSegment() : "");
        return guessMimeType(name);
    }

    public void takePersistableUriPermission(Uri uri, int modeFlags) {}
    public void releasePersistableUriPermission(Uri uri, int modeFlags) {}

    public Uri insert(Uri url, ContentValues values) { return url; }
    public int update(Uri uri, ContentValues values, String where, String[] selectionArgs) { return 1; }
    public int delete(Uri url, String where, String[] selectionArgs) { return 1; }

    private static String guessMimeType(String name) {
        if (name == null) return "application/octet-stream";
        String low = name.toLowerCase();
        if (low.endsWith(".mp4") || low.endsWith(".m4v") || low.endsWith(".mov")) return "video/mp4";
        if (low.endsWith(".mkv")) return "video/x-matroska";
        if (low.endsWith(".webm")) return "video/webm";
        if (low.endsWith(".mp3")) return "audio/mpeg";
        if (low.endsWith(".png")) return "image/png";
        if (low.endsWith(".jpg") || low.endsWith(".jpeg")) return "image/jpeg";
        if (low.endsWith(".pdf")) return "application/pdf";
        if (low.endsWith(".json")) return "application/json";
        if (low.endsWith(".txt")) return "text/plain";
        return "application/octet-stream";
    }
}
