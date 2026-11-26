package android.net.ssl;

import javax.net.ssl.SSLException;

/**
 * Minimal stub for Android 15/16 ECH config mismatch exception.
 */
public class EchConfigMismatchException extends SSLException {
    private final EchConfigList retryConfigs;

    public EchConfigMismatchException(String message) {
        this(message, null);
    }

    public EchConfigMismatchException(String message, EchConfigList retryConfigs) {
        super(message);
        this.retryConfigs = retryConfigs;
    }

    public EchConfigList getRetryConfigs() {
        return retryConfigs;
    }
}
