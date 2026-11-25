package android.graphics.drawable;

import android.content.res.Resources;
import android.graphics.Bitmap;

public class BitmapDrawable extends Drawable {
    private Bitmap bitmap;
    public BitmapDrawable(Resources res, Bitmap bitmap) { this.bitmap = bitmap; }
    public Bitmap getBitmap() { return bitmap; }
}
