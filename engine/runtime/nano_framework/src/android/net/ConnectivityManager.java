package android.net;

import nanoandroid.compat.ApiSupportLevel;
import nanoandroid.compat.NanoApiStatus;

@NanoApiStatus(
    level = ApiSupportLevel.PARTIAL,
    fallback = "android_art",
    notes = "Controllable simulated ConnectivityManager (wifi/cellular/offline modes); not real cellular modem telemetry"
)
public class ConnectivityManager {
    public static final int TRANSPORT_CELLULAR = 0;
    public static final int TRANSPORT_WIFI = 1;
    public static final int NET_CAPABILITY_INTERNET = 12;
    public static final int NET_CAPABILITY_VALIDATED = 16;

    private static volatile boolean connected = true;
    private static volatile boolean metered = false;
    private static volatile int activeTransport = TRANSPORT_WIFI;

    public static void setSimulatedNetworkState(boolean isConnected, boolean isMetered, int transportType) {
        connected = isConnected;
        metered = isMetered;
        activeTransport = transportType;
    }

    public boolean isActiveNetworkMetered() {
        return connected && metered;
    }

    public boolean isConnectedSimulated() {
        return connected;
    }

    public int getActiveTransportSimulated() {
        return activeTransport;
    }
}
