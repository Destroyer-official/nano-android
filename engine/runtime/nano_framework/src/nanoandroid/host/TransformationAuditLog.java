package nanoandroid.host;

import java.security.MessageDigest;
import java.util.*;
import java.util.concurrent.CopyOnWriteArrayList;

/**
 * Immutable session log of every bytecode rewrite performed by NanoClassLoader.
 * Ensures no silent bytecode mutation or heuristic fallback goes unreported.
 */
public final class TransformationAuditLog {

    public static final class Record {
        public final String className;
        public final String methodName;
        public final String ruleId;
        public final String beforeSha256;
        public final String afterSha256;
        public final String riskLevel; // "LOW" | "BEHAVIORAL_DIFF" | "DEGRADED_HEURISTIC"
        public final String details;

        public Record(String className, String methodName, String ruleId,
                      String beforeSha256, String afterSha256, String riskLevel, String details) {
            this.className = className;
            this.methodName = methodName;
            this.ruleId = ruleId;
            this.beforeSha256 = beforeSha256;
            this.afterSha256 = afterSha256;
            this.riskLevel = riskLevel;
            this.details = details;
        }
    }

    private static final List<Record> RECORDS = new CopyOnWriteArrayList<>();

    private TransformationAuditLog() {}

    public static void clear() {
        RECORDS.clear();
    }

    public static void record(String className, String methodName, String ruleId,
                              byte[] beforeBytes, byte[] afterBytes, String riskLevel, String details) {
        RECORDS.add(new Record(
                className,
                methodName != null ? methodName : "*",
                ruleId,
                sha256Hex(beforeBytes),
                sha256Hex(afterBytes),
                riskLevel != null ? riskLevel : "LOW",
                details != null ? details : ""
        ));
    }

    public static List<Record> getRecords() {
        return Collections.unmodifiableList(new ArrayList<>(RECORDS));
    }

    public static boolean hasDegradedHeuristic() {
        for (Record r : RECORDS) {
            if ("DEGRADED_HEURISTIC".equals(r.riskLevel)) {
                return true;
            }
        }
        return false;
    }

    public static int size() {
        return RECORDS.size();
    }

    private static String sha256Hex(byte[] data) {
        if (data == null) return "";
        try {
            MessageDigest md = MessageDigest.getInstance("SHA-256");
            byte[] sum = md.digest(data);
            StringBuilder sb = new StringBuilder(sum.length * 2);
            for (byte b : sum) {
                sb.append(Character.forDigit((b >> 4) & 0xF, 16));
                sb.append(Character.forDigit(b & 0xF, 16));
            }
            return sb.toString();
        } catch (Throwable t) {
            return "";
        }
    }
}
