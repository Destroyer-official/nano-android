package android.database.sqlite;

import java.io.Closeable;

public abstract class SQLiteClosable implements Closeable {
    private int mReferenceCount = 1;

    protected abstract void onAllReferencesReleased();

    public void acquireReference() {
        synchronized (this) {
            if (mReferenceCount <= 0) {
                throw new IllegalStateException("attempt to re-acquire an already closed object: " + this);
            }
            mReferenceCount++;
        }
    }

    public void releaseReference() {
        boolean doClose = false;
        synchronized (this) {
            mReferenceCount--;
            doClose = (mReferenceCount == 0);
        }
        if (doClose) {
            onAllReferencesReleased();
        }
    }

    @Override
    public void close() {
        releaseReference();
    }
}
