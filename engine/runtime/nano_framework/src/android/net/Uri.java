package android.net;

import java.io.File;
import java.io.IOException;
import java.net.URI;
import java.nio.file.Path;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

public class Uri implements android.os.Parcelable {
    private static final Map<String, File> REGISTERED_FILES = new ConcurrentHashMap<>();
    private final String uriString;

    private Uri(String uriString) {
        this.uriString = uriString != null ? uriString : "";
    }

    public static Uri parse(String uriString) {
        return new Uri(uriString);
    }

    public static Uri fromFile(File file) {
        if (file == null) return new Uri("");
        String str = file.toURI().toString();
        REGISTERED_FILES.put(str, file);
        return new Uri(str);
    }

    public static void registerUriFile(String uriString, File file) {
        if (uriString != null && file != null) {
            REGISTERED_FILES.put(uriString, file);
        }
    }

    public static File resolveRegisteredFile(Uri uri) {
        if (uri == null) return null;
        String s = uri.toString();
        File reg = REGISTERED_FILES.get(s);
        if (reg != null && reg.exists()) return reg;

        // Only allow direct path resolution if strictly contained inside sandbox or staged_files.
        String p = uri.getPath();
        if (p != null && !p.isEmpty()) {
            try {
                File f = new File(p);
                if (f.exists() && isWithinAllowedContentRoot(f)) {
                    return f;
                }
            } catch (IOException ignored) {}
        }
        return reg;
    }

    public static boolean isWithinAllowedContentRoot(File f) throws IOException {
        if (f == null) return false;
        Path canonFile = f.getCanonicalFile().toPath().normalize();
        String sandboxProp = System.getProperty("nano.sandbox.dir");
        File sandboxRoot = (sandboxProp != null && !sandboxProp.trim().isEmpty())
                ? new File(sandboxProp)
                : new File("cache", "sandbox");
        File stagedRoot = new File("cache", "staged_files");
        if (canonFile.startsWith(sandboxRoot.getCanonicalFile().toPath().normalize())) {
            return true;
        }
        if (stagedRoot.exists() && canonFile.startsWith(stagedRoot.getCanonicalFile().toPath().normalize())) {
            return true;
        }
        return false;
    }

    public String getScheme() {
        if (uriString == null) return null;
        int idx = uriString.indexOf(':');
        return idx != -1 ? uriString.substring(0, idx) : null;
    }

    public String getPath() {
        if (uriString == null) return null;
        if (uriString.startsWith("file:")) {
            try {
                return new File(new URI(uriString)).getAbsolutePath();
            } catch (Throwable ignored) {
                String s = uriString.replaceFirst("^file:/+", "");
                if (!s.contains(":") && !s.startsWith("/")) s = "/" + s;
                return s;
            }
        }
        int q = uriString.indexOf('?');
        String clean = q >= 0 ? uriString.substring(0, q) : uriString;
        int schemeIdx = clean.indexOf("://");
        if (schemeIdx >= 0) {
            int slash = clean.indexOf('/', schemeIdx + 3);
            return slash >= 0 ? clean.substring(slash) : "";
        }
        return clean;
    }

    public String getLastPathSegment() {
        String p = getPath();
        if (p == null || p.isEmpty()) return "";
        int s1 = p.lastIndexOf('/');
        int s2 = p.lastIndexOf('\\');
        int cut = Math.max(s1, s2);
        return cut >= 0 && cut < p.length() - 1 ? p.substring(cut + 1) : p;
    }

    @Override public int describeContents() { return 0; }
    @Override public void writeToParcel(android.os.Parcel dest, int flags) {}

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (!(o instanceof Uri)) return false;
        return uriString.equals(((Uri) o).uriString);
    }

    @Override
    public int hashCode() { return uriString.hashCode(); }

    @Override
    public String toString() { return uriString; }
}
