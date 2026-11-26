package nanoandroid.host;

import android.app.Activity;
import android.app.AlertDialog;
import android.app.Application;
import android.app.Dialog;
import android.app.LifecycleEventTracer;
import android.content.ClipData;
import android.content.ContentProvider;
import android.content.ContentResolver;
import android.content.Context;
import android.content.Intent;
import android.graphics.Bitmap;
import android.graphics.Typeface;
import android.graphics.drawable.BitmapDrawable;
import android.graphics.drawable.ColorDrawable;
import android.graphics.drawable.Drawable;
import android.graphics.drawable.GradientDrawable;
import android.graphics.drawable.LayerDrawable;
import android.net.Uri;
import android.os.Bundle;
import android.os.Looper;
import android.os.Message;
import android.os.MessageQueue;
import android.os.SystemClock;
import android.view.Gravity;
import android.view.KeyEvent;
import android.view.MotionEvent;
import android.view.View;
import android.view.ViewGroup;
import android.widget.*;

import java.awt.AlphaComposite;
import java.awt.BasicStroke;
import java.awt.Color;
import java.awt.Composite;
import java.awt.Font;
import java.awt.FontMetrics;
import java.awt.GradientPaint;
import java.awt.Graphics2D;
import java.awt.Paint;
import java.awt.Rectangle;
import java.awt.RenderingHints;
import java.awt.Shape;
import java.awt.geom.Ellipse2D;
import java.awt.geom.RoundRectangle2D;
import java.awt.image.BufferedImage;
import java.io.*;
import java.lang.management.ClassLoadingMXBean;
import java.lang.management.GarbageCollectorMXBean;
import java.lang.management.ManagementFactory;
import java.lang.reflect.Method;
import java.util.*;
import java.util.List;
import javax.imageio.ImageIO;

/**
 * HeadlessAppHost executes real Android APK/JAR bytecode on host OpenJDK 25:
 * - Decodes AndroidManifest.xml, resources.arsc, and binary AXML layouts for
 * ANY APK
 * - Executes Application.onCreate() and Activity lifecycle callbacks
 * - Runs authentic 2-pass Android Measure & Layout at 1080x2400
 * - Renders true View backgrounds, GradientDrawables, Bitmaps, Typography, and
 * controls via Java2D
 * - Exposes true hit-testing, scrolling, typing, virtual looper stepping, and
 * real JVM telemetry
 */
public class HeadlessAppHost {
    public static final int SCREEN_WIDTH = 1080;
    public static final int SCREEN_HEIGHT = 2400;
    public static final int STATUS_BAR_H = 68;
    public static final int NAV_BAR_H = 68;

    static {
        System.setProperty("java.awt.headless", "true");
    }

    private static class ActivityFrame {
        final String className;
        final Activity activity;
        final ActivityFrame caller;
        final int requestCode;

        ActivityFrame(String className, Activity activity, ActivityFrame caller, int requestCode) {
            this.className = className;
            this.activity = activity;
            this.caller = caller;
            this.requestCode = requestCode;
        }
    }

    private Activity activeActivity;
    private final Deque<ActivityFrame> activityStack = new ArrayDeque<>();
    private final List<File> stagedFiles = new ArrayList<>();
    private String lastFocusedNodeId = "";
    private ApkResourceTable activeApkTable;
    private NanoClassLoader activeClassLoader;
    private final Map<String, View> nodeMap = new LinkedHashMap<>();
    private final List<String> runtimeWarnings = new ArrayList<>();
    private String lastExceptionTrace = "";
    private String activeApkPath = "";
    private String activeAppPackage = "";
    private String activeActivityName = "";
    private int nodeCounter = 0;
    private boolean inspectorMode = false;
    private View lastInflatedRoot = null;

    private int viewportWidth = SCREEN_WIDTH;
    private int viewportHeight = SCREEN_HEIGHT;

    public static class NodeInfo {
        public String id;
        public String resName;
        public String clazz;
        public String text;
        public String contentDescription;
        public boolean clickable;
        public boolean enabled;
        public boolean checked;
        public String visibility;
        public int[] bounds;
        public java.util.List<NodeInfo> children = new ArrayList<>();
    }

    public synchronized String setDeviceProfile(int widthPx, int heightPx) {
        if (widthPx >= 320 && widthPx <= 3840)
            this.viewportWidth = widthPx;
        if (heightPx >= 480 && heightPx <= 3840)
            this.viewportHeight = heightPx;
        return String.format("{\"status\":\"ok\",\"width_px\":%d,\"height_px\":%d}", this.viewportWidth,
                this.viewportHeight);
    }

    public synchronized String launch(String jarOrApkPath, String activityClassName, String optionalApkPath) {
        long start = System.nanoTime();
        TransformationAuditLog.clear();
        nanoandroid.compat.ApiCompatibilityRegistry.clearSessionFaults();
        LifecycleEventTracer.clear();
        Context.clearTrackedSharedPreferences();
        lastInflatedRoot = null;
        ForensicTimeline.clear("launch-" + System.nanoTime());
        ForensicTimeline.record("APK_LAUNCHED", jarOrApkPath, activityClassName != null ? activityClassName : "auto");
        ContentResolver.clearProviders();
        Context.clearServices();
        DeterministicRuntime.reset();
        SystemClock.setUptimeMillis(0);
        runtimeWarnings.clear();
        lastExceptionTrace = "";
        nodeMap.clear();
        activeActivity = null;
        activityStack.clear();
        Dialog.clearActiveDialogs();
        Toast.clearToast();
        lastFocusedNodeId = "";
        refreshStagedFiles();
        if (activeApkTable != null) {
            activeApkTable.close();
            activeApkTable = null;
        }
        inspectorMode = false;
        Context.setGlobalApkTable(null);

        try {
            File primaryFile = new File(jarOrApkPath);
            File jarFile = jarOrApkPath.toLowerCase().endsWith(".jar") ? primaryFile : null;
            File apkFile = jarOrApkPath.toLowerCase().endsWith(".apk") ? primaryFile : null;
            if (optionalApkPath != null && !optionalApkPath.isEmpty()) {
                File optApk = new File(optionalApkPath);
                if (optApk.exists())
                    apkFile = optApk;
            }
            if (apkFile != null) {
                this.activeApkPath = apkFile.getAbsolutePath();
                this.activeApkTable = new ApkResourceTable(apkFile);
                Context.setGlobalApkTable(activeApkTable);
            } else {
                this.activeApkPath = primaryFile.getAbsolutePath();
            }

            if (activeApkTable != null) {
                activeAppPackage = activeApkTable.packageName;
                Context.seedManifestPermissions(activeAppPackage, activeApkTable.permissions);
                if (!activeApkTable.nativeLibs.isEmpty()) {
                    runtimeWarnings.add("Native .so libraries detected (" + activeApkTable.nativeLibs.size() +
                            "): ARM/Bionic native JNI cannot execute inside host Windows JVM; pure JVM & AXML UI layers run normally.");
                }
                if (activityClassName == null || activityClassName.isEmpty()
                        || "auto".equalsIgnoreCase(activityClassName)) {
                    if (!activeApkTable.launcherActivity.isEmpty()) {
                        activityClassName = activeApkTable.launcherActivity;
                    } else if (!activeApkTable.activities.isEmpty()) {
                        activityClassName = activeApkTable.activities.get(0);
                    }
                }
            }

            activeClassLoader = new NanoClassLoader(jarFile, apkFile, HeadlessAppHost.class.getClassLoader());
            Thread.currentThread().setContextClassLoader(activeClassLoader);
            ForensicTimeline.record("CLASS_LOADED", primaryFile.getName(),
                    activeApkTable != null ? ("dex_classes=" + activeClassLoader.getKotlinMetadataVersions().size())
                            : "jar_loaded");

            if ((activityClassName == null || activityClassName.isEmpty() || "auto".equalsIgnoreCase(activityClassName))
                    && jarFile != null) {
                activityClassName = discoverActivityInJar(jarFile, activeClassLoader);
            }
            if (activityClassName == null || activityClassName.isEmpty()
                    || "auto".equalsIgnoreCase(activityClassName)) {
                activityClassName = "MainActivity";
            }
            if (activeAppPackage == null || activeAppPackage.isEmpty()) {
                if (activityClassName.contains(".ui.")) {
                    activeAppPackage = activityClassName.substring(0, activityClassName.indexOf(".ui."));
                } else {
                    activeAppPackage = activityClassName.contains(".")
                            ? activityClassName.substring(0, activityClassName.lastIndexOf('.'))
                            : "com.example.app";
                }
            }
            this.activeActivityName = activityClassName;

            Activity.currentApplication = null;
            // Boot manifest ContentProviders BEFORE Application.onCreate() (matches AOSP
            // boot order)
            if (activeApkTable != null && !activeApkTable.providers.isEmpty()) {
                for (ApkResourceTable.ProviderDecl provDecl : activeApkTable.providers) {
                    try {
                        Class<?> pCls = activeClassLoader.loadClass(provDecl.className);
                        Object pObj = pCls.getDeclaredConstructor().newInstance();
                        if (pObj instanceof ContentProvider) {
                            ContentProvider cp = (ContentProvider) pObj;
                            cp.attachInfo(new Context(activeAppPackage), provDecl.authorities);
                            cp.performCreate();
                            if (provDecl.authorities != null && !provDecl.authorities.isEmpty()) {
                                for (String auth : provDecl.authorities.split(";")) {
                                    ContentResolver.registerProvider(auth.trim(), cp);
                                }
                            }
                        }
                    } catch (Throwable provErr) {
                        runtimeWarnings.add("ContentProvider.onCreate warning (" + provDecl.className + "): "
                                + unwrapThrowable(provErr));
                    }
                }
            }
            // Execute real Application subclass if present in AndroidManifest
            if (activeApkTable != null && !activeApkTable.applicationClass.isEmpty()) {
                try {
                    Class<?> appCls = activeClassLoader.loadClass(activeApkTable.applicationClass);
                    Object appObj = appCls.getDeclaredConstructor().newInstance();
                    if (appObj instanceof Application) {
                        Application app = (Application) appObj;
                        app.setPackageName(activeAppPackage);
                        Context appBase = new Context(activeAppPackage);
                        app.attachBaseContext(appBase);
                        Activity.currentApplication = app;
                        app.onCreate();
                    }
                } catch (Throwable appErr) {
                    runtimeWarnings.add("Application.onCreate warning (" + activeApkTable.applicationClass + "): "
                            + unwrapThrowable(appErr));
                }
            }

            NanoClassLoader.onStartActivityHook = (newIntent) -> handleStartActivityIntent(newIntent, -1);
            NanoClassLoader.onStartActivityForResultHook = this::handleStartActivityIntent;
            NanoClassLoader.onFinishActivityHook = this::handleFinishActivity;
            NanoClassLoader.onRecreateActivityHook = this::handleRecreateActivity;

            pushActivityInstance(activityClassName,
                    new Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LAUNCHER), null, -1);

            long elapsedMs = (System.nanoTime() - start) / 1_000_000;
            return buildLaunchJson(elapsedMs, null);
        } catch (Throwable t) {
            Throwable rootCause = unwrapThrowable(t);
            StringWriter sw = new StringWriter();
            rootCause.printStackTrace(new PrintWriter(sw));
            lastExceptionTrace = sw.toString();
            runtimeWarnings.add("Class/constructor load halted (" + activityClassName + "): "
                    + rootCause.getClass().getSimpleName() + ": " + rootCause.getMessage());
            attachStaticXmlPreviewIfPossible(activityClassName, new Context(activeAppPackage));
            long elapsedMs = (System.nanoTime() - start) / 1_000_000;
            return buildLaunchJson(elapsedMs, rootCause.getClass().getSimpleName() + ": " + rootCause.getMessage());
        }
    }

    private static int countVisibleLeafWidgets(View v) {
        if (v == null || v.getVisibility() != View.VISIBLE)
            return 0;
        if (v instanceof TextView) {
            CharSequence cs = ((TextView) v).getText();
            return (cs != null && !cs.toString().trim().isEmpty()) ? 1 : 0;
        }
        if (v instanceof ImageView) {
            return (((ImageView) v).getDrawable() != null) ? 1 : 0;
        }
        if (v instanceof SeekBar || v instanceof ProgressBar) {
            return 1;
        }
        int total = 0;
        if (v instanceof ViewGroup) {
            ViewGroup vg = (ViewGroup) v;
            for (int i = 0; i < vg.getChildCount(); i++) {
                total += countVisibleLeafWidgets(vg.getChildAt(i));
            }
        }
        return total;
    }

    private void attachStaticXmlPreviewIfPossible(String actClsName, Context ctx) {
        if (activeApkTable == null)
            return;
        int layoutId = findFallbackMainLayout(activeApkTable, actClsName);
        if (layoutId == 0)
            return;
        View preview = BinaryXmlInflater.inflateLayout(layoutId, ctx, null, false, activeApkTable);
        if (preview == null)
            return;
        inspectorMode = true;
        Activity previewAct = new Activity();
        previewAct.attach(ctx, new Intent(Intent.ACTION_MAIN));
        activeActivity = previewAct;
        LinearLayout wrapper = new LinearLayout(ctx);
        wrapper.setOrientation(LinearLayout.VERTICAL);
        TextView banner = new TextView(ctx);
        banner.setBackgroundColor(0xFF991B1B);
        banner.setTextColor(0xFFFFFFFF);
        banner.setTextSize(10f);
        banner.setPadding(24, 14, 24, 14);
        String layoutName = activeApkTable.getEntryName(layoutId);
        banner.setText("Static AXML Preview (res/layout/" + (layoutName != null ? layoutName : layoutId)
                + ") — Host JVM halted execution: connect ADB device for full OS run");
        wrapper.addView(banner, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT));
        wrapper.addView(preview, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.MATCH_PARENT, 1f));
        activeActivity.setContentView(wrapper);
    }

    private static File getStagedFilesDir() {
        String prop = System.getProperty("nano.staged.dir");
        File dir = (prop != null && !prop.trim().isEmpty()) ? new File(prop) : new File("cache", "staged_files");
        if (!dir.exists())
            dir.mkdirs();
        return dir;
    }

    private void refreshStagedFiles() {
        stagedFiles.clear();
        File dir = getStagedFilesDir();
        File sampleVid = new File(dir, "sample_1080p_clip.mp4");
        if (!sampleVid.exists()) {
            writeMinimalSampleMp4(sampleVid);
        }
        File sampleImg = new File(dir, "sample_frame.png");
        if (!sampleImg.exists()) {
            try {
                BufferedImage bi = new BufferedImage(320, 180, BufferedImage.TYPE_INT_ARGB);
                Graphics2D g2 = bi.createGraphics();
                g2.setColor(new Color(14, 165, 233));
                g2.fillRect(0, 0, 320, 180);
                g2.setColor(Color.WHITE);
                g2.drawString("Nano Test Media", 70, 95);
                g2.dispose();
                ImageIO.write(bi, "png", sampleImg);
            } catch (Throwable ignored) {
            }
        }
        File[] list = dir.listFiles();
        if (list != null) {
            Arrays.sort(list, Comparator.comparingLong(File::lastModified).reversed());
            for (File f : list) {
                if (f.isFile() && f.length() > 0) {
                    stagedFiles.add(f);
                    Uri.fromFile(f);
                }
            }
        }
    }

    private static void writeMinimalSampleMp4(File out) {
        try (FileOutputStream fos = new FileOutputStream(out)) {
            // ftyp + moov(mvhd + trak(tkhd)) + mdat padding so MediaMetadataRetriever reads
            // real 1920x1080 / 15s metadata
            byte[] ftyp = new byte[] { 0, 0, 0, 20, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 2, 0, 'i', 's', 'o',
                    'm' };
            ByteArrayOutputStream mvhd = new ByteArrayOutputStream();
            DataOutputStream md = new DataOutputStream(mvhd);
            md.writeInt(108);
            md.writeBytes("mvhd");
            md.writeInt(0);
            md.writeInt(0);
            md.writeInt(0);
            md.writeInt(1000);
            md.writeInt(15000); // 15s @ 1000Hz
            md.writeInt(0x00010000);
            md.writeShort(0x0100);
            md.write(new byte[70]); // matrix + pre_defined + next_track_ID

            ByteArrayOutputStream tkhd = new ByteArrayOutputStream();
            DataOutputStream td = new DataOutputStream(tkhd);
            td.writeInt(92);
            td.writeBytes("tkhd");
            td.writeInt(0x00000007);
            td.writeInt(0);
            td.writeInt(0);
            td.writeInt(1);
            td.writeInt(0);
            td.writeInt(15000);
            td.write(new byte[52]); // reserved + layer/vol + matrix
            td.writeInt(1920 << 16);
            td.writeInt(1080 << 16);

            byte[] trakBytes = tkhd.toByteArray();
            byte[] mvhdBytes = mvhd.toByteArray();
            int moovLen = 8 + mvhdBytes.length + 8 + trakBytes.length;

            fos.write(ftyp);
            DataOutputStream root = new DataOutputStream(fos);
            root.writeInt(moovLen);
            root.writeBytes("moov");
            root.write(mvhdBytes);
            root.writeInt(8 + trakBytes.length);
            root.writeBytes("trak");
            root.write(trakBytes);

            // 64KB synthetic mdat payload so size/bitrate calculations are realistic
            byte[] mdatHdr = new byte[] { 0, 1, 0, 8, 'm', 'd', 'a', 't' };
            fos.write(mdatHdr);
            byte[] buf = new byte[4096];
            for (int i = 0; i < 16; i++)
                fos.write(buf);
        } catch (Throwable ignored) {
        }
    }

    public synchronized String stageFile(String filePath) {
        if (filePath == null || filePath.trim().isEmpty()) {
            return "{\"status\":\"error\",\"error\":\"Missing file path\"}";
        }
        File src = new File(filePath.trim());
        if (!src.exists() || !src.isFile()) {
            return "{\"status\":\"error\",\"error\":\"File not found: " + escapeJson(filePath) + "\"}";
        }
        File dir = getStagedFilesDir();
        File dst = new File(dir, src.getName());
        if (!src.getAbsolutePath().equalsIgnoreCase(dst.getAbsolutePath())) {
            try (InputStream in = new FileInputStream(src); OutputStream out = new FileOutputStream(dst)) {
                byte[] buf = new byte[8192];
                int r;
                while ((r = in.read(buf)) != -1)
                    out.write(buf, 0, r);
            } catch (IOException e) {
                return "{\"status\":\"error\",\"error\":\"" + escapeJson(e.getMessage()) + "\"}";
            }
        }
        refreshStagedFiles();
        Uri uri = Uri.fromFile(dst);
        return String.format("{\"status\":\"ok\",\"name\":\"%s\",\"path\":\"%s\",\"uri\":\"%s\",\"size_bytes\":%d}",
                escapeJson(dst.getName()), escapeJson(dst.getAbsolutePath()), escapeJson(uri.toString()), dst.length());
    }

    public synchronized String listStagedFiles() {
        refreshStagedFiles();
        StringBuilder sb = new StringBuilder("{\"status\":\"ok\",\"files\":[");
        for (int i = 0; i < stagedFiles.size(); i++) {
            File f = stagedFiles.get(i);
            if (i > 0)
                sb.append(",");
            sb.append("{\"name\":\"").append(escapeJson(f.getName()))
                    .append("\",\"path\":\"").append(escapeJson(f.getAbsolutePath()))
                    .append("\",\"uri\":\"").append(escapeJson(Uri.fromFile(f).toString()))
                    .append("\",\"size_bytes\":").append(f.length()).append("}");
        }
        sb.append("]}");
        return sb.toString();
    }

    private void handleStartActivityIntent(Intent rawIntent, int requestCode) {
        if (rawIntent == null)
            return;
        Intent effective = rawIntent;
        if (Intent.ACTION_CHOOSER.equals(rawIntent.getAction()) && rawIntent.hasExtra(Intent.EXTRA_INTENT)) {
            Object inner = rawIntent.getExtras().get(Intent.EXTRA_INTENT);
            if (inner instanceof Intent)
                effective = (Intent) inner;
        }
        String action = effective.getAction();
        if (Intent.ACTION_OPEN_DOCUMENT.equals(action)
                || Intent.ACTION_GET_CONTENT.equals(action)
                || Intent.ACTION_PICK.equals(action)
                || "android.provider.action.PICK_IMAGES".equals(action)
                || Intent.ACTION_OPEN_DOCUMENT_TREE.equals(action)
                || Intent.ACTION_CREATE_DOCUMENT.equals(action)) {
            showFilePickerDialog(effective, requestCode);
            return;
        }
        String targetCls = effective.getComponent() != null ? effective.getComponent().getClassName() : null;
        if ((targetCls == null || targetCls.isEmpty()) && activeApkTable != null) {
            for (ApkResourceTable.ActivityIntentFilter aif : activeApkTable.activityIntentFilters) {
                if (aif.filter.match(effective)) {
                    targetCls = aif.activityName;
                    break;
                }
            }
        }
        if (targetCls != null && !targetCls.isEmpty() && activityStack.size() < 12) {
            if ((effective.getFlags() & Intent.FLAG_ACTIVITY_SINGLE_TOP) != 0
                    && activeActivity != null && targetCls.equals(activeActivityName)) {
                try {
                    activeActivity.performNewIntent(effective);
                } catch (Throwable err) {
                    runtimeWarnings.add("onNewIntent warning (" + targetCls + "): " + unwrapThrowable(err));
                }
                return;
            }
            ActivityFrame caller = activityStack.peekLast();
            try {
                if (caller != null && caller.activity != null) {
                    caller.activity.performPause();
                }
                pushActivityInstance(targetCls, effective, caller, requestCode);
            } catch (Throwable err) {
                runtimeWarnings.add("Navigation error (" + targetCls + "): " + unwrapThrowable(err));
            }
        }
    }

    private void showFilePickerDialog(Intent pickerIntent, int requestCode) {
        refreshStagedFiles();
        Activity caller = activeActivity;
        if (caller == null)
            return;
        String mime = pickerIntent.getType() != null ? pickerIntent.getType() : "*/*";
        String title = Intent.ACTION_CREATE_DOCUMENT.equals(pickerIntent.getAction())
                ? "Create Output File (" + mime + ")"
                : "Select Input File (" + mime + ")";

        if (Intent.ACTION_CREATE_DOCUMENT.equals(pickerIntent.getAction())) {
            String docName = pickerIntent.getStringExtra(Intent.EXTRA_TITLE);
            if (docName == null || docName.trim().isEmpty())
                docName = "output_file.mp4";
            File out = new File(getStagedFilesDir(), docName.replaceAll("[^a-zA-Z0-9._-]", "_"));
            Uri outUri = Uri.fromFile(out);
            Intent res = new Intent();
            res.setData(outUri);
            caller.dispatchActivityResult(requestCode, Activity.RESULT_OK, res);
            return;
        }

        CharSequence[] labels = new CharSequence[stagedFiles.size()];
        for (int i = 0; i < stagedFiles.size(); i++) {
            File f = stagedFiles.get(i);
            long kb = Math.max(1L, f.length() / 1024L);
            labels[i] = f.getName() + " ("
                    + (kb >= 1024 ? String.format(Locale.US, "%.1f MB", kb / 1024.0) : (kb + " KB")) + ")";
        }

        new AlertDialog.Builder(caller)
                .setTitle(title)
                .setItems(labels, (dlg, which) -> {
                    if (which >= 0 && which < stagedFiles.size()) {
                        File chosen = stagedFiles.get(which);
                        Uri chosenUri = Uri.fromFile(chosen);
                        Intent data = new Intent();
                        data.setData(chosenUri);
                        data.setClipData(
                                new ClipData(chosen.getName(), new String[] { mime }, new ClipData.Item(chosenUri)));
                        caller.dispatchActivityResult(requestCode, Activity.RESULT_OK, data);
                    }
                })
                .setNegativeButton("Cancel",
                        (dlg, which) -> caller.dispatchActivityResult(requestCode, Activity.RESULT_CANCELED, null))
                .show();
    }

    private void handleRecreateActivity(Activity targetAct) {
        if (targetAct == null)
            return;
        ActivityFrame top = activityStack.peekLast();
        if (top == null || top.activity != targetAct)
            return;
        Bundle savedState = new Bundle();
        try {
            targetAct.performPause();
            targetAct.performSaveInstanceState(savedState);
            targetAct.performStop();
            targetAct.performDestroy();
        } catch (Throwable err) {
            runtimeWarnings.add("recreate() teardown warning: " + unwrapThrowable(err));
        }
        activityStack.removeLast();
        try {
            pushActivityInstance(top.className, targetAct.getIntent(), top.caller, top.requestCode, savedState);
        } catch (Throwable err) {
            runtimeWarnings.add("recreate() restore warning: " + unwrapThrowable(err));
        }
    }

    public synchronized String recreateActiveActivity() {
        if (activeActivity == null) {
            return "{\"status\":\"error\",\"error\":\"No active activity\"}";
        }
        handleRecreateActivity(activeActivity);
        return String.format("{\"status\":\"ok\",\"action\":\"recreate\",\"activity\":\"%s\"}",
                escapeJson(activeActivityName));
    }

    public synchronized String simulateProcessDeathAndRestore() {
        if (activeActivity == null || activityStack.isEmpty()) {
            return "{\"status\":\"error\",\"error\":\"No active activity\"}";
        }
        ActivityFrame top = activityStack.peekLast();
        Intent lastIntent = top.activity.getIntent();
        Bundle savedState = new Bundle();
        try {
            top.activity.performPause();
            top.activity.performSaveInstanceState(savedState);
            top.activity.performStop();
            top.activity.performDestroy();
        } catch (Throwable err) {
            runtimeWarnings.add("Process death save warning: " + unwrapThrowable(err));
        }
        activityStack.removeLast();
        activeActivity = null;
        NanoClassLoader.currentActivity = null;
        try {
            pushActivityInstance(top.className, lastIntent, top.caller, top.requestCode, savedState);
            return String.format(
                    "{\"status\":\"ok\",\"action\":\"process_death_restore\",\"activity\":\"%s\",\"saved_keys\":%d}",
                    escapeJson(top.className), savedState.keySet().size());
        } catch (Throwable err) {
            Throwable rc = unwrapThrowable(err);
            return String.format("{\"status\":\"error\",\"action\":\"process_death_restore\",\"error\":\"%s\"}",
                    escapeJson(rc.getClass().getSimpleName() + ": " + rc.getMessage()));
        }
    }

    private void handleFinishActivity(Activity finishedAct) {
        if (activityStack.size() <= 1)
            return;
        ActivityFrame top = activityStack.peekLast();
        if (top == null || top.activity != finishedAct)
            return;
        activityStack.removeLast();
        try {
            finishedAct.performPause();
            finishedAct.performStop();
            finishedAct.performDestroy();
        } catch (Throwable ignored) {
        }

        ActivityFrame parent = activityStack.peekLast();
        if (parent != null) {
            activeActivity = parent.activity;
            activeActivityName = parent.className;
            NanoClassLoader.currentActivity = activeActivity;
            if (top.requestCode >= 0) {
                try {
                    parent.activity.dispatchActivityResult(top.requestCode, finishedAct.getResultCode(),
                            finishedAct.getResultData());
                } catch (Throwable err) {
                    runtimeWarnings.add("onActivityResult warning: " + unwrapThrowable(err));
                }
            }
            try {
                parent.activity.performResume();
            } catch (Throwable ignored) {
            }
        }
    }

    private void pushActivityInstance(String actClsName, Intent launchIntent, ActivityFrame caller, int requestCode)
            throws Exception {
        pushActivityInstance(actClsName, launchIntent, caller, requestCode, null);
    }

    private void pushActivityInstance(String actClsName, Intent launchIntent, ActivityFrame caller, int requestCode,
            Bundle savedInstanceState) throws Exception {
        Class<?> actClass = activeClassLoader.loadClass(actClsName);
        Activity newAct = (Activity) actClass.getDeclaredConstructor().newInstance();
        activeActivity = newAct;
        activeActivityName = actClsName;
        NanoClassLoader.currentActivity = newAct;
        ActivityFrame frame = new ActivityFrame(actClsName, newAct, caller, requestCode);
        activityStack.addLast(frame);

        Context ctx = new Context(activeAppPackage);
        newAct.attach(ctx, launchIntent != null ? launchIntent : new Intent(Intent.ACTION_MAIN));

        if (activeApkTable != null) {
            int themeId = activeApkTable.launcherThemeResId != 0 ? activeApkTable.launcherThemeResId
                    : activeApkTable.appThemeResId;
            if (themeId != 0)
                newAct.setTheme(themeId);
        }

        try {
            newAct.performCreate(savedInstanceState != null ? new Bundle(savedInstanceState) : null);
            ForensicTimeline.record("ACTIVITY_CREATED", actClsName,
                    savedInstanceState != null ? "restored_state" : "fresh");
            // Only continue onStart/onResume if onCreate didn't immediately redirect or
            // finish
            if (activeActivity == newAct && !newAct.isFinishing()) {
                newAct.performStart();
                if (savedInstanceState != null) {
                    newAct.performRestoreInstanceState(new Bundle(savedInstanceState));
                }
                newAct.performResume();
                ForensicTimeline.record("CALLBACK_EXECUTED", actClsName, "onStart+onResume");
            }
        } catch (Throwable lifeErr) {
            Throwable rootCause = unwrapThrowable(lifeErr);
            StringWriter sw = new StringWriter();
            rootCause.printStackTrace(new PrintWriter(sw));
            lastExceptionTrace = sw.toString();
            ForensicTimeline.record("EXCEPTION_RAISED", actClsName,
                    rootCause.getClass().getSimpleName() + ": " + rootCause.getMessage());
            runtimeWarnings.add("Activity lifecycle exception (" + actClsName + "): "
                    + rootCause.getClass().getSimpleName() + ": " + rootCause.getMessage());
        }

        if (activeActivity == newAct && !lastExceptionTrace.isEmpty() && activityStack.size() == 1) {
            inspectorMode = true;
            View measured = resolveMeasuredRoot();
            if (measured == null || countVisibleLeafWidgets(measured) == 0) {
                attachStaticXmlPreviewIfPossible(actClsName, ctx);
            }
        }
    }

    private int findFallbackMainLayout(ApkResourceTable table, String actClsName) {
        String simple = actClsName.contains(".") ? actClsName.substring(actClsName.lastIndexOf('.') + 1) : actClsName;
        String snake = simple.replaceAll("([a-z])([A-Z])", "$1_$2").toLowerCase();
        String[] candidates = new String[] {
                "activity_" + snake.replace("_activity", ""),
                snake,
                "activity_main",
                "main_activity",
                "fragment_main"
        };
        for (String name : candidates) {
            int id = table.getIdentifier(name, "layout", table.packageName);
            if (id != 0 && table.getFilePath(id) != null)
                return id;
        }
        return 0;
    }

    private String resolveCanonicalResultStatus(String fatalError) {
        if (fatalError != null && !fatalError.isEmpty()) {
            if (nanoandroid.compat.ApiCompatibilityRegistry.hasInvalidatingFault()) {
                return "UNSUPPORTED";
            }
            return "NANO_SIMULATION_FAIL";
        }
        if (nanoandroid.compat.ApiCompatibilityRegistry.hasInvalidatingFault()) {
            return "UNSUPPORTED";
        }
        if (TransformationAuditLog.hasDegradedHeuristic()) {
            return "DEGRADED_HEURISTIC";
        }
        if (!runtimeWarnings.isEmpty() || !nanoandroid.compat.ApiCompatibilityRegistry.getSessionFaults().isEmpty()) {
            return "BEHAVIORAL_DIFF";
        }
        return "NANO_SIMULATION_PASS";
    }

    private String buildLaunchJson(long elapsedMs, String fatalError) {
        boolean degraded = TransformationAuditLog.hasDegradedHeuristic();
        int transformCount = TransformationAuditLog.size();
        int faultCount = nanoandroid.compat.ApiCompatibilityRegistry.getSessionFaults().size();
        int kotlinClasses = activeClassLoader != null ? activeClassLoader.getKotlinMetadataVersions().size() : 0;
        String canonicalStatus = resolveCanonicalResultStatus(fatalError);

        StringBuilder sb = new StringBuilder();
        sb.append("{");
        if (fatalError != null) {
            sb.append("\"status\":\"error\",\"error\":\"").append(escapeJson(fatalError)).append("\",");
        } else if (!runtimeWarnings.isEmpty() && !lastExceptionTrace.isEmpty()) {
            sb.append("\"status\":\"partial\",");
        } else {
            sb.append("\"status\":\"ok\",");
        }
        sb.append("\"result_status\":\"").append(canonicalStatus).append("\",");
        sb.append("\"degraded_heuristic\":").append(degraded).append(",");
        sb.append("\"transformation_count\":").append(transformCount).append(",");
        sb.append("\"compat_fault_count\":").append(faultCount).append(",");
        sb.append("\"kotlin_metadata_classes\":").append(kotlinClasses).append(",");
        sb.append("\"package\":\"").append(escapeJson(activeAppPackage)).append("\",");
        sb.append("\"activity\":\"").append(escapeJson(activeActivityName)).append("\",");
        sb.append("\"state\":\"").append(fatalError == null ? "RESUMED" : "ERROR").append("\",");
        sb.append("\"startup_ms\":").append(elapsedMs);
        if (activeApkTable != null) {
            sb.append(",\"app_label\":\"").append(escapeJson(activeApkTable.appLabel)).append("\"");
            sb.append(",\"min_sdk\":").append(activeApkTable.minSdk);
            sb.append(",\"target_sdk\":").append(activeApkTable.targetSdk);
            sb.append(",\"activities\":[");
            for (int i = 0; i < Math.min(30, activeApkTable.activities.size()); i++) {
                if (i > 0)
                    sb.append(",");
                sb.append("\"").append(escapeJson(activeApkTable.activities.get(i))).append("\"");
            }
            sb.append("]");
            sb.append(",\"native_libs\":").append(activeApkTable.nativeLibs.size());
        }
        if (!runtimeWarnings.isEmpty()) {
            sb.append(",\"warnings\":[");
            for (int i = 0; i < runtimeWarnings.size(); i++) {
                if (i > 0)
                    sb.append(",");
                sb.append("\"").append(escapeJson(runtimeWarnings.get(i))).append("\"");
            }
            sb.append("]");
        }
        if (!lastExceptionTrace.isEmpty()) {
            String trimmed = lastExceptionTrace.length() > 1600 ? lastExceptionTrace.substring(0, 1600)
                    : lastExceptionTrace;
            sb.append(",\"exception_trace\":\"").append(escapeJson(trimmed)).append("\"");
        }
        sb.append("}");
        return sb.toString();
    }

    private View resolveMeasuredRoot() {
        if (activeActivity == null)
            return null;
        View root = activeActivity.getWindow() != null ? activeActivity.getWindow().getDecorView() : null;
        View content = activeActivity.getContentView();
        if (root instanceof ViewGroup && content != null && content.getParent() == null && root != content) {
            ((ViewGroup) root).addView(content);
        }
        if (root == null)
            root = content;
        if (root != null) {
            if (root != lastInflatedRoot) {
                lastInflatedRoot = root;
                ForensicTimeline.record("VIEW_INFLATED", activeActivityName, root.getClass().getSimpleName());
            }
            int appH = viewportHeight - STATUS_BAR_H - NAV_BAR_H;
            int wSpec = View.MeasureSpec.makeMeasureSpec(viewportWidth, View.MeasureSpec.EXACTLY);
            int hSpec = View.MeasureSpec.makeMeasureSpec(appH, View.MeasureSpec.EXACTLY);
            root.measure(wSpec, hSpec);
            root.layout(0, STATUS_BAR_H, viewportWidth, STATUS_BAR_H + appH);
        }
        return root;
    }

    private View resolveDialogRoot() {
        Dialog topDlg = Dialog.getTopActiveDialog();
        if (topDlg == null || topDlg.getWindow() == null)
            return null;
        View dlgRoot = topDlg.getWindow().getDecorView();
        if (dlgRoot == null)
            return null;
        int maxW = viewportWidth - 120;
        int maxH = viewportHeight - STATUS_BAR_H - NAV_BAR_H - 180;
        android.view.WindowManager.LayoutParams lp = topDlg.getWindow().getAttributes();
        boolean fullW = lp != null && lp.width == ViewGroup.LayoutParams.MATCH_PARENT;
        boolean fullH = lp != null && lp.height == ViewGroup.LayoutParams.MATCH_PARENT;
        int wSpec = View.MeasureSpec.makeMeasureSpec(maxW, fullW ? View.MeasureSpec.EXACTLY : View.MeasureSpec.AT_MOST);
        int hSpec = View.MeasureSpec.makeMeasureSpec(maxH, fullH ? View.MeasureSpec.EXACTLY : View.MeasureSpec.AT_MOST);
        dlgRoot.measure(wSpec, hSpec);
        int dw = fullW ? maxW
                : Math.max(420, Math.min(maxW, dlgRoot.getMeasuredWidth() > 0 ? dlgRoot.getMeasuredWidth() : maxW));
        int dh = fullH ? maxH
                : Math.max(220, Math.min(maxH, dlgRoot.getMeasuredHeight() > 0 ? dlgRoot.getMeasuredHeight() : 600));
        int left = (viewportWidth - dw) / 2;
        int top = STATUS_BAR_H + (viewportHeight - STATUS_BAR_H - NAV_BAR_H - dh) / 2;
        dlgRoot.layout(left, top, left + dw, top + dh);
        return dlgRoot;
    }

    public synchronized String observe() {
        if (activeActivity == null) {
            return String.format("{\"status\":\"error\",\"error\":\"No active activity\",\"exception_trace\":\"%s\"}",
                    escapeJson(lastExceptionTrace));
        }

        View root = resolveMeasuredRoot();
        View dlgRoot = resolveDialogRoot();
        if (root == null && dlgRoot == null) {
            return "{\"status\":\"ok\",\"screen\":{\"id\":\"root\",\"class\":\"EmptyWindow\",\"text\":\"No View hierarchy attached (Activity crashed or uses unsupported Compose/Native canvas)\",\"clickable\":false,\"bounds\":[0,68,1080,2332],\"children\":[]}}";
        }

        nodeMap.clear();
        nodeCounter = 0;
        Set<View> visited = Collections.newSetFromMap(new IdentityHashMap<>());
        NodeInfo rootNode = root != null
                ? buildAst(root, visited, 0, 0, 0)
                : new NodeInfo();
        if (rootNode.id == null) {
            rootNode.id = "root";
            rootNode.clazz = "FrameLayout";
            rootNode.text = "";
            rootNode.bounds = new int[] { 0, STATUS_BAR_H, SCREEN_WIDTH, SCREEN_HEIGHT - NAV_BAR_H };
        }
        if (dlgRoot != null) {
            NodeInfo dlgNode = buildAst(dlgRoot, visited, 1, 0, 0);
            if (dlgNode != null) {
                rootNode.children.add(dlgNode);
            }
        }

        Dialog topDlg = Dialog.getTopActiveDialog();
        String toast = Toast.getCurrentToastText();

        StringBuilder sb = new StringBuilder();
        sb.append("{\"status\":\"ok\",\"activity\":\"").append(escapeJson(activeActivityName)).append("\"");
        sb.append(",\"activity_stack\":[");
        int sIdx = 0;
        for (ActivityFrame f : activityStack) {
            if (sIdx++ > 0)
                sb.append(",");
            sb.append("\"").append(escapeJson(f.className)).append("\"");
        }
        sb.append("]");
        if (topDlg != null) {
            String dTitle = topDlg.getTitle() != null && topDlg.getTitle().length() > 0
                    ? topDlg.getTitle().toString()
                    : topDlg.getClass().getSimpleName();
            sb.append(",\"active_dialog\":\"").append(escapeJson(dTitle)).append("\"");
        }
        if (toast != null && !toast.isEmpty()) {
            sb.append(",\"toast\":\"").append(escapeJson(toast)).append("\"");
        }
        sb.append(",\"screen\":");
        serializeNode(rootNode, sb);
        sb.append("}");
        return sb.toString();
    }

    public synchronized String renderScreen(String outputPath) {
        View root = resolveMeasuredRoot();
        BufferedImage img = new BufferedImage(viewportWidth, viewportHeight, BufferedImage.TYPE_INT_ARGB);
        Graphics2D g = img.createGraphics();

        try {
            g.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON);
            g.setRenderingHint(RenderingHints.KEY_TEXT_ANTIALIASING, RenderingHints.VALUE_TEXT_ANTIALIAS_ON);
            g.setRenderingHint(RenderingHints.KEY_INTERPOLATION, RenderingHints.VALUE_INTERPOLATION_BILINEAR);

            // Determine theme window background color
            int windowBg = 0xFFFFFFFF;
            if (activeApkTable != null) {
                // android:attr/windowBackground = 0x01010054
                ApkResourceTable.ResValue rv = activeApkTable.resolveThemeAttr(0, 0x01010054);
                if (rv != null && rv.dataType >= 0x1c && rv.dataType <= 0x1f) {
                    windowBg = ApkResourceTable.expandColor(rv.dataType, rv.data);
                }
            }
            if (root != null && root.getBackground() instanceof ColorDrawable) {
                int c = ((ColorDrawable) root.getBackground()).getColor();
                if ((c >>> 24) > 0)
                    windowBg = c;
            }
            g.setColor(new Color(windowBg, true));
            g.fillRect(0, 0, viewportWidth, viewportHeight);

            // Real Android Status Bar
            int statusColor = (activeActivity != null && activeActivity.getWindow() != null
                    && activeActivity.getWindow().getStatusBarColor() != 0)
                            ? activeActivity.getWindow().getStatusBarColor()
                            : 0xFF0F172A;
            g.setColor(new Color(statusColor, true));
            g.fillRect(0, 0, viewportWidth, STATUS_BAR_H);
            g.setColor(contrastText(statusColor));
            g.setFont(new Font("SansSerif", Font.BOLD, 28));
            g.drawString("09:41", 40, 44);
            String title = (activeApkTable != null && !activeApkTable.appLabel.isEmpty())
                    ? activeApkTable.appLabel
                    : activeActivityName.substring(activeActivityName.lastIndexOf('.') + 1);
            g.setFont(new Font("SansSerif", Font.PLAIN, 24));
            g.drawString(title, 180, 43);
            g.drawString("5G  100%", Math.max(200, viewportWidth - 170), 44);

            // Render actual live View hierarchy OR honest diagnostic banner if Activity
            // crashed before setContentView
            if (root != null) {
                Set<View> visited = Collections.newSetFromMap(new IdentityHashMap<>());
                drawView(g, root, visited, 0);
            } else {
                drawDiagnosticScreen(g);
            }

            // Render active Dialog / AlertDialog window overlay above Activity
            View dlgRoot = resolveDialogRoot();
            if (dlgRoot != null) {
                g.setColor(new Color(0, 0, 0, 155));
                g.fillRect(0, STATUS_BAR_H, viewportWidth, viewportHeight - STATUS_BAR_H - NAV_BAR_H);
                Set<View> dlgVisited = Collections.newSetFromMap(new IdentityHashMap<>());
                drawView(g, dlgRoot, dlgVisited, 0);
            }

            // Render active Android Toast banner near bottom
            String toastText = Toast.getCurrentToastText();
            if (toastText != null && !toastText.isEmpty()) {
                g.setFont(new Font("SansSerif", Font.PLAIN, 28));
                FontMetrics fm = g.getFontMetrics();
                int tw = Math.min(viewportWidth - 140, fm.stringWidth(toastText) + 72);
                int th = fm.getHeight() + 34;
                int tx = (viewportWidth - tw) / 2;
                int ty = viewportHeight - NAV_BAR_H - th - 64;
                g.setColor(new Color(15, 23, 42, 238));
                g.fillRoundRect(tx, ty, tw, th, 44, 44);
                g.setColor(new Color(56, 189, 248, 120));
                g.setStroke(new BasicStroke(2f));
                g.drawRoundRect(tx, ty, tw, th, 44, 44);
                g.setColor(Color.WHITE);
                g.drawString(toastText, tx + 36, ty + (th - fm.getHeight()) / 2 + fm.getAscent());
            }

            // Real Android Bottom Gesture Bar
            int navColor = (activeActivity != null && activeActivity.getWindow() != null
                    && activeActivity.getWindow().getNavigationBarColor() != 0)
                            ? activeActivity.getWindow().getNavigationBarColor()
                            : 0xFF0F172A;
            g.setColor(new Color(navColor, true));
            g.fillRect(0, viewportHeight - NAV_BAR_H, viewportWidth, NAV_BAR_H);
            g.setColor(new Color(148, 163, 184));
            g.fillRoundRect(viewportWidth / 2 - 140, viewportHeight - 36, 280, 8, 8, 8);
        } finally {
            g.dispose();
        }

        File outFile = new File(outputPath);
        if (outFile.getParentFile() != null)
            outFile.getParentFile().mkdirs();
        try {
            ImageIO.write(img, "png", outFile);
            ForensicTimeline.record("SCREENSHOT_CAPTURED", activeActivityName,
                    viewportWidth + "x" + viewportHeight + " -> " + outFile.getName());
            return String.format("{\"status\":\"ok\",\"file\":\"%s\",\"width\":%d,\"height\":%d}",
                    escapeJson(outFile.getAbsolutePath()), viewportWidth, viewportHeight);
        } catch (IOException e) {
            return String.format("{\"status\":\"error\",\"error\":\"%s\"}", escapeJson(e.getMessage()));
        }
    }

    private void drawDiagnosticScreen(Graphics2D g) {
        g.setColor(new Color(15, 23, 42));
        g.fillRect(0, STATUS_BAR_H, SCREEN_WIDTH, SCREEN_HEIGHT - STATUS_BAR_H - NAV_BAR_H);
        g.setColor(new Color(239, 68, 68));
        g.fillRoundRect(60, 140, SCREEN_WIDTH - 120, 480, 24, 24);
        g.setColor(new Color(30, 41, 59));
        g.fillRoundRect(64, 144, SCREEN_WIDTH - 128, 472, 20, 20);
        g.setColor(Color.WHITE);
        g.setFont(new Font("SansSerif", Font.BOLD, 34));
        g.drawString("APK Runtime Diagnostic (No Views Rendered)", 96, 210);
        g.setFont(new Font("Monospaced", Font.PLAIN, 22));
        g.setColor(new Color(248, 113, 113));
        String msg = !runtimeWarnings.isEmpty() ? runtimeWarnings.get(0)
                : "Activity exited without attaching ContentView";
        int y = 270;
        for (String line : TextView.wrapLines(msg, g.getFontMetrics(), SCREEN_WIDTH - 200)) {
            g.drawString(line, 96, y);
            y += 32;
            if (y > 580)
                break;
        }
    }

    private void drawView(Graphics2D g, View v, Set<View> visited, int depth) {
        if (v == null || v.getVisibility() != View.VISIBLE || depth > 40 || !visited.add(v)) {
            return;
        }
        int l = v.getLeft();
        int t = v.getTop();
        int w = v.getWidth();
        int h = v.getHeight();
        if (w <= 0 || h <= 0) {
            return;
        }

        // Apply alpha if < 1.0f
        Composite prevComp = g.getComposite();
        if (v.getAlpha() < 0.99f) {
            g.setComposite(
                    AlphaComposite.getInstance(AlphaComposite.SRC_OVER, Math.max(0f, Math.min(1f, v.getAlpha()))));
        }

        Shape oldClip = g.getClip();
        Shape myClip = new Rectangle(l, t, w, h);
        g.clip(myClip);

        // 1. Background Drawable
        drawDrawable(g, v.getBackground(), l, t, w, h);

        // 2. View-specific content
        if (v instanceof Switch) {
            drawSwitch(g, (Switch) v, l, t, w, h);
        } else if (v instanceof CheckBox) {
            drawCheckBox(g, (CheckBox) v, l, t, w, h);
        } else if (v instanceof RadioButton) {
            drawRadioButton(g, (RadioButton) v, l, t, w, h);
        } else if (v instanceof SeekBar) {
            drawSeekBar(g, (SeekBar) v, l, t, w, h, true);
        } else if (v instanceof ProgressBar) {
            drawSeekBar(g, (ProgressBar) v, l, t, w, h, false);
        } else if (v instanceof EditText) {
            drawEditText(g, (EditText) v, l, t, w, h);
        } else if (v instanceof TextView) {
            drawTextView(g, (TextView) v, l, t, w, h);
        } else if (v instanceof ImageView) {
            Drawable d = ((ImageView) v).getDrawable();
            if (d != null) {
                drawDrawable(g, d,
                        l + v.getPaddingLeft(), t + v.getPaddingTop(),
                        Math.max(1, w - v.getPaddingLeft() - v.getPaddingRight()),
                        Math.max(1, h - v.getPaddingTop() - v.getPaddingBottom()));
            }
        }

        // 3. Structural wireframe outline during AXML Inspector / Static Preview ONLY
        // (never during live runtime)
        if (inspectorMode && depth >= 2 && v.getBackground() == null && !(v instanceof TextView)
                && !(v instanceof ScrollView)) {
            String rid = v.getResourceIdName();
            String scls = v.getClass().getSimpleName();
            String lowCls = scls.toLowerCase();
            if (!rid.isEmpty() || lowCls.contains("fragment") || lowCls.contains("toolbar")
                    || lowCls.contains("recycler") || lowCls.contains("drawer") || lowCls.contains("navigation")) {
                g.setColor(new Color(56, 189, 248, 85));
                g.setStroke(new BasicStroke(2f, BasicStroke.CAP_BUTT, BasicStroke.JOIN_MITER, 10f,
                        new float[] { 8f, 6f }, 0f));
                g.drawRect(l + 1, t + 1, Math.max(1, w - 3), Math.max(1, h - 3));
                String badgeTxt = !rid.isEmpty() ? ("#" + rid + " (" + scls + ")") : scls;
                g.setFont(new Font("Monospaced", Font.PLAIN, 18));
                g.setColor(new Color(71, 85, 105, 190));
                if (w >= 140 && h >= 42) {
                    g.drawString(badgeTxt, l + 10, t + 24);
                }
            }
        }

        // 4. Children (with ScrollView translation)
        if (v instanceof ViewGroup) {
            ViewGroup vg = (ViewGroup) v;
            int sx = 0, sy = 0;
            if (v instanceof ScrollView) {
                sx = ((ScrollView) v).getScrollX();
                sy = ((ScrollView) v).getScrollY();
            } else if (v instanceof HorizontalScrollView) {
                sx = ((HorizontalScrollView) v).getScrollX();
                sy = ((HorizontalScrollView) v).getScrollY();
            }
            if (sx != 0 || sy != 0) {
                g.translate(-sx, -sy);
            }
            for (int i = 0; i < vg.getChildCount(); i++) {
                drawView(g, vg.getChildAt(i), visited, depth + 1);
            }
            if (sx != 0 || sy != 0) {
                g.translate(sx, sy);
            }

            // Draw ScrollView thumb indicator if content taller than viewport
            if (v instanceof ScrollView && vg.getChildCount() > 0 && vg.getChildAt(0) != null) {
                int ch = vg.getChildAt(0).getMeasuredHeight();
                if (ch > h && h > 0) {
                    int thumbH = Math.max(40, (int) ((double) h * h / ch));
                    int maxScroll = Math.max(1, ch - h);
                    int thumbY = t + (int) ((double) sy / maxScroll * (h - thumbH));
                    g.setColor(new Color(148, 163, 184, 150));
                    g.fillRoundRect(l + w - 10, thumbY, 6, thumbH, 6, 6);
                }
            }
        }

        g.setClip(oldClip);
        g.setComposite(prevComp);
    }

    private void drawDrawable(Graphics2D g, Drawable d, int x, int y, int w, int h) {
        if (d == null || w <= 0 || h <= 0)
            return;

        if (d instanceof ColorDrawable) {
            int argb = ((ColorDrawable) d).getColor();
            if ((argb >>> 24) == 0)
                return;
            g.setColor(new Color(argb, true));
            g.fillRect(x, y, w, h);
        } else if (d instanceof GradientDrawable) {
            GradientDrawable gd = (GradientDrawable) d;
            float cr = Math.min(gd.getCornerRadius(), Math.min(w, h) / 2f);
            boolean oval = gd.getShape() == GradientDrawable.OVAL;
            Shape shape = oval
                    ? new Ellipse2D.Float(x, y, w, h)
                    : (cr > 0 ? new RoundRectangle2D.Float(x, y, w, h, cr * 2f, cr * 2f) : new Rectangle(x, y, w, h));

            int[] cols = gd.getColors();
            if (cols != null && cols.length >= 2) {
                Color c1 = new Color(cols[0], true);
                Color c2 = new Color(cols[cols.length - 1], true);
                Paint p;
                if (gd.getOrientation() == GradientDrawable.Orientation.LEFT_RIGHT) {
                    p = new GradientPaint(x, y, c1, x + w, y, c2);
                } else if (gd.getOrientation() == GradientDrawable.Orientation.TL_BR) {
                    p = new GradientPaint(x, y, c1, x + w, y + h, c2);
                } else {
                    p = new GradientPaint(x, y, c1, x, y + h, c2);
                }
                Paint oldP = g.getPaint();
                g.setPaint(p);
                g.fill(shape);
                g.setPaint(oldP);
            } else if ((gd.getColor() >>> 24) > 0) {
                g.setColor(new Color(gd.getColor(), true));
                g.fill(shape);
            }

            if (gd.getStrokeWidth() > 0 && (gd.getStrokeColor() >>> 24) > 0) {
                g.setColor(new Color(gd.getStrokeColor(), true));
                g.setStroke(new BasicStroke(Math.max(1f, gd.getStrokeWidth())));
                Shape strokeShape = oval
                        ? new Ellipse2D.Float(x + 1, y + 1, Math.max(1, w - 2), Math.max(1, h - 2))
                        : new RoundRectangle2D.Float(x + 1, y + 1, Math.max(1, w - 2), Math.max(1, h - 2), cr * 2f,
                                cr * 2f);
                g.draw(strokeShape);
            }
        } else if (d instanceof BitmapDrawable) {
            Bitmap bmp = ((BitmapDrawable) d).getBitmap();
            if (bmp != null && bmp.getAwtImage() != null) {
                BufferedImage bi = bmp.getAwtImage();
                double scale = Math.min((double) w / Math.max(1, bi.getWidth()),
                        (double) h / Math.max(1, bi.getHeight()));
                int dw = Math.max(1, (int) Math.round(bi.getWidth() * scale));
                int dh = Math.max(1, (int) Math.round(bi.getHeight() * scale));
                int dx = x + (w - dw) / 2;
                int dy = y + (h - dh) / 2;
                g.drawImage(bi, dx, dy, dw, dh, null);
            }
        } else if (d instanceof LayerDrawable) {
            LayerDrawable ld = (LayerDrawable) d;
            for (int i = 0; i < ld.getNumberOfLayers(); i++) {
                drawDrawable(g, ld.getDrawable(i), x, y, w, h);
            }
        }
    }

    private void drawTextView(Graphics2D g, TextView tv, int l, int t, int w, int h) {
        if (tv instanceof Button && tv.getBackground() == null) {
            g.setColor(new Color(37, 99, 235));
            g.fillRoundRect(l + 2, t + 2, Math.max(1, w - 4), Math.max(1, h - 4), 16, 16);
        }

        CharSequence cs = tv.getText();
        String txt = cs != null ? cs.toString() : "";
        if (txt.isEmpty())
            return;

        int argb = tv.getCurrentTextColor();
        if (tv instanceof Button && tv.getBackground() == null && argb == 0xFF000000) {
            argb = 0xFFFFFFFF;
        } else if (inspectorMode && tv.getBackground() == null && !(tv instanceof Button)
                && (argb & 0x00FFFFFF) == 0x00FFFFFF) {
            argb = 0xFF0F172A;
        }
        g.setColor(new Color(argb, true));
        Font font = tv.createAwtFont();
        g.setFont(font);
        FontMetrics fm = g.getFontMetrics();

        int pL = tv.getPaddingLeft();
        int pT = tv.getPaddingTop();
        int pR = tv.getPaddingRight();
        int pB = tv.getPaddingBottom();
        int contentW = Math.max(10, w - pL - pR);
        int contentH = Math.max(10, h - pT - pB);

        java.util.List<String> lines = TextView.wrapLines(txt, fm, contentW);
        int lineH = Math.max(18, fm.getHeight());
        int totalTextH = lines.size() * lineH;

        int grav = tv.getGravity();
        boolean centerV = (grav & Gravity.CENTER_VERTICAL) == Gravity.CENTER_VERTICAL
                || (tv instanceof Button && (grav & Gravity.TOP) != Gravity.TOP);
        boolean centerH = (grav & Gravity.CENTER_HORIZONTAL) == Gravity.CENTER_HORIZONTAL
                || (tv instanceof Button && (grav & Gravity.LEFT) != Gravity.LEFT);
        boolean rightH = (grav & Gravity.RIGHT) == Gravity.RIGHT;

        int startY = t + pT;
        if (centerV && contentH > totalTextH) {
            startY = t + pT + (contentH - totalTextH) / 2;
        }

        for (int i = 0; i < lines.size(); i++) {
            String line = lines.get(i);
            int lw = fm.stringWidth(line);
            int x = l + pL;
            if (centerH) {
                x = l + pL + Math.max(0, (contentW - lw) / 2);
            } else if (rightH) {
                x = l + w - pR - lw;
            }
            int baselineY = startY + i * lineH + fm.getAscent();
            g.drawString(line, x, baselineY);
        }
    }

    private void drawSwitch(Graphics2D g, Switch sw, int l, int t, int w, int h) {
        int trackW = 112, trackH = 60;
        int tx = l + w - sw.getPaddingRight() - trackW - 8;
        int ty = t + Math.max(0, (h - trackH) / 2);

        CharSequence cs = sw.getText();
        if (cs != null && cs.length() > 0) {
            g.setColor(new Color(sw.getCurrentTextColor(), true));
            g.setFont(sw.createAwtFont());
            FontMetrics fm = g.getFontMetrics();
            int availW = Math.max(40, tx - (l + sw.getPaddingLeft()) - 12);
            java.util.List<String> lines = TextView.wrapLines(cs.toString(), fm, availW);
            int sy = t + Math.max(0, (h - lines.size() * fm.getHeight()) / 2) + fm.getAscent();
            for (String line : lines) {
                g.drawString(line, l + sw.getPaddingLeft(), sy);
                sy += fm.getHeight();
            }
        }

        boolean on = sw.isChecked();
        g.setColor(on ? new Color(16, 185, 129) : new Color(71, 85, 105));
        g.fillRoundRect(tx, ty, trackW, trackH, trackH, trackH);
        int knobD = 46;
        int kx = on ? (tx + trackW - knobD - 7) : (tx + 7);
        int ky = ty + (trackH - knobD) / 2;
        g.setColor(Color.WHITE);
        g.fillOval(kx, ky, knobD, knobD);
    }

    private void drawCheckBox(Graphics2D g, CheckBox cb, int l, int t, int w, int h) {
        int boxD = 42;
        int bx = l + cb.getPaddingLeft();
        int by = t + Math.max(0, (h - boxD) / 2);
        if (cb.isChecked()) {
            g.setColor(new Color(59, 130, 246));
            g.fillRoundRect(bx, by, boxD, boxD, 10, 10);
            g.setColor(Color.WHITE);
            g.setStroke(new BasicStroke(4f));
            g.drawLine(bx + 10, by + 22, bx + 18, by + 30);
            g.drawLine(bx + 18, by + 30, bx + 32, by + 12);
        } else {
            g.setColor(new Color(148, 163, 184));
            g.setStroke(new BasicStroke(3f));
            g.drawRoundRect(bx, by, boxD, boxD, 10, 10);
        }
        CharSequence cs = cb.getText();
        if (cs != null && cs.length() > 0) {
            g.setColor(new Color(cb.getCurrentTextColor(), true));
            g.setFont(cb.createAwtFont());
            FontMetrics fm = g.getFontMetrics();
            g.drawString(cs.toString(), bx + boxD + 16, t + (h - fm.getHeight()) / 2 + fm.getAscent());
        }
    }

    private void drawRadioButton(Graphics2D g, RadioButton rb, int l, int t, int w, int h) {
        int rD = 42;
        int bx = l + rb.getPaddingLeft();
        int by = t + Math.max(0, (h - rD) / 2);
        g.setColor(rb.isChecked() ? new Color(59, 130, 246) : new Color(148, 163, 184));
        g.setStroke(new BasicStroke(3f));
        g.drawOval(bx, by, rD, rD);
        if (rb.isChecked()) {
            g.fillOval(bx + 10, by + 10, rD - 20, rD - 20);
        }
        CharSequence cs = rb.getText();
        if (cs != null && cs.length() > 0) {
            g.setColor(new Color(rb.getCurrentTextColor(), true));
            g.setFont(rb.createAwtFont());
            FontMetrics fm = g.getFontMetrics();
            g.drawString(cs.toString(), bx + rD + 16, t + (h - fm.getHeight()) / 2 + fm.getAscent());
        }
    }

    private void drawSeekBar(Graphics2D g, ProgressBar pb, int l, int t, int w, int h, boolean withThumb) {
        int barL = l + pb.getPaddingLeft() + 12;
        int barW = Math.max(20, w - pb.getPaddingLeft() - pb.getPaddingRight() - 24);
        int barH = withThumb ? 10 : 12;
        int barY = t + (h - barH) / 2;
        g.setColor(new Color(51, 65, 85));
        g.fillRoundRect(barL, barY, barW, barH, barH, barH);
        float frac = pb.getMax() > 0 ? Math.max(0f, Math.min(1f, (float) pb.getProgress() / pb.getMax())) : 0f;
        int fillW = Math.round(barW * frac);
        g.setColor(new Color(59, 130, 246));
        g.fillRoundRect(barL, barY, Math.max(4, fillW), barH, barH, barH);
        if (withThumb) {
            int thumbD = 32;
            g.setColor(Color.WHITE);
            g.fillOval(barL + fillW - thumbD / 2, t + (h - thumbD) / 2, thumbD, thumbD);
        }
    }

    private void drawEditText(Graphics2D g, EditText et, int l, int t, int w, int h) {
        if (et.getBackground() == null) {
            g.setColor(new Color(15, 23, 42, 30));
            g.fillRoundRect(l + 2, t + 2, w - 4, h - 4, 12, 12);
            g.setColor(new Color(100, 116, 139));
            g.setStroke(new BasicStroke(2f));
            g.drawRoundRect(l + 2, t + 2, w - 4, h - 4, 12, 12);
        }
        String val = et.getText() != null ? et.getText().toString() : "";
        boolean isHint = val.isEmpty() && et.getHint() != null && et.getHint().length() > 0;
        String shown = isHint ? et.getHint().toString() : val;
        if (!shown.isEmpty()) {
            g.setColor(isHint ? new Color(148, 163, 184) : new Color(et.getCurrentTextColor(), true));
            g.setFont(et.createAwtFont());
            FontMetrics fm = g.getFontMetrics();
            g.drawString(shown, l + et.getPaddingLeft() + 12, t + (h - fm.getHeight()) / 2 + fm.getAscent());
        }
    }

    private static Color contrastText(int bgArgb) {
        int r = (bgArgb >> 16) & 0xFF, gg = (bgArgb >> 8) & 0xFF, b = bgArgb & 0xFF;
        double lum = 0.299 * r + 0.587 * gg + 0.114 * b;
        return lum > 140 ? Color.BLACK : Color.WHITE;
    }

    private NodeInfo buildAst(View v, Set<View> visited, int depth, int scrollOffX, int scrollOffY) {
        if (v == null || v.getVisibility() != View.VISIBLE || depth > 40 || !visited.add(v)) {
            return null;
        }

        nodeCounter++;
        String nodeId = "node_" + nodeCounter;
        nodeMap.put(nodeId, v);

        NodeInfo n = new NodeInfo();
        n.id = nodeId;
        n.resName = v.getResourceIdName();
        n.clazz = v.getClass().getSimpleName();
        n.clickable = v.isClickable() || v instanceof Button;
        n.enabled = v.isEnabled();
        n.checked = (v instanceof CompoundButton) && ((CompoundButton) v).isChecked();
        n.visibility = "VISIBLE";
        n.contentDescription = v.getContentDescription() != null ? v.getContentDescription().toString() : "";
        n.bounds = new int[] {
                v.getLeft() - scrollOffX,
                v.getTop() - scrollOffY,
                v.getRight() - scrollOffX,
                v.getBottom() - scrollOffY
        };

        if (v instanceof TextView) {
            CharSequence cs = ((TextView) v).getText();
            n.text = cs != null ? cs.toString() : "";
            if (n.text.isEmpty() && v instanceof EditText && ((EditText) v).getHint() != null) {
                n.text = ((EditText) v).getHint().toString();
            }
        } else if (v.getText() != null && v.getText().length() > 0) {
            n.text = v.getText().toString();
        } else if (v.getContentDescription() != null && v.getContentDescription().length() > 0) {
            n.text = v.getContentDescription().toString();
        } else {
            n.text = "";
        }

        if (v instanceof ViewGroup) {
            ViewGroup vg = (ViewGroup) v;
            int sx = scrollOffX, sy = scrollOffY;
            if (v instanceof ScrollView) {
                sx += ((ScrollView) v).getScrollX();
                sy += ((ScrollView) v).getScrollY();
            } else if (v instanceof HorizontalScrollView) {
                sx += ((HorizontalScrollView) v).getScrollX();
                sy += ((HorizontalScrollView) v).getScrollY();
            }
            for (int i = 0; i < vg.getChildCount(); i++) {
                NodeInfo child = buildAst(vg.getChildAt(i), visited, depth + 1, sx, sy);
                if (child != null) {
                    n.children.add(child);
                }
            }
        }
        return n;
    }

    private void serializeNode(NodeInfo n, StringBuilder sb) {
        if (n == null) {
            sb.append("null");
            return;
        }
        sb.append("{");
        sb.append("\"id\":\"").append(n.id).append("\",");
        if (n.resName != null && !n.resName.isEmpty()) {
            sb.append("\"resource_id\":\"").append(escapeJson(n.resName)).append("\",");
        }
        sb.append("\"class\":\"").append(n.clazz).append("\",");
        if (!n.text.isEmpty()) {
            sb.append("\"text\":\"").append(escapeJson(n.text)).append("\",");
        }
        if (n.contentDescription != null && !n.contentDescription.isEmpty()) {
            sb.append("\"content_description\":\"").append(escapeJson(n.contentDescription)).append("\",");
        }
        sb.append("\"clickable\":").append(n.clickable).append(",");
        if (n.checked) {
            sb.append("\"checked\":true,");
        }
        sb.append("\"bounds\":[").append(n.bounds[0]).append(",").append(n.bounds[1]).append(",")
                .append(n.bounds[2]).append(",").append(n.bounds[3]).append("]");

        if (!n.children.isEmpty()) {
            sb.append(",\"children\":[");
            for (int i = 0; i < n.children.size(); i++) {
                if (i > 0)
                    sb.append(",");
                serializeNode(n.children.get(i), sb);
            }
            sb.append("]");
        }
        sb.append("}");
    }

    public synchronized String dispatchAction(String actionType, String target, String extra) {
        if (activeActivity == null) {
            return "{\"status\":\"error\",\"error\":\"No active activity\"}";
        }
        ForensicTimeline.record("ACTION_DISPATCHED", actionType,
                target + (extra != null && !extra.isEmpty() ? ":" + extra : ""));
        View root = resolveMeasuredRoot();
        View dlgRoot = resolveDialogRoot();
        View activeTree = dlgRoot != null ? dlgRoot : root;
        nodeMap.clear();
        nodeCounter = 0;
        Set<View> buildVisited = Collections.newSetFromMap(new IdentityHashMap<>());
        if (root != null) {
            buildAst(root, buildVisited, 0, 0, 0);
        }
        if (dlgRoot != null) {
            buildAst(dlgRoot, buildVisited, 1, 0, 0);
        }

        if ("back".equalsIgnoreCase(actionType)) {
            Dialog topDlg = Dialog.getTopActiveDialog();
            if (topDlg != null) {
                if (topDlg.isCancelable())
                    topDlg.cancel();
                flushQueue(50);
                return "{\"status\":\"ok\",\"action\":\"back\",\"message\":\"Dismissed dialog\",\"mutated\":true}";
            }
            int stackBefore = activityStack.size();
            activeActivity.onBackPressed();
            if (activityStack.size() == stackBefore && stackBefore > 1 && !activeActivity.isFinishing()) {
                activeActivity.finish();
            }
            flushQueue(50);
            return "{\"status\":\"ok\",\"action\":\"back\",\"mutated\":true}";
        }

        if ("keyevent".equalsIgnoreCase(actionType)) {
            String k = (target + " " + extra).toUpperCase();
            if (k.contains("BACK") || k.contains("4")) {
                return dispatchAction("back", "", "");
            }
            if (k.contains("ENTER") || k.contains("66")) {
                View focus = nodeMap.get(lastFocusedNodeId);
                if (focus instanceof TextView) {
                    boolean handled = ((TextView) focus).triggerEditorAction(6);
                    flushQueue(50);
                    return String.format(
                            "{\"status\":\"ok\",\"action\":\"keyevent\",\"target\":\"ENTER\",\"mutated\":%b}", handled);
                }
            }
            return "{\"status\":\"ok\",\"action\":\"keyevent\",\"mutated\":false}";
        }

        if ("scroll".equalsIgnoreCase(actionType) || "swipe".equalsIgnoreCase(actionType)) {
            return handleSwipeOrScroll(activeTree, target, extra);
        }

        View targetView = null;
        int tapX = -1, tapY = -1;
        if (target != null && nodeMap.containsKey(target)) {
            targetView = nodeMap.get(target);
        } else if (target != null && !target.isEmpty()) {
            if (target.contains(",") && !target.contains("->")) {
                try {
                    String[] coords = target.split(",");
                    tapX = Integer.parseInt(coords[0].trim());
                    tapY = Integer.parseInt(coords[1].trim());
                    targetView = hitTestView(activeTree, tapX, tapY,
                            Collections.newSetFromMap(new IdentityHashMap<>()));
                } catch (Exception ignored) {
                }
            }
            if (targetView == null) {
                String lower = target.toLowerCase();
                // Prefer matching visible dialog items first, then reverse-iterate for topmost
                // view
                List<View> candidates = new ArrayList<>(nodeMap.values());
                Collections.reverse(candidates);
                for (View v : candidates) {
                    if (v.getResourceIdName().equalsIgnoreCase(target)) {
                        targetView = v;
                        break;
                    }
                    CharSequence cs = (v instanceof TextView) ? ((TextView) v).getText() : v.getText();
                    String txt = cs != null ? cs.toString() : null;
                    if (txt != null && (txt.equalsIgnoreCase(target) || txt.toLowerCase().contains(lower))) {
                        targetView = v;
                        break;
                    }
                }
            }
        }

        if (targetView == null) {
            return String.format("{\"status\":\"error\",\"error\":\"Target view '%s' not found\"}", escapeJson(target));
        }

        CharSequence cs = (targetView instanceof TextView) ? ((TextView) targetView).getText() : targetView.getText();
        String viewText = cs != null ? cs.toString() : "";

        if ("click".equalsIgnoreCase(actionType) || "tap".equalsIgnoreCase(actionType)) {
            if (targetView instanceof EditText) {
                for (Map.Entry<String, View> me : nodeMap.entrySet()) {
                    if (me.getValue() == targetView) {
                        lastFocusedNodeId = me.getKey();
                        break;
                    }
                }
            }
            if (targetView instanceof SeekBar && tapX >= 0) {
                SeekBar sb = (SeekBar) targetView;
                int bw = Math.max(1, sb.getWidth() - sb.getPaddingLeft() - sb.getPaddingRight());
                float frac = Math.max(0f, Math.min(1f, (float) (tapX - sb.getLeft() - sb.getPaddingLeft()) / bw));
                sb.setProgressFromUser(Math.round(frac * Math.max(1, sb.getMax())));
                flushQueue(50);
                return String.format(
                        "{\"status\":\"ok\",\"action\":\"click\",\"target\":\"%s\",\"resolved_text\":\"%s\",\"mutated\":true}",
                        escapeJson(target), escapeJson("SeekBar:" + sb.getProgress()));
            }
            try {
                boolean success = targetView.performClick();
                View cur = targetView;
                while (!success && cur.getParent() instanceof View) {
                    cur = (View) cur.getParent();
                    success = cur.performClick();
                }
                if (!success) {
                    long now = SystemClock.uptimeMillis();
                    float mx = tapX >= 0 ? tapX : (targetView.getLeft() + targetView.getWidth() / 2f);
                    float my = tapY >= 0 ? tapY : (targetView.getTop() + targetView.getHeight() / 2f);
                    success = dispatchMotionSequence(targetView, mx, my, mx, my, now);
                }
                flushQueue(50);
                return String.format(
                        "{\"status\":\"ok\",\"action\":\"click\",\"target\":\"%s\",\"resolved_text\":\"%s\",\"mutated\":%b}",
                        escapeJson(target), escapeJson(viewText), success);
            } catch (Throwable clickErr) {
                Throwable rc = unwrapThrowable(clickErr);
                runtimeWarnings.add("Click error on '" + viewText + "': " + rc);
                return String.format("{\"status\":\"error\",\"action\":\"click\",\"target\":\"%s\",\"error\":\"%s\"}",
                        escapeJson(target), escapeJson(rc.getClass().getSimpleName() + ": " + rc.getMessage()));
            }
        } else if ("long_click".equalsIgnoreCase(actionType) || "long_press".equalsIgnoreCase(actionType)) {
            boolean handled = targetView.performLongClick();
            View cur = targetView;
            while (!handled && cur.getParent() instanceof View) {
                cur = (View) cur.getParent();
                handled = cur.performLongClick();
            }
            flushQueue(50);
            return String.format(
                    "{\"status\":\"ok\",\"action\":\"long_click\",\"target\":\"%s\",\"resolved_text\":\"%s\",\"mutated\":%b}",
                    escapeJson(target), escapeJson(viewText), handled);
        } else if ("type".equalsIgnoreCase(actionType)) {
            for (Map.Entry<String, View> me : nodeMap.entrySet()) {
                if (me.getValue() == targetView) {
                    lastFocusedNodeId = me.getKey();
                    break;
                }
            }
            String textVal = extra != null ? extra : "";
            boolean submit = textVal.endsWith("\\n") || textVal.endsWith("\n");
            if (submit)
                textVal = textVal.replaceAll("(\\\\n|\\n)+$", "");
            if (targetView instanceof TextView) {
                ((TextView) targetView).setText(textVal);
                if (submit)
                    ((TextView) targetView).triggerEditorAction(6);
            } else {
                targetView.setText(textVal);
            }
            flushQueue(50);
            return String.format(
                    "{\"status\":\"ok\",\"action\":\"type\",\"target\":\"%s\",\"resolved_text\":\"%s\",\"mutated\":true}",
                    escapeJson(target), escapeJson(textVal));
        }

        return String.format("{\"status\":\"error\",\"error\":\"Unsupported action '%s'\"}", escapeJson(actionType));
    }

    private String handleSwipeOrScroll(View root, String target, String extra) {
        String combined = ((target != null ? target : "") + " " + (extra != null ? extra : "")).trim();
        int sx = 540, sy = 1500, ex = 540, ey = 700;
        boolean hasCoords = false;
        if (combined.contains("->")) {
            try {
                String coordPart = combined.split("\\s+")[0];
                String[] pts = coordPart.split("->");
                String[] p1 = pts[0].split(",");
                String[] p2 = pts[1].split(",");
                sx = Integer.parseInt(p1[0].trim());
                sy = Integer.parseInt(p1[1].trim());
                ex = Integer.parseInt(p2[0].trim());
                ey = Integer.parseInt(p2[1].trim());
                hasCoords = true;
            } catch (Exception ignored) {
            }
        }

        String up = combined.toUpperCase();
        boolean horiz = hasCoords ? (Math.abs(ex - sx) > Math.abs(ey - sy))
                : (up.contains("LEFT") || up.contains("RIGHT"));

        if (hasCoords && root != null) {
            View hit = hitTestView(root, sx, sy, Collections.newSetFromMap(new IdentityHashMap<>()));
            if (hit instanceof SeekBar) {
                SeekBar sb = (SeekBar) hit;
                int bw = Math.max(1, sb.getWidth() - sb.getPaddingLeft() - sb.getPaddingRight());
                float frac = Math.max(0f, Math.min(1f, (float) (ex - sb.getLeft() - sb.getPaddingLeft()) / bw));
                sb.setProgressFromUser(Math.round(frac * Math.max(1, sb.getMax())));
                flushQueue(50);
                return String.format(
                        "{\"status\":\"ok\",\"action\":\"swipe\",\"target\":\"%s\",\"resolved_text\":\"SeekBar:%d\",\"mutated\":true}",
                        escapeJson(combined), sb.getProgress());
            }
            if (hit != null && dispatchMotionSequence(hit, sx, sy, ex, ey, SystemClock.uptimeMillis())) {
                flushQueue(50);
                return String.format("{\"status\":\"ok\",\"action\":\"swipe\",\"target\":\"%s\",\"mutated\":true}",
                        escapeJson(combined));
            }
        }

        boolean scrolled;
        if (horiz) {
            int dx = hasCoords ? (sx - ex) : (up.contains("RIGHT") ? -420 : 420);
            scrolled = scrollVisibleHorizontalScrollView(root, dx);
        } else {
            int dy = hasCoords ? (sy - ey) : (up.contains("UP") || up.contains("-") ? -450 : 450);
            scrolled = scrollVisibleScrollView(root, dy);
        }
        flushQueue(50);
        return String.format("{\"status\":\"ok\",\"action\":\"scroll\",\"target\":\"%s\",\"mutated\":%b}",
                escapeJson(combined), scrolled);
    }

    private boolean dispatchMotionSequence(View startView, float sx, float sy, float ex, float ey, long t0) {
        View cur = startView;
        while (cur != null) {
            MotionEvent down = MotionEvent.obtain(t0, t0, MotionEvent.ACTION_DOWN, sx, sy, 0);
            if (cur.dispatchTouchEvent(down)) {
                for (int step = 1; step <= 4; step++) {
                    float f = step / 4.0f;
                    float mx = sx + (ex - sx) * f;
                    float my = sy + (ey - sy) * f;
                    MotionEvent mv = MotionEvent.obtain(t0, t0 + step * 35L, MotionEvent.ACTION_MOVE, mx, my, 0);
                    cur.dispatchTouchEvent(mv);
                }
                MotionEvent up = MotionEvent.obtain(t0, t0 + 160L, MotionEvent.ACTION_UP, ex, ey, 0);
                cur.dispatchTouchEvent(up);
                return true;
            }
            if (cur.getParent() instanceof View)
                cur = (View) cur.getParent();
            else
                cur = null;
        }
        return false;
    }

    private boolean scrollVisibleHorizontalScrollView(View v, int dx) {
        if (v == null || v.getVisibility() != View.VISIBLE)
            return false;
        if (v instanceof ViewGroup) {
            ViewGroup vg = (ViewGroup) v;
            for (int i = vg.getChildCount() - 1; i >= 0; i--) {
                if (scrollVisibleHorizontalScrollView(vg.getChildAt(i), dx))
                    return true;
            }
        }
        if (v instanceof HorizontalScrollView) {
            HorizontalScrollView hsv = (HorizontalScrollView) v;
            int before = hsv.getScrollX();
            hsv.scrollBy(dx, 0);
            return hsv.getScrollX() != before;
        }
        return false;
    }

    private boolean scrollVisibleScrollView(View v, int dy) {
        if (v == null || v.getVisibility() != View.VISIBLE)
            return false;
        if (v instanceof ViewGroup) {
            ViewGroup vg = (ViewGroup) v;
            for (int i = vg.getChildCount() - 1; i >= 0; i--) {
                if (scrollVisibleScrollView(vg.getChildAt(i), dy))
                    return true;
            }
        }
        if (v instanceof ScrollView) {
            ScrollView sv = (ScrollView) v;
            int before = sv.getScrollY();
            sv.scrollBy(0, dy);
            return sv.getScrollY() != before;
        }
        return false;
    }

    private View hitTestView(View v, int x, int y, Set<View> visited) {
        if (v == null || v.getVisibility() != View.VISIBLE || !visited.add(v))
            return null;
        if (x < v.getLeft() || x > v.getRight() || y < v.getTop() || y > v.getBottom()) {
            return null;
        }
        if (v instanceof ViewGroup) {
            ViewGroup vg = (ViewGroup) v;
            int cx = x + (v instanceof ScrollView ? ((ScrollView) v).getScrollX()
                    : (v instanceof HorizontalScrollView ? ((HorizontalScrollView) v).getScrollX() : 0));
            int cy = y + (v instanceof ScrollView ? ((ScrollView) v).getScrollY()
                    : (v instanceof HorizontalScrollView ? ((HorizontalScrollView) v).getScrollY() : 0));
            for (int i = vg.getChildCount() - 1; i >= 0; i--) {
                View hit = hitTestView(vg.getChildAt(i), cx, cy, visited);
                if (hit != null) {
                    if (hit.isClickable() || hit instanceof Button || hit instanceof EditText
                            || !(hit instanceof ViewGroup)) {
                        return hit;
                    }
                    return hit;
                }
            }
        }
        return v;
    }

    public synchronized String stepTime(long deltaMs) {
        try {
            int processed = flushQueue(deltaMs);
            if (processed > 0) {
                ForensicTimeline.record("CALLBACK_EXECUTED", "Looper.main",
                        "processed_tasks=" + processed + ",delta_ms=" + deltaMs);
            }
            return String.format("{\"status\":\"ok\",\"delta_ms\":%d,\"virtual_uptime_ms\":%d,\"processed_tasks\":%d}",
                    deltaMs, SystemClock.uptimeMillis(), processed);
        } catch (IllegalStateException watchdogErr) {
            ForensicTimeline.record("EXCEPTION_RAISED", "Looper.main", watchdogErr.getMessage());
            runtimeWarnings.add(watchdogErr.getMessage());
            return String.format("{\"status\":\"error\",\"error\":\"%s\",\"virtual_uptime_ms\":%d}",
                    escapeJson(watchdogErr.getMessage()), SystemClock.uptimeMillis());
        }
    }

    public synchronized String inspectRuntime() {
        Runtime rt = Runtime.getRuntime();
        double totalMB = rt.totalMemory() / (1024.0 * 1024.0);
        double freeMB = rt.freeMemory() / (1024.0 * 1024.0);
        double usedMB = totalMB - freeMB;
        double maxMB = rt.maxMemory() / (1024.0 * 1024.0);

        long gcCount = 0;
        for (GarbageCollectorMXBean gc : ManagementFactory.getGarbageCollectorMXBeans()) {
            long c = gc.getCollectionCount();
            if (c > 0)
                gcCount += c;
        }
        ClassLoadingMXBean clBean = ManagementFactory.getClassLoadingMXBean();
        int loadedClasses = clBean.getLoadedClassCount();
        int totalViews = nodeMap.size();

        java.lang.management.ThreadMXBean tm = ManagementFactory.getThreadMXBean();
        int threadCount = tm.getThreadCount();
        long[] deadlocked = tm.findDeadlockedThreads();
        int deadlockedCount = deadlocked != null ? deadlocked.length : 0;
        int queueDepth = Looper.getMainLooper().getQueue().size();

        return String.format(Locale.US,
                "{\"status\":\"ok\",\"package\":\"%s\",\"activity\":\"%s\",\"used_mb\":%.2f,\"free_mb\":%.2f,\"max_mb\":%.2f,\"gc_cycles\":%d,\"loaded_classes\":%d,\"total_views\":%d,\"virtual_uptime_ms\":%d,\"thread_count\":%d,\"deadlocked_threads\":%d,\"looper_queue_depth\":%d,\"timeline_events\":%d,\"transformation_count\":%d,\"degraded_heuristic\":%b,\"compat_fault_count\":%d,\"result_status\":\"%s\"}",
                escapeJson(activeAppPackage), escapeJson(activeActivityName),
                usedMB, freeMB, maxMB, gcCount, loadedClasses, totalViews, SystemClock.uptimeMillis(),
                threadCount, deadlockedCount, queueDepth, ForensicTimeline.size(),
                TransformationAuditLog.size(), TransformationAuditLog.hasDegradedHeuristic(),
                nanoandroid.compat.ApiCompatibilityRegistry.getSessionFaults().size(),
                resolveCanonicalResultStatus(null));
    }

    public synchronized String inspectTimeline() {
        return ForensicTimeline.toJson();
    }

    public synchronized String inspectThreads() {
        java.lang.management.ThreadMXBean tm = ManagementFactory.getThreadMXBean();
        long[] deadlockedIds = tm.findDeadlockedThreads();
        Set<Long> deadSet = new HashSet<>();
        if (deadlockedIds != null) {
            for (long id : deadlockedIds)
                deadSet.add(id);
        }
        java.lang.management.ThreadInfo[] infos = tm.dumpAllThreads(false, false);
        StringBuilder sb = new StringBuilder();
        sb.append("{\"status\":\"ok\",\"thread_count\":").append(tm.getThreadCount())
                .append(",\"deadlocked_count\":").append(deadSet.size())
                .append(",\"looper_queue_depth\":").append(Looper.getMainLooper().getQueue().size())
                .append(",\"threads\":[");
        boolean first = true;
        for (java.lang.management.ThreadInfo ti : infos) {
            if (ti == null)
                continue;
            if (!first)
                sb.append(",");
            first = false;
            sb.append("{\"id\":").append(ti.getThreadId())
                    .append(",\"name\":\"").append(escapeJson(ti.getThreadName())).append("\"")
                    .append(",\"state\":\"").append(escapeJson(ti.getThreadState().name())).append("\"")
                    .append(",\"deadlocked\":").append(deadSet.contains(ti.getThreadId()))
                    .append(",\"blocked_count\":").append(ti.getBlockedCount())
                    .append(",\"waited_count\":").append(ti.getWaitedCount()).append("}");
        }
        sb.append("]}");
        return sb.toString();
    }

    public synchronized String inspectStorage() {
        Map<String, String> sqlTables = android.database.sqlite.SQLiteDatabase.snapshotActiveTables();
        Map<String, String> prefs = Context.snapshotSharedPreferences(activeAppPackage);
        StringBuilder sb = new StringBuilder();
        sb.append("{\"status\":\"ok\",\"sqlite_tables\":{");
        boolean first = true;
        for (Map.Entry<String, String> e : sqlTables.entrySet()) {
            if (!first)
                sb.append(",");
            first = false;
            sb.append("\"").append(escapeJson(e.getKey())).append("\":\"").append(escapeJson(e.getValue()))
                    .append("\"");
        }
        sb.append("},\"preferences\":{");
        first = true;
        for (Map.Entry<String, String> e : prefs.entrySet()) {
            if (!first)
                sb.append(",");
            first = false;
            sb.append("\"").append(escapeJson(e.getKey())).append("\":\"").append(escapeJson(e.getValue()))
                    .append("\"");
        }
        sb.append("}}");
        return sb.toString();
    }

    public synchronized String listLayouts() {
        if (activeApkTable == null) {
            return "{\"status\":\"ok\",\"layouts\":[]}";
        }
        List<Map.Entry<Integer, String>> entries = activeApkTable.getEntriesByType("layout");
        StringBuilder sb = new StringBuilder();
        sb.append("{\"status\":\"ok\",\"layouts\":[");
        boolean first = true;
        for (Map.Entry<Integer, String> e : entries) {
            String path = activeApkTable.getFilePath(e.getKey());
            if (path == null)
                continue;
            if (!first)
                sb.append(",");
            first = false;
            sb.append("{\"id\":").append(e.getKey())
                    .append(",\"name\":\"").append(escapeJson(e.getValue())).append("\"")
                    .append(",\"path\":\"").append(escapeJson(path)).append("\"}");
        }
        sb.append("]}");
        return sb.toString();
    }

    public synchronized String previewLayout(String nameOrId) {
        if (activeApkTable == null) {
            return "{\"status\":\"error\",\"error\":\"No APK resource table loaded\"}";
        }
        int resId = 0;
        try {
            if (nameOrId.startsWith("0x"))
                resId = Integer.parseInt(nameOrId.substring(2), 16);
            else
                resId = Integer.parseInt(nameOrId);
        } catch (NumberFormatException ignored) {
            resId = activeApkTable.getIdentifier(nameOrId, "layout", activeApkTable.packageName);
        }
        if (resId == 0) {
            return "{\"status\":\"error\",\"error\":\"Layout not found: " + escapeJson(nameOrId) + "\"}";
        }
        Context ctx = new Context(activeAppPackage);
        View inflated = BinaryXmlInflater.inflateLayout(resId, ctx, null, false, activeApkTable);
        if (inflated == null) {
            return "{\"status\":\"error\",\"error\":\"Failed to inflate layout XML for " + escapeJson(nameOrId) + "\"}";
        }
        inspectorMode = true;
        Activity previewAct = new Activity();
        previewAct.attach(ctx, new Intent(Intent.ACTION_MAIN));
        activeActivity = previewAct;
        LinearLayout wrapper = new LinearLayout(ctx);
        wrapper.setOrientation(LinearLayout.VERTICAL);
        TextView bar = new TextView(ctx);
        bar.setBackgroundColor(0xFF1E293B);
        bar.setTextColor(0xFF38BDF8);
        bar.setTextSize(11f);
        bar.setPadding(24, 14, 24, 14);
        String name = activeApkTable.getEntryName(resId);
        bar.setText("AXML Layout Inspector: " + (name != null ? name : String.valueOf(resId)));
        wrapper.addView(bar, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT));
        wrapper.addView(inflated, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.MATCH_PARENT, 1f));
        activeActivity.setContentView(wrapper);
        return "{\"status\":\"ok\",\"layout\":\"" + escapeJson(name != null ? name : nameOrId) + "\",\"res_id\":"
                + resId + "}";
    }

    private int flushQueue(long maxDelta) {
        if (maxDelta > 0) {
            SystemClock.advanceUptimeMillis(maxDelta);
        }
        MessageQueue q = Looper.getMainLooper().getQueue();
        int count = 0;
        int limit = DeterministicRuntime.getMaxLooperTasksPerStep();
        long target = SystemClock.uptimeMillis();
        while (true) {
            Message msg = q.pollNextDueMessage(target);
            if (msg == null)
                break;
            if (++count > limit) {
                throw new IllegalStateException(
                        "Deterministic Looper watchdog triggered: exceeded " + limit + " tasks in step");
            }
            try {
                if (msg.target != null)
                    msg.target.dispatchMessage(msg);
                else if (msg.callback != null)
                    msg.callback.run();
            } catch (Throwable err) {
                runtimeWarnings.add("Looper task error: " + unwrapThrowable(err));
            }
        }
        return count;
    }

    private static Throwable unwrapThrowable(Throwable t) {
        Throwable cur = t;
        while (cur.getCause() != null && (cur instanceof java.lang.reflect.InvocationTargetException
                || cur instanceof ExceptionInInitializerError)) {
            cur = cur.getCause();
        }
        return cur;
    }

    private static String discoverActivityInJar(File jarFile, NanoClassLoader loader) {
        String firstAct = null;
        try (java.util.jar.JarFile jf = new java.util.jar.JarFile(jarFile)) {
            java.util.Enumeration<java.util.jar.JarEntry> en = jf.entries();
            while (en.hasMoreElements()) {
                String name = en.nextElement().getName();
                if (!name.endsWith("Activity.class") || name.contains("$"))
                    continue;
                String clsName = name.substring(0, name.length() - 6).replace('/', '.');
                try {
                    Class<?> cls = loader.loadClass(clsName);
                    if (Activity.class.isAssignableFrom(cls)) {
                        if (clsName.endsWith(".MainActivity"))
                            return clsName;
                        if (firstAct == null)
                            firstAct = clsName;
                    }
                } catch (Throwable ignored) {
                }
            }
        } catch (Throwable ignored) {
        }
        return firstAct != null ? firstAct : "MainActivity";
    }

    private static Method findMethod(Class<?> clazz, String name, Class<?>... paramTypes) {
        Class<?> cur = clazz;
        while (cur != null && cur != Object.class) {
            try {
                return cur.getDeclaredMethod(name, paramTypes);
            } catch (NoSuchMethodException e) {
                cur = cur.getSuperclass();
            }
        }
        return null;
    }

    private static String escapeJson(String s) {
        if (s == null)
            return "";
        return s.replace("\\", "\\\\").replace("\"", "\\\"").replace("\n", "\\n").replace("\r", "\\r").replace("\t",
                "\\t");
    }

    public static void main(String[] args) throws IOException {
        System.setOut(new PrintStream(System.out, true, java.nio.charset.StandardCharsets.UTF_8));
        HeadlessAppHost host = new HeadlessAppHost();
        BufferedReader reader = new BufferedReader(new InputStreamReader(System.in, java.nio.charset.StandardCharsets.UTF_8));

        if (args.length >= 1) {
            String p1 = args[0];
            String p2 = args.length >= 2 ? args[1] : "";
            String p3 = args.length >= 3 ? args[2] : "";
            System.out.println(host.launch(p1, p2, p3));
            System.out.flush();
        }

        String line;
        while ((line = reader.readLine()) != null) {
            line = line.trim();
            if (line.isEmpty())
                continue;

            try {
                String[] parts = line.split("\\s+", 4);
                String cmd = parts[0].toUpperCase();

                switch (cmd) {
                    case "LAUNCH":
                        if (parts.length >= 2) {
                            String p1 = parts[1];
                            String p2 = parts.length >= 3 ? parts[2] : "";
                            String p3 = parts.length >= 4 ? parts[3] : "";
                            System.out.println(host.launch(p1, p2, p3));
                        } else {
                            System.out.println(
                                    "{\"status\":\"error\",\"error\":\"Usage: LAUNCH <jarOrApkPath> [activity] [apkPath]\"}");
                        }
                        break;
                    case "OBSERVE":
                        System.out.println(host.observe());
                        break;
                    case "RENDER":
                        String outPath = (parts.length >= 2) ? parts[1] : "live_render.png";
                        System.out.println(host.renderScreen(outPath));
                        break;
                    case "INSPECT":
                        System.out.println(host.inspectRuntime());
                        break;
                    case "TIMELINE":
                        System.out.println(host.inspectTimeline());
                        break;
                    case "THREADS":
                        System.out.println(host.inspectThreads());
                        break;
                    case "INSPECT_STORAGE":
                        System.out.println(host.inspectStorage());
                        break;
                    case "LIST_LAYOUTS":
                        System.out.println(host.listLayouts());
                        break;
                    case "PREVIEW_LAYOUT":
                        if (parts.length >= 2) {
                            System.out.println(host.previewLayout(parts[1]));
                        } else {
                            System.out.println("{\"status\":\"error\",\"error\":\"Usage: PREVIEW_LAYOUT <nameOrId>\"}");
                        }
                        break;
                    case "ACTION":
                        if (parts.length >= 2) {
                            String actType = parts[1];
                            String target = (parts.length >= 3) ? parts[2] : "";
                            String extra = (parts.length >= 4) ? parts[3] : "";
                            System.out.println(host.dispatchAction(actType, target, extra));
                        } else {
                            System.out.println(
                                    "{\"status\":\"error\",\"error\":\"Usage: ACTION <type> [target] [extra]\"}");
                        }
                        break;
                    case "STEP":
                        long delta = (parts.length >= 2) ? Long.parseLong(parts[1]) : 300;
                        System.out.println(host.stepTime(delta));
                        break;
                    case "SET_PROFILE":
                        if (parts.length >= 3) {
                            int wp = Integer.parseInt(parts[1]);
                            int hp = Integer.parseInt(parts[2]);
                            System.out.println(host.setDeviceProfile(wp, hp));
                        } else {
                            System.out.println(
                                    "{\"status\":\"error\",\"error\":\"Usage: SET_PROFILE <widthPx> <heightPx>\"}");
                        }
                        break;
                    case "RECREATE":
                        System.out.println(host.recreateActiveActivity());
                        break;
                    case "SIMULATE_PROCESS_DEATH":
                        System.out.println(host.simulateProcessDeathAndRestore());
                        break;
                    case "STAGE_FILE":
                        if (parts.length >= 2) {
                            String rest = line.substring(parts[0].length()).trim();
                            System.out.println(host.stageFile(rest));
                        } else {
                            System.out.println("{\"status\":\"error\",\"error\":\"Usage: STAGE_FILE <path>\"}");
                        }
                        break;
                    case "LIST_STAGED":
                        System.out.println(host.listStagedFiles());
                        break;
                    case "EXIT":
                        System.out.println("{\"status\":\"ok\",\"state\":\"EXITED\"}");
                        System.out.flush();
                        System.exit(0);
                        return;
                    default:
                        System.out.println(
                                "{\"status\":\"error\",\"error\":\"Unknown command: " + escapeJson(cmd) + "\"}");
                }
            } catch (Throwable cmdErr) {
                Throwable rc = unwrapThrowable(cmdErr);
                System.out.println("{\"status\":\"error\",\"error\":\""
                        + escapeJson(rc.getClass().getSimpleName() + ": " + rc.getMessage()) + "\"}");
            }
            System.out.flush();
        }
        System.exit(0);
    }
}
