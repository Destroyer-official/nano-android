package android.os;

public interface IBinder {
    String getInterfaceDescriptor();
    boolean pingBinder();
    boolean isBinderAlive();
    IInterface queryLocalInterface(String descriptor);

    interface IInterface {
        IBinder asBinder();
    }
}
