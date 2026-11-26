package nanoandroid.compat;

import java.io.File;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.Statement;

/**
 * CLI/stdin JSON-over-SQLite tool backed by org.xerial:sqlite-jdbc:3.54.1.0
 * managing Nano-Android's persistent compatibility.db (contracts, API 26-37
 * matrix, device/OEM quirks, and continuous conformance/differential evidence).
 */
public final class CompatibilityDbTool {
    private CompatibilityDbTool() {}

    public static void initSchema(Connection conn) throws Exception {
        try (Statement st = conn.createStatement()) {
            st.execute("PRAGMA journal_mode=WAL;");
            st.execute("PRAGMA foreign_keys=ON;");
            st.execute("CREATE TABLE IF NOT EXISTS contracts (" +
                    "api TEXT PRIMARY KEY, " +
                    "level TEXT NOT NULL, " +
                    "since_api INTEGER NOT NULL, " +
                    "until_api INTEGER NOT NULL, " +
                    "fallback_backend TEXT NOT NULL, " +
                    "notes TEXT NOT NULL, " +
                    "pass_count INTEGER NOT NULL DEFAULT 0, " +
                    "fail_count INTEGER NOT NULL DEFAULT 0, " +
                    "diverge_count INTEGER NOT NULL DEFAULT 0, " +
                    "confidence REAL NOT NULL DEFAULT 0.5)");
            st.execute("CREATE TABLE IF NOT EXISTS api_matrix (" +
                    "api TEXT NOT NULL, " +
                    "api_level INTEGER NOT NULL, " +
                    "status TEXT NOT NULL, " +
                    "evidence_count INTEGER NOT NULL DEFAULT 0, " +
                    "confidence REAL NOT NULL DEFAULT 0.5, " +
                    "PRIMARY KEY (api, api_level))");
            st.execute("CREATE TABLE IF NOT EXISTS evidence_runs (" +
                    "id INTEGER PRIMARY KEY AUTOINCREMENT, " +
                    "run_kind TEXT NOT NULL, " +
                    "api TEXT NOT NULL, " +
                    "method_name TEXT NOT NULL DEFAULT '', " +
                    "backend TEXT NOT NULL, " +
                    "api_level INTEGER NOT NULL, " +
                    "passed INTEGER NOT NULL, " +
                    "details TEXT NOT NULL DEFAULT '', " +
                    "recorded_at TEXT NOT NULL DEFAULT (datetime('now')))");
            st.execute("CREATE TABLE IF NOT EXISTS device_quirks (" +
                    "profile_id TEXT NOT NULL, " +
                    "oem_or_formfactor TEXT NOT NULL, " +
                    "api TEXT NOT NULL, " +
                    "quirk_summary TEXT NOT NULL, " +
                    "severity TEXT NOT NULL, " +
                    "PRIMARY KEY (profile_id, api, quirk_summary))");
        }
    }

    public static void main(String[] args) throws Exception {
        if (args.length < 2) {
            System.err.println("Usage: CompatibilityDbTool <dbPath> <init|upsert_contract|record_evidence|upsert_quirk|heatmap>");
            System.exit(2);
        }
        File dbFile = new File(args[0]).getCanonicalFile();
        if (dbFile.getParentFile() != null) {
            dbFile.getParentFile().mkdirs();
        }
        Class.forName("org.sqlite.JDBC");
        String cmd = args[1];
        try (Connection conn = DriverManager.getConnection("jdbc:sqlite:" + dbFile.getAbsolutePath())) {
            initSchema(conn);
            switch (cmd) {
                case "init":
                    System.out.println("{\"status\":\"ok\",\"db\":\"" + esc(dbFile.getAbsolutePath()) + "\"}");
                    break;
                case "upsert_contract": {
                    // args: <db> upsert_contract <api> <level> <since> <until> <fallback> <notes> <confidence>
                    String api = args[2], level = args[3];
                    int since = Integer.parseInt(args[4]), until = Integer.parseInt(args[5]);
                    String fallback = args.length > 6 ? args[6] : "";
                    String notes = args.length > 7 ? args[7] : "";
                    double conf = args.length > 8 ? Double.parseDouble(args[8]) : 0.75;
                    conn.setAutoCommit(false);
                    try (PreparedStatement ps = conn.prepareStatement(
                            "INSERT INTO contracts(api, level, since_api, until_api, fallback_backend, notes, confidence) VALUES(?,?,?,?,?,?,?) " +
                                    "ON CONFLICT(api) DO UPDATE SET level=excluded.level, since_api=excluded.since_api, until_api=excluded.until_api, fallback_backend=excluded.fallback_backend, notes=excluded.notes")) {
                        ps.setString(1, api);
                        ps.setString(2, level);
                        ps.setInt(3, since);
                        ps.setInt(4, until);
                        ps.setString(5, fallback);
                        ps.setString(6, notes);
                        ps.setDouble(7, conf);
                        ps.executeUpdate();
                    }
                    try (PreparedStatement mx = conn.prepareStatement(
                            "INSERT INTO api_matrix(api, api_level, status, evidence_count, confidence) VALUES(?,?,?,?,?) " +
                                    "ON CONFLICT(api, api_level) DO UPDATE SET status=excluded.status")) {
                        for (int lvl = Math.max(26, since); lvl <= Math.min(37, until); lvl++) {
                            mx.setString(1, api);
                            mx.setInt(2, lvl);
                            mx.setString(3, level);
                            mx.setInt(4, 1);
                            mx.setDouble(5, conf);
                            mx.addBatch();
                        }
                        mx.executeBatch();
                    }
                    conn.commit();
                    System.out.println("{\"status\":\"ok\",\"api\":\"" + esc(api) + "\"}");
                    break;
                }
                case "record_evidence": {
                    // args: <db> record_evidence <run_kind> <api> <method> <backend> <api_level> <passed 0|1> <details>
                    String kind = args[2], api = args[3], method = args[4], backend = args[5];
                    int apiLvl = Integer.parseInt(args[6]);
                    int passed = Integer.parseInt(args[7]);
                    String details = args.length > 8 ? args[8] : "";
                    conn.setAutoCommit(false);
                    try (PreparedStatement ps = conn.prepareStatement(
                            "INSERT INTO evidence_runs(run_kind, api, method_name, backend, api_level, passed, details) VALUES(?,?,?,?,?,?,?)")) {
                        ps.setString(1, kind);
                        ps.setString(2, api);
                        ps.setString(3, method);
                        ps.setString(4, backend);
                        ps.setInt(5, apiLvl);
                        ps.setInt(6, passed);
                        ps.setString(7, details);
                        ps.executeUpdate();
                    }
                    // Ensure contract row exists
                    try (PreparedStatement ensure = conn.prepareStatement(
                            "INSERT OR IGNORE INTO contracts(api, level, since_api, until_api, fallback_backend, notes, confidence) VALUES(?, 'PARTIAL', 26, 37, 'android_art', 'Discovered via continuous verification', 0.5)")) {
                        ensure.setString(1, api);
                        ensure.executeUpdate();
                    }
                    String col = passed == 1 ? "pass_count" : ("differential".equalsIgnoreCase(kind) ? "diverge_count" : "fail_count");
                    try (PreparedStatement upd = conn.prepareStatement(
                            "UPDATE contracts SET " + col + " = " + col + " + 1, " +
                                    "confidence = MIN(1.0, MAX(0.05, CAST(pass_count + (CASE WHEN ?=1 THEN 1 ELSE 0 END) + 1.0 AS REAL) / (pass_count + fail_count + diverge_count + 2.0))) " +
                                    "WHERE api = ?")) {
                        upd.setInt(1, passed);
                        upd.setString(2, api);
                        upd.executeUpdate();
                    }
                    try (PreparedStatement mx = conn.prepareStatement(
                            "INSERT INTO api_matrix(api, api_level, status, evidence_count, confidence) VALUES(?, ?, ?, 1, ?) " +
                                    "ON CONFLICT(api, api_level) DO UPDATE SET " +
                                    "status = CASE WHEN ?=0 AND 'differential'=? THEN 'BEHAVIORAL_DIFF' WHEN ?=1 THEN status ELSE status END, " +
                                    "evidence_count = evidence_count + 1, " +
                                    "confidence = MIN(1.0, MAX(0.05, (confidence * evidence_count + ?) / (evidence_count + 1.0)))")) {
                        String initSt = (passed == 0 && "differential".equalsIgnoreCase(kind)) ? "BEHAVIORAL_DIFF" : "FULLY_IMPLEMENTED";
                        mx.setString(1, api);
                        mx.setInt(2, apiLvl);
                        mx.setString(3, initSt);
                        mx.setDouble(4, passed == 1 ? 0.9 : 0.3);
                        mx.setInt(5, passed);
                        mx.setString(6, kind.toLowerCase());
                        mx.setInt(7, passed);
                        mx.setDouble(8, passed == 1 ? 1.0 : 0.1);
                        mx.executeUpdate();
                    }
                    conn.commit();
                    System.out.println("{\"status\":\"ok\",\"recorded\":true}");
                    break;
                }
                case "upsert_quirk": {
                    // args: <db> upsert_quirk <profile_id> <formfactor> <api> <summary> <severity>
                    try (PreparedStatement ps = conn.prepareStatement(
                            "INSERT INTO device_quirks(profile_id, oem_or_formfactor, api, quirk_summary, severity) VALUES(?,?,?,?,?) " +
                                    "ON CONFLICT(profile_id, api, quirk_summary) DO UPDATE SET oem_or_formfactor=excluded.oem_or_formfactor, severity=excluded.severity")) {
                        ps.setString(1, args[2]);
                        ps.setString(2, args[3]);
                        ps.setString(3, args[4]);
                        ps.setString(4, args[5]);
                        ps.setString(5, args[6]);
                        ps.executeUpdate();
                    }
                    System.out.println("{\"status\":\"ok\",\"quirk_saved\":true}");
                    break;
                }
                case "heatmap": {
                    StringBuilder sb = new StringBuilder();
                    sb.append("{\"status\":\"ok\",\"contracts\":[");
                    boolean first = true;
                    try (Statement st = conn.createStatement();
                         ResultSet rs = st.executeQuery("SELECT api, level, since_api, until_api, fallback_backend, notes, pass_count, fail_count, diverge_count, confidence FROM contracts ORDER BY api")) {
                        while (rs.next()) {
                            if (!first) sb.append(",");
                            first = false;
                            sb.append("{\"api\":\"").append(esc(rs.getString(1))).append("\"")
                                    .append(",\"level\":\"").append(esc(rs.getString(2))).append("\"")
                                    .append(",\"since_api\":").append(rs.getInt(3))
                                    .append(",\"until_api\":").append(rs.getInt(4))
                                    .append(",\"fallback_backend\":\"").append(esc(rs.getString(5))).append("\"")
                                    .append(",\"notes\":\"").append(esc(rs.getString(6))).append("\"")
                                    .append(",\"pass_count\":").append(rs.getInt(7))
                                    .append(",\"fail_count\":").append(rs.getInt(8))
                                    .append(",\"diverge_count\":").append(rs.getInt(9))
                                    .append(",\"confidence\":").append(String.format(java.util.Locale.US, "%.4f", rs.getDouble(10)))
                                    .append("}");
                        }
                    }
                    sb.append("],\"api_levels\":[");
                    first = true;
                    try (Statement st = conn.createStatement();
                         ResultSet rs = st.executeQuery("SELECT api_level, COUNT(*) as total, SUM(CASE WHEN status='FULLY_IMPLEMENTED' OR status='BEHAVIORAL_DIFF' OR status='PARTIAL' THEN 1 ELSE 0 END) as supported, AVG(confidence) as avg_conf, SUM(evidence_count) as ev_sum FROM api_matrix GROUP BY api_level ORDER BY api_level")) {
                        while (rs.next()) {
                            if (!first) sb.append(",");
                            first = false;
                            sb.append("{\"api_level\":").append(rs.getInt(1))
                                    .append(",\"tracked_apis\":").append(rs.getInt(2))
                                    .append(",\"supported_apis\":").append(rs.getInt(3))
                                    .append(",\"avg_confidence\":").append(String.format(java.util.Locale.US, "%.4f", rs.getDouble(4)))
                                    .append(",\"evidence_events\":").append(rs.getInt(5)).append("}");
                        }
                    }
                    sb.append("],\"quirks\":[");
                    first = true;
                    try (Statement st = conn.createStatement();
                         ResultSet rs = st.executeQuery("SELECT profile_id, oem_or_formfactor, api, quirk_summary, severity FROM device_quirks ORDER BY profile_id, api")) {
                        while (rs.next()) {
                            if (!first) sb.append(",");
                            first = false;
                            sb.append("{\"profile_id\":\"").append(esc(rs.getString(1))).append("\"")
                                    .append(",\"form_factor\":\"").append(esc(rs.getString(2))).append("\"")
                                    .append(",\"api\":\"").append(esc(rs.getString(3))).append("\"")
                                    .append(",\"quirk_summary\":\"").append(esc(rs.getString(4))).append("\"")
                                    .append(",\"severity\":\"").append(esc(rs.getString(5))).append("\"}");
                        }
                    }
                    sb.append("]}");
                    System.out.println(sb.toString());
                    break;
                }
                default:
                    System.err.println("Unknown command: " + cmd);
                    System.exit(2);
            }
        }
    }

    private static String esc(String s) {
        if (s == null) return "";
        return s.replace("\\", "\\\\").replace("\"", "\\\"").replace("\n", "\\n").replace("\r", "\\r");
    }
}
