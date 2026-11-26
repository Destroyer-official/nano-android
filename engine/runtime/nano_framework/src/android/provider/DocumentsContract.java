package android.provider;

import android.net.Uri;
import nanoandroid.compat.ApiSupportLevel;
import nanoandroid.compat.NanoApiStatus;

@NanoApiStatus(
    level = ApiSupportLevel.PARTIAL,
    fallback = "android_art",
    notes = "DocumentsContract URI document ID helpers for SAF and FileProvider interop"
)
public final class DocumentsContract {
    public static final String EXTRA_INITIAL_URI = "android.provider.extra.INITIAL_URI";

    public static final class Document {
        public static final String COLUMN_DOCUMENT_ID = "document_id";
        public static final String COLUMN_DISPLAY_NAME = "_display_name";
        public static final String COLUMN_MIME_TYPE = "mime_type";
        public static final String COLUMN_SIZE = "_size";
        public static final String MIME_TYPE_DIR = "vnd.android.document/directory";
    }

    private DocumentsContract() {}

    public static boolean isDocumentUri(android.content.Context context, Uri uri) {
        if (uri == null || !"content".equalsIgnoreCase(uri.getScheme())) return false;
        String path = uri.getPath();
        return path != null && (path.contains("/document/") || path.contains("/tree/"));
    }

    public static String getDocumentId(Uri documentUri) {
        if (documentUri == null) return "";
        String path = documentUri.getPath();
        if (path == null) return "";
        int idx = path.lastIndexOf("/document/");
        if (idx >= 0) {
            return path.substring(idx + "/document/".length());
        }
        return documentUri.getLastPathSegment();
    }
}
