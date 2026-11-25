package android.app;

import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.view.View;

public class Activity {
    public enum State {
        UNINITIALIZED,
        CREATED,
        STARTED,
        RESUMED,
        PAUSED,
        STOPPED,
        DESTROYED
    }

    private Context context;
    private Intent intent;
    private State state = State.UNINITIALIZED;
    private boolean isFinishing = false;
    private View contentView;
    private final Handler mainHandler = new Handler(Looper.getMainLooper());

    public Activity() {}

    public synchronized void attach(Context context, Intent intent) {
        this.context = context;
        this.intent = intent;
    }

    protected void onCreate(Bundle savedInstanceState) {
        this.state = State.CREATED;
    }

    protected void onStart() {
        this.state = State.STARTED;
    }

    protected void onResume() {
        this.state = State.RESUMED;
    }

    protected void onPause() {
        this.state = State.PAUSED;
    }

    protected void onStop() {
        this.state = State.STOPPED;
    }

    protected void onDestroy() {
        this.state = State.DESTROYED;
    }

    public void runOnUiThread(Runnable action) {
        if (Thread.currentThread() == Looper.getMainLooper().getThread()) {
            action.run();
        } else {
            mainHandler.post(action);
        }
    }

    public synchronized void finish() {
        this.isFinishing = true;
    }

    public synchronized boolean isFinishing() {
        return this.isFinishing;
    }

    public synchronized State getState() {
        return this.state;
    }

    public synchronized Intent getIntent() {
        return this.intent;
    }

    public synchronized void setIntent(Intent intent) {
        this.intent = intent;
    }

    public synchronized Context getApplicationContext() {
        return this.context;
    }

    public synchronized String getPackageName() {
        return context != null ? context.getPackageName() : "com.example.app";
    }

    public synchronized SharedPreferences getSharedPreferences(String name, int mode) {
        return context != null ? context.getSharedPreferences(name, mode) : null;
    }

    public synchronized void setContentView(View view) {
        this.contentView = view;
    }

    public synchronized View getContentView() {
        return this.contentView;
    }

    public synchronized View findViewById(int id) {
        return contentView != null ? contentView.findViewById(id) : null;
    }
}
