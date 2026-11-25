package android.app;

public abstract class FragmentManager {
    public abstract FragmentTransaction beginTransaction();
    public abstract Fragment findFragmentById(int id);
    public abstract Fragment findFragmentByTag(String tag);
    public abstract boolean executePendingTransactions();
}
