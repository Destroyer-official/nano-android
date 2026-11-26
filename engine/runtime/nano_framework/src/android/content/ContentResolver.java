package android.content;

import android.net.Uri;
import android.database.Cursor;
import android.database.MatrixCursor;
import android.os.ParcelFileDescriptor;
import java.io.*;
import java.util.*;
import java.util.concurrent.ConcurrentHashMap;
import nanoandroid.compat.ApiSupportLevel;
import nanoandroid.compat.NanoApiStatus;

@NanoApiStatus(
    level = ApiSupportLevel.BEHAVIORAL_DIFF,
    fallback = "android_art",
    notes = "Sandboxed File/MediaStore + local ContentProvider authority dispatch + persisted URI grants"
)
public class ContentResolver {
    private static final Map<String, ContentProvider> REGISTERED_PROVIDERS = new ConcurrentHashMap<>();
    private static final Map<String, Integer> PERSISTED_URI_PERMISSIONS = new ConcurrentHashMap<>();

    public static void registerProvider(String authority, ContentProvider provider) {
        if (authority != null && !authority.trim().isEmpty() && provider != null) {
            REGISTERED_PROVIDERS.put(authority.trim(), provider);
        }
    }

    public static ContentProvider getProvider(String authority) {
        return authority != null ? REGISTERED_PROVIDERS.get(authority) : null;
    }

    public static void clearProviders() {
        REGISTERED_PROVIDERS.clear();
        PERSISTED_URI_PERMISSIONS.clear();
    }

    public static Map<String, Integer> getPersistedUriPermissions() {
        return Collections.unmodifiableMap(new LinkedHashMap<>(PERSISTED_URI_PERMISSIONS));
    }

    private static ContentProvider resolveProviderForUri(Uri uri) {
        if (uri == null || !"content".equalsIgnoreCase(uri.getScheme())) return null;
        String s = uri.toString();
        int idx = s.indexOf("://");
        if (idx < 0) return null;
        String rest = s.substring(idx + 3);
        int slash = rest.indexOf('/');
        String auth = slash >= 0 ? rest.substring(0, slash) : rest;
        return REGISTERED_PROVIDERS.get(auth);
    }

    public Cursor query(Uri uri, String[] projection, String selection, String[] selectionArgs, String sortOrder) {
        ContentProvider provider = resolveProviderForUri(uri);
        if (provider != null) {
            return provider.query(uri, projection, selection, selectionArgs, sortOrder);
        }
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
            File cand = new File(uri.getPath());
            try {
                if (Uri.isWithinAllowedContentRoot(cand)) {
                    f = cand;
                } else {
                    throw new SecurityException("ContentResolver output path escapes sandbox: " + uri);
                }
            } catch (IOException e) {
                throw new SecurityException("Invalid ContentResolver output path: " + uri, e);
            }
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
        ContentProvider provider = resolveProviderForUri(uri);
        if (provider != null) {
            return provider.getType(uri);
        }
        File f = Uri.resolveRegisteredFile(uri);
        String name = (f != null) ? f.getName() : (uri != null ? uri.getLastPathSegment() : "");
        return guessMimeType(name);
    }

    public void takePersistableUriPermission(Uri uri, int modeFlags) {
        if (uri == null) return;
        int valid = modeFlags & (Intent.FLAG_GRANT_READ_URI_PERMISSION | Intent.FLAG_GRANT_WRITE_URI_PERMISSION);
        PERSISTED_URI_PERMISSIONS.merge(uri.toString(), valid, (a, b) -> a | b);
    }

    public void releasePersistableUriPermission(Uri uri, int modeFlags) {
        if (uri == null) return;
        PERSISTED_URI_PERMISSIONS.computeIfPresent(uri.toString(), (k, cur) -> {
            int rem = cur & ~modeFlags;
            return rem == 0 ? null : rem;
        });
    }

    public boolean hasPersistedUriPermission(Uri uri, int modeFlags) {
        if (uri == null) return false;
        Integer cur = PERSISTED_URI_PERMISSIONS.get(uri.toString());
        return cur != null && (cur & modeFlags) == modeFlags;
    }

    public Uri insert(Uri url, ContentValues values) {
        ContentProvider provider = resolveProviderForUri(url);
        if (provider != null) {
            return provider.insert(url, values);
        }
        return url;
    }

    public int update(Uri uri, ContentValues values, String where, String[] selectionArgs) {
        ContentProvider provider = resolveProviderForUri(uri);
        if (provider != null) {
            return provider.update(uri, values, where, selectionArgs);
        }
        return 1;
    }

    public int delete(Uri url, String where, String[] selectionArgs) {
        ContentProvider provider = resolveProviderForUri(url);
        if (provider != null) {
            return provider.delete(url, where, selectionArgs);
        }
        return 1;
    }

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
