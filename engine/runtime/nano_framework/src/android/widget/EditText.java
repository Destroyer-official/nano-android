package android.widget;

import android.content.Context;
import android.text.Editable;
import android.text.SpannableStringBuilder;

public class EditText extends TextView {
    private Editable editableText = new SpannableStringBuilder("");

    public EditText() { super(); }
    public EditText(Context context) { super(context); }
    public EditText(Context context, android.util.AttributeSet attrs) { super(context, attrs); }
    public EditText(Context context, android.util.AttributeSet attrs, int defStyleAttr) { super(context, attrs, defStyleAttr); }

    @Override
    public Editable getText() {
        if (editableText == null) {
            editableText = new SpannableStringBuilder(super.getText() != null ? super.getText().toString() : "");
        }
        return editableText;
    }

    @Override
    public void setText(CharSequence text) {
        String newStr = text != null ? text.toString() : "";
        if (text instanceof Editable) {
            this.editableText = (Editable) text;
        } else {
            this.editableText = new SpannableStringBuilder(newStr);
        }
        super.setText(newStr);
    }

    public void setSelection(int index) {}
    public void setSelection(int start, int stop) {}
    public void selectAll() {}
}
