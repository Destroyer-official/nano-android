import android.app.*;
import android.content.*;
import android.database.Cursor;
import android.database.MatrixCursor;
import android.net.Uri;
import android.os.*;
import android.view.Choreographer;
import androidx.core.content.FileProvider;
import androidx.datastore.preferences.core.PreferencesDataStore;
import nanoandroid.compat.ApiCompatibilityRegistry;
import nanoandroid.host.*;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;
import org.objectweb.asm.*;

import java.io.*;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.nio.charset.StandardCharsets;
import java.nio.file.Path;
import java.util.*;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.jar.JarEntry;
import java.util.jar.JarOutputStream;
import java.util.zip.ZipEntry;
import java.util.zip.ZipOutputStream;

import static org.junit.jupiter.api.Assertions.*;

public class Phase1RuntimeConformanceTest {

    // ---- Test Components ----

    public static class TestContentProvider extends ContentProvider {
        @Override public boolean onCreate() { return true; }
        @Override public Cursor query(Uri uri, String[] projection, String sel, String[] args, String sort) {
            MatrixCursor mc = new MatrixCursor(new String[]{"value"});
            mc.addRow(new Object[]{"provider_ok:" + uri.getLastPathSegment()});
            return mc;
        }
        @Override public String getType(Uri uri) { return "vnd.android.cursor.item/test"; }
        @Override public Uri insert(Uri uri, ContentValues values) { return uri; }
        @Override public int delete(Uri uri, String sel, String[] args) { return 1; }
        @Override public int update(Uri uri, ContentValues values, String sel, String[] args) { return 1; }
    }

    public static class TestApplication extends Application {
        public static boolean created = false;
        @Override public void onCreate() {
            super.onCreate();
            created = true;
        }
    }

    public static class TestLocalService extends Service {
        private final Binder binder = new Binder();
        public int startCount = 0;

        @Override public void onCreate() {
            super.onCreate();
            binder.attachInterface(() -> binder, "com.example.ITestService");
        }
        @Override public int onStartCommand(Intent intent, int flags, int startId) {
            super.onStartCommand(intent, flags, startId);
            startCount++;
            startForeground(77, "ForegroundNotification");
            return START_STICKY;
        }
        @Override public IBinder onBind(Intent intent) {
            super.onBind(intent);
            return binder;
        }
    }

    public static class StatefulActivity extends Activity {
        public String restoredCounter = "";
        @Override protected void onCreate(Bundle savedInstanceState) {
            super.onCreate(savedInstanceState);
            if (savedInstanceState != null) {
                restoredCounter = "create:" + savedInstanceState.getString("counter");
            }
        }
        @Override protected void onSaveInstanceState(Bundle outState) {
            super.onSaveInstanceState(outState);
            outState.putString("counter", "42");
        }
        @Override protected void onRestoreInstanceState(Bundle savedInstanceState) {
            super.onRestoreInstanceState(savedInstanceState);
            restoredCounter += "|restore:" + savedInstanceState.getString("counter");
        }
    }

    @Test
    void bytecodeAuditLogsAsmTransformsKotlinMetadataAndNeverHidesDegradedFallbacks(@TempDir Path tmpDir) throws Exception {
        TransformationAuditLog.clear();
        ApiCompatibilityRegistry.clearSessionFaults();

        // Generate an in-memory JAR with ASM containing @kotlin.Metadata and abstract class instantiated with NEW
        ClassWriter cwMeta = new ClassWriter(ClassWriter.COMPUTE_MAXS);
        cwMeta.visit(Opcodes.V1_8, Opcodes.ACC_PUBLIC, "com/example/KotlinFixture", null, "java/lang/Object", null);
        AnnotationVisitor av = cwMeta.visitAnnotation("Lkotlin/Metadata;", true);
        av.visit("mv", new int[]{2, 0, 21});
        av.visitEnd();
        MethodVisitor mv = cwMeta.visitMethod(Opcodes.ACC_PUBLIC, "<init>", "()V", null, null);
        mv.visitCode();
        mv.visitVarInsn(Opcodes.ALOAD, 0);
        mv.visitMethodInsn(Opcodes.INVOKESPECIAL, "java/lang/Object", "<init>", "()V", false);
        mv.visitInsn(Opcodes.RETURN);
        mv.visitMaxs(1, 1);
        mv.visitEnd();
        cwMeta.visitEnd();

        ClassWriter cwAbs = new ClassWriter(ClassWriter.COMPUTE_MAXS);
        cwAbs.visit(Opcodes.V1_8, Opcodes.ACC_PUBLIC | Opcodes.ACC_ABSTRACT, "com/example/AbstractHost", null, "java/lang/Object", null);
        MethodVisitor ma = cwAbs.visitMethod(Opcodes.ACC_PUBLIC | Opcodes.ACC_ABSTRACT, "compute", "()I", null, null);
        ma.visitEnd();
        MethodVisitor maInit = cwAbs.visitMethod(Opcodes.ACC_PUBLIC, "<init>", "()V", null, null);
        maInit.visitCode();
        maInit.visitVarInsn(Opcodes.ALOAD, 0);
        maInit.visitMethodInsn(Opcodes.INVOKESPECIAL, "java/lang/Object", "<init>", "()V", false);
        maInit.visitInsn(Opcodes.RETURN);
        maInit.visitMaxs(1, 1);
        maInit.visitEnd();
        // Reference NEW on itself to exercise STRIP_ABSTRACT rule
        MethodVisitor trigger = cwAbs.visitMethod(Opcodes.ACC_PUBLIC | Opcodes.ACC_STATIC, "make", "()Lcom/example/AbstractHost;", null, null);
        trigger.visitCode();
        trigger.visitTypeInsn(Opcodes.NEW, "com/example/AbstractHost");
        trigger.visitInsn(Opcodes.DUP);
        trigger.visitMethodInsn(Opcodes.INVOKESPECIAL, "com/example/AbstractHost", "<init>", "()V", false);
        trigger.visitInsn(Opcodes.ARETURN);
        trigger.visitMaxs(2, 0);
        trigger.visitEnd();
        cwAbs.visitEnd();

        File jarFile = tmpDir.resolve("asm_fixture.jar").toFile();
        try (JarOutputStream jos = new JarOutputStream(new FileOutputStream(jarFile))) {
            jos.putNextEntry(new JarEntry("com/example/KotlinFixture.class"));
            jos.write(cwMeta.toByteArray());
            jos.closeEntry();
            jos.putNextEntry(new JarEntry("com/example/AbstractHost.class"));
            jos.write(cwAbs.toByteArray());
            jos.closeEntry();
        }

        NanoClassLoader ncl = new NanoClassLoader(jarFile, null, Phase1RuntimeConformanceTest.class.getClassLoader());
        assertEquals("2.0.21", ncl.getKotlinMetadataVersions().get("com/example/KotlinFixture"));

        Class<?> absCls = ncl.loadClass("com.example.AbstractHost");
        Object inst = absCls.getMethod("make").invoke(null);
        assertNotNull(inst);

        // Dummy interface proxy also records DEGRADED_HEURISTIC + CompatibilityFault
        Runnable dummy = (Runnable) NanoClassLoader.createDummyInterfaceProxy(Runnable.class);
        assertNotNull(dummy);
        assertTrue(TransformationAuditLog.hasDegradedHeuristic(), "Degraded fallback must be explicitly flagged");
        assertFalse(ApiCompatibilityRegistry.getSessionFaults().isEmpty());
        assertFalse(TransformationAuditLog.getRecords().isEmpty());
        assertNotEquals("", TransformationAuditLog.getRecords().get(0).beforeSha256);

        // Desugaring j$ shims execute accurately
        assertEquals("ok", j$.util.Optional.ofNullable("ok").orElse("missing"));
        assertTrue(j$.util.Objects.nonNull("x"));
        assertEquals(2L, j$.util.stream.DesugarCollections.stream(List.of("a", "b")).count());
    }

    @Test
    void lifecycleCallbacksProcessDeathServicesContentProviderAndBroadcastPermissions(@TempDir Path tmpDir) throws Exception {
        System.setProperty("nano.sandbox.dir", tmpDir.toString());
        LifecycleEventTracer.clear();
        ContentResolver.clearProviders();
        Context.clearServices();

        List<String> appCallbacksSeen = new ArrayList<>();
        TestApplication app = new TestApplication();
        Context base = new Context("com.example.lifecycle");
        app.attachBaseContext(base);
        app.setPackageName("com.example.lifecycle");
        Activity.currentApplication = app;

        app.registerActivityLifecycleCallbacks(new Application.ActivityLifecycleCallbacks() {
            @Override public void onActivityCreated(Activity a, Bundle b) { appCallbacksSeen.add("C:" + (b != null)); }
            @Override public void onActivityStarted(Activity a) { appCallbacksSeen.add("S"); }
            @Override public void onActivityResumed(Activity a) { appCallbacksSeen.add("R"); }
            @Override public void onActivityPaused(Activity a) { appCallbacksSeen.add("P"); }
            @Override public void onActivityStopped(Activity a) { appCallbacksSeen.add("ST"); }
            @Override public void onActivitySaveInstanceState(Activity a, Bundle out) { appCallbacksSeen.add("SAVE"); }
            @Override public void onActivityDestroyed(Activity a) { appCallbacksSeen.add("D"); }
        });

        // Boot ContentProvider -> Application order
        TestContentProvider provider = new TestContentProvider();
        provider.attachInfo(base, "com.example.lifecycle.provider");
        assertTrue(provider.performCreate());
        ContentResolver.registerProvider("com.example.lifecycle.provider", provider);
        app.onCreate();

        UriMatcher matcher = new UriMatcher(UriMatcher.NO_MATCH);
        matcher.addURI("com.example.lifecycle.provider", "items/#", 101);
        Uri itemUri = Uri.parse("content://com.example.lifecycle.provider/items/42");
        assertEquals(101, matcher.match(itemUri));
        try (Cursor cur = base.getContentResolver().query(itemUri, null, null, null, null)) {
            assertTrue(cur.moveToFirst());
            assertEquals("provider_ok:42", cur.getString(0));
        }

        // Activity recreate with savedInstanceState restoration
        StatefulActivity act = new StatefulActivity();
        act.setApplication(app);
        act.attach(base, new Intent(Intent.ACTION_MAIN));
        act.performCreate(null);
        act.performStart();
        act.performResume();
        act.recreate();
        assertEquals("create:42|restore:42", act.restoredCounter);
        assertTrue(appCallbacksSeen.contains("SAVE"));
        assertTrue(appCallbacksSeen.contains("C:true"));

        // Verify ContentProvider boot event preceded Application.onCreate()
        List<LifecycleEventTracer.Event> events = LifecycleEventTracer.getEvents();
        int cpIdx = -1, appIdx = -1;
        for (int i = 0; i < events.size(); i++) {
            if ("CONTENT_PROVIDER_CREATE".equals(events.get(i).eventType) && cpIdx < 0) cpIdx = i;
            if ("APPLICATION_CREATE".equals(events.get(i).eventType) && appIdx < 0) appIdx = i;
        }
        assertTrue(cpIdx >= 0 && appIdx > cpIdx, "ContentProvider must initialize before Application");

        // Service start, foreground & bind/unbind
        Intent svcIntent = new Intent(base, TestLocalService.class);
        assertNotNull(base.startService(svcIntent));
        Service activeSvc = Context.getActiveServices().get(TestLocalService.class.getName());
        assertNotNull(activeSvc);
        assertTrue(((TestLocalService) activeSvc).isForeground());

        AtomicBoolean bound = new AtomicBoolean(false);
        ServiceConnection conn = new ServiceConnection() {
            @Override public void onServiceConnected(ComponentName name, IBinder service) {
                assertNotNull(service.queryLocalInterface("com.example.ITestService"));
                bound.set(true);
            }
            @Override public void onServiceDisconnected(ComponentName name) {}
        };
        assertTrue(base.bindService(svcIntent, conn, 1));
        assertTrue(bound.get());
        base.unbindService(conn);
        assertTrue(base.stopService(svcIntent));

        // BroadcastReceiver RECEIVER_NOT_EXPORTED vs RECEIVER_EXPORTED
        AtomicInteger internalCount = new AtomicInteger(0);
        AtomicInteger exportedCount = new AtomicInteger(0);
        base.registerReceiver(new BroadcastReceiver() {
            @Override public void onReceive(Context c, Intent i) { internalCount.incrementAndGet(); }
        }, new IntentFilter("com.example.PING"), Context.RECEIVER_NOT_EXPORTED);
        base.registerReceiver(new BroadcastReceiver() {
            @Override public void onReceive(Context c, Intent i) { exportedCount.incrementAndGet(); }
        }, new IntentFilter("com.example.PING"), Context.RECEIVER_EXPORTED);

        base.sendBroadcast(new Intent("com.example.PING"), true); // external
        assertEquals(0, internalCount.get());
        assertEquals(1, exportedCount.get());
        base.sendBroadcast(new Intent("com.example.PING"), false); // internal
        assertEquals(1, internalCount.get());
        assertEquals(2, exportedCount.get());

        // Deep-link IntentFilter scheme/host/path matching
        IntentFilter deepFilter = new IntentFilter(Intent.ACTION_VIEW);
        deepFilter.addDataScheme("nano");
        deepFilter.addDataAuthority("app.example", null);
        deepFilter.addDataPath("/video", 2);
        assertTrue(deepFilter.match(new Intent(Intent.ACTION_VIEW, Uri.parse("nano://app.example/video/123"))));
        assertFalse(deepFilter.match(new Intent(Intent.ACTION_VIEW, Uri.parse("http://other.host/video/123"))));
    }

    @Test
    void deterministicClockHandlerThreadChoreographerWatchdogAndStorage(@TempDir Path tmpDir) throws Exception {
        System.setProperty("nano.sandbox.dir", tmpDir.toString());
        DeterministicRuntime.reset(999L);
        SystemClock.setUseVirtualTime(true);
        SystemClock.setUptimeMillis(0);

        int r1 = DeterministicRuntime.nextInt(10000);
        DeterministicRuntime.reset(999L);
        int r2 = DeterministicRuntime.nextInt(10000);
        assertEquals(r1, r2);

        // Choreographer + HandlerThread deterministic dispatch
        HandlerThread bg = new HandlerThread("bg-worker");
        bg.start();
        Handler bgHandler = new Handler(bg.getLooper());
        List<Long> seenTimes = new ArrayList<>();
        bgHandler.postDelayed(() -> seenTimes.add(SystemClock.uptimeMillis()), 250L);
        Choreographer.getInstance().postFrameCallback(nanos -> seenTimes.add(nanos / 1_000_000L));

        assertEquals(0, bg.getLooper().drainDueMessages());
        assertEquals(1, bg.getLooper().stepBy(300L));
        assertEquals(1, Looper.getMainLooper().drainDueMessages());
        assertEquals(List.of(300L, 300L), seenTimes);
        assertTrue(bg.quitSafely());

        // Infinite-loop Handler watchdog prevents hangs
        DeterministicRuntime.setMaxLooperTasksPerStep(20);
        Handler mainHandler = new Handler(Looper.getMainLooper());
        Runnable runaway = new Runnable() {
            @Override public void run() { mainHandler.post(this); }
        };
        mainHandler.post(runaway);
        assertThrows(IllegalStateException.class, () -> Looper.getMainLooper().stepBy(10L));
        Looper.getMainLooper().getQueue().removeCallbacksAndMessages(runaway);
        DeterministicRuntime.reset();

        // Explicit NONDETERMINISTIC boundary reporting
        SystemClock.setUseVirtualTime(false);
        SystemClock.uptimeMillis();
        assertFalse(DeterministicRuntime.getBoundaries().isEmpty());
        SystemClock.setUseVirtualTime(true);

        // SharedPreferences listener + PreferencesDataStore + FileProvider + PersistableUriPermission
        Context ctx = new Context("com.example.storage");
        SharedPreferences prefs = ctx.getSharedPreferences("cfg", Context.MODE_PRIVATE);
        List<String> prefEvents = new ArrayList<>();
        prefs.registerOnSharedPreferenceChangeListener((sp, k) -> prefEvents.add(k));
        assertTrue(prefs.edit().putString("theme", "dark").commit());
        assertEquals(List.of("theme"), prefEvents);

        PreferencesDataStore ds = new PreferencesDataStore(ctx, "user_settings");
        ds.updateData(m -> { m.put("onboarding_done", true); m.put("volume", 80); });
        PreferencesDataStore dsReloaded = new PreferencesDataStore(ctx, "user_settings");
        assertTrue(dsReloaded.getBoolean("onboarding_done", false));
        assertEquals(80, dsReloaded.getInt("volume", 0));

        File doc = new File(ctx.getFilesDir(), "sample.txt");
        try (FileOutputStream fos = new FileOutputStream(doc)) {
            fos.write("sandbox-data".getBytes(StandardCharsets.UTF_8));
        }
        Uri contentUri = FileProvider.getUriForFile(ctx, "com.example.storage.fileprovider", doc);
        assertTrue(contentUri.toString().startsWith("content://com.example.storage.fileprovider/files/"));
        try (InputStream in = ctx.getContentResolver().openInputStream(contentUri)) {
            assertEquals("sandbox-data", new String(in.readAllBytes(), StandardCharsets.UTF_8));
        }
        ctx.getContentResolver().takePersistableUriPermission(contentUri, Intent.FLAG_GRANT_READ_URI_PERMISSION);
        assertTrue(ctx.getContentResolver().hasPersistedUriPermission(contentUri, Intent.FLAG_GRANT_READ_URI_PERMISSION));
        ctx.getContentResolver().releasePersistableUriPermission(contentUri, Intent.FLAG_GRANT_READ_URI_PERMISSION);
        assertFalse(ctx.getContentResolver().hasPersistedUriPermission(contentUri, Intent.FLAG_GRANT_READ_URI_PERMISSION));
        assertTrue(ctx.getSandboxStorageBytes() > 0);

        // AlarmManager virtual clock alarm scheduling + snapshot/rewind + Locale/TimeZone
        long snap = DeterministicRuntime.captureClockSnapshot();
        AtomicBoolean alarmFired = new AtomicBoolean(false);
        AlarmManager am = new AlarmManager(ctx);
        am.setExact(AlarmManager.ELAPSED_REALTIME_WAKEUP, SystemClock.elapsedRealtime() + 500L, "sync_job", () -> alarmFired.set(true), null);
        assertEquals(0, Looper.getMainLooper().drainDueMessages());
        assertFalse(alarmFired.get());
        Looper.getMainLooper().stepBy(500L);
        assertTrue(alarmFired.get());
        DeterministicRuntime.restoreClockSnapshot(snap);
        assertEquals(snap, SystemClock.uptimeMillis());
        DeterministicRuntime.setTestTimeZone("Asia/Tokyo");
        assertEquals("Asia/Tokyo", DeterministicRuntime.getTestTimeZone().getID());

        // Coroutines main dispatcher bridge + MediaStore/DocumentsContract + SQLite integrity_check + Storage quota
        AtomicBoolean coroutinesRan = new AtomicBoolean(false);
        NanoCoroutineMainBridge.getInstance().execute(() -> coroutinesRan.set(true));
        Looper.getMainLooper().drainDueMessages();
        assertTrue(coroutinesRan.get());
        assertEquals("42", android.provider.DocumentsContract.getDocumentId(Uri.parse("content://com.example.docs/document/42")));
        assertEquals("_data", android.provider.MediaStore.Video.Media.DATA);

        try (android.database.sqlite.SQLiteDatabase db = android.database.sqlite.SQLiteDatabase.openOrCreateDatabase(ctx.getDatabasePath("integrity.db").getAbsolutePath(), null)) {
            assertEquals("ok", db.integrityCheck());
        }

        // Storage quota enforcement blocks oversized sandbox writes
        Context.setStorageQuotaBytes(1024L);
        try {
            assertThrows(SecurityException.class, () -> ctx.openFileOutput("too_big_after_quota.txt", 0));
        } finally {
            Context.setStorageQuotaBytes(256L * 1024L * 1024L);
        }

        // Compatibility JSON export check
        ApiCompatibilityRegistry.registerAnnotatedClass(AlarmManager.class);
        assertTrue(ApiCompatibilityRegistry.exportRegistryJson().contains("android.app.AlarmManager"));
    }
}
