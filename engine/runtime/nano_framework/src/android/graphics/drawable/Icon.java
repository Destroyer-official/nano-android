package android.graphics.drawable;

import android.graphics.Bitmap;

public final class Icon {
    private Bitmap bitmap;

    private Icon() {}

    public static Icon createWithBitmap(Bitmap bitmap) {
        Icon icon = new Icon();
        icon.bitmap = bitmap;
        return icon;
    }

    public static Icon createWithResource(String resPackage, int resId) {
        return new Icon();
    }
}
