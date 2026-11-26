package androidx.appcompat.widget;

import android.content.Context;
import android.util.AttributeSet;
import android.widget.LinearLayout;
import android.widget.TextView;

public class Toolbar extends LinearLayout {
    private CharSequence title = "";
    private CharSequence subtitle = "";
    private TextView titleView;

    public Toolbar(Context context) {
        super(context);
        init(context);
    }

    public Toolbar(Context context, AttributeSet attrs) {
        super(context, attrs);
        init(context);
    }

    public Toolbar(Context context, AttributeSet attrs, int defStyleAttr) {
        super(context, attrs, defStyleAttr);
        init(context);
    }

    private void init(Context context) {
        setOrientation(HORIZONTAL);
        titleView = new TextView(context);
        titleView.setTextSize(18f);
        addView(titleView);
    }

    public void setTitle(CharSequence title) {
        this.title = title != null ? title : "";
        if (titleView != null) titleView.setText(this.title);
    }

    public CharSequence getTitle() { return title; }
    public void setSubtitle(CharSequence subtitle) { this.subtitle = subtitle != null ? subtitle : ""; }
    public CharSequence getSubtitle() { return subtitle; }
}
