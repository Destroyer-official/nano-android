package android.content.res;

import android.content.Context;
import android.graphics.Bitmap;
import android.graphics.drawable.BitmapDrawable;
import android.graphics.drawable.ColorDrawable;
import android.graphics.drawable.Drawable;
import android.util.DisplayMetrics;
import android.util.TypedValue;
import java.awt.image.BufferedImage;
import java.io.ByteArrayInputStream;
import java.io.InputStream;
import java.util.HashMap;
import java.util.Map;
import nanoandroid.host.ApkResourceTable;
import nanoandroid.host.BinaryXmlInflater;

public class Resources {
    private final DisplayMetrics metrics = new DisplayMetrics();
    private final Configuration configuration = new Configuration();
    private final Map<String, Integer> identifierMap = new HashMap<>();
    private final Map<Integer, String> stringMap = new HashMap<>();
    private final Map<Integer, Integer> dimenMap = new HashMap<>();
    private final Map<Integer, Integer> colorMap = new HashMap<>();
    private ApkResourceTable apkTable;

    public class Theme {
        private int themeResId = 0;

        public void applyStyle(int resId, boolean force) {
            if (force || themeResId == 0) {
                this.themeResId = resId;
            }
        }

        public void setTo(Theme other) {
            if (other != null) {
                this.themeResId = other.themeResId;
            }
        }

        public void rebase() {}

        public boolean resolveAttribute(int resid, TypedValue outValue, boolean resolveRefs) {
            if (outValue != null) {
                if (apkTable != null) {
                    ApkResourceTable.ResValue rv = apkTable.resolveThemeAttr(themeResId, resid);
                    if (rv != null) {
                        outValue.type = rv.dataType;
                        outValue.data = rv.data;
                        outValue.string = rv.stringVal;
                        return true;
                    }
                }
                outValue.data = 0;
                outValue.type = TypedValue.TYPE_INT_HEX;
            }
            return true;
        }

        public TypedArray obtainStyledAttributes(int[] attrs) {
            return new TypedArray(Resources.this);
        }

        public TypedArray obtainStyledAttributes(int resid, int[] attrs) {
            return new TypedArray(Resources.this);
        }

        public TypedArray obtainStyledAttributes(android.util.AttributeSet set, int[] attrs, int defStyleAttr, int defStyleRes) {
            return new TypedArray(Resources.this);
        }
    }

    public Resources() {
        identifierMap.put("status_bar_height:dimen:android", 10001);
        dimenMap.put(10001, (int) (28 * metrics.density));
    }

    public void setApkTable(ApkResourceTable table) {
        this.apkTable = table;
    }

    public ApkResourceTable getApkTable() {
        return apkTable;
    }

    public DisplayMetrics getDisplayMetrics() {
        return metrics;
    }

    public Configuration getConfiguration() {
        return configuration;
    }

    public String getResourceName(int resid) {
        if (apkTable != null) {
            String t = apkTable.getEntryType(resid);
            String e = apkTable.getEntryName(resid);
            if (e != null) {
                return apkTable.packageName + ":" + (t != null ? t : "id") + "/" + e;
            }
        }
        return "res_" + resid;
    }

    public String getResourcePackageName(int resid) {
        return apkTable != null ? apkTable.packageName : "com.example.app";
    }

    public String getResourceTypeName(int resid) {
        if (apkTable != null) {
            String t = apkTable.getEntryType(resid);
            if (t != null) return t;
        }
        return "id";
    }

    public String getResourceEntryName(int resid) {
        if (apkTable != null) {
            String e = apkTable.getEntryName(resid);
            if (e != null) return e;
        }
        return "entry_" + resid;
    }

    public int getIdentifier(String name, String defType, String defPackage) {
        if (apkTable != null) {
            int id = apkTable.getIdentifier(name, defType, defPackage);
            if (id != 0) return id;
        }
        String key = name + ":" + defType + ":" + defPackage;
        Integer id = identifierMap.get(key);
        return id != null ? id : 0;
    }

    public int getDimensionPixelSize(int id) {
        if (apkTable != null) {
            Float px = apkTable.getDimensionPx(id, metrics.density, metrics.scaledDensity);
            if (px != null) return Math.round(px);
        }
        Integer px = dimenMap.get(id);
        return px != null ? px : (int) (24 * metrics.density);
    }

    public int getDimensionPixelOffset(int id) {
        return getDimensionPixelSize(id);
    }

    public float getDimension(int id) {
        if (apkTable != null) {
            Float px = apkTable.getDimensionPx(id, metrics.density, metrics.scaledDensity);
            if (px != null) return px;
        }
        return (float) getDimensionPixelSize(id);
    }

    public int getColor(int id) {
        if (apkTable != null) {
            Integer c = apkTable.getColor(id);
            if (c != null) return c;
        }
        Integer c = colorMap.get(id);
        return c != null ? c : 0xFF212121;
    }

    public int getColor(int id, Theme theme) {
        return getColor(id);
    }

    public ColorStateList getColorStateList(int id) {
        return ColorStateList.valueOf(getColor(id));
    }

    public ColorStateList getColorStateList(int id, Theme theme) {
        return getColorStateList(id);
    }

    public Drawable getDrawable(int id) {
        if (apkTable != null) {
            Drawable d = BinaryXmlInflater.loadDrawableById(id, null, apkTable);
            if (d != null) return d;
        }
        return new ColorDrawable(getColor(id));
    }

    public Drawable getDrawable(int id, Theme theme) {
        return getDrawable(id);
    }

    public String getString(int id) {
        if (apkTable != null) {
            String s = apkTable.getString(id);
            if (s != null) return s;
        }
        String s = stringMap.get(id);
        return s != null ? s : "";
    }

    public String getString(int id, Object... formatArgs) {
        String base = getString(id);
        try {
            return String.format(base, formatArgs);
        } catch (Exception e) {
            return base;
        }
    }

    public String[] getStringArray(int id) {
        return new String[0];
    }

    public int[] getIntArray(int id) {
        return new int[0];
    }

    public CharSequence getText(int id) {
        return getString(id);
    }

    public boolean getBoolean(int id) {
        if (apkTable != null) {
            ApkResourceTable.ResValue rv = apkTable.resolveValue(id);
            if (rv != null && rv.dataType == ApkResourceTable.TYPE_INT_BOOLEAN) {
                return rv.data != 0;
            }
        }
        return false;
    }

    public int getInteger(int id) {
        if (apkTable != null) {
            ApkResourceTable.ResValue rv = apkTable.resolveValue(id);
            if (rv != null && (rv.dataType == ApkResourceTable.TYPE_INT_DEC || rv.dataType == ApkResourceTable.TYPE_INT_HEX)) {
                return rv.data;
            }
        }
        return 0;
    }

    public String getQuantityString(int id, int quantity, Object... formatArgs) {
        return getString(id, formatArgs);
    }

    public InputStream openRawResource(int id) {
        if (apkTable != null) {
            String path = apkTable.getFilePath(id);
            if (path != null) {
                byte[] b = apkTable.readZipEntry(path);
                if (b != null) return new ByteArrayInputStream(b);
            }
        }
        return new ByteArrayInputStream(new byte[0]);
    }

    public Theme newTheme() {
        return new Theme();
    }

    public XmlResourceParser getLayout(int id) {
        return new HeadlessXmlResourceParser();
    }

    public XmlResourceParser getXml(int id) {
        return new HeadlessXmlResourceParser();
    }
}
