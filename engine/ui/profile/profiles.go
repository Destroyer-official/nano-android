package profile

import "fmt"

// DeviceProfile models an Android viewport, density, locale, font-scale, and scenario environment.
type DeviceProfile struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Category        string  `json:"category"` // "hardware", "scenario"
	WidthPx         int     `json:"width_px"`
	HeightPx        int     `json:"height_px"`
	DensityDPI      int     `json:"density_dpi"`
	DensityScale    float64 `json:"density_scale"`
	FontScale       float64 `json:"font_scale"`
	APILevel        int     `json:"api_level"`
	Locale          string  `json:"locale"`
	TimeZone        string  `json:"time_zone"`
	NightMode       bool    `json:"night_mode"`
	RTLLayout       bool    `json:"rtl_layout"`
	NavigationMode  string  `json:"navigation_mode"` // "gesture", "3button"
	Offline         bool    `json:"offline"`
	SlowNetworkMs   int     `json:"slow_network_ms,omitempty"`
	PermissionDeny  bool    `json:"permission_deny"`
	LowStorage      bool    `json:"low_storage"`
	LowMemoryMB     int     `json:"low_memory_mb,omitempty"`
	BatterySaver    bool    `json:"battery_saver"`
}

// PxToDp converts pixels to density-independent pixels using the profile's density scale.
func (p DeviceProfile) PxToDp(px int) float64 {
	scale := p.DensityScale
	if scale <= 0 {
		scale = 2.625
	}
	return float64(px) / scale
}

// ReusableProfiles returns the canonical 13+ reusable Android device & scenario profiles.
func ReusableProfiles() map[string]DeviceProfile {
	base := DeviceProfile{
		ID:             "clean_install",
		Name:           "Clean Install (Pixel 8 / API 35)",
		Category:       "scenario",
		WidthPx:        1080,
		HeightPx:       2400,
		DensityDPI:     420,
		DensityScale:   2.625,
		FontScale:      1.0,
		APILevel:       35,
		Locale:         "en-US",
		TimeZone:       "UTC",
		NavigationMode: "gesture",
	}

	profiles := []DeviceProfile{
		base,
		func() DeviceProfile { p := base; p.ID = "existing_app_data"; p.Name = "Existing App Data"; return p }(),
		func() DeviceProfile { p := base; p.ID = "no_network"; p.Name = "Airplane / No Network"; p.Offline = true; return p }(),
		func() DeviceProfile { p := base; p.ID = "slow_network"; p.Name = "Slow 3G Network (800ms RTT)"; p.SlowNetworkMs = 800; return p }(),
		func() DeviceProfile { p := base; p.ID = "permission_denied"; p.Name = "Dangerous Permissions Denied"; p.PermissionDeny = true; return p }(),
		func() DeviceProfile { p := base; p.ID = "low_storage"; p.Name = "Low Storage Quota"; p.LowStorage = true; return p }(),
		func() DeviceProfile { p := base; p.ID = "low_memory"; p.Name = "Low Memory (256MB Heap)"; p.LowMemoryMB = 256; return p }(),
		func() DeviceProfile { p := base; p.ID = "battery_saver"; p.Name = "Battery Saver Active"; p.BatterySaver = true; return p }(),
		func() DeviceProfile { p := base; p.ID = "dark_mode"; p.Name = "Dark Mode (Night UI)"; p.NightMode = true; return p }(),
		func() DeviceProfile { p := base; p.ID = "large_font"; p.Name = "Accessibility Large Font (1.5x)"; p.FontScale = 1.5; return p }(),
		func() DeviceProfile {
			p := base
			p.ID = "small_screen"
			p.Name = "Compact Phone (720x1280 xhdpi)"
			p.WidthPx = 720
			p.HeightPx = 1280
			p.DensityDPI = 320
			p.DensityScale = 2.0
			return p
		}(),
		func() DeviceProfile {
			p := base
			p.ID = "landscape"
			p.Name = "Landscape Phone"
			p.WidthPx = 2400
			p.HeightPx = 1080
			return p
		}(),
		func() DeviceProfile {
			p := base
			p.ID = "rtl_arabic"
			p.Name = "RTL Locale (ar-EG)"
			p.Locale = "ar-EG"
			p.RTLLayout = true
			return p
		}(),
		func() DeviceProfile {
			p := base
			p.ID = "api_26"
			p.Name = "Legacy Android 8.0 (API 26)"
			p.APILevel = 26
			return p
		}(),
		func() DeviceProfile {
			p := base
			p.ID = "api_37"
			p.Name = "Android 17 Preview (API 37)"
			p.APILevel = 37
			return p
		}(),
	}

	out := make(map[string]DeviceProfile, len(profiles))
	for _, p := range profiles {
		out[p.ID] = p
	}
	return out
}

// Get returns a profile by ID or an explicit error if unknown.
func Get(id string) (DeviceProfile, error) {
	all := ReusableProfiles()
	p, ok := all[id]
	if !ok {
		return DeviceProfile{}, fmt.Errorf("unknown device/scenario profile %q", id)
	}
	return p, nil
}
