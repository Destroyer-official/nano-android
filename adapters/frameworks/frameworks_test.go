package frameworks

import (
	"strings"
	"testing"
)

func TestProbeAllAndTranslateAllTwelveAdapters(t *testing.T) {
	specs := ProbeAll(".")
	if len(specs) != 12 {
		t.Fatalf("expected all 12 framework adapters, got %d", len(specs))
	}

	for _, s := range specs {
		cmd, err := TranslateCommand(s.ID, ".", TranslateRequest{
			ArtifactOrTarget: "flows/sample.yaml",
			PackageName:      "com.example.app",
			DeviceSerial:     "emulator-5554",
			ExtraArgs:        []string{"devices"},
		})
		if err != nil {
			t.Fatalf("TranslateCommand(%s) failed: %v", s.ID, err)
		}
		if cmd.Binary == "" || strings.Contains(cmd.NativeSemantics, "placeholder") {
			t.Fatalf("unexpected translation for %s: %+v", s.ID, cmd)
		}
	}

	if _, err := TranslateCommand(FrameworkMaestro, ".", TranslateRequest{ArtifactOrTarget: "flow.yaml; rm -rf /"}); err == nil {
		t.Fatalf("expected shell injection argument blocked")
	}
}
