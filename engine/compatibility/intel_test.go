package compatibility_test

import (
	"os"
	"path/filepath"
	"testing"

	"nano_android/engine/compatibility/intel"
	"nano_android/engine/compatibility/registry"
)

func TestSQLiteCompatibilityIntelligenceContinuousLearning(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "compatibility.db")
	store, err := intel.OpenStore(dbPath)
	if err != nil {
		t.Fatalf("OpenStore failed: %v", err)
	}
	if err := store.SeedDefaults(); err != nil {
		t.Fatalf("SeedDefaults failed: %v", err)
	}
	st, err := os.Stat(dbPath)
	if err != nil || st.Size() == 0 {
		t.Fatalf("expected non-empty SQLite file at %s", dbPath)
	}

	if err := store.RecordConformanceEvidence("android.database.sqlite.SQLiteDatabase", "endTransaction", registry.BackendNanoJVM, 37, true, "ACID rollback verified"); err != nil {
		t.Fatalf("RecordConformanceEvidence failed: %v", err)
	}
	if err := store.RecordDifferentialEvidence(registry.BackendNanoJVM, registry.BackendAndroidART, "android.view.View", 35, false, "Subpixel font kerning delta on ART vs Java2D"); err != nil {
		t.Fatalf("RecordDifferentialEvidence failed: %v", err)
	}

	hm, err := store.QueryHeatmap()
	if err != nil {
		t.Fatalf("QueryHeatmap failed: %v", err)
	}
	if len(hm.Contracts) < 12 {
		t.Fatalf("expected >=12 seeded contracts, got %d", len(hm.Contracts))
	}
	if len(hm.APILevels) != 12 {
		t.Fatalf("expected API levels 26..37 (12 entries), got %d", len(hm.APILevels))
	}
	if len(hm.Quirks) < 3 {
		t.Fatalf("expected >=3 seeded device/OEM quirks, got %d", len(hm.Quirks))
	}

	var foundSQL, foundView bool
	for _, c := range hm.Contracts {
		if c.API == "android.database.sqlite.SQLiteDatabase" {
			foundSQL = true
			if c.PassCount < 1 {
				t.Errorf("expected SQL pass_count >= 1, got %+v", c)
			}
		}
		if c.API == "android.view.View" {
			foundView = true
			if c.DivergeCount < 1 {
				t.Errorf("expected View diverge_count >= 1, got %+v", c)
			}
		}
	}
	if !foundSQL || !foundView {
		t.Fatalf("expected both SQLiteDatabase and View contracts in heatmap")
	}
}
