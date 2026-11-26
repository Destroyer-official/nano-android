package androidx.core.content;

import android.content.Context;
import android.net.Uri;
import java.io.File;
import java.io.IOException;
import java.nio.file.Path;

/**
 * Sandboxed androidx.core.content.FileProvider emitting contained content:// URIs backed by real files.
 */
public class FileProvider {
    public static Uri getUriForFile(Context context, String authority, File file) {
        if (context == null || authority == null || authority.trim().isEmpty() || file == null) {
            throw new IllegalArgumentException("Invalid FileProvider arguments");
        }
        try {
            Path canon = file.getCanonicalFile().toPath().normalize();
            File filesRoot = context.getFilesDir();
            File cacheRoot = context.getCacheDir();
            boolean inFiles = filesRoot != null && canon.startsWith(filesRoot.getCanonicalFile().toPath().normalize());
            boolean inCache = cacheRoot != null && canon.startsWith(cacheRoot.getCanonicalFile().toPath().normalize());
            boolean inAllowed = Uri.isWithinAllowedContentRoot(file);
            if (!inFiles && !inCache && !inAllowed) {
                throw new SecurityException("FileProvider path escapes configured app sandbox roots: " + canon);
            }
            String rel = file.getName();
            if (inFiles && filesRoot != null) {
                rel = "files/" + filesRoot.getCanonicalFile().toPath().normalize().relativize(canon).toString().replace('\\', '/');
            } else if (inCache && cacheRoot != null) {
                rel = "cache/" + cacheRoot.getCanonicalFile().toPath().normalize().relativize(canon).toString().replace('\\', '/');
            }
            String uriStr = "content://" + authority + "/" + rel;
            Uri.registerUriFile(uriStr, file.getCanonicalFile());
            return Uri.parse(uriStr);
        } catch (IOException e) {
            throw new IllegalArgumentException("Failed to canonicalize FileProvider path", e);
        }
    }
}
