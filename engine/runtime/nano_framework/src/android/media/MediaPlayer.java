package android.media;

import android.content.Context;
import android.net.Uri;
import android.view.Surface;

public class MediaPlayer {
    public interface OnCompletionListener { void onCompletion(MediaPlayer mp); }
    public interface OnErrorListener { boolean onError(MediaPlayer mp, int what, int extra); }
    public interface OnPreparedListener { void onPrepared(MediaPlayer mp); }

    public MediaPlayer() {}
    public void setDataSource(Context context, Uri uri) {}
    public void setDataSource(String path) {}
    public void setSurface(Surface surface) {}
    public void prepareAsync() {}
    public void prepare() {}
    public void start() {}
    public void pause() {}
    public void stop() {}
    public void release() {}
    public void reset() {}
    public boolean isPlaying() { return false; }
    public int getDuration() { return 120000; }
    public int getCurrentPosition() { return 0; }
    public void seekTo(int msec) {}
    public void setLooping(boolean looping) {}
    public void setVolume(float left, float right) {}
    public void setOnCompletionListener(OnCompletionListener l) {}
    public void setOnErrorListener(OnErrorListener l) {}
    public void setOnPreparedListener(OnPreparedListener l) {}
}
