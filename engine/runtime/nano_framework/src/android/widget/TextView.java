package android.widget;

import android.content.Context;
import android.graphics.Color;
import android.graphics.Typeface;
import android.text.InputFilter;
import android.util.DisplayMetrics;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import java.awt.Font;
import java.awt.FontMetrics;
import java.awt.Graphics2D;
import java.awt.image.BufferedImage;
import java.util.ArrayList;
import java.util.List;

public class TextView extends View {
    private static final Graphics2D MEASURE_G = new BufferedImage(1, 1, BufferedImage.TYPE_INT_ARGB).createGraphics();

    private int textColor = Color.BLACK;
    private float textSizePx = 36.75f; // default ~14sp at 2.625 density
    private Typeface typeface = Typeface.DEFAULT;
    private int gravity = Gravity.TOP | Gravity.LEFT;
    private InputFilter[] filters = new InputFilter[0];
    private CharSequence hint = "";
    private int hintTextColor = Color.GRAY;
    private int inputType = 0;
    private int imeOptions = 0;
    private int maxLines = Integer.MAX_VALUE;
    private int minLines = 0;
    private final List<android.text.TextWatcher> textWatchers = new ArrayList<>();
    private OnEditorActionListener editorActionListener;

    public interface OnEditorActionListener {
        boolean onEditorAction(TextView v, int actionId, android.view.KeyEvent event);
    }

    public TextView() { super(); }
    public TextView(Context context) { super(context); initDensitySize(); }
    public TextView(Context context, android.util.AttributeSet attrs) { super(context, attrs); initDensitySize(); }
    public TextView(Context context, android.util.AttributeSet attrs, int defStyleAttr) { super(context, attrs, defStyleAttr); initDensitySize(); }

    private void initDensitySize() {
        DisplayMetrics m = (getContext() != null && getContext().getResources() != null)
                ? getContext().getResources().getDisplayMetrics() : new DisplayMetrics();
        this.textSizePx = 14.0f * m.scaledDensity;
    }

    public void setText(CharSequence text) {
        String oldStr = super.getText() != null ? super.getText().toString() : "";
        String newStr = text != null ? text.toString() : "";
        if (!textWatchers.isEmpty()) {
            for (android.text.TextWatcher w : new ArrayList<>(textWatchers)) {
                try { w.beforeTextChanged(oldStr, 0, oldStr.length(), newStr.length()); } catch (Throwable ignored) {}
            }
        }
        super.setText(newStr);
        notifyAfterTextChanged(oldStr, newStr);
    }

    protected final void notifyAfterTextChanged(String oldStr, String newStr) {
        if (textWatchers.isEmpty()) return;
        android.text.Editable ed = (this instanceof EditText)
                ? ((EditText) this).getText()
                : new android.text.SpannableStringBuilder(newStr);
        for (android.text.TextWatcher w : new ArrayList<>(textWatchers)) {
            try { w.onTextChanged(newStr, 0, oldStr.length(), newStr.length()); } catch (Throwable ignored) {}
            try { w.afterTextChanged(ed); } catch (Throwable ignored) {}
        }
    }

    public void setOnEditorActionListener(OnEditorActionListener l) {
        this.editorActionListener = l;
    }

    public boolean triggerEditorAction(int actionId) {
        if (editorActionListener != null) {
            return editorActionListener.onEditorAction(this, actionId, new android.view.KeyEvent(android.view.KeyEvent.ACTION_DOWN, android.view.KeyEvent.KEYCODE_ENTER));
        }
        return false;
    }

    public void setText(int resid) {
        String s = getResources().getString(resid);
        super.setText(s != null && !s.isEmpty() ? s : "res_" + resid);
    }

    public CharSequence getText() {
        return super.getText();
    }

    public void append(CharSequence text) {
        String current = getText() != null ? getText().toString() : "";
        setText(current + (text != null ? text.toString() : ""));
    }

    public void append(CharSequence text, int start, int end) {
        if (text != null) {
            append(text.subSequence(start, end));
        }
    }

    public int length() {
        CharSequence t = getText();
        return t != null ? t.length() : 0;
    }

    public void setTextColor(int color) { this.textColor = color; }
    public int getCurrentTextColor() { return textColor; }

    public void setTextSize(float sizeSp) {
        setTextSize(TypedValue.COMPLEX_UNIT_SP, sizeSp);
    }

    public void setTextSize(int unit, float size) {
        DisplayMetrics m = (getContext() != null && getContext().getResources() != null)
                ? getContext().getResources().getDisplayMetrics() : new DisplayMetrics();
        this.textSizePx = TypedValue.applyDimension(unit, size, m);
    }

    public float getTextSize() { return textSizePx; }

    public void setTypeface(Typeface tf) {
        this.typeface = tf != null ? tf : Typeface.DEFAULT;
    }
    public void setTypeface(Typeface tf, int style) {
        this.typeface = Typeface.create(tf, style);
    }
    public Typeface getTypeface() { return typeface; }

    public void setGravity(int gravity) { this.gravity = gravity; }
    public int getGravity() { return gravity; }

    public void setCompoundDrawablesWithIntrinsicBounds(android.graphics.drawable.Drawable left, android.graphics.drawable.Drawable top, android.graphics.drawable.Drawable right, android.graphics.drawable.Drawable bottom) {}
    public void setCompoundDrawablesWithIntrinsicBounds(int left, int top, int right, int bottom) {}
    public void setCompoundDrawablePadding(int pad) {}

    public void setLineSpacing(float add, float mult) {}
    public void setLetterSpacing(float letterSpacing) {}
    public void setSingleLine(boolean singleLine) { if (singleLine) this.maxLines = 1; }
    public void setMaxLines(int maxLines) { this.maxLines = maxLines; }
    public void setMinLines(int minLines) { this.minLines = minLines; }
    public void setLines(int lines) { this.minLines = lines; this.maxLines = lines; }
    public void setEllipsize(Object where) {}
    public void setEllipsize(android.text.TextUtils.TruncateAt where) {}
    public android.text.TextUtils.TruncateAt getEllipsize() { return android.text.TextUtils.TruncateAt.END; }
    public void setAllCaps(boolean allCaps) {}
    public void addTextChangedListener(android.text.TextWatcher watcher) {
        if (watcher != null && !textWatchers.contains(watcher)) {
            textWatchers.add(watcher);
        }
    }
    public void removeTextChangedListener(android.text.TextWatcher watcher) {
        textWatchers.remove(watcher);
    }

    public void setHintTextColor(int color) { this.hintTextColor = color; }
    public int getHintTextColor() { return hintTextColor; }

    public void setInputType(int type) { this.inputType = type; }
    public int getInputType() { return inputType; }

    public void setImeOptions(int options) { this.imeOptions = options; }
    public int getImeOptions() { return imeOptions; }

    public void setHint(CharSequence hint) { this.hint = hint != null ? hint : ""; }
    public void setHint(int resid) {
        String s = getResources().getString(resid);
        this.hint = s != null ? s : "";
    }
    public CharSequence getHint() { return hint; }

    public void setFilters(InputFilter[] filters) {
        this.filters = filters != null ? filters : new InputFilter[0];
    }
    public InputFilter[] getFilters() { return filters; }

    public Font createAwtFont() {
        String fam = typeface != null ? typeface.getFamilyName() : "SansSerif";
        int st = typeface != null ? typeface.getStyle() : 0;
        int awtStyle = Font.PLAIN;
        if ((st & Typeface.BOLD) != 0 && (st & Typeface.ITALIC) != 0) awtStyle = Font.BOLD | Font.ITALIC;
        else if ((st & Typeface.BOLD) != 0) awtStyle = Font.BOLD;
        else if ((st & Typeface.ITALIC) != 0) awtStyle = Font.ITALIC;
        int px = Math.max(12, Math.round(textSizePx));
        return new Font(fam, awtStyle, px);
    }

    public static List<String> wrapLines(String raw, FontMetrics fm, int maxWidth) {
        List<String> out = new ArrayList<>();
        if (raw == null || raw.isEmpty()) return out;
        String[] paragraphs = raw.split("\n", -1);
        int limit = Math.max(40, maxWidth);
        for (String p : paragraphs) {
            if (p.isEmpty()) {
                out.add("");
                continue;
            }
            if (fm.stringWidth(p) <= limit) {
                out.add(p);
                continue;
            }
            String[] words = p.split("\\s+");
            StringBuilder line = new StringBuilder();
            for (String w : words) {
                if (line.length() == 0) {
                    line.append(w);
                } else if (fm.stringWidth(line + " " + w) <= limit) {
                    line.append(' ').append(w);
                } else {
                    out.add(line.toString());
                    line = new StringBuilder(w);
                }
            }
            if (line.length() > 0) {
                out.add(line.toString());
            }
        }
        return out;
    }

    @Override
    protected void onMeasure(int widthMeasureSpec, int heightMeasureSpec) {
        int wMode = MeasureSpec.getMode(widthMeasureSpec);
        int wSize = MeasureSpec.getSize(widthMeasureSpec);
        int hMode = MeasureSpec.getMode(heightMeasureSpec);
        int hSize = MeasureSpec.getSize(heightMeasureSpec);

        int padH = getPaddingLeft() + getPaddingRight();
        int padV = getPaddingTop() + getPaddingBottom();

        Font font = createAwtFont();
        FontMetrics fm;
        synchronized (MEASURE_G) {
            fm = MEASURE_G.getFontMetrics(font);
        }

        CharSequence cs = getText();
        String txt = (cs != null && cs.length() > 0) ? cs.toString() : (hint != null ? hint.toString() : "");

        int width;
        if (wMode == MeasureSpec.EXACTLY) {
            width = wSize;
        } else {
            int maxLineW = 0;
            if (!txt.isEmpty()) {
                for (String p : txt.split("\n", -1)) {
                    maxLineW = Math.max(maxLineW, fm.stringWidth(p));
                }
            } else {
                maxLineW = (this instanceof Button) ? 88 : 10;
            }
            width = maxLineW + padH + 6;
            if (this instanceof Switch) {
                width += 130;
            } else if (this instanceof CheckBox || this instanceof RadioButton) {
                width += 64;
            }
            if (this instanceof Button) {
                width = Math.max(width, 120);
            }
            if (wMode == MeasureSpec.AT_MOST) {
                width = Math.min(width, wSize);
            }
        }

        int height;
        if (hMode == MeasureSpec.EXACTLY) {
            height = hSize;
        } else {
            int availW = Math.max(40, width - padH);
            if (this instanceof Switch) availW = Math.max(40, availW - 130);
            List<String> lines = txt.isEmpty() ? java.util.Collections.singletonList(" ") : wrapLines(txt, fm, availW);
            int lineCount = Math.min(maxLines, Math.max(minLines, Math.max(1, lines.size())));
            int textH = lineCount * Math.max(20, fm.getHeight());
            height = textH + padV + 4;
            if (this instanceof Button && !(this instanceof CompoundButton)) {
                height = Math.max(height, 88);
            } else if (this instanceof Switch || this instanceof CheckBox || this instanceof RadioButton) {
                height = Math.max(height, 76);
            } else if (this instanceof EditText) {
                height = Math.max(height, 96);
            }
            if (hMode == MeasureSpec.AT_MOST) {
                height = Math.min(height, hSize);
            }
        }

        setMeasuredDimension(Math.max(0, width), Math.max(0, height));
    }

    public void setLinkTextColor(int color) {}
    public void setLinkTextColor(android.content.res.ColorStateList colors) {}
}
