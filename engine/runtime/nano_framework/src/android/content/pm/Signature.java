package android.content.pm;

import java.io.Serializable;

public class Signature implements Serializable {
    private final byte[] mSignature;

    public Signature(byte[] signature) {
        this.mSignature = signature != null ? signature.clone() : new byte[0];
    }

    public Signature(String text) {
        this.mSignature = text != null ? text.getBytes() : new byte[0];
    }

    public byte[] toByteArray() {
        return mSignature != null ? mSignature.clone() : new byte[0];
    }

    public String toCharsString() {
        return new String(mSignature);
    }
}
