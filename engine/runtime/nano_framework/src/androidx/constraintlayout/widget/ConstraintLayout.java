package androidx.constraintlayout.widget;

import android.content.Context;
import android.util.AttributeSet;
import android.widget.RelativeLayout;

public class ConstraintLayout extends RelativeLayout {
    public ConstraintLayout(Context context) { super(context); }
    public ConstraintLayout(Context context, AttributeSet attrs) { super(context, attrs); }
    public ConstraintLayout(Context context, AttributeSet attrs, int defStyleAttr) { super(context, attrs, defStyleAttr); }

    public static class LayoutParams extends RelativeLayout.LayoutParams {
        public static final int MATCH_CONSTRAINT = 0;
        public static final int PARENT_ID = 0;
        public LayoutParams(int width, int height) { super(width, height); }
        public LayoutParams(Context c, AttributeSet attrs) { super(c, attrs); }
    }
}
