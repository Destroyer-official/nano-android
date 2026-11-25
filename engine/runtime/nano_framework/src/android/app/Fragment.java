package android.app;

import android.content.Context;
import android.os.Bundle;
import android.view.LayoutInflater;
import android.view.View;
import android.view.ViewGroup;

public class Fragment {
    private Activity activity;
    private View view;

    public Activity getActivity() { return activity; }
    public void setActivity(Activity a) { this.activity = a; }
    public Context getContext() { return activity; }
    public View getView() { return view; }
    public void setView(View v) { this.view = v; }
    public android.content.res.Resources getResources() { return activity != null ? activity.getResources() : new android.content.res.Resources(); }
    public String getString(int resId) { return getResources().getString(resId); }
    public void onAttach(Context context) { if (context instanceof Activity) this.activity = (Activity) context; }
    public void onCreate(Bundle savedInstanceState) {}
    public View onCreateView(LayoutInflater inflater, ViewGroup container, Bundle savedInstanceState) { return null; }
    public void onViewCreated(View view, Bundle savedInstanceState) {}
    public void onStart() {}
    public void onResume() {}
    public void onPause() {}
    public void onStop() {}
    public void onDestroy() {}
}
