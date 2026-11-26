import android.content.ContentResolver;
import android.content.ContentValues;
import android.content.Context;
import android.content.pm.PackageManager;
import android.database.Cursor;
import android.database.sqlite.SQLiteDatabase;
import android.database.sqlite.SQLiteOpenHelper;
import android.net.Uri;
import nanoandroid.host.ApkResourceTable;
import nanoandroid.host.NanoClassLoader;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.io.File;
import java.io.FileOutputStream;
import java.nio.charset.StandardCharsets;
import java.nio.file.Path;
import java.util.List;
import java.util.zip.ZipEntry;
import java.util.zip.ZipOutputStream;

import static org.junit.jupiter.api.Assertions.*;

public class Phase0SecurityAndSQLiteTest {

    @Test
    void contextEnforcesCanonicalSandboxContainment(@TempDir Path tmpDir) throws Exception {
        System.setProperty("nano.sandbox.dir", tmpDir.toString());
        Context ctx = new Context("com.example.phase0");

        assertThrows(SecurityException.class, () -> ctx.openFileOutput("../escape.txt", 0));
        assertThrows(SecurityException.class, () -> ctx.openFileInput("../../secret.txt"));
        assertThrows(SecurityException.class, () -> ctx.getDatabasePath("../outside.db"));
        assertThrows(SecurityException.class, () -> ctx.getNanoSandboxDir("../../evil"));

        try (FileOutputStream fos = ctx.openFileOutput("ok.txt", 0)) {
            fos.write("hello".getBytes(StandardCharsets.UTF_8));
        }
        File expected = new File(ctx.getFilesDir(), "ok.txt");
        assertTrue(expected.exists());
        assertTrue(expected.getCanonicalPath().startsWith(tmpDir.toFile().getCanonicalPath()));
    }

    @Test
    void contentResolverBlocksOutsideHostFileUris(@TempDir Path tmpDir) throws Exception {
        System.setProperty("nano.sandbox.dir", tmpDir.toString());
        File outside = tmpDir.resolve("outside_host.txt").toFile();
        // Outside sandbox subdir (sandbox subdir is tmpDir/com.pkg/... or check staged_files)
        File parentOutside = File.createTempFile("nano_outside_", ".txt");
        parentOutside.deleteOnExit();

        ContentResolver cr = new ContentResolver();
        Uri evilUri = Uri.fromFile(parentOutside);
        // fromFile explicitly registers; test unregistered parsed file:// URI outside sandbox:
        Uri rawHostUri = Uri.parse("file:///" + parentOutside.getAbsolutePath().replace("\\", "/"));
        assertThrows(SecurityException.class, () -> cr.openOutputStream(rawHostUri));
        assertThrows(java.io.FileNotFoundException.class, () -> cr.openInputStream(rawHostUri));
    }

    @Test
    void permissionsAreStatefulAndNotUnconditionallyGranted() {
        String pkg = "com.example.permtest";
        Context.resetPermissions(pkg);
        Context ctx = new Context(pkg);

        // Undeclared permissions must default to DENIED
        assertEquals(PackageManager.PERMISSION_DENIED, ctx.checkSelfPermission("android.permission.READ_SMS"));
        assertEquals(PackageManager.PERMISSION_DENIED, ctx.checkCallingOrSelfPermission("android.permission.CAMERA"));

        // Seeding manifest grants normal permissions but keeps dangerous runtime permissions DENIED
        Context.seedManifestPermissions(pkg, List.of(
                "android.permission.INTERNET",
                "android.permission.READ_SMS",
                "android.permission.ACCESS_LOCAL_NETWORK"
        ));
        assertEquals(PackageManager.PERMISSION_GRANTED, ctx.checkSelfPermission("android.permission.INTERNET"));
        assertEquals(PackageManager.PERMISSION_DENIED, ctx.checkSelfPermission("android.permission.READ_SMS"));
        assertEquals(PackageManager.PERMISSION_DENIED, ctx.checkSelfPermission("android.permission.ACCESS_LOCAL_NETWORK"));

        // Explicit grant & revoke updates state accurately
        Context.setPermissionState(pkg, "android.permission.READ_SMS", PackageManager.PERMISSION_GRANTED);
        assertEquals(PackageManager.PERMISSION_GRANTED, ctx.checkSelfPermission("android.permission.READ_SMS"));
        Context.setPermissionState(pkg, "android.permission.READ_SMS", PackageManager.PERMISSION_DENIED);
        assertEquals(PackageManager.PERMISSION_DENIED, ctx.checkSelfPermission("android.permission.READ_SMS"));

        PackageManager pm = ctx.getPackageManager();
        assertTrue(pm.hasSystemFeature("android.hardware.touchscreen"));
        assertFalse(pm.hasSystemFeature("android.hardware.vulkan.level"));
    }

    @Test
    void sqliteSupportsRealAcidRollbackSavepointsAndForeignKeys(@TempDir Path tmpDir) {
        System.setProperty("nano.sandbox.dir", tmpDir.toString());
        Context ctx = new Context("com.example.sqltest");
        File dbFile = ctx.getDatabasePath("test_real.db");

        SQLiteOpenHelper helper = new SQLiteOpenHelper(ctx, "test_real.db", null, 2) {
            @Override
            public void onCreate(SQLiteDatabase db) {
                db.execSQL("CREATE TABLE parents (id INTEGER PRIMARY KEY, name TEXT NOT NULL);");
                db.execSQL("CREATE TABLE children (id INTEGER PRIMARY KEY, parent_id INTEGER NOT NULL, FOREIGN KEY(parent_id) REFERENCES parents(id));");
            }
            @Override
            public void onUpgrade(SQLiteDatabase db, int oldVersion, int newVersion) {}
        };

        SQLiteDatabase db = helper.getWritableDatabase();
        assertEquals(2, db.getVersion());

        // 1. Failed transaction MUST roll back (never persist when setTransactionSuccessful() is omitted)
        db.beginTransaction();
        ContentValues p1 = new ContentValues();
        p1.put("id", 1);
        p1.put("name", "should_rollback");
        db.insertOrThrow("parents", null, p1);
        db.endTransaction(); // no setTransactionSuccessful()

        try (Cursor c = db.rawQuery("SELECT COUNT(*) FROM parents;", null)) {
            assertTrue(c.moveToFirst());
            assertEquals(0L, c.getLong(0), "Uncommitted transaction must roll back");
        }

        // 2. Committed outer transaction + rolled-back nested savepoint
        db.beginTransaction();
        ContentValues p2 = new ContentValues();
        p2.put("id", 10);
        p2.put("name", "committed_parent");
        db.insertOrThrow("parents", null, p2);

        db.beginTransaction(); // nested savepoint
        ContentValues p3 = new ContentValues();
        p3.put("id", 20);
        p3.put("name", "rolled_back_nested");
        db.insertOrThrow("parents", null, p3);
        db.endTransaction(); // rolls back nested only

        db.setTransactionSuccessful();
        db.endTransaction();

        try (Cursor c = db.rawQuery("SELECT id, name FROM parents;", null)) {
            assertEquals(1, c.getCount());
            assertTrue(c.moveToFirst());
            assertEquals(10L, c.getLong(0));
            assertEquals("committed_parent", c.getString(1));
        }

        // 3. Real Foreign Key enforcement
        ContentValues orphan = new ContentValues();
        orphan.put("id", 100);
        orphan.put("parent_id", 999);
        assertThrows(RuntimeException.class, () -> db.insertOrThrow("children", null, orphan));

        helper.close();
        assertTrue(dbFile.exists());
    }

    @Test
    void classLoaderAndApkResourceTableBlockUnsafeEntries(@TempDir Path tmpDir) throws Exception {
        File slipJar = tmpDir.resolve("slip.jar").toFile();
        try (ZipOutputStream zos = new ZipOutputStream(new FileOutputStream(slipJar))) {
            zos.putNextEntry(new ZipEntry("../evil.class"));
            zos.write(new byte[]{1, 2, 3});
            zos.closeEntry();
        }
        assertThrows(SecurityException.class, () -> new NanoClassLoader(slipJar, null, getClass().getClassLoader()));

        File validApk = tmpDir.resolve("mini.apk").toFile();
        try (ZipOutputStream zos = new ZipOutputStream(new FileOutputStream(validApk))) {
            zos.putNextEntry(new ZipEntry("dummy.txt"));
            zos.write("ok".getBytes(StandardCharsets.UTF_8));
            zos.closeEntry();
        }
        try (ApkResourceTable tbl = new ApkResourceTable(validApk)) {
            assertThrows(SecurityException.class, () -> tbl.readZipEntry("../etc/passwd"));
        }
    }
}
