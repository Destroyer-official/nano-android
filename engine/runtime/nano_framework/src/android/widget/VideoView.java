package android.widget;

import android.content.Context;
import android.net.Uri;
import android.view.SurfaceView;

public class VideoView extends SurfaceView {
    public VideoView(Context context) { super(context); }
    public void setVideoURI(Uri uri) {}
    public void setVideoPath(String path) {}
    public void start() {}
    public void pause() {}
    public void stopPlayback() {}
    public boolean isPlaying() { return false; }
    public int getDuration() { return 120000; }
    public int getCurrentPosition() { return 0; }
    public void seekTo(int msec) {}
}
