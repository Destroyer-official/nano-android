package uitree

import (
	"testing"
)

// 5 Real uiautomator dump XML samples representing diverse Android screens

const sample1SettingsMain = `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
  <node index="0" text="" resource-id="" class="android.widget.FrameLayout" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[0,0][1080,2400]">
    <node index="0" text="" resource-id="com.android.settings:id/settings_homepage_container" class="android.widget.LinearLayout" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[0,0][1080,2400]">
      <node index="0" text="Settings" resource-id="com.android.settings:id/homepage_title" class="android.widget.TextView" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[64,180][900,280]" />
      <node index="1" text="" resource-id="com.android.settings:id/recycler_view" class="androidx.recyclerview.widget.RecyclerView" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="true" focused="false" scrollable="true" long-clickable="false" password="false" selected="false" bounds="[0,300][1080,2400]">
        <node index="0" text="" resource-id="com.android.settings:id/network_item" class="android.widget.LinearLayout" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="true" enabled="true" focusable="true" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[0,300][1080,480]">
          <node index="0" text="Network &amp; internet" resource-id="android:id/title" class="android.widget.TextView" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[192,340][900,400]" />
          <node index="1" text="Wi-Fi, Mobile, Data usage, Hotspot" resource-id="android:id/summary" class="android.widget.TextView" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[192,404][900,450]" />
        </node>
        <node index="1" text="" resource-id="com.android.settings:id/connected_devices_item" class="android.widget.LinearLayout" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="true" enabled="true" focusable="true" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[0,480][1080,660]">
          <node index="0" text="Connected devices" resource-id="android:id/title" class="android.widget.TextView" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[192,520][900,580]" />
        </node>
        <node index="2" text="" resource-id="com.android.settings:id/apps_item" class="android.widget.LinearLayout" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="true" enabled="true" focusable="true" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[0,660][1080,840]">
          <node index="0" text="Apps" resource-id="android:id/title" class="android.widget.TextView" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[192,700][900,760]" />
        </node>
        <node index="3" text="" resource-id="com.android.settings:id/battery_item" class="android.widget.LinearLayout" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="true" enabled="true" focusable="true" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[0,840][1080,1020]">
          <node index="0" text="Battery" resource-id="android:id/title" class="android.widget.TextView" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[192,880][900,940]" />
          <node index="1" text="84% - Should last until 11:30 PM" resource-id="android:id/summary" class="android.widget.TextView" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[192,944][900,990]" />
        </node>
      </node>
    </node>
  </node>
</hierarchy>`

const sample2WiFiSettings = `UI hierachy dumped to: /dev/tty
<hierarchy rotation="0">
  <node index="0" text="" resource-id="" class="android.widget.FrameLayout" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[0,0][1080,2400]">
    <node index="0" text="" resource-id="com.android.settings:id/action_bar" class="android.view.ViewGroup" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[0,80][1080,240]">
      <node index="0" text="" resource-id="" class="android.widget.ImageButton" package="com.android.settings" content-desc="Navigate up" checkable="false" checked="false" clickable="true" enabled="true" focusable="true" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[0,80][168,240]" />
      <node index="1" text="Internet" resource-id="" class="android.widget.TextView" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[192,130][450,190]" />
    </node>
    <node index="1" text="" resource-id="com.android.settings:id/switch_bar" class="android.widget.LinearLayout" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="true" enabled="true" focusable="true" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[48,280][1032,440]">
      <node index="0" text="Wi-Fi" resource-id="com.android.settings:id/switch_text" class="android.widget.TextView" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[96,330][300,390]" />
      <node index="1" text="" resource-id="com.android.settings:id/switch_widget" class="android.widget.Switch" package="com.android.settings" content-desc="Wi-Fi" checkable="true" checked="true" clickable="true" enabled="true" focusable="true" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[880,330][984,390]" />
    </node>
    <node index="2" text="" resource-id="android:id/list" class="androidx.recyclerview.widget.RecyclerView" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="true" focused="false" scrollable="true" long-clickable="false" password="false" selected="false" bounds="[0,460][1080,2400]">
      <node index="0" text="" resource-id="com.android.settings:id/wifi_network_lab" class="android.widget.LinearLayout" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="true" enabled="true" focusable="true" focused="false" scrollable="false" long-clickable="true" password="false" selected="false" bounds="[48,480][1032,620]">
        <node index="0" text="AndroidLab_5G" resource-id="android:id/title" class="android.widget.TextView" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[192,510][600,560]" />
        <node index="1" text="Connected" resource-id="android:id/summary" class="android.widget.TextView" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[192,564][450,600]" />
      </node>
      <node index="1" text="Add network" resource-id="com.android.settings:id/add_network" class="android.widget.Button" package="com.android.settings" content-desc="" checkable="false" checked="false" clickable="true" enabled="true" focusable="true" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[48,640][1032,760]" />
    </node>
  </node>
</hierarchy>`

const sample3ScamShieldAlert = `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
  <node index="0" text="" resource-id="com.example.scamshield:id/root_layout" class="android.widget.FrameLayout" package="com.example.scamshield" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[0,0][1080,2400]">
    <node index="0" text="" resource-id="com.example.scamshield:id/card_alert" class="androidx.cardview.widget.CardView" package="com.example.scamshield" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[48,200][1032,1500]">
      <node index="0" text="SUSPECTED SCAM SMS" resource-id="com.example.scamshield:id/alert_badge" class="android.widget.TextView" package="com.example.scamshield" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[96,260][500,320]" />
      <node index="1" text="Sender: +1 (555) 019-2831" resource-id="com.example.scamshield:id/txt_sender" class="android.widget.TextView" package="com.example.scamshield" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[96,340][800,400]" />
      <node index="2" text="Received 2m ago" resource-id="com.example.scamshield:id/txt_time" class="android.widget.TextView" package="com.example.scamshield" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[96,410][400,450]" />
      <node index="3" text="Urgent: Your bank account is locked. Click http://evil.bank/verify to unlock immediately." resource-id="com.example.scamshield:id/txt_body" class="android.widget.TextView" package="com.example.scamshield" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[96,480][984,650]" />
    </node>
    <node index="1" text="" resource-id="com.example.scamshield:id/btn_block_sender" class="android.widget.Button" package="com.example.scamshield" content-desc="" checkable="false" checked="false" clickable="true" enabled="true" focusable="true" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[48,1620][1032,1764]">
      <node index="0" text="" class="android.widget.FrameLayout" package="com.example.scamshield" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[48,1620][1032,1764]">
        <node index="0" text="" class="android.widget.LinearLayout" package="com.example.scamshield" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[48,1620][1032,1764]">
          <node index="0" text="Block &amp; Notify" class="android.widget.TextView" package="com.example.scamshield" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[48,1620][1032,1764]" />
        </node>
      </node>
    </node>
    <node index="2" text="Dismiss" resource-id="com.example.scamshield:id/btn_dismiss" class="android.widget.Button" package="com.example.scamshield" content-desc="" checkable="false" checked="false" clickable="true" enabled="true" focusable="true" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[48,1800][1032,1944]" />
  </node>
</hierarchy>`

const sample4VideoCompressor = `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
  <node index="0" text="" resource-id="" class="android.widget.FrameLayout" package="com.example.videocodec" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[0,0][1080,2400]">
    <node index="0" text="Video Transcoder &amp; Benchmark" resource-id="com.example.videocodec:id/header" class="android.widget.TextView" package="com.example.videocodec" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[48,100][1032,200]" />
    <node index="1" text="" resource-id="com.example.videocodec:id/video_preview" class="android.view.SurfaceView" package="com.example.videocodec" content-desc="Camera preview" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[48,220][1032,1000]" />
    <node index="2" text="" resource-id="com.example.videocodec:id/progress_encode" class="android.widget.ProgressBar" package="com.example.videocodec" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[48,1020][1032,1080]" />
    <node index="3" text="" resource-id="com.example.videocodec:id/seek_quality" class="android.widget.SeekBar" package="com.example.videocodec" content-desc="Target Bitrate" checkable="false" checked="false" clickable="true" enabled="true" focusable="true" focused="false" scrollable="true" long-clickable="false" password="false" selected="false" bounds="[48,1100][1032,1180]" />
    <node index="4" text="input_4k_60fps.mp4" resource-id="com.example.videocodec:id/edt_filename" class="android.widget.EditText" package="com.example.videocodec" content-desc="" checkable="false" checked="false" clickable="true" enabled="true" focusable="true" focused="false" scrollable="false" long-clickable="true" password="false" selected="false" bounds="[48,1220][1032,1340]" />
    <node index="5" text="Start Transcode (VVC / AV1)" resource-id="com.example.videocodec:id/btn_start" class="android.widget.Button" package="com.example.videocodec" content-desc="" checkable="false" checked="false" clickable="true" enabled="true" focusable="true" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[48,1380][1032,1520]" />
  </node>
</hierarchy>`

const sample5ComposeUI = `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
  <node index="0" text="" resource-id="" class="androidx.compose.ui.platform.ComposeView" package="com.example.composeapp" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[0,0][1080,2400]">
    <node index="0" text="" resource-id="testTag:login_form" class="android.view.View" package="com.example.composeapp" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[48,100][1032,1800]">
      <node index="0" text="Welcome to DroidForge" resource-id="testTag:txt_title" class="android.widget.TextView" package="com.example.composeapp" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[48,120][800,200]" />
      <node index="1" text="user@example.test" resource-id="testTag:input_email" class="android.widget.EditText" package="com.example.composeapp" content-desc="Email address" checkable="false" checked="false" clickable="true" enabled="true" focusable="true" focused="false" scrollable="false" long-clickable="true" password="false" selected="false" bounds="[48,240][1032,360]" />
      <node index="2" text="••••••••" resource-id="testTag:input_password" class="android.widget.EditText" package="com.example.composeapp" content-desc="Password" checkable="false" checked="false" clickable="true" enabled="true" focusable="true" focused="false" scrollable="false" long-clickable="true" password="true" selected="false" bounds="[48,400][1032,520]" />
      <node index="3" text="Sign In" resource-id="testTag:btn_submit" class="android.widget.Button" package="com.example.composeapp" content-desc="" checkable="false" checked="false" clickable="true" enabled="true" focusable="true" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[48,560][1032,700]" />
      <node index="4" text="Remember me" resource-id="testTag:chk_remember" class="android.widget.CheckBox" package="com.example.composeapp" content-desc="" checkable="true" checked="true" clickable="true" enabled="true" focusable="true" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[48,740][400,820]" />
    </node>
  </node>
</hierarchy>`

func TestParseXML_FiveRealSamples(t *testing.T) {
	samples := []struct {
		name          string
		xml           string
		expectedNodes int
	}{
		{"SettingsMain", sample1SettingsMain, 14},
		{"WiFiSettings", sample2WiFiSettings, 12},
		{"ScamShieldAlert", sample3ScamShieldAlert, 11},
		{"VideoCompressor", sample4VideoCompressor, 7},
		{"ComposeUI", sample5ComposeUI, 7},
	}

	for _, tc := range samples {
		t.Run(tc.name, func(t *testing.T) {
			hierarchy, err := ParseXML([]byte(tc.xml))
			if err != nil {
				t.Fatalf("ParseXML failed on %s: %v", tc.name, err)
			}
			if hierarchy == nil {
				t.Fatalf("expected non-nil hierarchy on %s", tc.name)
			}

			flat := FlattenAllNodes(hierarchy)
			if len(flat) != tc.expectedNodes {
				t.Errorf("sample %s: got %d total raw nodes, expected %d", tc.name, len(flat), tc.expectedNodes)
			}

			// Verify bounds were correctly parsed on root
			root := hierarchy.Nodes[0]
			if root.ParsedBounds[2] != 1080 || root.ParsedBounds[3] != 2400 {
				t.Errorf("sample %s: root bounds parsed incorrectly: %v", tc.name, root.ParsedBounds)
			}
		})
	}
}

func TestParseBounds_Formats(t *testing.T) {
	tests := []struct {
		input       string
		expected    [4]int
		expectError bool
	}{
		{"[48,1620][1032,1764]", [4]int{48, 1620, 1032, 1764}, false},
		{"[0,0][1080,2400]", [4]int{0, 0, 1080, 2400}, false},
		{"[10,20][30,40]", [4]int{10, 20, 30, 40}, false},
		{"invalid", [4]int{}, true},
		{"[0,0]", [4]int{}, true},
	}

	for _, tc := range tests {
		res, err := ParseBounds(tc.input)
		if tc.expectError && err == nil {
			t.Errorf("expected error for %q, got none", tc.input)
		}
		if !tc.expectError && err != nil {
			t.Errorf("unexpected error for %q: %v", tc.input, err)
		}
		if !tc.expectError && res != tc.expected {
			t.Errorf("for %q, got %v, expected %v", tc.input, res, tc.expected)
		}
	}
}

func TestParseXML_LeadingBannerStrip(t *testing.T) {
	raw := []byte("UI hierachy dumped to: /dev/tty\n<hierarchy rotation=\"0\"><node bounds=\"[0,0][1080,2400]\" class=\"android.widget.FrameLayout\" /></hierarchy>")
	h, err := ParseXML(raw)
	if err != nil {
		t.Fatalf("failed to parse with banner: %v", err)
	}
	if len(h.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(h.Nodes))
	}
	if h.Nodes[0].ParsedBounds[2] != 1080 {
		t.Errorf("unexpected bounds: %v", h.Nodes[0].ParsedBounds)
	}
}
