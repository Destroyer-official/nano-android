package android.widget;

import android.content.Context;

public abstract class CompoundButton extends Button {
    public interface OnCheckedChangeListener {
        void onCheckedChanged(CompoundButton buttonView, boolean isChecked);
    }

    private boolean checked = false;
    private OnCheckedChangeListener listener;

    public CompoundButton() { super(); setClickable(true); }
    public CompoundButton(Context context) { super(context); setClickable(true); }
    public CompoundButton(Context context, android.util.AttributeSet attrs) { super(context, attrs); setClickable(true); }
    public CompoundButton(Context context, android.util.AttributeSet attrs, int defStyleAttr) { super(context, attrs, defStyleAttr); setClickable(true); }

    public boolean isChecked() { return checked; }

    public void setChecked(boolean checked) {
        if (this.checked != checked) {
            this.checked = checked;
            if (listener != null) {
                listener.onCheckedChanged(this, checked);
            }
        }
    }

    public void toggle() {
        setChecked(!checked);
    }

    @Override
    public boolean performClick() {
        toggle();
        boolean handled = super.performClick();
        return handled || true;
    }

    public void setOnCheckedChangeListener(OnCheckedChangeListener l) {
        this.listener = l;
    }
}
