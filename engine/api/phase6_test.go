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
	"nano_android/engine/mcp_compiler"
	"nano_android/engine/security/httpauth"
)

func TestPhase6EndpointsPluginsAdaptersPoolsReportsAnalytics(t *testing.T) {
	t.Setenv("NANO_DEFAULT_APP", "")
	tmpRoot := t.TempDir()

	srcAPK, _ := filepath.Abs(filepath.Join("..", "..", "fixtures", "apks", "minimal_valid.apk"))
	rawAPK, err := os.ReadFile(srcAPK)
	if err != nil {
		t.Fatalf("read fixture apk: %v", err)
	}
	_ = os.MkdirAll(filepath.Join(tmpRoot, "fixtures", "apks"), 0750)
	_ = os.WriteFile(filepath.Join(tmpRoot, "fixtures", "apks", "minimal_valid.apk"), rawAPK, 0600)

	coord := mcp_compiler.NewIntegratedCoordinator()
	t.Cleanup(func() { coord.Close() })
	svc := api.NewService(tmpRoot, coord, nil, nil)

	tok, _ := httpauth.GenerateBootToken()
	mux := http.NewServeMux()
	svc.MountRoutes(mux)
	ts := httptest.NewServer(httpauth.Middleware(httpauth.Config{BindHost: "127.0.0.1", AuthToken: tok}, mux))
	t.Cleanup(ts.Close)

	call := func(method, path string, body any) (*http.Response, []byte) {
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

	// 0. Unauthenticated requests must fail with 401
	for _, p := range []string{"/api/v1/plugins", "/api/v1/pools", "/api/v1/analytics"} {
		uResp, _ := ts.Client().Get(ts.URL + p)
		_ = uResp.Body.Close()
		if uResp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401 unauthenticated for %s, got %d", p, uResp.StatusCode)
		}
	}

	// 1. Plugin SDK list
	resp, body := call(http.MethodGet, "/api/v1/plugins", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"api_shim"`) {
		t.Fatalf("/api/v1/plugins failed (%d): %s", resp.StatusCode, string(body))
	}

	// 2. Adapters list + IDE config generation
	resp, body = call(http.MethodGet, "/api/v1/adapters", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"maestro"`) || !strings.Contains(string(body), `"antigravity"`) {
		t.Fatalf("/api/v1/adapters failed (%d): %s", resp.StatusCode, string(body))
	}
	resp, body = call(http.MethodGet, "/api/v1/adapters?client=vscode", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `.vscode`) {
		t.Fatalf("/api/v1/adapters?client=vscode failed (%d): %s", resp.StatusCode, string(body))
	}

	// 3. Pools persistent reserve -> discover shows reserved -> release frees slot -> sharded run
	resp, body = call(http.MethodPost, "/api/v1/pools/reserve", map[string]any{"actor": "dev_bob"})
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"reservation"`) {
		t.Fatalf("/api/v1/pools/reserve failed (%d): %s", resp.StatusCode, string(body))
	}
	var resOut struct {
		Reservation struct {
			ID string `json:"id"`
		} `json:"reservation"`
	}
	_ = json.Unmarshal(body, &resOut)
	resp, body = call(http.MethodGet, "/api/v1/pools", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), resOut.Reservation.ID) {
		t.Fatalf("expected persistent reservation %q in /api/v1/pools list: %s", resOut.Reservation.ID, string(body))
	}
	resp, _ = call(http.MethodPost, "/api/v1/pools/release", map[string]any{"reservation_id": resOut.Reservation.ID})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/api/v1/pools/release failed: %d", resp.StatusCode)
	}
	resp, body = call(http.MethodPost, "/api/v1/pools/shard", map[string]any{"actor": "api_ci", "max_shards": 2, "cases": []string{"C1", "C2"}})
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"COMPLETED"`) {
		t.Fatalf("/api/v1/pools/shard failed (%d): %s", resp.StatusCode, string(body))
	}

	// 4. Unified signed report + Verify endpoint + tamper detection + exports + Analytics
	resp, body = call(http.MethodPost, "/api/v1/reports/unified", map[string]any{"artifact": "minimal_valid.apk", "actor": "release_bot"})
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"signature_ed25519"`) {
		t.Fatalf("/api/v1/reports/unified failed (%d): %s", resp.StatusCode, string(body))
	}
	var repOut struct {
		Record struct {
			ID string `json:"id"`
		} `json:"record"`
		Report map[string]any `json:"report"`
	}
	_ = json.Unmarshal(body, &repOut)

	resp, body = call(http.MethodPost, "/api/v1/reports/verify", repOut.Report)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"verified":true`) {
		t.Fatalf("/api/v1/reports/verify failed (%d): %s", resp.StatusCode, string(body))
	}
	repOut.Report["version_name"] = "tampered-9.9.9"
	resp, _ = call(http.MethodPost, "/api/v1/reports/verify", repOut.Report)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 BLOCKED_POLICY on tampered report verify, got %d", resp.StatusCode)
	}

	for _, fmtName := range []string{"html", "pdf", "junit", "sarif", "zip"} {
		expResp, expBody := call(http.MethodGet, "/api/v1/reports/"+repOut.Record.ID+"?format="+fmtName, nil)
		if expResp.StatusCode != http.StatusOK || len(expBody) < 20 {
			t.Fatalf("export format=%s failed (%d)", fmtName, expResp.StatusCode)
		}
		if fmtName == "pdf" && !strings.HasPrefix(string(expBody), "%PDF-1.4") {
			t.Fatalf("expected valid PDF header from report export")
		}
	}

	resp, body = call(http.MethodGet, "/api/v1/analytics", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"total_runs":1`) {
		t.Fatalf("/api/v1/analytics failed (%d): %s", resp.StatusCode, string(body))
	}
}
