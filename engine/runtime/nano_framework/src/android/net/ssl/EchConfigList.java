package android.net.ssl;

/**
 * Minimal stub for Android 15/16 ECH (Encrypted Client Hello) config list
 * so Conscrypt / OkHttp 5 link cleanly on host JVM.
 */
public final class EchConfigList {
    private final byte[] rawBytes;

    public EchConfigList(byte[] rawBytes) {
        this.rawBytes = rawBytes != null ? rawBytes : new byte[0];
    }

    public byte[] getEncoded() {
        return rawBytes;
    }
}
