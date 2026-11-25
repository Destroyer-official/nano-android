package android.os;

public class Process {
    public static int myPid() {
        return (int) java.lang.ProcessHandle.current().pid();
    }
    public static int myUid() {
        return 10001;
    }
    public static int myTid() {
        return (int) Thread.currentThread().getId();
    }
}
