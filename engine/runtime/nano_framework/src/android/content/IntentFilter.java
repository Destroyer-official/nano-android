package android.content;

import android.net.Uri;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;

public class IntentFilter {
    private final List<String> actions = new ArrayList<>();
    private final List<String> categories = new ArrayList<>();
    private final List<String> dataSchemes = new ArrayList<>();
    private final List<String> dataAuthorities = new ArrayList<>();
    private final List<String> dataPaths = new ArrayList<>();

    public IntentFilter() {}

    public IntentFilter(String action) {
        if (action != null) actions.add(action);
    }

    public void addAction(String action) {
        if (action != null && !actions.contains(action)) actions.add(action);
    }

    public boolean matchAction(String action) {
        return actions.contains(action);
    }

    public List<String> getActions() {
        return Collections.unmodifiableList(actions);
    }

    public void addCategory(String category) {
        if (category != null && !categories.contains(category)) categories.add(category);
    }

    public void addDataScheme(String scheme) {
        if (scheme != null && !dataSchemes.contains(scheme)) dataSchemes.add(scheme);
    }

    public void addDataAuthority(String host, String port) {
        if (host == null) return;
        String entry = (port != null && !port.isEmpty()) ? (host + ":" + port) : host;
        if (!dataAuthorities.contains(entry)) dataAuthorities.add(entry);
    }

    public void addDataPath(String path, int type) {
        if (path != null && !dataPaths.contains(path)) dataPaths.add(path);
    }

    public boolean match(Intent intent) {
        if (intent == null) return false;
        if (!actions.isEmpty()) {
            String act = intent.getAction();
            if (act == null || !actions.contains(act)) return false;
        }
        if (!dataSchemes.isEmpty()) {
            Uri data = intent.getData();
            String scheme = data != null ? data.getScheme() : null;
            if (scheme == null || !dataSchemes.contains(scheme)) return false;
        }
        if (!dataAuthorities.isEmpty()) {
            Uri data = intent.getData();
            String raw = data != null ? data.toString() : "";
            String host = extractAuthority(raw);
            if (host == null) return false;
            boolean hostOk = false;
            for (String auth : dataAuthorities) {
                String aHost = auth.contains(":") ? auth.substring(0, auth.indexOf(':')) : auth;
                if (aHost.equalsIgnoreCase(host) || "*".equals(aHost)) {
                    hostOk = true;
                    break;
                }
            }
            if (!hostOk) return false;
        }
        if (!dataPaths.isEmpty()) {
            Uri data = intent.getData();
            String path = data != null && data.getPath() != null ? data.getPath() : "";
            boolean pathOk = false;
            for (String p : dataPaths) {
                if (path.startsWith(p) || path.equals(p)) {
                    pathOk = true;
                    break;
                }
            }
            if (!pathOk) return false;
        }
        return true;
    }

    private static String extractAuthority(String uriStr) {
        int idx = uriStr.indexOf("://");
        if (idx < 0) return null;
        String rest = uriStr.substring(idx + 3);
        int slash = rest.indexOf('/');
        String auth = slash >= 0 ? rest.substring(0, slash) : rest;
        int colon = auth.indexOf(':');
        return colon >= 0 ? auth.substring(0, colon) : auth;
    }
}
