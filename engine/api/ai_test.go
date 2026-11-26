package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nano_android/engine/api"
	"nano_android/engine/intelligence/ai"
	"nano_android/engine/mcp_compiler"
	"nano_android/engine/security/httpauth"
)

func TestPhase5GraphAndAIEndpointsEndToEnd(t *testing.T) {
	t.Setenv("NANO_DEFAULT_APP", "")
	tmpRoot := t.TempDir()

	// Copy minimal_valid.apk fixture into isolated tmpRoot/fixtures/apks
	srcAPK, _ := filepath.Abs(filepath.Join("..", "..", "fixtures", "apks", "minimal_valid.apk"))
	rawAPK, err := os.ReadFile(srcAPK)
	if err != nil {
		t.Fatalf("read fixture apk %s: %v", srcAPK, err)
	}
	dstDir := filepath.Join(tmpRoot, "fixtures", "apks")
	_ = os.MkdirAll(dstDir, 0750)
	dstAPK := filepath.Join(dstDir, "minimal_valid.apk")
	if err := os.WriteFile(dstAPK, rawAPK, 0600); err != nil {
		t.Fatal(err)
	}

	coord := mcp_compiler.NewIntegratedCoordinator()
	t.Cleanup(func() { coord.Close() })
	svc := api.NewService(tmpRoot, coord, nil, nil)

	tok, _ := httpauth.GenerateBootToken()
	mux := http.NewServeMux()
	svc.MountRoutes(mux)
	ts := httptest.NewServer(httpauth.Middleware(httpauth.Config{BindHost: "127.0.0.1", AuthToken: tok}, mux))
	t.Cleanup(ts.Close)

	callJSON := func(method, path string, body any) (*http.Response, []byte) {
		var rdr *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rdr = bytes.NewReader(b)
		} else {
			rdr = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(method, ts.URL+path, rdr)
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", "application/json")
		resp, rErr := ts.Client().Do(req)
		if rErr != nil {
			t.Fatalf("%s %s: %v", method, path, rErr)
		}
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		_ = resp.Body.Close()
		return resp, buf.Bytes()
	}

	// 1. Build Behavior Graph
	resp, body := callJSON(http.MethodPost, "/api/v1/graph/build", map[string]any{"artifact": "minimal_valid.apk"})
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"architecture"`) {
		t.Fatalf("/api/v1/graph/build failed (%d): %s", resp.StatusCode, string(body))
	}

	// 1b. Query Behavior Graph
	resp, body = callJSON(http.MethodPost, "/api/v1/graph/query", map[string]any{"kind": "package", "limit": 50})
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"total_matching"`) {
		t.Fatalf("/api/v1/graph/query failed (%d): %s", resp.StatusCode, string(body))
	}

	// 2. Analyze Change Impact
	resp, body = callJSON(http.MethodPost, "/api/v1/graph/impact", map[string]any{"changed": []string{"MainActivity"}})
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"risk_level"`) {
		t.Fatalf("/api/v1/graph/impact failed (%d): %s", resp.StatusCode, string(body))
	}

	// 3. I1 Understand APK
	resp, body = callJSON(http.MethodPost, "/api/v1/ai/understand", map[string]any{"artifact": "minimal_valid.apk", "max_steps": 10})
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"markdown_docs"`) {
		t.Fatalf("/api/v1/ai/understand failed (%d): %s", resp.StatusCode, string(body))
	}

	// 4. I2 Generate Tests -> must be PENDING_APPROVAL
	resp, body = callJSON(http.MethodPost, "/api/v1/ai/generate_tests", map[string]any{"artifact": "minimal_valid.apk"})
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"PENDING_APPROVAL"`) {
		t.Fatalf("/api/v1/ai/generate_tests failed (%d): %s", resp.StatusCode, string(body))
	}
	var genOut struct {
		Proposals []ai.Proposal `json:"proposals"`
	}
	_ = json.Unmarshal(body, &genOut)
	if len(genOut.Proposals) == 0 {
		t.Fatalf("expected >= 1 generated test proposal")
	}

	// 5. Empty approver must fail closed (403)
	resp, body = callJSON(http.MethodPost, "/api/v1/ai/proposals/approve", map[string]any{
		"id": genOut.Proposals[0].ID, "approved_by": "", "apply": true,
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for empty approver, got %d (%s)", resp.StatusCode, string(body))
	}

	// 6. Explicit human approver succeeds and applies inside cache/ai_approved
	resp, body = callJSON(http.MethodPost, "/api/v1/ai/proposals/approve", map[string]any{
		"id": genOut.Proposals[0].ID, "approved_by": "studio_lead", "apply": true,
	})
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"APPLIED"`) {
		t.Fatalf("expected 200 APPLIED, got %d (%s)", resp.StatusCode, string(body))
	}

	// 7. I4 Diagnose trace + proposal
	resp, body = callJSON(http.MethodPost, "/api/v1/ai/diagnose", map[string]any{
		"trace": "java.lang.NullPointerException: empty cart\n\tat com.example.App.onClick(App.java:10)\n",
	})
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"ranked_causes"`) {
		t.Fatalf("/api/v1/ai/diagnose failed (%d): %s", resp.StatusCode, string(body))
	}

	// 8. Multi-agent Coordinate endpoint
	resp, body = callJSON(http.MethodPost, "/api/v1/ai/coordinate", map[string]any{"artifact": "minimal_valid.apk", "max_steps": 12})
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"test_proposals"`) {
		t.Fatalf("/api/v1/ai/coordinate failed (%d): %s", resp.StatusCode, string(body))
	}

	// 9. Engineering Memory PUT requires actor + GET returns stored entry
	resp, _ = callJSON(http.MethodPost, "/api/v1/ai/memory", map[string]any{"key": "k1", "category": "known_quirk", "value": "use id selector", "actor": ""})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 when mutating memory with empty actor, got %d", resp.StatusCode)
	}
	resp, body = callJSON(http.MethodPost, "/api/v1/ai/memory", map[string]any{"key": "k1", "category": "known_quirk", "value": "use id selector", "actor": "studio_lead"})
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"use id selector"`) {
		t.Fatalf("/api/v1/ai/memory PUT failed (%d): %s", resp.StatusCode, string(body))
	}

	// 10. Graph build & I1 understand must also work on standalone .jar artifacts (e.g. prism-v.jar)
	srcJar, _ := filepath.Abs(filepath.Join("..", "..", "tools", "prism-v.jar"))
	if rawJar, jErr := os.ReadFile(srcJar); jErr == nil {
		_ = os.MkdirAll(filepath.Join(tmpRoot, "tools"), 0750)
		_ = os.WriteFile(filepath.Join(tmpRoot, "tools", "prism-v.jar"), rawJar, 0600)
		resp, body = callJSON(http.MethodPost, "/api/v1/graph/build", map[string]any{"artifact": "prism-v.jar"})
		if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"architecture"`) {
			t.Fatalf("/api/v1/graph/build on prism-v.jar failed (%d): %s", resp.StatusCode, string(body))
		}
	}
}
