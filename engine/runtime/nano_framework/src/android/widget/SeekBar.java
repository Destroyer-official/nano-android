package android.widget;

import android.content.Context;
import android.view.MotionEvent;

public class SeekBar extends ProgressBar {
    public interface OnSeekBarChangeListener {
        void onProgressChanged(SeekBar seekBar, int progress, boolean fromUser);
        void onStartTrackingTouch(SeekBar seekBar);
        void onStopTrackingTouch(SeekBar seekBar);
    }

    private OnSeekBarChangeListener listener;

    public SeekBar() { super(); }
    public SeekBar(Context context) { super(context); }
    public SeekBar(Context context, android.util.AttributeSet attrs) { super(context, attrs); }
    public SeekBar(Context context, android.util.AttributeSet attrs, int defStyleAttr) { super(context, attrs, defStyleAttr); }

    public void setOnSeekBarChangeListener(OnSeekBarChangeListener l) {
        this.listener = l;
    }

    public OnSeekBarChangeListener getOnSeekBarChangeListener() {
        return listener;
    }

    public void setProgressFromUser(int progress) {
        int max = Math.max(1, getMax());
        int clamped = Math.max(0, Math.min(max, progress));
        if (listener != null) {
            try { listener.onStartTrackingTouch(this); } catch (Throwable ignored) {}
        }
        setProgress(clamped);
        if (listener != null) {
            try { listener.onProgressChanged(this, clamped, true); } catch (Throwable ignored) {}
            try { listener.onStopTrackingTouch(this); } catch (Throwable ignored) {}
        }
    }

    @Override
    public boolean onTouchEvent(MotionEvent event) {
        if (event == null || !isEnabled()) return super.onTouchEvent(event);
        int width = Math.max(1, getWidth() - getPaddingLeft() - getPaddingRight());
        float rel = (event.getX() - getLeft() - getPaddingLeft()) / (float) width;
        if (event.getX() >= 0 && event.getX() <= getWidth() && getWidth() > 0 && event.getX() < getLeft()) {
            // event already in local view coordinates
            rel = (event.getX() - getPaddingLeft()) / (float) width;
        }
        rel = Math.max(0f, Math.min(1f, rel));
        int newProg = Math.round(rel * Math.max(1, getMax()));
        int act = event.getAction();
        if (act == MotionEvent.ACTION_DOWN) {
            if (listener != null) {
                try { listener.onStartTrackingTouch(this); } catch (Throwable ignored) {}
            }
            setProgress(newProg);
            if (listener != null) {
                try { listener.onProgressChanged(this, newProg, true); } catch (Throwable ignored) {}
            }
            return true;
        } else if (act == MotionEvent.ACTION_MOVE) {
            setProgress(newProg);
            if (listener != null) {
                try { listener.onProgressChanged(this, newProg, true); } catch (Throwable ignored) {}
            }
            return true;
        } else if (act == MotionEvent.ACTION_UP || act == MotionEvent.ACTION_CANCEL) {
            setProgress(newProg);
            if (listener != null) {
                try { listener.onProgressChanged(this, newProg, true); } catch (Throwable ignored) {}
                try { listener.onStopTrackingTouch(this); } catch (Throwable ignored) {}
            }
            return true;
        }
        return true;
    }
}
