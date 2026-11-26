package android.location;

import nanoandroid.compat.ApiSupportLevel;
import nanoandroid.compat.NanoApiStatus;

@NanoApiStatus(
    level = ApiSupportLevel.PARTIAL,
    fallback = "android_art",
    notes = "Simulated deterministic GPS/Network LocationManager; never presented as real hardware GPS telemetry"
)
public class LocationManager {
    public static final String GPS_PROVIDER = "gps";
    public static final String NETWORK_PROVIDER = "network";

    private static volatile double simLatitude = 37.4220;
    private static volatile double simLongitude = -122.0841;
    private static volatile boolean providerEnabled = true;

    public static void setSimulatedCoordinates(double lat, double lon, boolean enabled) {
        simLatitude = lat;
        simLongitude = lon;
        providerEnabled = enabled;
    }

    public boolean isProviderEnabled(String provider) {
        return providerEnabled;
    }

    public double[] getLastSimulatedLatLon(String provider) {
        if (!providerEnabled) return null;
        return new double[]{simLatitude, simLongitude};
    }
}
