package ide

import (
	"os"
	"strings"
	"testing"
)

func TestGenerateAndInstallAllSevenIDEClients(t *testing.T) {
	tmp := t.TempDir()
	for _, client := range ListClients() {
		dst, bundle, err := InstallConfig(client, "bin/nano", tmp)
		if err != nil {
			t.Fatalf("InstallConfig(%s) failed: %v", client, err)
		}
		if _, err := os.Stat(dst); err != nil {
			t.Fatalf("expected config written at %s: %v", dst, err)
		}
		if !strings.Contains(bundle.Content, "bin/nano") {
			t.Fatalf("expected binary path inside %s config", client)
		}
	}
}
