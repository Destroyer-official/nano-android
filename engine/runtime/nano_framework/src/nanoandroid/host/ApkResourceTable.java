package nanoandroid.host;

import java.awt.image.BufferedImage;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.nio.charset.StandardCharsets;
import java.util.*;
import java.util.zip.ZipEntry;
import java.util.zip.ZipFile;
import javax.imageio.ImageIO;
import javax.imageio.ImageReader;
import javax.imageio.stream.ImageInputStream;

/**
 * Pure Java parser for Android APK packages:
 * - Decodes AndroidManifest.xml (binary AXML) for package, application, theme, activities & launcher
 * - Decodes resources.arsc (including AAPT2 sparse & offset16 types) for strings, colors, dimens,
 *   file paths (layouts/drawables), IDs, arrays, and theme styles.
 * - Extracts real PNG/WebP/Bitmap resources and native .so inventory.
 */
public class ApkResourceTable implements AutoCloseable {

    public static final int MAX_ZIP_ENTRY_BYTES = 64 * 1024 * 1024;
    public static final long MAX_TOTAL_ENTRY_BYTES = 512L * 1024 * 1024;
    public static final int MAX_IMAGE_DIM = 8192;
    public static final long MAX_IMAGE_PIXELS = 67108864L; // 8192 * 8192
    private long totalBytesRead = 0L;

    public static final int TYPE_NULL = 0x00;
    public static final int TYPE_REFERENCE = 0x01;
    public static final int TYPE_ATTRIBUTE = 0x02;
    public static final int TYPE_STRING = 0x03;
    public static final int TYPE_FLOAT = 0x04;
    public static final int TYPE_DIMENSION = 0x05;
    public static final int TYPE_FRACTION = 0x06;
    public static final int TYPE_DYNAMIC_REFERENCE = 0x07;
    public static final int TYPE_INT_DEC = 0x10;
    public static final int TYPE_INT_HEX = 0x11;
    public static final int TYPE_INT_BOOLEAN = 0x12;
    public static final int TYPE_INT_COLOR_ARGB8 = 0x1c;
    public static final int TYPE_INT_COLOR_RGB8 = 0x1d;
    public static final int TYPE_INT_COLOR_ARGB4 = 0x1e;
    public static final int TYPE_INT_COLOR_RGB4 = 0x1f;

    public static class ResValue {
        public final int dataType;
        public final int data;
        public final String stringVal;

        public ResValue(int dataType, int data, String stringVal) {
            this.dataType = dataType;
            this.data = data;
            this.stringVal = stringVal;
        }
    }

    public static class StyleBag {
        public int parentResId;
        public Map<Integer, ResValue> attrs = new HashMap<>();
    }

    private final File apkFile;
    private ZipFile zipFile;
    private List<String> globalStrings = Collections.emptyList();

    // Metadata from AndroidManifest.xml
    public String packageName = "com.example.app";
    public String applicationClass = "";
    public String appLabel = "";
    public int appThemeResId = 0;
    public String launcherActivity = "";
    public int launcherThemeResId = 0;
    public static class ProviderDecl {
        public final String className;
        public final String authorities;
        public final boolean exported;
        public ProviderDecl(String className, String authorities, boolean exported) {
            this.className = className;
            this.authorities = authorities;
            this.exported = exported;
        }
    }

    public static class ActivityIntentFilter {
        public final String activityName;
        public final android.content.IntentFilter filter;
        public ActivityIntentFilter(String activityName, android.content.IntentFilter filter) {
            this.activityName = activityName;
            this.filter = filter;
        }
    }

    public final List<String> activities = new ArrayList<>();
    public final List<String> services = new ArrayList<>();
    public final List<String> receivers = new ArrayList<>();
    public final List<ProviderDecl> providers = new ArrayList<>();
    public final List<ActivityIntentFilter> activityIntentFilters = new ArrayList<>();
    public final List<String> permissions = new ArrayList<>();
    public final List<String> nativeLibs = new ArrayList<>();
    public int minSdk = 0;
    public int targetSdk = 0;

    // Resource maps
    private final Map<Integer, String> resIdToName = new HashMap<>();
    private final Map<Integer, String> resIdToType = new HashMap<>();
    private final Map<String, Integer> nameToResId = new HashMap<>();
    private final Map<Integer, ResValue> values = new HashMap<>();
    private final Map<Integer, Integer> valueScore = new HashMap<>();
    private final Map<Integer, StyleBag> styles = new HashMap<>();
    private final Map<String, BufferedImage> imageCache = new HashMap<>();

    public ApkResourceTable(File apkFile) throws IOException {
        this.apkFile = apkFile;
        if (apkFile != null && apkFile.exists()) {
            this.zipFile = new ZipFile(apkFile);
            inspectNativeLibs();
            byte[] arsc = readZipEntry("resources.arsc");
            if (arsc != null) {
                parseArsc(arsc);
            }
            byte[] manifest = readZipEntry("AndroidManifest.xml");
            if (manifest != null) {
                parseManifest(manifest);
            }
        }
    }

    public ZipFile getZipFile() {
        return zipFile;
    }

    public File getApkFile() {
        return apkFile;
    }

    public byte[] readZipEntry(String name) {
        if (zipFile == null || name == null) return null;
        if (name.contains("..") || name.contains("\\") || name.indexOf('\0') >= 0) {
            throw new SecurityException("Unsafe ZIP entry name blocked: " + name);
        }
        ZipEntry entry = zipFile.getEntry(name);
        if (entry == null) return null;
        if (entry.getSize() > MAX_ZIP_ENTRY_BYTES) {
            throw new SecurityException("ZIP entry exceeds size limit: " + name);
        }
        try (InputStream is = zipFile.getInputStream(entry)) {
            ByteArrayOutputStream baos = new ByteArrayOutputStream();
            byte[] buf = new byte[8192];
            int n;
            int entryTotal = 0;
            while ((n = is.read(buf)) != -1) {
                entryTotal += n;
                totalBytesRead += n;
                if (entryTotal > MAX_ZIP_ENTRY_BYTES || totalBytesRead > MAX_TOTAL_ENTRY_BYTES) {
                    throw new SecurityException("ZIP decompression limit exceeded while reading: " + name);
                }
                baos.write(buf, 0, n);
            }
            return baos.toByteArray();
        } catch (IOException e) {
            return null;
        }
    }

    public BufferedImage loadBitmap(String zipEntryPath) {
        if (zipEntryPath == null || zipFile == null) return null;
        if (zipEntryPath.contains("..") || zipEntryPath.contains("\\") || zipEntryPath.indexOf('\0') >= 0) {
            return null;
        }
        if (imageCache.containsKey(zipEntryPath)) {
            return imageCache.get(zipEntryPath);
        }
        String lower = zipEntryPath.toLowerCase();
        if (lower.endsWith(".png") || lower.endsWith(".jpg") || lower.endsWith(".jpeg")) {
            try {
                ZipEntry entry = zipFile.getEntry(zipEntryPath);
                if (entry != null && entry.getSize() <= MAX_ZIP_ENTRY_BYTES) {
                    try (InputStream is = zipFile.getInputStream(entry);
                         ImageInputStream iis = ImageIO.createImageInputStream(is)) {
                        if (iis == null) return null;
                        Iterator<ImageReader> readers = ImageIO.getImageReaders(iis);
                        if (!readers.hasNext()) return null;
                        ImageReader reader = readers.next();
                        try {
                            reader.setInput(iis, true, true);
                            int w = reader.getWidth(0);
                            int h = reader.getHeight(0);
                            if (w <= 0 || h <= 0 || w > MAX_IMAGE_DIM || h > MAX_IMAGE_DIM || ((long) w * h) > MAX_IMAGE_PIXELS) {
                                throw new SecurityException("Image dimensions exceed safety limit: " + w + "x" + h);
                            }
                            BufferedImage img = reader.read(0);
                            if (img != null) {
                                imageCache.put(zipEntryPath, img);
                                return img;
                            }
                        } finally {
                            reader.dispose();
                        }
                    }
                }
            } catch (SecurityException se) {
                throw se;
            } catch (Throwable ignored) {}
        }
        return null;
    }

    @Override
    public void close() {
        if (zipFile != null) {
            try {
                zipFile.close();
            } catch (IOException ignored) {}
            zipFile = null;
        }
    }

    private void inspectNativeLibs() {
        if (zipFile == null) return;
        Enumeration<? extends ZipEntry> en = zipFile.entries();
        while (en.hasMoreElements()) {
            String n = en.nextElement().getName();
            if (n.endsWith(".so")) {
                nativeLibs.add(n);
            }
        }
    }

    public String getEntryName(int resId) {
        return resIdToName.get(resId);
    }

    public String getEntryType(int resId) {
        return resIdToType.get(resId);
    }

    public List<Map.Entry<Integer, String>> getEntriesByType(String targetType) {
        List<Map.Entry<Integer, String>> out = new ArrayList<>();
        for (Map.Entry<Integer, String> e : resIdToType.entrySet()) {
            if (targetType.equals(e.getValue())) {
                String n = resIdToName.get(e.getKey());
                out.add(new AbstractMap.SimpleEntry<>(e.getKey(), n != null ? n : ("res_" + e.getKey())));
            }
        }
        out.sort(Comparator.comparing(Map.Entry::getValue));
        return out;
    }

    public int getIdentifier(String name, String defType, String defPackage) {
        if (name == null) return 0;
        String key = name + ":" + (defType != null ? defType : "") + ":" + (defPackage != null ? defPackage : packageName);
        Integer id = nameToResId.get(key);
        if (id != null) return id;
        key = name + ":" + (defType != null ? defType : "") + ":" + packageName;
        id = nameToResId.get(key);
        if (id != null) return id;
        id = nameToResId.get(name + ":" + (defType != null ? defType : ""));
        return id != null ? id : 0;
    }

    public ResValue resolveValue(int resId) {
        return resolveValue(resId, 0);
    }

    private ResValue resolveValue(int resId, int depth) {
        if (depth > 8) return null;
        ResValue v = values.get(resId);
        if (v == null) return null;
        if ((v.dataType == TYPE_REFERENCE || v.dataType == TYPE_DYNAMIC_REFERENCE) && v.data != 0 && v.data != resId) {
            ResValue next = resolveValue(v.data, depth + 1);
            if (next != null) return next;
        }
        if (v.dataType == TYPE_ATTRIBUTE && v.data != 0) {
            ResValue attrVal = resolveThemeAttr(appThemeResId, v.data);
            if (attrVal != null) return attrVal;
        }
        return v;
    }

    public String getString(int resId) {
        ResValue v = resolveValue(resId, 0);
        if (v != null && v.stringVal != null) {
            return v.stringVal;
        }
        if (v != null && v.dataType == TYPE_STRING && v.data >= 0 && v.data < globalStrings.size()) {
            return globalStrings.get(v.data);
        }
        String name = resIdToName.get(resId);
        return name != null ? name : null;
    }

    public String getFilePath(int resId) {
        ResValue v = resolveValue(resId, 0);
        if (v != null && v.dataType == TYPE_STRING) {
            String s = v.stringVal != null ? v.stringVal :
                    (v.data >= 0 && v.data < globalStrings.size() ? globalStrings.get(v.data) : null);
            if (s != null && (s.startsWith("res/") || s.endsWith(".xml") || s.endsWith(".png") || s.endsWith(".webp") || s.endsWith(".jpg"))) {
                return s;
            }
        }
        return null;
    }

    public Integer getColor(int resId) {
        return getColor(resId, 0);
    }

    private Integer getColor(int resId, int depth) {
        if (depth > 8) return null;
        ResValue v = resolveValue(resId, depth);
        if (v == null) return null;
        if (v.dataType >= 0x1c && v.dataType <= 0x1f) {
            return expandColor(v.dataType, v.data);
        }
        if (v.dataType == TYPE_STRING && v.stringVal != null && v.stringVal.startsWith("#")) {
            return parseHexColor(v.stringVal);
        }
        return null;
    }

    public Float getDimensionPx(int resId, float density, float scaledDensity) {
        ResValue v = resolveValue(resId, 0);
        if (v == null) return null;
        if (v.dataType == TYPE_DIMENSION) {
            return complexToDimensionPx(v.data, density, scaledDensity);
        }
        if (v.dataType == TYPE_FLOAT) {
            return Float.intBitsToFloat(v.data);
        }
        return null;
    }

    public ResValue resolveThemeAttr(int themeResId, int attrResId) {
        int cur = themeResId != 0 ? themeResId : (launcherThemeResId != 0 ? launcherThemeResId : appThemeResId);
        for (int i = 0; i < 12 && cur != 0; i++) {
            StyleBag bag = styles.get(cur);
            if (bag == null) break;
            ResValue v = bag.attrs.get(attrResId);
            if (v != null) {
                if (v.dataType == TYPE_REFERENCE && v.data != 0) {
                    ResValue resolved = resolveValue(v.data, 0);
                    return resolved != null ? resolved : v;
                }
                return v;
            }
            cur = bag.parentResId;
        }
        return null;
    }

    public static int expandColor(int type, int data) {
        if (type == TYPE_INT_COLOR_ARGB8) return data;
        if (type == TYPE_INT_COLOR_RGB8) return 0xFF000000 | (data & 0x00FFFFFF);
        if (type == TYPE_INT_COLOR_ARGB4) {
            int a = ((data >> 12) & 0xF) * 17;
            int r = ((data >> 8) & 0xF) * 17;
            int g = ((data >> 4) & 0xF) * 17;
            int b = (data & 0xF) * 17;
            return (a << 24) | (r << 16) | (g << 8) | b;
        }
        if (type == TYPE_INT_COLOR_RGB4) {
            int r = ((data >> 8) & 0xF) * 17;
            int g = ((data >> 4) & 0xF) * 17;
            int b = (data & 0xF) * 17;
            return 0xFF000000 | (r << 16) | (g << 8) | b;
        }
        return data;
    }

    public static Integer parseHexColor(String s) {
        if (s == null || !s.startsWith("#")) return null;
        try {
            String hex = s.substring(1).trim();
            if (hex.length() == 6) return (int) (0xFF000000L | Long.parseLong(hex, 16));
            if (hex.length() == 8) return (int) Long.parseLong(hex, 16);
            if (hex.length() == 3) {
                int r = Integer.parseInt(hex.substring(0, 1), 16) * 17;
                int g = Integer.parseInt(hex.substring(1, 2), 16) * 17;
                int b = Integer.parseInt(hex.substring(2, 3), 16) * 17;
                return 0xFF000000 | (r << 16) | (g << 8) | b;
            }
        } catch (Exception ignored) {}
        return null;
    }

    public static float complexToFloat(int complex) {
        final float[] RADIX_MULTS = { 1.0f, 1.0f / (1 << 7), 1.0f / (1 << 15), 1.0f / (1 << 23) };
        int mantissa = (complex & 0xffffff00);
        int radix = (complex >> 4) & 0x3;
        return mantissa * (1.0f / (1 << 8)) * RADIX_MULTS[radix];
    }

    public static float complexToDimensionPx(int data, float density, float scaledDensity) {
        float val = complexToFloat(data);
        int unit = data & 0xF;
        switch (unit) {
            case 0: return val; // px
            case 1: return val * density; // dp
            case 2: return val * scaledDensity; // sp
            case 3: return val * (density * 160.0f / 72.0f); // pt
            case 4: return val * (density * 160.0f); // in
            case 5: return val * (density * 160.0f / 25.4f); // mm
            default: return val * density;
        }
    }

    // ==================== BINARY PARSING (resources.arsc & AXML) ====================

    private void parseArsc(byte[] data) {
        if (data.length < 12) return;
        ByteBuffer buf = ByteBuffer.wrap(data).order(ByteOrder.LITTLE_ENDIAN);
        int tableType = buf.getShort(0) & 0xFFFF;
        if (tableType != 0x0002) return;
        int tableHeaderSize = buf.getShort(2) & 0xFFFF;
        int offset = tableHeaderSize;

        while (offset + 8 <= data.length) {
            int chunkType = buf.getShort(offset) & 0xFFFF;
            int chunkHeaderSize = buf.getShort(offset + 2) & 0xFFFF;
            int chunkSize = buf.getInt(offset + 4);
            if (chunkSize < 8 || offset + chunkSize > data.length) break;

            if (chunkType == 0x0001) { // RES_STRING_POOL_TYPE
                globalStrings = parseStringPool(data, offset, chunkSize);
            } else if (chunkType == 0x0200) { // RES_TABLE_PACKAGE_TYPE
                parsePackage(data, buf, offset, chunkHeaderSize, chunkSize);
            }
            offset += chunkSize;
        }
    }

    private void parsePackage(byte[] data, ByteBuffer buf, int pkgStart, int pkgHeaderSize, int pkgSize) {
        int pkgId = buf.getInt(pkgStart + 8);
        String pkgName = readUtf16Fixed(data, pkgStart + 12, 128);
        if (pkgId == 0x7f && pkgName != null && !pkgName.isEmpty() && "com.example.app".equals(packageName)) {
            packageName = pkgName;
        }

        int typeStringsOff = buf.getInt(pkgStart + 268);
        int keyStringsOff = buf.getInt(pkgStart + 276);

        List<String> typeStrings = Collections.emptyList();
        List<String> keyStrings = Collections.emptyList();
        if (typeStringsOff > 0 && pkgStart + typeStringsOff + 8 <= pkgStart + pkgSize) {
            int tSize = buf.getInt(pkgStart + typeStringsOff + 4);
            typeStrings = parseStringPool(data, pkgStart + typeStringsOff, tSize);
        }
        if (keyStringsOff > 0 && pkgStart + keyStringsOff + 8 <= pkgStart + pkgSize) {
            int kSize = buf.getInt(pkgStart + keyStringsOff + 4);
            keyStrings = parseStringPool(data, pkgStart + keyStringsOff, kSize);
        }

        int off = pkgStart + pkgHeaderSize;
        int pkgEnd = pkgStart + pkgSize;
        while (off + 8 <= pkgEnd) {
            int cType = buf.getShort(off) & 0xFFFF;
            int cHdrSize = buf.getShort(off + 2) & 0xFFFF;
            int cSize = buf.getInt(off + 4);
            if (cSize < 8 || off + cSize > pkgEnd) break;

            if (cType == 0x0201) { // RES_TABLE_TYPE_TYPE
                parseTypeChunk(data, buf, off, cHdrSize, cSize, pkgId, pkgName, typeStrings, keyStrings);
            }
            off += cSize;
        }
    }

    private void parseTypeChunk(byte[] data, ByteBuffer buf, int chunkStart, int headerSize, int chunkSize,
                                int pkgId, String pkgName, List<String> typeStrings, List<String> keyStrings) {
        if (headerSize < 20) return;
        int typeId = data[chunkStart + 8] & 0xFF;
        int flags = data[chunkStart + 9] & 0xFF;
        int entryCount = buf.getInt(chunkStart + 12);
        int entriesStart = buf.getInt(chunkStart + 16);

        String typeName = (typeId - 1 >= 0 && typeId - 1 < typeStrings.size()) ? typeStrings.get(typeId - 1) : "unknown";
        int score = scoreConfig(data, buf, chunkStart + 20);

        boolean isSparse = (flags & 0x01) != 0;
        boolean isOffset16 = (flags & 0x02) != 0;
        int tableOff = chunkStart + headerSize;
        int dataBase = chunkStart + entriesStart;
        int chunkEnd = chunkStart + chunkSize;

        if (isSparse) {
            for (int i = 0; i < entryCount && tableOff + i * 4 + 4 <= dataBase; i++) {
                int entryIdx = buf.getShort(tableOff + i * 4) & 0xFFFF;
                int offsetDiv4 = buf.getShort(tableOff + i * 4 + 2) & 0xFFFF;
                int entryPos = dataBase + offsetDiv4 * 4;
                if (entryPos < chunkEnd) {
                    readResEntry(data, buf, entryPos, chunkEnd, pkgId, pkgName, typeId, typeName, entryIdx, score, keyStrings);
                }
            }
        } else if (isOffset16) {
            for (int i = 0; i < entryCount && tableOff + i * 2 + 2 <= dataBase; i++) {
                int off16 = buf.getShort(tableOff + i * 2) & 0xFFFF;
                if (off16 == 0xFFFF) continue;
                int entryPos = dataBase + off16 * 4;
                if (entryPos < chunkEnd) {
                    readResEntry(data, buf, entryPos, chunkEnd, pkgId, pkgName, typeId, typeName, i, score, keyStrings);
                }
            }
        } else {
            for (int i = 0; i < entryCount && tableOff + i * 4 + 4 <= dataBase; i++) {
                int off32 = buf.getInt(tableOff + i * 4);
                if (off32 == -1) continue;
                int entryPos = dataBase + off32;
                if (entryPos >= dataBase && entryPos < chunkEnd) {
                    readResEntry(data, buf, entryPos, chunkEnd, pkgId, pkgName, typeId, typeName, i, score, keyStrings);
                }
            }
        }
    }

    private int scoreConfig(byte[] data, ByteBuffer buf, int cfgStart) {
        if (cfgStart + 28 > data.length) return 10;
        int cfgSize = buf.getInt(cfgStart);
        int score = 100;
        if (cfgSize >= 12) {
            byte l0 = data[cfgStart + 8];
            byte l1 = data[cfgStart + 9];
            if (l0 == 0 && l1 == 0) {
                score += 50; // default locale
            } else if (l0 == 'e' && l1 == 'n') {
                score += 40; // English
            } else {
                score -= 60; // other locale
            }
        }
        if (cfgSize >= 16) {
            int density = buf.getShort(cfgStart + 14) & 0xFFFF;
            if (density == 480) score += 25; // xxhdpi preferred
            else if (density == 320) score += 20;
            else if (density == 640) score += 18;
            else if (density == 240) score += 15;
            else if (density == 0) score += 10;
            else if (density == 65534 || density == 65535) score -= 5; // anydpi / nodpi fine for vectors
        }
        return score;
    }

    private void readResEntry(byte[] data, ByteBuffer buf, int entryPos, int chunkEnd,
                              int pkgId, String pkgName, int typeId, String typeName, int entryIdx, int score, List<String> keyStrings) {
        if (entryPos + 8 > chunkEnd) return;
        int entrySize = buf.getShort(entryPos) & 0xFFFF;
        int entryFlags = buf.getShort(entryPos + 2) & 0xFFFF;
        boolean isCompact = (entryFlags & 0x0008) != 0;
        boolean isComplex = (entryFlags & 0x0001) != 0;

        int resId = (pkgId << 24) | (typeId << 16) | (entryIdx & 0xFFFF);

        if (isCompact) {
            // Compact ResTable_entry (Android 13+): size carries dataType in high byte or packed
            int keyIdx = buf.getShort(entryPos) & 0xFFFF;
            int dataType = (entryFlags >> 8) & 0xFF;
            int valData = buf.getInt(entryPos + 4);
            String keyName = (keyIdx >= 0 && keyIdx < keyStrings.size()) ? keyStrings.get(keyIdx) : "res_" + entryIdx;
            recordName(resId, keyName, typeName, pkgName);
            String sVal = (dataType == TYPE_STRING && valData >= 0 && valData < globalStrings.size()) ? globalStrings.get(valData) : null;
            recordValue(resId, new ResValue(dataType, valData, sVal), score);
            return;
        }

        int keyIdx = buf.getInt(entryPos + 4);
        String keyName = (keyIdx >= 0 && keyIdx < keyStrings.size()) ? keyStrings.get(keyIdx) : "res_" + entryIdx;
        recordName(resId, keyName, typeName, pkgName);

        if (!isComplex) {
            int valPos = entryPos + entrySize;
            if (valPos + 8 <= chunkEnd) {
                int valType = data[valPos + 3] & 0xFF;
                int valData = buf.getInt(valPos + 4);
                String sVal = (valType == TYPE_STRING && valData >= 0 && valData < globalStrings.size()) ? globalStrings.get(valData) : null;
                recordValue(resId, new ResValue(valType, valData, sVal), score);
            }
        } else {
            if (entryPos + 16 <= chunkEnd) {
                int parentId = buf.getInt(entryPos + 8);
                int count = buf.getInt(entryPos + 12);
                StyleBag bag = styles.get(resId);
                Integer prevScore = valueScore.get(resId);
                if (bag == null || prevScore == null || score >= prevScore) {
                    bag = new StyleBag();
                    bag.parentResId = parentId;
                    int mapPos = entryPos + Math.max(entrySize, 16);
                    for (int m = 0; m < count && mapPos + 12 <= chunkEnd; m++) {
                        int attrId = buf.getInt(mapPos);
                        int valType = data[mapPos + 7] & 0xFF;
                        int valData = buf.getInt(mapPos + 8);
                        String sVal = (valType == TYPE_STRING && valData >= 0 && valData < globalStrings.size()) ? globalStrings.get(valData) : null;
                        bag.attrs.put(attrId, new ResValue(valType, valData, sVal));
                        mapPos += 12;
                    }
                    styles.put(resId, bag);
                    valueScore.put(resId, score);
                }
            }
        }
    }

    private void recordName(int resId, String keyName, String typeName, String pkgName) {
        if (!resIdToName.containsKey(resId)) {
            resIdToName.put(resId, keyName);
            resIdToType.put(resId, typeName);
            nameToResId.put(keyName + ":" + typeName + ":" + pkgName, resId);
            nameToResId.put(keyName + ":" + typeName, resId);
        }
    }

    private void recordValue(int resId, ResValue val, int score) {
        Integer prev = valueScore.get(resId);
        if (prev == null || score > prev) {
            values.put(resId, val);
            valueScore.put(resId, score);
        }
    }

    // ==================== BINARY ANDROIDMANIFEST.XML PARSER ====================

    private void parseManifest(byte[] axml) {
        if (axml.length < 8) return;
        ByteBuffer buf = ByteBuffer.wrap(axml).order(ByteOrder.LITTLE_ENDIAN);
        int type = buf.getShort(0) & 0xFFFF;
        if (type != 0x0003) return;

        List<String> pool = Collections.emptyList();
        int offset = 8;
        String currentActivity = null;
        int currentActTheme = 0;
        boolean inIntentFilter = false;
        boolean hasMain = false;
        boolean hasLauncher = false;
        android.content.IntentFilter currentFilter = null;

        while (offset + 8 <= axml.length) {
            int chunkType = buf.getShort(offset) & 0xFFFF;
            int headerSize = buf.getShort(offset + 2) & 0xFFFF;
            int chunkSize = buf.getInt(offset + 4);
            if (chunkSize < 8 || offset + chunkSize > axml.length) break;

            if (chunkType == 0x0001) {
                pool = parseStringPool(axml, offset, chunkSize);
            } else if (chunkType == 0x0102 && offset + 36 <= axml.length) { // START_ELEMENT
                int nameIdx = buf.getInt(offset + 20);
                String tag = (nameIdx >= 0 && nameIdx < pool.size()) ? pool.get(nameIdx) : "";
                int attrStart = buf.getShort(offset + 24) & 0xFFFF;
                int attrSize = buf.getShort(offset + 26) & 0xFFFF;
                int attrCount = buf.getShort(offset + 28) & 0xFFFF;
                int aOff = offset + 16 + attrStart;

                Map<String, String> strAttrs = new HashMap<>();
                Map<String, Integer> resAttrs = new HashMap<>();
                Map<String, Integer> intAttrs = new HashMap<>();

                for (int i = 0; i < attrCount && aOff + 20 <= axml.length; i++) {
                    int aNameIdx = buf.getInt(aOff + 4);
                    int aRawValIdx = buf.getInt(aOff + 8);
                    int aType = axml[aOff + 15] & 0xFF;
                    int aData = buf.getInt(aOff + 16);

                    String aName = (aNameIdx >= 0 && aNameIdx < pool.size()) ? pool.get(aNameIdx) : "";
                    String sVal = null;
                    if (aRawValIdx != -1 && aRawValIdx < pool.size()) {
                        sVal = pool.get(aRawValIdx);
                    } else if (aType == TYPE_STRING && aData >= 0 && aData < pool.size()) {
                        sVal = pool.get(aData);
                    } else if (aType == TYPE_REFERENCE) {
                        resAttrs.put(aName, aData);
                        sVal = getString(aData);
                    } else if (aType == TYPE_INT_DEC || aType == TYPE_INT_HEX || aType == TYPE_INT_BOOLEAN) {
                        intAttrs.put(aName, aData);
                    }
                    if (!aName.isEmpty() && sVal != null) {
                        strAttrs.put(aName, sVal);
                    }
                    aOff += attrSize;
                }

                if ("manifest".equals(tag)) {
                    if (strAttrs.containsKey("package")) packageName = strAttrs.get("package");
                } else if ("uses-sdk".equals(tag)) {
                    if (intAttrs.containsKey("minSdkVersion")) minSdk = intAttrs.get("minSdkVersion");
                    if (intAttrs.containsKey("targetSdkVersion")) targetSdk = intAttrs.get("targetSdkVersion");
                } else if ("application".equals(tag)) {
                    if (strAttrs.containsKey("name")) applicationClass = resolveComponentName(strAttrs.get("name"));
                    if (strAttrs.containsKey("label")) appLabel = strAttrs.get("label");
                    if (resAttrs.containsKey("theme")) appThemeResId = resAttrs.get("theme");
                } else if ("activity".equals(tag)) {
                    currentActivity = resolveComponentName(strAttrs.get("name"));
                    currentActTheme = resAttrs.getOrDefault("theme", 0);
                    if (!currentActivity.isEmpty() && !activities.contains(currentActivity)) {
                        activities.add(currentActivity);
                    }
                } else if ("activity-alias".equals(tag)) {
                    currentActivity = resolveComponentName(strAttrs.get("targetActivity"));
                } else if ("service".equals(tag)) {
                    String svc = resolveComponentName(strAttrs.get("name"));
                    if (!svc.isEmpty() && !services.contains(svc)) services.add(svc);
                } else if ("receiver".equals(tag)) {
                    String rec = resolveComponentName(strAttrs.get("name"));
                    if (!rec.isEmpty() && !receivers.contains(rec)) receivers.add(rec);
                } else if ("provider".equals(tag)) {
                    String prov = resolveComponentName(strAttrs.get("name"));
                    String auth = strAttrs.getOrDefault("authorities", "");
                    boolean exp = intAttrs.getOrDefault("exported", 0) != 0;
                    if (!prov.isEmpty()) providers.add(new ProviderDecl(prov, auth, exp));
                } else if ("intent-filter".equals(tag)) {
                    inIntentFilter = true;
                    hasMain = false;
                    hasLauncher = false;
                    currentFilter = currentActivity != null ? new android.content.IntentFilter() : null;
                } else if ("action".equals(tag) && inIntentFilter) {
                    String act = strAttrs.get("name");
                    if ("android.intent.action.MAIN".equals(act)) hasMain = true;
                    if (currentFilter != null && act != null) currentFilter.addAction(act);
                } else if ("category".equals(tag) && inIntentFilter) {
                    String cat = strAttrs.get("name");
                    if ("android.intent.category.LAUNCHER".equals(cat)) hasLauncher = true;
                    if (currentFilter != null && cat != null) currentFilter.addCategory(cat);
                } else if ("data".equals(tag) && inIntentFilter && currentFilter != null) {
                    if (strAttrs.containsKey("scheme")) currentFilter.addDataScheme(strAttrs.get("scheme"));
                    if (strAttrs.containsKey("host")) currentFilter.addDataAuthority(strAttrs.get("host"), strAttrs.get("port"));
                    if (strAttrs.containsKey("pathPrefix")) currentFilter.addDataPath(strAttrs.get("pathPrefix"), 2);
                    if (strAttrs.containsKey("path")) currentFilter.addDataPath(strAttrs.get("path"), 0);
                } else if ("uses-permission".equals(tag)) {
                    if (strAttrs.containsKey("name")) permissions.add(strAttrs.get("name"));
                }
            } else if (chunkType == 0x0103 && offset + 24 <= axml.length) { // END_ELEMENT
                int nameIdx = buf.getInt(offset + 20);
                String tag = (nameIdx >= 0 && nameIdx < pool.size()) ? pool.get(nameIdx) : "";
                if ("intent-filter".equals(tag)) {
                    if (hasMain && hasLauncher && currentActivity != null && !currentActivity.isEmpty() && launcherActivity.isEmpty()) {
                        launcherActivity = currentActivity;
                        launcherThemeResId = currentActTheme;
                    }
                    if (currentFilter != null && currentActivity != null && !currentActivity.isEmpty()) {
                        activityIntentFilters.add(new ActivityIntentFilter(currentActivity, currentFilter));
                    }
                    currentFilter = null;
                    inIntentFilter = false;
                } else if ("activity".equals(tag) || "activity-alias".equals(tag)) {
                    currentActivity = null;
                    currentActTheme = 0;
                }
            }
            offset += chunkSize;
        }
    }

    private String resolveComponentName(String raw) {
        if (raw == null || raw.isEmpty()) return "";
        if (raw.startsWith(".")) return packageName + raw;
        if (!raw.contains(".")) return packageName + "." + raw;
        return raw;
    }

    // ==================== SHARED STRING POOL DECODER ====================

    public static List<String> parseStringPool(byte[] data, int poolStart, int poolSize) {
        List<String> list = new ArrayList<>();
        if (poolStart + 28 > data.length) return list;
        ByteBuffer buf = ByteBuffer.wrap(data).order(ByteOrder.LITTLE_ENDIAN);

        int headerSize = buf.getShort(poolStart + 2) & 0xFFFF;
        int stringCount = buf.getInt(poolStart + 8);
        int flags = buf.getInt(poolStart + 16);
        int stringsStart = buf.getInt(poolStart + 20);
        boolean isUtf8 = (flags & 0x00000100) != 0;

        int offTableStart = poolStart + headerSize;
        int strDataStart = poolStart + stringsStart;
        int poolEnd = Math.min(data.length, poolStart + poolSize);

        for (int i = 0; i < stringCount; i++) {
            if (offTableStart + i * 4 + 4 > poolEnd) break;
            int relOff = buf.getInt(offTableStart + i * 4);
            int p = strDataStart + relOff;
            if (p < strDataStart || p >= poolEnd) {
                list.add("");
                continue;
            }
            if (isUtf8) {
                // Skip UTF-16 length (1 or 2 bytes)
                int u16Len = data[p++] & 0xFF;
                if ((u16Len & 0x80) != 0 && p < poolEnd) p++;
                // Read UTF-8 byte length (1 or 2 bytes)
                if (p >= poolEnd) { list.add(""); continue; }
                int b1 = data[p++] & 0xFF;
                int byteLen = b1;
                if ((b1 & 0x80) != 0 && p < poolEnd) {
                    int b2 = data[p++] & 0xFF;
                    byteLen = ((b1 & 0x7F) << 8) | b2;
                }
                int avail = Math.max(0, Math.min(byteLen, poolEnd - p));
                list.add(new String(data, p, avail, StandardCharsets.UTF_8));
            } else {
                if (p + 2 > poolEnd) { list.add(""); continue; }
                int c1 = buf.getShort(p) & 0xFFFF;
                p += 2;
                int charLen = c1;
                if ((c1 & 0x8000) != 0 && p + 2 <= poolEnd) {
                    int c2 = buf.getShort(p) & 0xFFFF;
                    p += 2;
                    charLen = ((c1 & 0x7FFF) << 16) | c2;
                }
                int byteLen = Math.max(0, Math.min(charLen * 2, poolEnd - p));
                list.add(new String(data, p, byteLen, StandardCharsets.UTF_16LE));
            }
        }
        return list;
    }

    private static String readUtf16Fixed(byte[] data, int off, int maxChars) {
        StringBuilder sb = new StringBuilder();
        ByteBuffer buf = ByteBuffer.wrap(data).order(ByteOrder.LITTLE_ENDIAN);
        for (int i = 0; i < maxChars && off + i * 2 + 2 <= data.length; i++) {
            char ch = (char) buf.getShort(off + i * 2);
            if (ch == 0) break;
            sb.append(ch);
        }
        return sb.toString();
    }
}
