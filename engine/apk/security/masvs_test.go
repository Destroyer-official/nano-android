package security_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"nano_android/engine/apk/ingest"
	"nano_android/engine/apk/manifest"
	"nano_android/engine/apk/resources"
	"nano_android/engine/apk/security"
	"nano_android/engine/apk/signing"
)

func TestOWASPMASVSAuditAndSARIFExport(t *testing.T) {
	b := true
	rep := &ingest.IntelligenceReport{
		Path:   "app-release.apk",
		SHA256: "abc123",
		Manifest: &manifest.Info{
			Package:              "com.example.vuln",
			Debuggable:           true,
			UsesCleartextTraffic: &b,
			AllowBackup:          &b,
			Services: []manifest.Component{
				{Kind: "service", Name: "com.example.vuln.ExportedSvc", Exported: &b},
			},
		},
		Resources: &resources.Table{
			Strings: []string{"AKIAIOSFODNN7EXAMPLEKEY"},
		},
		NativeLibs: []signing.NativeLibInfo{
			{Path: "lib/arm64-v8a/libold.so", ABI: "arm64-v8a", MaxAlign: 0x1000, Is16KBAligned: false},
		},
	}

	audit := security.AuditAPK(rep)
	if audit.PassedGate || len(audit.Findings) < 5 {
		t.Fatalf("expected failed MASVS gate with >= 5 findings, got %+v", audit)
	}
	sarifBytes, err := audit.ExportSARIF()
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(sarifBytes, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["version"] != "2.1.0" {
		t.Fatalf("expected SARIF 2.1.0, got %v", parsed["version"])
	}

	// Verify real fixture passes/reports properly
	eng := ingest.NewEngine(t.TempDir(), "")
	realRep, err := eng.Analyze(filepath.Join("..", "..", "..", "fixtures", "apks", "minimal_valid.apk"))
	if err != nil {
		t.Fatal(err)
	}
	realAudit := security.AuditAPK(realRep)
	if realAudit.PackageName != "com.example.nanoref" {
		t.Fatalf("expected package com.example.nanoref, got %q", realAudit.PackageName)
	}
}
