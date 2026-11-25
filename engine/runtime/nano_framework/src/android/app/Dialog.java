package android.app;

import android.content.Context;
import android.content.DialogInterface;
import android.view.View;
import android.view.Window;
import java.util.Collections;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;

public class Dialog implements DialogInterface {
    private static final List<Dialog> ACTIVE_DIALOGS = new CopyOnWriteArrayList<>();

    private final Context context;
    private final Window window = new Window();
    private boolean isShowing = false;
    private boolean cancelable = true;
    private boolean canceledOnTouchOutside = true;
    private CharSequence title = "";
    private DialogInterface.OnDismissListener onDismissListener;
    private DialogInterface.OnCancelListener onCancelListener;
    private DialogInterface.OnShowListener onShowListener;
    private DialogInterface.OnKeyListener onKeyListener;

    public Dialog(Context context) { this.context = context; }
    public Dialog(Context context, int themeResId) { this.context = context; }

    public Context getContext() { return context; }
    public Window getWindow() { return window; }
    public android.view.LayoutInflater getLayoutInflater() {
        return context != null ? android.view.LayoutInflater.from(context) : null;
    }
    public final boolean requestWindowFeature(int featureId) { return window.requestFeature(featureId); }
    public void create() {}
    public void hide() { dismiss(); }

    private Activity ownerActivity;
    public final void setOwnerActivity(Activity activity) { this.ownerActivity = activity; }
    public final Activity getOwnerActivity() { return ownerActivity; }

    public void setTitle(CharSequence title) { this.title = title != null ? title : ""; }
    public void setTitle(int titleId) {
        if (context != null) setTitle(context.getString(titleId));
    }
    public CharSequence getTitle() { return title; }

    public void setContentView(View view) { window.setDecorView(view); }
    public void setContentView(int layoutResID) {
        if (context != null) {
            View v = android.view.LayoutInflater.from(context).inflate(layoutResID, null);
            window.setDecorView(v);
        }
    }
    public void setContentView(View view, android.view.ViewGroup.LayoutParams params) {
        if (view != null && params != null) view.setLayoutParams(params);
        window.setDecorView(view);
    }

    public View findViewById(int id) {
        View decor = window.getDecorView();
        return decor != null ? decor.findViewById(id) : null;
    }

    public void setCancelable(boolean flag) { this.cancelable = flag; }
    public boolean isCancelable() { return cancelable; }
    public void setCanceledOnTouchOutside(boolean cancel) { this.canceledOnTouchOutside = cancel; }
    public boolean isCanceledOnTouchOutside() { return canceledOnTouchOutside; }

    public void setOnDismissListener(DialogInterface.OnDismissListener listener) { this.onDismissListener = listener; }
    public void setOnCancelListener(DialogInterface.OnCancelListener listener) { this.onCancelListener = listener; }
    public void setOnShowListener(DialogInterface.OnShowListener listener) { this.onShowListener = listener; }
    public void setOnKeyListener(DialogInterface.OnKeyListener listener) { this.onKeyListener = listener; }
    public DialogInterface.OnKeyListener getOnKeyListener() { return onKeyListener; }
    public void onBackPressed() {
        if (cancelable) cancel();
    }

    public void show() {
        if (!this.isShowing) {
            this.isShowing = true;
            ACTIVE_DIALOGS.remove(this);
            ACTIVE_DIALOGS.add(this);
            if (onShowListener != null) {
                try { onShowListener.onShow(this); } catch (Throwable ignored) {}
            }
        }
    }

    @Override
    public void dismiss() {
        if (this.isShowing) {
            this.isShowing = false;
            ACTIVE_DIALOGS.remove(this);
            if (onDismissListener != null) {
                try { onDismissListener.onDismiss(this); } catch (Throwable ignored) {}
            }
        }
    }

    @Override
    public void cancel() {
        if (onCancelListener != null) {
            try { onCancelListener.onCancel(this); } catch (Throwable ignored) {}
        }
        dismiss();
    }

    public boolean isShowing() { return isShowing; }

    public static Dialog getTopActiveDialog() {
        if (ACTIVE_DIALOGS.isEmpty()) return null;
        for (int i = ACTIVE_DIALOGS.size() - 1; i >= 0; i--) {
            Dialog d = ACTIVE_DIALOGS.get(i);
            if (d != null && d.isShowing() && d.getWindow().getDecorView() != null) {
                return d;
            }
        }
        return null;
    }

    public static List<Dialog> getActiveDialogs() {
        return Collections.unmodifiableList(ACTIVE_DIALOGS);
    }

    public static void clearActiveDialogs() {
        ACTIVE_DIALOGS.clear();
    }
}
