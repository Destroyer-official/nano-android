package android.view;

import android.graphics.SurfaceTexture;

public class Surface {
    public static final int ROTATION_0 = 0;
    public static final int ROTATION_90 = 1;
    public static final int ROTATION_180 = 2;
    public static final int ROTATION_270 = 3;

    public Surface(SurfaceTexture surfaceTexture) {}
    public void release() {}
}
