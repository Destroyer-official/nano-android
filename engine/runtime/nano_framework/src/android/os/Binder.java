package android.os;

public class Binder implements IBinder {
    private String descriptor;
    private IInterface owner;
    private volatile boolean alive = true;

    public Binder() {}

    public void attachInterface(IInterface owner, String descriptor) {
        this.owner = owner;
        this.descriptor = descriptor;
    }

    @Override
    public String getInterfaceDescriptor() {
        return descriptor;
    }

    @Override
    public boolean pingBinder() {
        return alive;
    }

    @Override
    public boolean isBinderAlive() {
        return alive;
    }

    @Override
    public IInterface queryLocalInterface(String descriptor) {
        if (this.descriptor != null && this.descriptor.equals(descriptor)) {
            return owner;
        }
        return null;
    }

    public void markDead() {
        this.alive = false;
    }
}
