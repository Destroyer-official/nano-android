import android.content.ContentValues;
import android.content.Context;
import android.content.SharedPreferences;
import android.database.sqlite.SQLiteDatabase;
import nanoandroid.compat.CompatibilityDbTool;
import nanoandroid.host.ForensicTimeline;
import nanoandroid.host.HeadlessAppHost;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.io.FileOutputStream;
import java.nio.file.Files;
import java.nio.file.Path;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.ResultSet;
import java.sql.Statement;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.*;

public class Phase3TelemetryAndCompatDbTest {

    @Test
    void forensicTimelineMaintainsMonotonicSequenceAndHostTelemetry(@TempDir Path tempDir) throws Exception {
        ForensicTimeline.clear("junit-phase3");
        ForensicTimeline.record("APK_LAUNCHED", "fixture.apk", "MainActivity");
        ForensicTimeline.record("ACTIVITY_CREATED", "MainActivity", "fresh");
        ForensicTimeline.record("CALLBACK_EXECUTED", "Looper.main", "processed=2");

        String json = ForensicTimeline.toJson();
        assertTrue(json.contains("\"count\":3"));
        assertTrue(json.contains("\"APK_LAUNCHED\""));
        assertTrue(json.contains("\"correlation_id\":\"junit-phase3\""));

        HeadlessAppHost host = new HeadlessAppHost();
        String threadsJson = host.inspectThreads();
        assertTrue(threadsJson.contains("\"status\":\"ok\""));
        assertTrue(threadsJson.contains("\"deadlocked_count\":0"));
        assertTrue(threadsJson.contains("\"looper_queue_depth\":"));

        String runtimeJson = host.inspectRuntime();
        assertTrue(runtimeJson.contains("\"thread_count\":"));
        assertTrue(runtimeJson.contains("\"deadlocked_threads\":0"));
        assertTrue(runtimeJson.contains("\"timeline_events\":3"));
    }

    @Test
    void compatibilityDbToolPersistsContractsMatrixAndContinuousEvidence(@TempDir Path tempDir) throws Exception {
        Path db = tempDir.resolve("compat_phase3.db");
        CompatibilityDbTool.main(new String[]{db.toString(), "init"});
        assertTrue(Files.exists(db));

        CompatibilityDbTool.main(new String[]{
                db.toString(), "upsert_contract",
                "android.app.Activity", "FULLY_IMPLEMENTED", "26", "37", "", "Activity lifecycle", "0.95"
        });
        CompatibilityDbTool.main(new String[]{
                db.toString(), "record_evidence",
                "conformance", "android.app.Activity", "recreate", "nano_jvm", "37", "1", "Verified state save"
        });
        CompatibilityDbTool.main(new String[]{
                db.toString(), "record_evidence",
                "differential", "android.app.Activity", "nano_vs_art", "nano_jvm_vs_android_art", "35", "0", "Minor timing delta"
        });
        CompatibilityDbTool.main(new String[]{
                db.toString(), "upsert_quirk",
                "foldable_unfolded", "foldable", "android.app.Activity", "Posture config change", "MEDIUM"
        });

        Class.forName("org.sqlite.JDBC");
        try (Connection c = DriverManager.getConnection("jdbc:sqlite:" + db.toAbsolutePath());
             Statement st = c.createStatement();
             ResultSet rs = st.executeQuery("SELECT pass_count, diverge_count FROM contracts WHERE api='android.app.Activity'")) {
            assertTrue(rs.next());
            assertEquals(1, rs.getInt("pass_count"));
            assertEquals(1, rs.getInt("diverge_count"));
        }
    }

    @Test
    void storageAndFileTelemetryCaptureLiveSqliteRowsAndSharedPrefs(@TempDir Path tempDir) throws Exception {
        System.setProperty("nano.sandbox.dir", tempDir.toString());
        try {
            Context.clearTrackedSharedPreferences();
            ForensicTimeline.clear("junit-storage");

            Context ctx = new Context("com.phase3.storage");
            try (FileOutputStream fos = ctx.openFileOutput("sample.txt", Context.MODE_PRIVATE)) {
                fos.write(new byte[]{1, 2, 3});
            }

            SharedPreferences prefs = ctx.getSharedPreferences("app_settings", Context.MODE_PRIVATE);
            assertTrue(prefs.edit().putString("theme", "dark").putInt("launch_count", 2).commit());

            Map<String, String> prefSnap = Context.snapshotSharedPreferences("com.phase3.storage");
            assertEquals("dark", prefSnap.get("com.phase3.storage/app_settings#theme"));
            assertEquals("2", prefSnap.get("com.phase3.storage/app_settings#launch_count"));

            SQLiteDatabase db = SQLiteDatabase.openOrCreateDatabase(ctx.getDatabasePath("state.db").getAbsolutePath(), null);
            db.execSQL("CREATE TABLE items (id INTEGER PRIMARY KEY, label TEXT)");
            ContentValues cv = new ContentValues();
            cv.put("label", "sensor_a");
            assertTrue(db.insert("items", null, cv) > 0);

            Map<String, String> tblSnap = SQLiteDatabase.snapshotActiveTables();
            assertEquals("rows=1", tblSnap.get("state.db#items"));
            db.close();

            String timelineJson = ForensicTimeline.toJson();
            assertTrue(timelineJson.contains("\"FILE_OP\""));
            assertTrue(timelineJson.contains("\"DB_MUTATION\""));
        } finally {
            System.clearProperty("nano.sandbox.dir");
            Context.clearTrackedSharedPreferences();
        }
    }
}
