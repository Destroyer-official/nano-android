package main

import (
	"archive/zip"
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nano_android/engine/security/httpauth"
)

func setupHardenedStudioTestHandler(t *testing.T) (http.Handler, string) {
	t.Helper()
	t.Setenv("NANO_DEFAULT_APP", "")
	srv := NewStudioServer()
	t.Cleanup(func() { srv.coord.Close() })

	tok, err := httpauth.GenerateBootToken()
	if err != nil {
		t.Fatal(err)
	}
	cfg := httpauth.Config{
		BindHost:       "127.0.0.1",
		AuthToken:      tok,
		PublicGetPaths: map[string]bool{"/": true},
	}
	mux := http.NewServeMux()
	srv.mountRoutes(mux)
	return httpauth.Middleware(cfg, mux), tok
}

func TestStudioSecurityBlocksUnauthenticatedAndTraversalRequests(t *testing.T) {
	h, tok := setupHardenedStudioTestHandler(t)

	// 1. Unauthenticated /api/launch -> 401
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8088/api/launch", strings.NewReader(`{"app":"downloads/test.apk"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without bearer token, got %d", rec.Code)
	}

	// 2. DNS Rebinding Host header -> 403
	reqHost := httptest.NewRequest(http.MethodGet, "http://attacker.com:8088/api/security", nil)
	reqHost.Header.Set("Authorization", "Bearer "+tok)
	recHost := httptest.NewRecorder()
	h.ServeHTTP(recHost, reqHost)
	if recHost.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-loopback Host header, got %d", recHost.Code)
	}

	// 3. Path traversal on /api/launch -> 403
	outside := filepath.Join(t.TempDir(), "secret.apk")
	_ = os.WriteFile(outside, []byte("PK\x03\x04"), 0644)
	reqLaunch := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8088/api/launch", strings.NewReader(`{"app":`+`"`+filepath.ToSlash(outside)+`"`+`}`))
	reqLaunch.Header.Set("Authorization", "Bearer "+tok)
	reqLaunch.Header.Set("Content-Type", "application/json")
	recLaunch := httptest.NewRecorder()
	h.ServeHTTP(recLaunch, reqLaunch)
	if recLaunch.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for outside-workspace /api/launch, got %d (%s)", recLaunch.Code, recLaunch.Body.String())
	}

	// 4. Arbitrary host file on /api/stage_file -> 403
	outsideTxt := filepath.Join(t.TempDir(), "id_rsa")
	_ = os.WriteFile(outsideTxt, []byte("PRIVATE"), 0644)
	reqStage := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8088/api/stage_file", strings.NewReader(`{"path":`+`"`+filepath.ToSlash(outsideTxt)+`"`+`}`))
	reqStage.Header.Set("Authorization", "Bearer "+tok)
	reqStage.Header.Set("Content-Type", "application/json")
	recStage := httptest.NewRecorder()
	h.ServeHTTP(recStage, reqStage)
	if recStage.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for arbitrary host path on /api/stage_file, got %d (%s)", recStage.Code, recStage.Body.String())
	}

	// 5. Malformed/Janus upload on /api/upload -> 422 rejected before launch
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("apk", "bad.apk")
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, _ := zw.Create("../../evil.txt")
	_, _ = w.Write([]byte("x"))
	_ = zw.Close()
	_, _ = fw.Write(zipBuf.Bytes())
	_ = mw.Close()

	reqUp := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8088/api/upload", &body)
	reqUp.Header.Set("Authorization", "Bearer "+tok)
	reqUp.Header.Set("Content-Type", mw.FormDataContentType())
	recUp := httptest.NewRecorder()
	h.ServeHTTP(recUp, reqUp)
	if recUp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for zip-slip APK upload, got %d (%s)", recUp.Code, recUp.Body.String())
	}

	// 6. Authenticated /api/security succeeds -> 200
	reqSec := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8088/api/security", nil)
	reqSec.Header.Set("Authorization", "Bearer "+tok)
	recSec := httptest.NewRecorder()
	h.ServeHTTP(recSec, reqSec)
	if recSec.Code != http.StatusOK {
		t.Fatalf("expected 200 for authenticated /api/security, got %d", recSec.Code)
	}

	// 7. Embedded modular frontend (/) and /static/app.js + shared /api/v1/health
	reqHome := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8088/", nil)
	recHome := httptest.NewRecorder()
	h.ServeHTTP(recHome, reqHome)
	if recHome.Code != http.StatusOK || !strings.Contains(recHome.Body.String(), "NANO-ANDROID STUDIO") {
		t.Fatalf("expected modular Studio HTML shell, got code=%d", recHome.Code)
	}

	reqJS := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8088/static/app.js", nil)
	reqJS.Header.Set("Authorization", "Bearer "+tok)
	recJS := httptest.NewRecorder()
	h.ServeHTTP(recJS, reqJS)
	if recJS.Code != http.StatusOK || !strings.Contains(recJS.Body.String(), "differential-lab") || !strings.Contains(recJS.Body.String(), "database-inspector") || !strings.Contains(recJS.Body.String(), "/api/v1/ai/explore") || !strings.Contains(recJS.Body.String(), "/api/v1/graph/build") || !strings.Contains(recJS.Body.String(), "/api/v1/pools/shard") || !strings.Contains(recJS.Body.String(), "/api/v1/reports/unified") {
		t.Fatalf("expected modular app.js with all 19 workspaces + Phase 5 & Phase 6 endpoints, got code=%d", recJS.Code)
	}

	reqV1 := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8088/api/v1/health", nil)
	reqV1.Header.Set("Authorization", "Bearer "+tok)
	recV1 := httptest.NewRecorder()
	h.ServeHTTP(recV1, reqV1)
	if recV1.Code != http.StatusOK || !strings.Contains(recV1.Body.String(), `"api_version":"v1"`) {
		t.Fatalf("expected 200 JSON from shared /api/v1/health, got code=%d body=%s", recV1.Code, recV1.Body.String())
	}
}
