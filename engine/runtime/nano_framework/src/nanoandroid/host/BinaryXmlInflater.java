package nanoandroid.host;

import android.content.Context;
import android.graphics.*;
import android.graphics.drawable.*;
import android.util.AttributeSet;
import android.util.DisplayMetrics;
import android.view.*;
import android.widget.*;
import java.awt.Graphics2D;
import java.awt.RenderingHints;
import java.awt.geom.Path2D;
import java.awt.image.BufferedImage;
import java.lang.reflect.Constructor;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.util.*;

/**
 * Universal Binary AXML Layout & Drawable Inflater for ANY Android APK.
 * Decodes AAPT2 compiled binary XML files directly from the APK archive using ApkResourceTable.
 */
public class BinaryXmlInflater {

    private static final float DENSITY = 2.625f;

    public static class AxmlAttr {
        public final String namespace;
        public final String name;
        public final int nameResId;
        public final int dataType;
        public final int data;
        public final String stringVal;

        public AxmlAttr(String ns, String name, int nameResId, int dataType, int data, String stringVal) {
            this.namespace = ns;
            this.name = name;
            this.nameResId = nameResId;
            this.dataType = dataType;
            this.data = data;
            this.stringVal = stringVal;
        }
    }

    public static class AxmlAttributeSet implements AttributeSet {
        private final List<AxmlAttr> attrs;
        private final ApkResourceTable table;

        public AxmlAttributeSet(List<AxmlAttr> attrs, ApkResourceTable table) {
            this.attrs = attrs;
            this.table = table;
        }

        private AxmlAttr find(String namespace, String name) {
            for (AxmlAttr a : attrs) {
                if (name.equals(a.name)) {
                    if (namespace == null || namespace.isEmpty() || namespace.equals(a.namespace)) {
                        return a;
                    }
                }
            }
            for (AxmlAttr a : attrs) {
                if (name.equals(a.name)) return a;
            }
            return null;
        }

        @Override public int getAttributeCount() { return attrs.size(); }
        @Override public String getAttributeNamespace(int index) { return attrs.get(index).namespace; }
        @Override public String getAttributeName(int index) { return attrs.get(index).name; }
        @Override public String getAttributeValue(int index) { return valueToString(attrs.get(index)); }
        @Override public String getAttributeValue(String ns, String name) {
            AxmlAttr a = find(ns, name);
            return a != null ? valueToString(a) : null;
        }
        @Override public String getPositionDescription() { return "binary-axml"; }
        @Override public int getAttributeNameResource(int index) { return attrs.get(index).nameResId; }
        @Override public boolean getAttributeBooleanValue(String ns, String name, boolean defVal) {
            AxmlAttr a = find(ns, name);
            if (a == null) return defVal;
            if (a.dataType == ApkResourceTable.TYPE_INT_BOOLEAN) return a.data != 0;
            String s = valueToString(a);
            if ("true".equalsIgnoreCase(s)) return true;
            if ("false".equalsIgnoreCase(s)) return false;
            return defVal;
        }
        @Override public int getAttributeResourceValue(String ns, String name, int defVal) {
            AxmlAttr a = find(ns, name);
            if (a != null && a.dataType == ApkResourceTable.TYPE_REFERENCE) return a.data;
            return defVal;
        }
        @Override public int getAttributeIntValue(String ns, String name, int defVal) {
            AxmlAttr a = find(ns, name);
            if (a != null && (a.dataType == ApkResourceTable.TYPE_INT_DEC || a.dataType == ApkResourceTable.TYPE_INT_HEX)) return a.data;
            return defVal;
        }
        @Override public float getAttributeFloatValue(String ns, String name, float defVal) {
            AxmlAttr a = find(ns, name);
            if (a != null && a.dataType == ApkResourceTable.TYPE_FLOAT) return Float.intBitsToFloat(a.data);
            return defVal;
        }
        @Override public int getIdAttributeResourceValue(int defVal) {
            return getAttributeResourceValue(null, "id", defVal);
        }

        private String valueToString(AxmlAttr a) {
            if (a.stringVal != null) return a.stringVal;
            if (a.dataType == ApkResourceTable.TYPE_REFERENCE && table != null) {
                String s = table.getString(a.data);
                if (s != null) return s;
                return "@" + Integer.toHexString(a.data);
            }
            if (a.dataType == ApkResourceTable.TYPE_INT_BOOLEAN) return a.data != 0 ? "true" : "false";
            if (a.dataType == ApkResourceTable.TYPE_FLOAT) return String.valueOf(Float.intBitsToFloat(a.data));
            if (a.dataType == ApkResourceTable.TYPE_DIMENSION) {
                return String.valueOf(ApkResourceTable.complexToDimensionPx(a.data, DENSITY, DENSITY));
            }
            return String.valueOf(a.data);
        }
    }

    public static View inflateLayout(int layoutResId, Context context, ViewGroup root, boolean attachToRoot, ApkResourceTable table) {
        return inflateLayoutInternal(layoutResId, context, root, attachToRoot, table, 0);
    }

    private static View inflateLayoutInternal(int layoutResId, Context context, ViewGroup root, boolean attachToRoot, ApkResourceTable table, int depth) {
        if (table == null || depth > 8) return null;
        String path = table.getFilePath(layoutResId);
        if (path == null) return null;
        byte[] data = table.readZipEntry(path);
        if (data == null || data.length < 16) return null;

        ByteBuffer buf = ByteBuffer.wrap(data).order(ByteOrder.LITTLE_ENDIAN);
        if ((buf.getShort(0) & 0xFFFF) != 0x0003) return null;

        List<String> pool = Collections.emptyList();
        int[] resMap = new int[0];
        int offset = 8;

        List<ViewGroup> stack = new ArrayList<>();
        if (root != null) {
            stack.add(root);
        }
        View topCreated = null;

        while (offset + 8 <= data.length) {
            int chunkType = buf.getShort(offset) & 0xFFFF;
            int headerSize = buf.getShort(offset + 2) & 0xFFFF;
            int chunkSize = buf.getInt(offset + 4);
            if (chunkSize < 8 || offset + chunkSize > data.length) break;

            if (chunkType == 0x0001) {
                pool = ApkResourceTable.parseStringPool(data, offset, chunkSize);
            } else if (chunkType == 0x0180) { // XML_RESOURCE_MAP
                int count = (chunkSize - 8) / 4;
                resMap = new int[count];
                for (int i = 0; i < count; i++) {
                    resMap[i] = buf.getInt(offset + 8 + i * 4);
                }
            } else if (chunkType == 0x0102 && offset + 36 <= data.length) { // START_ELEMENT
                int nameIdx = buf.getInt(offset + 20);
                String tag = (nameIdx >= 0 && nameIdx < pool.size()) ? pool.get(nameIdx) : "View";

                int attrStart = buf.getShort(offset + 24) & 0xFFFF;
                int attrSize = buf.getShort(offset + 26) & 0xFFFF;
                int attrCount = buf.getShort(offset + 28) & 0xFFFF;
                int aOff = offset + 16 + attrStart;

                List<AxmlAttr> attrs = new ArrayList<>(attrCount);
                for (int i = 0; i < attrCount && aOff + 20 <= data.length; i++) {
                    int nsIdx = buf.getInt(aOff);
                    int aNameIdx = buf.getInt(aOff + 4);
                    int aRawVal = buf.getInt(aOff + 8);
                    int aType = data[aOff + 15] & 0xFF;
                    int aData = buf.getInt(aOff + 16);

                    String ns = (nsIdx >= 0 && nsIdx < pool.size()) ? pool.get(nsIdx) : "";
                    String aName = (aNameIdx >= 0 && aNameIdx < pool.size()) ? pool.get(aNameIdx) : "";
                    int aNameResId = (aNameIdx >= 0 && aNameIdx < resMap.length) ? resMap[aNameIdx] : 0;
                    String sVal = (aRawVal >= 0 && aRawVal < pool.size()) ? pool.get(aRawVal) :
                            (aType == ApkResourceTable.TYPE_STRING && aData >= 0 && aData < pool.size() ? pool.get(aData) : null);

                    attrs.add(new AxmlAttr(ns, aName, aNameResId, aType, aData, sVal));
                    aOff += attrSize;
                }

                if ("include".equals(tag)) {
                    int includeRes = 0;
                    for (AxmlAttr a : attrs) {
                        if ("layout".equals(a.name) && a.dataType == ApkResourceTable.TYPE_REFERENCE) {
                            includeRes = a.data;
                            break;
                        }
                    }
                    ViewGroup parent = stack.isEmpty() ? null : stack.get(stack.size() - 1);
                    View inc = inflateLayoutInternal(includeRes, context, parent, parent != null, table, depth + 1);
                    if (inc != null) {
                        applyAttributes(inc, parent, new AxmlAttributeSet(attrs, table), attrs, context, table);
                        if (topCreated == null) topCreated = inc;
                    }
                    stack.add(null);
                } else if ("merge".equals(tag)) {
                    stack.add(stack.isEmpty() ? null : stack.get(stack.size() - 1));
                } else {
                    AttributeSet attrSet = new AxmlAttributeSet(attrs, table);
                    View v = instantiateView(tag, context, attrSet);
                    if (!(v instanceof ViewGroup)) {
                        int peek = offset + chunkSize;
                        if (peek + 8 <= data.length && (buf.getShort(peek) & 0xFFFF) == 0x0102) {
                            v = new FrameLayout(context);
                        }
                    }
                    ViewGroup parent = stack.isEmpty() ? null : stack.get(stack.size() - 1);
                    applyAttributes(v, parent, attrSet, attrs, context, table);

                    if (parent != null && (parent != root || attachToRoot)) {
                        parent.addView(v);
                    }
                    if (topCreated == null) {
                        topCreated = v;
                    }
                    stack.add(v instanceof ViewGroup ? (ViewGroup) v : null);
                }
            } else if (chunkType == 0x0103) { // END_ELEMENT
                if (!stack.isEmpty()) {
                    stack.remove(stack.size() - 1);
                }
            }

            offset += chunkSize;
        }

        return (attachToRoot && root != null) ? root : topCreated;
    }

    private static View instantiateView(String tag, Context context, AttributeSet attrs) {
        if (tag == null) return new FrameLayout(context);

        // First try exact custom class if package-qualified and not pure Material/AndroidX widget that needs full Theme.AppCompat
        if (tag.contains(".") && !tag.startsWith("android.widget.") && !tag.startsWith("android.view.")) {
            try {
                ClassLoader cl = Thread.currentThread().getContextClassLoader();
                if (cl == null && context != null) cl = context.getClassLoader();
                if (cl != null) {
                    Class<?> cls = cl.loadClass(tag);
                    if (View.class.isAssignableFrom(cls)) {
                        try {
                            Constructor<?> c = cls.getConstructor(Context.class, AttributeSet.class);
                            return (View) c.newInstance(context, attrs);
                        } catch (Throwable ignored) {
                            try {
                                Constructor<?> c = cls.getConstructor(Context.class);
                                return (View) c.newInstance(context);
                            } catch (Throwable ignored2) {
                                try {
                                    View unsafeView = (View) NanoClassLoader.getUnsafe().allocateInstance(cls);
                                    unsafeView.setUseSafeLayoutFallback(true);
                                    NanoClassLoader.initViewDefaults(unsafeView, context);
                                    NanoClassLoader.populateFields(unsafeView, context);
                                    return unsafeView;
                                } catch (Throwable ignored3) {}
                            }
                        }
                    }
                }
            } catch (Throwable ignored) {}
        }

        String simple = tag.contains(".") ? tag.substring(tag.lastIndexOf('.') + 1) : tag;
        String low = simple.toLowerCase();

        if (low.contains("composeview")) {
            LinearLayout composeBox = new LinearLayout(context);
            composeBox.setOrientation(LinearLayout.VERTICAL);
            composeBox.setResourceIdName("ComposeView");
            return composeBox;
        }
        if (low.contains("toolbar")) {
            LinearLayout bar = new LinearLayout(context);
            bar.setOrientation(LinearLayout.HORIZONTAL);
            return bar;
        }
        if (low.contains("linearlayout") || low.contains("appbarlayout") || low.contains("radiogroup") || low.contains("bottomnavigation") || low.contains("tablayout") || low.contains("navigationview") || low.contains("bottomappbar")) {
            LinearLayout ll = new LinearLayout(context);
            ll.setOrientation(low.contains("bottomnavigation") || low.contains("tablayout") || low.contains("bottomappbar") ? LinearLayout.HORIZONTAL : LinearLayout.VERTICAL);
            return ll;
        }
        if (low.contains("nestedscrollview") || low.equals("scrollview")) return new ScrollView(context);
        if (low.contains("horizontalscrollview")) return new HorizontalScrollView(context);
        if (low.contains("recyclerview")) {
            LinearLayout rv = new LinearLayout(context);
            rv.setOrientation(LinearLayout.VERTICAL);
            return rv;
        }
        if (low.contains("relativelayout")) return new RelativeLayout(context);
        if (low.contains("cardview") || low.contains("framelayout") || low.contains("coordinator") || low.contains("constraintlayout") || low.contains("collapsing") || low.contains("swiperefresh") || low.contains("viewpager") || low.contains("drawerlayout") || low.contains("fragment")) {
            return new FrameLayout(context);
        }
        if (low.contains("floatingactionbutton")) {
            Button fab = new Button(context);
            GradientDrawable circle = new GradientDrawable();
            circle.setShape(GradientDrawable.OVAL);
            circle.setColor(0xFF3B82F6);
            fab.setBackground(circle);
            fab.setText("+");
            fab.setTextColor(0xFFFFFFFF);
            return fab;
        }
        if (low.contains("switch")) return new Switch(context);
        if (low.contains("checkbox")) return new CheckBox(context);
        if (low.contains("radiobutton")) return new RadioButton(context);
        if (low.contains("seekbar")) return new SeekBar(context);
        if (low.contains("progressbar")) return new ProgressBar(context);
        if (low.contains("edittext")) return new EditText(context);
        if (low.contains("button") && !low.contains("imagebutton")) return new Button(context);
        if (low.contains("imagebutton") || low.contains("imageview")) return new ImageView(context);
        if (low.contains("textview")) return new TextView(context);

        return new View(context);
    }

    private static void applyAttributes(View v, ViewGroup parent, AttributeSet attrSet, List<AxmlAttr> attrs, Context ctx, ApkResourceTable table) {
        if (v == null || attrs == null) return;

        int lpW = ViewGroup.LayoutParams.WRAP_CONTENT;
        int lpH = ViewGroup.LayoutParams.WRAP_CONTENT;
        float lpWeight = 0f;
        int mL = 0, mT = 0, mR = 0, mB = 0;
        int pL = v.getPaddingLeft(), pT = v.getPaddingTop(), pR = v.getPaddingRight(), pB = v.getPaddingBottom();

        for (AxmlAttr a : attrs) {
            String n = a.name;
            switch (n) {
                case "id":
                    if (a.data != 0) {
                        v.setId(a.data);
                        String eName = table.getEntryName(a.data);
                        if (eName != null) v.setResourceIdName(eName);
                    }
                    break;
                case "orientation":
                    if (v instanceof LinearLayout) {
                        ((LinearLayout) v).setOrientation(a.data == 1 ? LinearLayout.VERTICAL : LinearLayout.HORIZONTAL);
                    }
                    break;
                case "layout_width":
                    lpW = resolveLayoutSize(a, table);
                    break;
                case "layout_height":
                    lpH = resolveLayoutSize(a, table);
                    break;
                case "layout_weight":
                    lpWeight = resolveFloat(a);
                    break;
                case "layout_margin":
                    int m = resolveDimenPx(a, table);
                    mL = mT = mR = mB = m;
                    break;
                case "layout_marginLeft":
                case "layout_marginStart":
                    mL = resolveDimenPx(a, table);
                    break;
                case "layout_marginRight":
                case "layout_marginEnd":
                    mR = resolveDimenPx(a, table);
                    break;
                case "layout_marginTop":
                    mT = resolveDimenPx(a, table);
                    break;
                case "layout_marginBottom":
                    mB = resolveDimenPx(a, table);
                    break;
                case "layout_marginHorizontal":
                    mL = mR = resolveDimenPx(a, table);
                    break;
                case "layout_marginVertical":
                    mT = mB = resolveDimenPx(a, table);
                    break;
                case "padding":
                    int p = resolveDimenPx(a, table);
                    pL = pT = pR = pB = p;
                    break;
                case "paddingLeft":
                case "paddingStart":
                    pL = resolveDimenPx(a, table);
                    break;
                case "paddingRight":
                case "paddingEnd":
                    pR = resolveDimenPx(a, table);
                    break;
                case "paddingTop":
                    pT = resolveDimenPx(a, table);
                    break;
                case "paddingBottom":
                    pB = resolveDimenPx(a, table);
                    break;
                case "paddingHorizontal":
                    pL = pR = resolveDimenPx(a, table);
                    break;
                case "paddingVertical":
                    pT = pB = resolveDimenPx(a, table);
                    break;
                case "text":
                    String txt = resolveString(a, table);
                    if (txt != null) {
                        if (v instanceof TextView) ((TextView) v).setText(txt);
                        else v.setText(txt);
                    }
                    break;
                case "title":
                    String title = resolveString(a, table);
                    if (title != null && v instanceof ViewGroup && !(v instanceof TextView)) {
                        TextView tvTitle = new TextView(ctx);
                        tvTitle.setText(title);
                        tvTitle.setTextSize(20f);
                        tvTitle.setTextColor(0xFFFFFFFF);
                        tvTitle.setTypeface(Typeface.DEFAULT_BOLD);
                        ((ViewGroup) v).addView(tvTitle);
                    }
                    break;
                case "hint":
                    String hint = resolveString(a, table);
                    if (hint != null && v instanceof TextView) ((TextView) v).setHint(hint);
                    break;
                case "contentDescription":
                    String desc = resolveString(a, table);
                    if (desc != null) v.setContentDescription(desc);
                    break;
                case "textSize":
                    if (v instanceof TextView) {
                        int px = resolveDimenPx(a, table);
                        if (px > 0) ((TextView) v).setTextSize(android.util.TypedValue.COMPLEX_UNIT_PX, px);
                    }
                    break;
                case "textColor":
                    if (v instanceof TextView) {
                        Integer col = resolveColor(a, table);
                        if (col != null) ((TextView) v).setTextColor(col);
                    }
                    break;
                case "textStyle":
                    if (v instanceof TextView) {
                        ((TextView) v).setTypeface(Typeface.create(Typeface.DEFAULT, a.data));
                    }
                    break;
                case "gravity":
                    if (v instanceof TextView) ((TextView) v).setGravity(a.data);
                    else if (v instanceof LinearLayout) ((LinearLayout) v).setGravity(a.data);
                    break;
                case "visibility":
                    // Android AXML enum: 0=visible, 1=invisible, 2=gone -> View constants 0, 4, 8
                    if (a.data == 2 || a.data == 8) v.setVisibility(View.GONE);
                    else if (a.data == 1 || a.data == 4) v.setVisibility(View.INVISIBLE);
                    else v.setVisibility(View.VISIBLE);
                    break;
                case "clickable":
                    v.setClickable(a.data != 0);
                    break;
                case "checked":
                    if (v instanceof CompoundButton) ((CompoundButton) v).setChecked(a.data != 0);
                    break;
                case "background":
                case "cardBackgroundColor":
                    Drawable bg = resolveDrawable(a, ctx, table);
                    if (bg != null) v.setBackground(bg);
                    break;
                case "src":
                case "srcCompat":
                    if (v instanceof ImageView) {
                        Drawable d = resolveDrawable(a, ctx, table);
                        if (d != null) ((ImageView) v).setImageDrawable(d);
                    }
                    break;
                case "cardCornerRadius":
                    if (v.getBackground() instanceof GradientDrawable) {
                        ((GradientDrawable) v.getBackground()).setCornerRadius(resolveDimenPx(a, table));
                    } else if (v.getBackground() == null) {
                        GradientDrawable gd = new GradientDrawable();
                        gd.setColor(0xFFFFFFFF);
                        gd.setCornerRadius(resolveDimenPx(a, table));
                        v.setBackground(gd);
                    }
                    break;
            }
        }

        v.setPadding(pL, pT, pR, pB);
        ViewGroup.LayoutParams lp = null;
        if (parent != null) {
            try {
                lp = parent.generateLayoutParams(attrSet);
            } catch (Throwable ignored) {}
            if (lp == null) {
                Class<?> cur = parent.getClass();
                while (cur != null && cur != Object.class) {
                    try {
                        java.lang.reflect.Method m = cur.getDeclaredMethod("generateDefaultLayoutParams");
                        m.setAccessible(true);
                        lp = (ViewGroup.LayoutParams) m.invoke(parent);
                        if (lp != null) break;
                    } catch (Throwable ignored) {}
                    cur = cur.getSuperclass();
                }
            }
        }
        if (lp == null) {
            lp = (parent instanceof RelativeLayout)
                    ? new RelativeLayout.LayoutParams(lpW, lpH)
                    : new LinearLayout.LayoutParams(lpW, lpH, lpWeight);
        }
        lp.width = lpW;
        lp.height = lpH;
        if (lp instanceof LinearLayout.LayoutParams) {
            ((LinearLayout.LayoutParams) lp).weight = lpWeight;
        }
        if (lp instanceof ViewGroup.MarginLayoutParams) {
            ((ViewGroup.MarginLayoutParams) lp).setMargins(mL, mT, mR, mB);
        }
        if (lp instanceof RelativeLayout.LayoutParams) {
            RelativeLayout.LayoutParams rlp = (RelativeLayout.LayoutParams) lp;
            for (AxmlAttr a : attrs) {
                switch (a.name) {
                    case "layout_alignParentTop":
                        if (a.data != 0) rlp.addRule(RelativeLayout.ALIGN_PARENT_TOP);
                        break;
                    case "layout_alignParentBottom":
                        if (a.data != 0) rlp.addRule(RelativeLayout.ALIGN_PARENT_BOTTOM);
                        break;
                    case "layout_alignParentLeft":
                        if (a.data != 0) rlp.addRule(RelativeLayout.ALIGN_PARENT_LEFT);
                        break;
                    case "layout_alignParentStart":
                        if (a.data != 0) rlp.addRule(RelativeLayout.ALIGN_PARENT_START);
                        break;
                    case "layout_alignParentRight":
                        if (a.data != 0) rlp.addRule(RelativeLayout.ALIGN_PARENT_RIGHT);
                        break;
                    case "layout_alignParentEnd":
                        if (a.data != 0) rlp.addRule(RelativeLayout.ALIGN_PARENT_END);
                        break;
                    case "layout_centerInParent":
                        if (a.data != 0) rlp.addRule(RelativeLayout.CENTER_IN_PARENT);
                        break;
                    case "layout_centerHorizontal":
                        if (a.data != 0) rlp.addRule(RelativeLayout.CENTER_HORIZONTAL);
                        break;
                    case "layout_centerVertical":
                        if (a.data != 0) rlp.addRule(RelativeLayout.CENTER_VERTICAL);
                        break;
                    case "layout_toRightOf":
                        if (a.data != 0) rlp.addRule(RelativeLayout.RIGHT_OF, a.data);
                        break;
                    case "layout_toEndOf":
                        if (a.data != 0) rlp.addRule(RelativeLayout.END_OF, a.data);
                        break;
                    case "layout_toLeftOf":
                        if (a.data != 0) rlp.addRule(RelativeLayout.LEFT_OF, a.data);
                        break;
                    case "layout_toStartOf":
                        if (a.data != 0) rlp.addRule(RelativeLayout.START_OF, a.data);
                        break;
                    case "layout_below":
                        if (a.data != 0) rlp.addRule(RelativeLayout.BELOW, a.data);
                        break;
                    case "layout_above":
                        if (a.data != 0) rlp.addRule(RelativeLayout.ABOVE, a.data);
                        break;
                    case "layout_alignTop":
                        if (a.data != 0) rlp.addRule(RelativeLayout.ALIGN_TOP, a.data);
                        break;
                    case "layout_alignBottom":
                        if (a.data != 0) rlp.addRule(RelativeLayout.ALIGN_BOTTOM, a.data);
                        break;
                    case "layout_alignLeft":
                        if (a.data != 0) rlp.addRule(RelativeLayout.ALIGN_LEFT, a.data);
                        break;
                    case "layout_alignStart":
                        if (a.data != 0) rlp.addRule(RelativeLayout.ALIGN_START, a.data);
                        break;
                    case "layout_alignRight":
                        if (a.data != 0) rlp.addRule(RelativeLayout.ALIGN_RIGHT, a.data);
                        break;
                    case "layout_alignEnd":
                        if (a.data != 0) rlp.addRule(RelativeLayout.ALIGN_END, a.data);
                        break;
                }
            }
        }
        v.setLayoutParams(lp);
    }

    private static int resolveLayoutSize(AxmlAttr a, ApkResourceTable table) {
        if (a.dataType == ApkResourceTable.TYPE_INT_DEC || a.dataType == ApkResourceTable.TYPE_INT_HEX) {
            if (a.data == -1 || a.data == -2) return a.data;
            return Math.max(1, a.data);
        }
        if (a.dataType == ApkResourceTable.TYPE_DIMENSION || a.dataType == ApkResourceTable.TYPE_REFERENCE || a.dataType == 0x02) {
            int px = resolveDimenPx(a, table);
            return px > 0 ? px : Math.round(56f * DENSITY);
        }
        return ViewGroup.LayoutParams.WRAP_CONTENT;
    }

    private static int resolveDimenPx(AxmlAttr a, ApkResourceTable table) {
        if (a.dataType == ApkResourceTable.TYPE_DIMENSION) {
            return Math.round(ApkResourceTable.complexToDimensionPx(a.data, DENSITY, DENSITY));
        }
        if (a.dataType == ApkResourceTable.TYPE_REFERENCE && table != null) {
            Float px = table.getDimensionPx(a.data, DENSITY, DENSITY);
            if (px != null) return Math.round(px);
        }
        if (a.dataType == ApkResourceTable.TYPE_ATTRIBUTE && table != null) {
            ApkResourceTable.ResValue rv = table.resolveThemeAttr(0, a.data);
            if (rv != null && rv.dataType == ApkResourceTable.TYPE_DIMENSION) {
                return Math.round(ApkResourceTable.complexToDimensionPx(rv.data, DENSITY, DENSITY));
            }
            if (rv != null && rv.dataType == ApkResourceTable.TYPE_REFERENCE) {
                Float px = table.getDimensionPx(rv.data, DENSITY, DENSITY);
                if (px != null) return Math.round(px);
            }
        }
        if (a.dataType == ApkResourceTable.TYPE_INT_DEC) return a.data;
        return 0;
    }

    private static float resolveFloat(AxmlAttr a) {
        if (a.dataType == ApkResourceTable.TYPE_FLOAT) return Float.intBitsToFloat(a.data);
        if (a.dataType == ApkResourceTable.TYPE_INT_DEC) return a.data;
        return 0f;
    }

    private static String resolveString(AxmlAttr a, ApkResourceTable table) {
        if (a.dataType == ApkResourceTable.TYPE_REFERENCE && table != null) {
            return table.getString(a.data);
        }
        if (a.dataType == ApkResourceTable.TYPE_ATTRIBUTE && table != null) {
            ApkResourceTable.ResValue rv = table.resolveThemeAttr(0, a.data);
            if (rv != null && rv.stringVal != null) return rv.stringVal;
        }
        return a.stringVal;
    }

    private static Integer resolveColor(AxmlAttr a, ApkResourceTable table) {
        if (a.dataType >= 0x1c && a.dataType <= 0x1f) {
            return ApkResourceTable.expandColor(a.dataType, a.data);
        }
        if (a.dataType == ApkResourceTable.TYPE_REFERENCE && table != null) {
            return table.getColor(a.data);
        }
        if (a.dataType == ApkResourceTable.TYPE_ATTRIBUTE && table != null) {
            ApkResourceTable.ResValue rv = table.resolveThemeAttr(0, a.data);
            if (rv != null && rv.dataType >= 0x1c && rv.dataType <= 0x1f) {
                return ApkResourceTable.expandColor(rv.dataType, rv.data);
            }
        }
        if (a.stringVal != null && a.stringVal.startsWith("#")) {
            return ApkResourceTable.parseHexColor(a.stringVal);
        }
        return null;
    }

    public static Drawable resolveDrawable(AxmlAttr a, Context ctx, ApkResourceTable table) {
        Integer col = resolveColor(a, table);
        if (col != null) return new ColorDrawable(col);

        int resId = a.data;
        if (a.dataType == ApkResourceTable.TYPE_ATTRIBUTE && table != null) {
            ApkResourceTable.ResValue rv = table.resolveThemeAttr(0, a.data);
            if (rv != null && rv.dataType == ApkResourceTable.TYPE_REFERENCE) resId = rv.data;
        }
        if (resId == 0 || table == null) return null;
        return loadDrawableById(resId, ctx, table);
    }

    public static Drawable loadDrawableById(int resId, Context ctx, ApkResourceTable table) {
        if (table == null) return null;
        Integer col = table.getColor(resId);
        if (col != null) return new ColorDrawable(col);

        String path = table.getFilePath(resId);
        if (path == null) return null;

        String lower = path.toLowerCase();
        if (lower.endsWith(".png") || lower.endsWith(".jpg") || lower.endsWith(".jpeg")) {
            BufferedImage bi = table.loadBitmap(path);
            if (bi != null) {
                return new BitmapDrawable(ctx != null ? ctx.getResources() : null, new Bitmap(bi));
            }
        } else if (lower.endsWith(".xml")) {
            byte[] xmlData = table.readZipEntry(path);
            if (xmlData != null) {
                return parseDrawableXml(xmlData, ctx, table);
            }
        }
        return null;
    }

    public static Drawable parseDrawableXml(byte[] data, Context ctx, ApkResourceTable table) {
        if (data == null || data.length < 16) return null;
        ByteBuffer buf = ByteBuffer.wrap(data).order(ByteOrder.LITTLE_ENDIAN);
        if ((buf.getShort(0) & 0xFFFF) != 0x0003) return null;

        List<String> pool = Collections.emptyList();
        int offset = 8;

        String rootTag = "";
        GradientDrawable shapeDrawable = null;
        java.util.List<Drawable> layerDrawables = new ArrayList<>();
        java.util.List<Integer> layerIds = new ArrayList<>();
        int currentItemId = 0;
        Drawable currentItemDrawable = null;
        int vecW = 48, vecH = 48;
        float vpW = 24f, vpH = 24f;
        BufferedImage vecImg = null;
        Graphics2D vecG = null;

        while (offset + 8 <= data.length) {
            int chunkType = buf.getShort(offset) & 0xFFFF;
            int chunkSize = buf.getInt(offset + 4);
            if (chunkSize < 8 || offset + chunkSize > data.length) break;

            if (chunkType == 0x0001) {
                pool = ApkResourceTable.parseStringPool(data, offset, chunkSize);
            } else if (chunkType == 0x0103 && offset + 24 <= data.length) {
                int nameIdx = buf.getInt(offset + 20);
                String endTag = (nameIdx >= 0 && nameIdx < pool.size()) ? pool.get(nameIdx) : "";
                if ("layer-list".equals(rootTag) && "item".equals(endTag)) {
                    Drawable itemD = currentItemDrawable != null ? currentItemDrawable : new ColorDrawable(0);
                    layerDrawables.add(itemD);
                    layerIds.add(currentItemId);
                    currentItemId = 0;
                    currentItemDrawable = null;
                    shapeDrawable = null;
                }
            } else if (chunkType == 0x0102 && offset + 36 <= data.length) {
                int nameIdx = buf.getInt(offset + 20);
                String tag = (nameIdx >= 0 && nameIdx < pool.size()) ? pool.get(nameIdx) : "";
                if (rootTag.isEmpty()) rootTag = tag;

                int attrStart = buf.getShort(offset + 24) & 0xFFFF;
                int attrSize = buf.getShort(offset + 26) & 0xFFFF;
                int attrCount = buf.getShort(offset + 28) & 0xFFFF;
                int aOff = offset + 16 + attrStart;

                List<AxmlAttr> attrs = new ArrayList<>(attrCount);
                for (int i = 0; i < attrCount && aOff + 20 <= data.length; i++) {
                    int nsIdx = buf.getInt(aOff);
                    int aNameIdx = buf.getInt(aOff + 4);
                    int aRawVal = buf.getInt(aOff + 8);
                    int aType = data[aOff + 15] & 0xFF;
                    int aData = buf.getInt(aOff + 16);
                    String ns = (nsIdx >= 0 && nsIdx < pool.size()) ? pool.get(nsIdx) : "";
                    String aName = (aNameIdx >= 0 && aNameIdx < pool.size()) ? pool.get(aNameIdx) : "";
                    String sVal = (aRawVal >= 0 && aRawVal < pool.size()) ? pool.get(aRawVal) :
                            (aType == ApkResourceTable.TYPE_STRING && aData >= 0 && aData < pool.size() ? pool.get(aData) : null);
                    attrs.add(new AxmlAttr(ns, aName, 0, aType, aData, sVal));
                    aOff += attrSize;
                }

                if ("layer-list".equals(rootTag) && "item".equals(tag)) {
                    currentItemId = 0;
                    currentItemDrawable = null;
                    shapeDrawable = null;
                    for (AxmlAttr a : attrs) {
                        if ("id".equals(a.name) && a.data != 0) {
                            currentItemId = a.data;
                        } else if ("drawable".equals(a.name) && a.data != 0) {
                            currentItemDrawable = loadDrawableById(a.data, ctx, table);
                        }
                    }
                } else if ("shape".equals(tag)) {
                    shapeDrawable = new GradientDrawable();
                    for (AxmlAttr a : attrs) {
                        if ("shape".equals(a.name)) shapeDrawable.setShape(a.data);
                    }
                    if ("layer-list".equals(rootTag)) {
                        currentItemDrawable = shapeDrawable;
                    }
                } else if (shapeDrawable != null && "solid".equals(tag)) {
                    for (AxmlAttr a : attrs) {
                        if ("color".equals(a.name)) {
                            Integer c = resolveColor(a, table);
                            if (c != null) shapeDrawable.setColor(c);
                        }
                    }
                } else if (shapeDrawable != null && "corners".equals(tag)) {
                    for (AxmlAttr a : attrs) {
                        if ("radius".equals(a.name)) shapeDrawable.setCornerRadius(resolveDimenPx(a, table));
                    }
                } else if (shapeDrawable != null && "stroke".equals(tag)) {
                    int sw = 0; int sc = 0;
                    for (AxmlAttr a : attrs) {
                        if ("width".equals(a.name)) sw = resolveDimenPx(a, table);
                        if ("color".equals(a.name)) { Integer c = resolveColor(a, table); if (c != null) sc = c; }
                    }
                    shapeDrawable.setStroke(sw, sc);
                } else if (shapeDrawable != null && "gradient".equals(tag)) {
                    Integer startC = null, endC = null;
                    for (AxmlAttr a : attrs) {
                        if ("startColor".equals(a.name)) startC = resolveColor(a, table);
                        if ("endColor".equals(a.name)) endC = resolveColor(a, table);
                    }
                    if (startC != null && endC != null) {
                        shapeDrawable.setColors(new int[]{startC, endC});
                    }
                } else if ("vector".equals(tag)) {
                    for (AxmlAttr a : attrs) {
                        if ("viewportWidth".equals(a.name)) vpW = resolveFloat(a);
                        if ("viewportHeight".equals(a.name)) vpH = resolveFloat(a);
                        if ("width".equals(a.name)) vecW = Math.max(24, Math.min(256, resolveDimenPx(a, table)));
                        if ("height".equals(a.name)) vecH = Math.max(24, Math.min(256, resolveDimenPx(a, table)));
                    }
                    vecImg = new BufferedImage(vecW, vecH, BufferedImage.TYPE_INT_ARGB);
                    vecG = vecImg.createGraphics();
                    vecG.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON);
                    vecG.scale(vecW / Math.max(1f, vpW), vecH / Math.max(1f, vpH));
                } else if ("path".equals(tag) && vecG != null) {
                    String pathData = null;
                    Integer fillCol = 0xFF94A3B8;
                    for (AxmlAttr a : attrs) {
                        if ("pathData".equals(a.name)) pathData = resolveString(a, table);
                        if ("fillColor".equals(a.name)) { Integer c = resolveColor(a, table); if (c != null) fillCol = c; }
                    }
                    if (pathData != null && !pathData.isEmpty()) {
                        Path2D p2d = parseSvgPath(pathData);
                        vecG.setColor(new java.awt.Color(fillCol, true));
                        vecG.fill(p2d);
                    }
                }
            }
            offset += chunkSize;
        }

        if ("layer-list".equals(rootTag)) {
            LayerDrawable ld = new LayerDrawable(layerDrawables.toArray(new Drawable[0]));
            for (int idx = 0; idx < layerIds.size(); idx++) {
                int lid = layerIds.get(idx);
                if (lid != 0) {
                    ld.setDrawableByLayerId(lid, layerDrawables.get(idx));
                }
            }
            return ld;
        }
        if (vecG != null) {
            vecG.dispose();
            return new BitmapDrawable(ctx != null ? ctx.getResources() : null, new Bitmap(vecImg));
        }
        return shapeDrawable;
    }

    private static Path2D parseSvgPath(String d) {
        Path2D path = new Path2D.Float();
        try {
            List<String> tokens = new ArrayList<>();
            int i = 0;
            while (i < d.length()) {
                char c = d.charAt(i);
                if (Character.isLetter(c)) {
                    tokens.add(String.valueOf(c));
                    i++;
                } else if (c == '-' || c == '.' || Character.isDigit(c)) {
                    int start = i++;
                    boolean hasDot = (c == '.');
                    while (i < d.length()) {
                        char ch = d.charAt(i);
                        if (Character.isDigit(ch)) i++;
                        else if (ch == '.' && !hasDot) { hasDot = true; i++; }
                        else break;
                    }
                    tokens.add(d.substring(start, i));
                } else {
                    i++;
                }
            }

            float cx = 0, cy = 0, sx = 0, sy = 0;
            char cmd = 'M';
            int t = 0;
            while (t < tokens.size()) {
                String tok = tokens.get(t);
                if (tok.length() == 1 && Character.isLetter(tok.charAt(0))) {
                    cmd = tok.charAt(0);
                    t++;
                    if (cmd == 'Z' || cmd == 'z') {
                        path.closePath();
                        cx = sx; cy = sy;
                    }
                    continue;
                }
                switch (cmd) {
                    case 'M': case 'm':
                        float mx = Float.parseFloat(tokens.get(t++));
                        float my = Float.parseFloat(tokens.get(t++));
                        if (cmd == 'm') { mx += cx; my += cy; }
                        path.moveTo(mx, my);
                        cx = sx = mx; cy = sy = my;
                        cmd = (cmd == 'm') ? 'l' : 'L';
                        break;
                    case 'L': case 'l':
                        float lx = Float.parseFloat(tokens.get(t++));
                        float ly = Float.parseFloat(tokens.get(t++));
                        if (cmd == 'l') { lx += cx; ly += cy; }
                        path.lineTo(lx, ly);
                        cx = lx; cy = ly;
                        break;
                    case 'H': case 'h':
                        float hx = Float.parseFloat(tokens.get(t++));
                        if (cmd == 'h') hx += cx;
                        path.lineTo(hx, cy);
                        cx = hx;
                        break;
                    case 'V': case 'v':
                        float vy = Float.parseFloat(tokens.get(t++));
                        if (cmd == 'v') vy += cy;
                        path.lineTo(cx, vy);
                        cy = vy;
                        break;
                    case 'C': case 'c':
                        float c1x = Float.parseFloat(tokens.get(t++));
                        float c1y = Float.parseFloat(tokens.get(t++));
                        float c2x = Float.parseFloat(tokens.get(t++));
                        float c2y = Float.parseFloat(tokens.get(t++));
                        float ex = Float.parseFloat(tokens.get(t++));
                        float ey = Float.parseFloat(tokens.get(t++));
                        if (cmd == 'c') { c1x += cx; c1y += cy; c2x += cx; c2y += cy; ex += cx; ey += cy; }
                        path.curveTo(c1x, c1y, c2x, c2y, ex, ey);
                        cx = ex; cy = ey;
                        break;
                    case 'Q': case 'q':
                        float qx = Float.parseFloat(tokens.get(t++));
                        float qy = Float.parseFloat(tokens.get(t++));
                        float x2 = Float.parseFloat(tokens.get(t++));
                        float y2 = Float.parseFloat(tokens.get(t++));
                        if (cmd == 'q') { qx += cx; qy += cy; x2 += cx; y2 += cy; }
                        path.quadTo(qx, qy, x2, y2);
                        cx = x2; cy = y2;
                        break;
                    default:
                        t++;
                        break;
                }
            }
        } catch (Throwable ignored) {}
        return path;
    }
}
