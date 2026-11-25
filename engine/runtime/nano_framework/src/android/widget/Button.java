package android.widget;

import android.content.Context;

public class Button extends TextView {
    public Button() { super(); setClickable(true); }
    public Button(Context context) { super(context); setClickable(true); }
    public Button(Context context, android.util.AttributeSet attrs) { super(context, attrs); setClickable(true); }
    public Button(Context context, android.util.AttributeSet attrs, int defStyleAttr) { super(context, attrs, defStyleAttr); setClickable(true); }
}
