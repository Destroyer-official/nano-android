package androidx.cardview.widget;

import android.content.Context;
import android.util.AttributeSet;
import android.widget.FrameLayout;

public class CardView extends FrameLayout {
    private float radius = 16f;
    private float cardElevation = 4f;
    private int cardBackgroundColor = 0xFFFFFFFF;

    public CardView(Context context) { super(context); }
    public CardView(Context context, AttributeSet attrs) { super(context, attrs); }
    public CardView(Context context, AttributeSet attrs, int defStyleAttr) { super(context, attrs, defStyleAttr); }

    public void setRadius(float radius) { this.radius = radius; }
    public float getRadius() { return radius; }
    public void setCardElevation(float elevation) { this.cardElevation = elevation; }
    public float getCardElevation() { return cardElevation; }
    public void setCardBackgroundColor(int color) { this.cardBackgroundColor = color; setBackgroundColor(color); }
    public int getCardBackgroundColorInt() { return cardBackgroundColor; }
}
