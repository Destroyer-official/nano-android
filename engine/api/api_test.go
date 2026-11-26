package api_test

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
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

func setupTestAPIServer(t *testing.T) (*httptest.Server, *api.Service, string) {
	t.Helper()
	t.Setenv("NANO_DEFAULT_APP", "")
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	coord := mcp_compiler.NewIntegratedCoordinator()
	t.Cleanup(func() { coord.Close() })

	svc := api.NewService(root, coord, nil, nil)
	tok, err := httpauth.GenerateBootToken()
	if err != nil {
		t.Fatal(err)
	}
	cfg := httpauth.Config{
		BindHost:  "127.0.0.1",
		AuthToken: tok,
	}
	mux := http.NewServeMux()
	svc.MountRoutes(mux)

	ts := httptest.NewServer(httpauth.Middleware(cfg, mux))
	t.Cleanup(ts.Close)
	return ts, svc, tok
}

func makeSmallPNG(col color.RGBA) string {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetRGBA(x, y, col)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestSharedV1RESTEndpointsAndAuthAndWebSocket(t *testing.T) {
	ts, svc, tok := setupTestAPIServer(t)
	client := ts.Client()

	// 1. Unauthenticated /api/v1/health -> 401
	unauthResp, err := client.Get(ts.URL + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	_ = unauthResp.Body.Close()
	if unauthResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated /api/v1/health, got %d", unauthResp.StatusCode)
	}

	authedReq := func(method, path string, body []byte) *http.Response {
		req, _ := http.NewRequest(method, ts.URL+path, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		if len(body) > 0 {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s %s failed: %v", method, path, err)
		}
		return resp
	}

	// 2. Authenticated /api/v1/health, /projects, /apks, /devices, /policies
	for _, p := range []string{"/api/v1/health", "/api/v1/projects", "/api/v1/apks", "/api/v1/devices", "/api/v1/policies"} {
		resp := authedReq(http.MethodGet, p, nil)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 on %s, got %d", p, resp.StatusCode)
		}
	}

	// 3. Static inspect & compatibility for fixtures/apks/minimal_valid.apk
	respInsp := authedReq(http.MethodGet, "/api/v1/apks/minimal_valid.apk/inspect", nil)
	_ = respInsp.Body.Close()
	if respInsp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on apk inspect, got %d", respInsp.StatusCode)
	}
	respCompat := authedReq(http.MethodGet, "/api/v1/apks/minimal_valid.apk/compatibility", nil)
	_ = respCompat.Body.Close()
	if respCompat.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on apk compatibility, got %d", respCompat.StatusCode)
	}

	// 4. WebSocket handshake WITHOUT Origin -> 403 (CSRF protection), WITH Origin -> 101 Switching Protocols
	wsAddr := strings.TrimPrefix(ts.URL, "http://")
	connBad, err := client.Transport.(*http.Transport).DialContext(t.Context(), "tcp", wsAddr)
	if err != nil {
		t.Fatalf("ws dial failed: %v", err)
	}
	keyBytes := make([]byte, 16)
	_, _ = rand.Read(keyBytes)
	wsKey := base64.StdEncoding.EncodeToString(keyBytes)
	_, _ = connBad.Write([]byte("GET /api/v1/ws HTTP/1.1\r\nHost: " + wsAddr + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nAuthorization: Bearer " + tok + "\r\nSec-WebSocket-Key: " + wsKey + "\r\nSec-WebSocket-Version: 13\r\n\r\n"))
	badStatus, _ := bufio.NewReader(connBad).ReadString('\n')
	_ = connBad.Close()
	if !strings.Contains(badStatus, "403 Forbidden") {
		t.Fatalf("expected 403 Forbidden for WS upgrade without Origin, got %q", badStatus)
	}

	conn, err := client.Transport.(*http.Transport).DialContext(t.Context(), "tcp", wsAddr)
	if err != nil {
		t.Fatalf("ws dial failed: %v", err)
	}
	defer conn.Close()
	wsReq := "GET /api/v1/ws HTTP/1.1\r\n" +
		"Host: " + wsAddr + "\r\n" +
		"Origin: http://" + wsAddr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Authorization: Bearer " + tok + "\r\n" +
		"Sec-WebSocket-Key: " + wsKey + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	_, _ = conn.Write([]byte(wsReq))
	br := bufio.NewReader(conn)
	statusLine, _ := br.ReadString('\n')
	if !strings.Contains(statusLine, "101 Switching Protocols") {
		t.Fatalf("expected 101 Switching Protocols on /api/v1/ws, got %q", statusLine)
	}
	for {
		ln, _ := br.ReadString('\n')
		if ln == "\r\n" || ln == "" {
			break
		}
	}
	// Read WS_CONNECTED frame
	hdr := make([]byte, 2)
	if _, err := br.Read(hdr); err != nil {
		t.Fatalf("failed reading initial ws frame: %v", err)
	}
	payloadLen := int(hdr[1] & 0x7f)
	frame := make([]byte, payloadLen)
	_, _ = br.Read(frame)
	if !strings.Contains(string(frame), "WS_CONNECTED") {
		t.Fatalf("expected WS_CONNECTED payload, got %q", string(frame))
	}

	// 5. Execution launch + events + lab run + reports/{id}
	launchPayload, _ := json.Marshal(map[string]any{"artifact": filepath.Join(svc.WorkspaceRoot, "tools", "prism-v.jar"), "activity": "com.prismv.ui.MainActivity"})
	respExec := authedReq(http.MethodPost, "/api/v1/executions", launchPayload)
	var execRec api.ExecutionRecord
	_ = json.NewDecoder(respExec.Body).Decode(&execRec)
	_ = respExec.Body.Close()
	if respExec.StatusCode != http.StatusCreated || execRec.ID == "" {
		t.Fatalf("expected 201 created execution, got status=%d rec=%+v", respExec.StatusCode, execRec)
	}

	respEv := authedReq(http.MethodGet, "/api/v1/executions/"+execRec.ID+"/events", nil)
	_ = respEv.Body.Close()
	if respEv.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on execution events, got %d", respEv.StatusCode)
	}

	snapBody, _ := json.Marshal(map[string]any{"name": "phase4_snap"})
	respSnap := authedReq(http.MethodPost, "/api/v1/executions/"+execRec.ID+"/snapshot", snapBody)
	_ = respSnap.Body.Close()
	if respSnap.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on snapshot save, got %d", respSnap.StatusCode)
	}
	respRest := authedReq(http.MethodPost, "/api/v1/executions/"+execRec.ID+"/restore", snapBody)
	_ = respRest.Body.Close()
	if respRest.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on snapshot restore, got %d", respRest.StatusCode)
	}
	respPD := authedReq(http.MethodPost, "/api/v1/executions/"+execRec.ID+"/process_death", nil)
	_ = respPD.Body.Close()
	if respPD.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on process_death, got %d", respPD.StatusCode)
	}

	// 6. Visual baseline approve & compare
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(svc.WorkspaceRoot, "cache", "baselines", "clean_install", "api_test_snap.png")) })
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(svc.WorkspaceRoot, "cache", "baselines", "clean_install", "api_test_snap.json")) })
	pngB64 := makeSmallPNG(color.RGBA{10, 20, 30, 255})
	appBody, _ := json.Marshal(map[string]any{"name": "api_test_snap", "profile_id": "clean_install", "approved_by": "ci_reviewer", "image_b64_png": pngB64})
	respApp := authedReq(http.MethodPost, "/api/v1/visual/approve", appBody)
	_ = respApp.Body.Close()
	if respApp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on visual/approve, got %d", respApp.StatusCode)
	}
	cmpBody, _ := json.Marshal(map[string]any{"name": "api_test_snap", "profile_id": "clean_install", "mode": "pixel_exact", "candidate_b64_png": pngB64})
	respCmp := authedReq(http.MethodPost, "/api/v1/visual/compare", cmpBody)
	_ = respCmp.Body.Close()
	if respCmp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on visual/compare, got %d", respCmp.StatusCode)
	}

	// 7. Lab run & report fetch
	runLabBody, _ := json.Marshal(map[string]any{"artifact": filepath.Join(svc.WorkspaceRoot, "tools", "prism-v.jar")})
	respLab := authedReq(http.MethodPost, "/api/v1/tests/network/run", runLabBody)
	var labOut struct {
		Report api.ReportRecord `json:"report"`
	}
	_ = json.NewDecoder(respLab.Body).Decode(&labOut)
	_ = respLab.Body.Close()
	if respLab.StatusCode != http.StatusOK || labOut.Report.ID == "" {
		t.Fatalf("expected 200 and report id from /tests/network/run, got %d", respLab.StatusCode)
	}
	respRep := authedReq(http.MethodGet, "/api/v1/reports/"+labOut.Report.ID, nil)
	_ = respRep.Body.Close()
	if respRep.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on /reports/%s, got %d", labOut.Report.ID, respRep.StatusCode)
	}
}
