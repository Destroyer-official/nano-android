package android.view;

import android.content.Context;
import android.graphics.SurfaceTexture;

public class TextureView extends View {
    public interface SurfaceTextureListener {
        void onSurfaceTextureAvailable(SurfaceTexture surface, int width, int height);
        void onSurfaceTextureSizeChanged(SurfaceTexture surface, int width, int height);
        boolean onSurfaceTextureDestroyed(SurfaceTexture surface);
        void onSurfaceTextureUpdated(SurfaceTexture surface);
    }

    private SurfaceTextureListener listener;

    public TextureView(Context context) { super(context); }
    public void setSurfaceTextureListener(SurfaceTextureListener listener) { this.listener = listener; }
    public SurfaceTexture getSurfaceTexture() { return new SurfaceTexture(0); }
    public boolean isAvailable() { return true; }
    public void setTransform(android.graphics.Matrix transform) {}
    public android.graphics.Matrix getTransform(android.graphics.Matrix transform) { return transform != null ? transform : new android.graphics.Matrix(); }
}
