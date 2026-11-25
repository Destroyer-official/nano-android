package android.app;

import android.content.Context;
import android.content.DialogInterface;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.widget.Button;
import android.widget.CheckBox;
import android.widget.FrameLayout;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import java.util.HashMap;
import java.util.Map;

public class AlertDialog extends Dialog implements DialogInterface {
    public static final int THEME_TRADITIONAL = 1;
    public static final int THEME_HOLO_DARK = 2;
    public static final int THEME_HOLO_LIGHT = 3;
    public static final int THEME_DEVICE_DEFAULT_DARK = 4;
    public static final int THEME_DEVICE_DEFAULT_LIGHT = 5;

    private CharSequence message;
    private View customView;
    private View customTitleView;
    private CharSequence[] items;
    private DialogInterface.OnClickListener itemsListener;
    private DialogInterface.OnMultiChoiceClickListener multiChoiceListener;
    private boolean[] checkedItems;
    private int checkedItem = -1;
    private boolean isSingleChoice = false;
    private boolean isMultiChoice = false;

    private final Map<Integer, CharSequence> buttonTexts = new HashMap<>();
    private final Map<Integer, DialogInterface.OnClickListener> buttonListeners = new HashMap<>();
    private final Map<Integer, Button> buttonViews = new HashMap<>();

    public AlertDialog(Context context) {
        super(context);
    }

    protected AlertDialog(Context context, int themeResId) {
        super(context, themeResId);
    }

    protected AlertDialog(Context context, boolean cancelable, DialogInterface.OnCancelListener cancelListener) {
        super(context);
        setCancelable(cancelable);
        setOnCancelListener(cancelListener);
    }

    @Override
    public void setTitle(CharSequence title) {
        super.setTitle(title);
    }

    public void setCustomTitle(View customTitleView) {
        this.customTitleView = customTitleView;
    }

    public void setMessage(CharSequence message) {
        this.message = message;
    }

    public void setView(View view) {
        this.customView = view;
    }

    public void setButton(int whichButton, CharSequence text, DialogInterface.OnClickListener listener) {
        buttonTexts.put(whichButton, text);
        buttonListeners.put(whichButton, listener);
    }

    public void setButton(CharSequence text, DialogInterface.OnClickListener listener) {
        setButton(BUTTON_POSITIVE, text, listener);
    }

    public void setButton2(CharSequence text, DialogInterface.OnClickListener listener) {
        setButton(BUTTON_NEGATIVE, text, listener);
    }

    public void setButton3(CharSequence text, DialogInterface.OnClickListener listener) {
        setButton(BUTTON_NEUTRAL, text, listener);
    }

    public Button getButton(int whichButton) {
        return buttonViews.get(whichButton);
    }

    private void rebuildContentView() {
        Context ctx = getContext();
        LinearLayout card = new LinearLayout(ctx);
        card.setOrientation(LinearLayout.VERTICAL);
        card.setPadding(42, 36, 42, 30);
        GradientDrawable bg = new GradientDrawable();
        bg.setColor(0xFF162032);
        bg.setCornerRadius(28f);
        bg.setStroke(2, 0xFF334155);
        card.setBackground(bg);
        LinearLayout.LayoutParams cardLp = new LinearLayout.LayoutParams(900, ViewGroup.LayoutParams.WRAP_CONTENT);
        card.setLayoutParams(cardLp);

        if (customTitleView != null) {
            card.addView(customTitleView);
        } else if (getTitle() != null && getTitle().length() > 0) {
            TextView tvTitle = new TextView(ctx);
            tvTitle.setText(getTitle());
            tvTitle.setTextColor(0xFFF8FAFC);
            tvTitle.setTextSize(19f);
            tvTitle.setTypeface(Typeface.DEFAULT_BOLD);
            LinearLayout.LayoutParams tLp = new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
            tLp.bottomMargin = 18;
            card.addView(tvTitle, tLp);
        }

        if (message != null && message.length() > 0) {
            TextView tvMsg = new TextView(ctx);
            tvMsg.setText(message);
            tvMsg.setTextColor(0xFFCBD5E1);
            tvMsg.setTextSize(14f);
            LinearLayout.LayoutParams mLp = new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
            mLp.bottomMargin = 20;
            card.addView(tvMsg, mLp);
        }

        if (items != null && items.length > 0) {
            ScrollView sv = new ScrollView(ctx);
            LinearLayout list = new LinearLayout(ctx);
            list.setOrientation(LinearLayout.VERTICAL);
            for (int i = 0; i < items.length; i++) {
                final int idx = i;
                CharSequence label = items[i] != null ? items[i] : "";
                if (isMultiChoice) {
                    CheckBox cb = new CheckBox(ctx);
                    cb.setText(label);
                    cb.setTextColor(0xFFE2E8F0);
                    cb.setTextSize(14f);
                    boolean initChecked = (checkedItems != null && i < checkedItems.length) && checkedItems[i];
                    cb.setChecked(initChecked);
                    cb.setPadding(16, 14, 16, 14);
                    cb.setOnClickListener(v -> {
                        boolean newVal = cb.isChecked();
                        if (checkedItems != null && idx < checkedItems.length) checkedItems[idx] = newVal;
                        if (multiChoiceListener != null) multiChoiceListener.onClick(AlertDialog.this, idx, newVal);
                    });
                    list.addView(cb, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));
                } else {
                    Button rowBtn = new Button(ctx);
                    String prefix = (isSingleChoice && idx == checkedItem) ? "● " : (isSingleChoice ? "○ " : "");
                    rowBtn.setText(prefix + label);
                    rowBtn.setTextColor(0xFFF8FAFC);
                    rowBtn.setTextSize(14f);
                    rowBtn.setGravity(Gravity.LEFT | Gravity.CENTER_VERTICAL);
                    GradientDrawable rowBg = new GradientDrawable();
                    rowBg.setColor((isSingleChoice && idx == checkedItem) ? 0xFF1E3A5F : 0xFF0F172A);
                    rowBg.setCornerRadius(14f);
                    rowBtn.setBackground(rowBg);
                    rowBtn.setPadding(24, 18, 24, 18);
                    LinearLayout.LayoutParams rLp = new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
                    rLp.bottomMargin = 10;
                    rowBtn.setOnClickListener(v -> {
                        if (!isSingleChoice) {
                            dismiss();
                        } else {
                            checkedItem = idx;
                            rebuildContentView();
                        }
                        if (itemsListener != null) {
                            itemsListener.onClick(AlertDialog.this, idx);
                        }
                    });
                    list.addView(rowBtn, rLp);
                }
            }
            sv.addView(list, new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));
            LinearLayout.LayoutParams svLp = new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, Math.min(900, items.length * 115 + 30));
            svLp.bottomMargin = 16;
            card.addView(sv, svLp);
        }

        if (customView != null) {
            if (customView.getParent() instanceof ViewGroup) {
                ((ViewGroup) customView.getParent()).removeView(customView);
            }
            LinearLayout.LayoutParams cvLp = new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
            cvLp.bottomMargin = 18;
            card.addView(customView, cvLp);
        }

        if (!buttonTexts.isEmpty()) {
            LinearLayout bar = new LinearLayout(ctx);
            bar.setOrientation(LinearLayout.HORIZONTAL);
            bar.setGravity(Gravity.RIGHT | Gravity.CENTER_VERTICAL);
            int[] order = new int[]{BUTTON_NEUTRAL, BUTTON_NEGATIVE, BUTTON_POSITIVE};
            for (int which : order) {
                CharSequence label = buttonTexts.get(which);
                if (label == null) continue;
                Button b = new Button(ctx);
                b.setText(label);
                b.setTextSize(13f);
                b.setTypeface(Typeface.DEFAULT_BOLD);
                b.setTextColor(which == BUTTON_POSITIVE ? 0xFF0F172A : 0xFF38BDF8);
                GradientDrawable btnBg = new GradientDrawable();
                btnBg.setColor(which == BUTTON_POSITIVE ? 0xFF38BDF8 : 0xFF1E293B);
                btnBg.setCornerRadius(14f);
                b.setBackground(btnBg);
                b.setPadding(24, 14, 24, 14);
                final DialogInterface.OnClickListener listener = buttonListeners.get(which);
                b.setOnClickListener(v -> {
                    if (listener != null) listener.onClick(AlertDialog.this, which);
                    dismiss();
                });
                buttonViews.put(which, b);
                LinearLayout.LayoutParams bLp = new LinearLayout.LayoutParams(ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
                bLp.leftMargin = 14;
                bar.addView(b, bLp);
            }
            card.addView(bar, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));
        }

        FrameLayout wrapper = new FrameLayout(ctx);
        FrameLayout.LayoutParams wrapLp = new FrameLayout.LayoutParams(ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        wrapLp.gravity = Gravity.CENTER;
        wrapper.addView(card, wrapLp);
        setContentView(wrapper);
    }

    @Override
    public void show() {
        if (getWindow().getDecorView() == null) {
            rebuildContentView();
        }
        super.show();
    }

    public static class Builder {
        private final Context context;
        private CharSequence title;
        private View customTitleView;
        private CharSequence message;
        private View view;
        private boolean cancelable = true;
        private DialogInterface.OnCancelListener onCancelListener;
        private DialogInterface.OnDismissListener onDismissListener;
        private CharSequence positiveText;
        private DialogInterface.OnClickListener positiveListener;
        private CharSequence negativeText;
        private DialogInterface.OnClickListener negativeListener;
        private CharSequence neutralText;
        private DialogInterface.OnClickListener neutralListener;
        private CharSequence[] items;
        private DialogInterface.OnClickListener itemsListener;
        private boolean isSingleChoice = false;
        private int checkedItem = -1;
        private boolean isMultiChoice = false;
        private boolean[] checkedItems;
        private DialogInterface.OnMultiChoiceClickListener multiChoiceListener;

        public Builder(Context context) { this.context = context; }
        public Builder(Context context, int themeResId) { this.context = context; }

        public Context getContext() { return context; }

        public Builder setTitle(int titleId) {
            if (context != null) this.title = context.getString(titleId);
            return this;
        }
        public Builder setTitle(CharSequence title) { this.title = title; return this; }
        public Builder setCustomTitle(View customTitleView) { this.customTitleView = customTitleView; return this; }

        public Builder setMessage(int messageId) {
            if (context != null) this.message = context.getString(messageId);
            return this;
        }
        public Builder setMessage(CharSequence message) { this.message = message; return this; }

        public Builder setIcon(int iconId) { return this; }
        public Builder setIcon(android.graphics.drawable.Drawable icon) { return this; }
        public Builder setCancelable(boolean cancelable) { this.cancelable = cancelable; return this; }
        public Builder setOnCancelListener(DialogInterface.OnCancelListener listener) { this.onCancelListener = listener; return this; }
        public Builder setOnDismissListener(DialogInterface.OnDismissListener listener) { this.onDismissListener = listener; return this; }
        public Builder setOnKeyListener(DialogInterface.OnKeyListener listener) { return this; }

        public Builder setPositiveButton(int textId, DialogInterface.OnClickListener listener) {
            return setPositiveButton(context != null ? context.getString(textId) : "OK", listener);
        }
        public Builder setPositiveButton(CharSequence text, DialogInterface.OnClickListener listener) {
            this.positiveText = text;
            this.positiveListener = listener;
            return this;
        }

        public Builder setNegativeButton(int textId, DialogInterface.OnClickListener listener) {
            return setNegativeButton(context != null ? context.getString(textId) : "Cancel", listener);
        }
        public Builder setNegativeButton(CharSequence text, DialogInterface.OnClickListener listener) {
            this.negativeText = text;
            this.negativeListener = listener;
            return this;
        }

        public Builder setNeutralButton(int textId, DialogInterface.OnClickListener listener) {
            return setNeutralButton(context != null ? context.getString(textId) : "Neutral", listener);
        }
        public Builder setNeutralButton(CharSequence text, DialogInterface.OnClickListener listener) {
            this.neutralText = text;
            this.neutralListener = listener;
            return this;
        }

        public Builder setItems(CharSequence[] items, DialogInterface.OnClickListener listener) {
            this.items = items;
            this.itemsListener = listener;
            return this;
        }

        public Builder setItems(int itemsId, DialogInterface.OnClickListener listener) {
            this.items = context != null ? context.getResources().getStringArray(itemsId) : new CharSequence[0];
            this.itemsListener = listener;
            return this;
        }

        public Builder setSingleChoiceItems(CharSequence[] items, int checkedItem, DialogInterface.OnClickListener listener) {
            this.items = items;
            this.checkedItem = checkedItem;
            this.itemsListener = listener;
            this.isSingleChoice = true;
            return this;
        }

        public Builder setMultiChoiceItems(CharSequence[] items, boolean[] checkedItems, DialogInterface.OnMultiChoiceClickListener listener) {
            this.items = items;
            this.checkedItems = checkedItems;
            this.multiChoiceListener = listener;
            this.isMultiChoice = true;
            return this;
        }

        public Builder setView(View view) { this.view = view; return this; }
        public Builder setView(int layoutResId) {
            if (context != null) {
                this.view = android.view.LayoutInflater.from(context).inflate(layoutResId, null);
            }
            return this;
        }

        public AlertDialog create() {
            AlertDialog dialog = new AlertDialog(context);
            if (title != null) dialog.setTitle(title);
            if (customTitleView != null) dialog.setCustomTitle(customTitleView);
            if (message != null) dialog.setMessage(message);
            if (view != null) dialog.setView(view);
            dialog.setCancelable(cancelable);
            dialog.setOnCancelListener(onCancelListener);
            dialog.setOnDismissListener(onDismissListener);
            dialog.items = items;
            dialog.itemsListener = itemsListener;
            dialog.isSingleChoice = isSingleChoice;
            dialog.checkedItem = checkedItem;
            dialog.isMultiChoice = isMultiChoice;
            dialog.checkedItems = checkedItems;
            dialog.multiChoiceListener = multiChoiceListener;
            if (positiveText != null) dialog.setButton(BUTTON_POSITIVE, positiveText, positiveListener);
            if (negativeText != null) dialog.setButton(BUTTON_NEGATIVE, negativeText, negativeListener);
            if (neutralText != null) dialog.setButton(BUTTON_NEUTRAL, neutralText, neutralListener);
            dialog.rebuildContentView();
            return dialog;
        }

        public AlertDialog show() {
            AlertDialog dialog = create();
            dialog.show();
            return dialog;
        }
    }
}
