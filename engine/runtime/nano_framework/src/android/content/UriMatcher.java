package android.content;

import android.net.Uri;
import java.util.ArrayList;
import java.util.List;

public class UriMatcher {
    public static final int NO_MATCH = -1;

    private static final class Rule {
        final String authority;
        final String[] segments;
        final int code;

        Rule(String authority, String[] segments, int code) {
            this.authority = authority;
            this.segments = segments;
            this.code = code;
        }
    }

    private final int rootCode;
    private final List<Rule> rules = new ArrayList<>();

    public UriMatcher(int code) {
        this.rootCode = code;
    }

    public void addURI(String authority, String path, int code) {
        if (code < 0) {
            throw new IllegalArgumentException("code " + code + " is invalid: must be non-negative");
        }
        String cleanPath = path == null ? "" : path;
        if (cleanPath.startsWith("/")) cleanPath = cleanPath.substring(1);
        String[] segs = cleanPath.isEmpty() ? new String[0] : cleanPath.split("/");
        rules.add(new Rule(authority, segs, code));
    }

    public int match(Uri uri) {
        if (uri == null) return NO_MATCH;
        String s = uri.toString();
        String auth = null;
        int idx = s.indexOf("://");
        if (idx >= 0) {
            String rest = s.substring(idx + 3);
            int slash = rest.indexOf('/');
            auth = slash >= 0 ? rest.substring(0, slash) : rest;
        }
        String rawPath = uri.getPath();
        if (rawPath == null || rawPath.isEmpty()) {
            if (auth == null) return rootCode;
        }
        String trimmed = rawPath == null ? "" : rawPath;
        if (trimmed.startsWith("/")) trimmed = trimmed.substring(1);
        String[] pathSegs = trimmed.isEmpty() ? new String[0] : trimmed.split("/");

        for (Rule r : rules) {
            if (r.authority != null && !r.authority.equals(auth)) continue;
            if (r.segments.length != pathSegs.length) continue;
            boolean ok = true;
            for (int i = 0; i < r.segments.length; i++) {
                String pat = r.segments[i];
                String val = pathSegs[i];
                if ("*".equals(pat)) continue;
                if ("#".equals(pat)) {
                    if (!isNumeric(val)) { ok = false; break; }
                    continue;
                }
                if (!pat.equals(val)) { ok = false; break; }
            }
            if (ok) return r.code;
        }
        return NO_MATCH;
    }

    private static boolean isNumeric(String s) {
        if (s == null || s.isEmpty()) return false;
        for (int i = 0; i < s.length(); i++) {
            if (!Character.isDigit(s.charAt(i))) return false;
        }
        return true;
    }
}
