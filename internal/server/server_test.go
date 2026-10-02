package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"marketlab/internal/sim"
)

func TestHealthSessionReuseAndIsolation(t *testing.T) {
	app, base := newTestServer(t, Options{})
	clientA := newClient(t)
	clientB := newClient(t)

	response, err := clientA.Get(base + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", response.StatusCode)
	}

	a := getSnapshot(t, clientA, base)
	if again := getSnapshot(t, clientA, base); again.SessionID != a.SessionID {
		t.Fatalf("cookie did not reuse session: %q != %q", again.SessionID, a.SessionID)
	}
	b := getSnapshot(t, clientB, base)
	if a.SessionID == b.SessionID {
		t.Fatal("independent visitors shared a session")
	}

	stepped := sendCommand(t, clientA, base, a.SessionID, "step-a", map[string]any{"type": "simulation.step"})
	if stepped.LogicalTime <= a.LogicalTime {
		t.Fatalf("logical time did not advance: %d -> %d", a.LogicalTime, stepped.LogicalTime)
	}
	if isolated := getSnapshot(t, clientB, base); isolated.LogicalTime != b.LogicalTime {
		t.Fatalf("visitor B changed with A: %d -> %d", b.LogicalTime, isolated.LogicalTime)
	}
	if stepped.Seq <= a.Seq {
		t.Fatalf("transport sequence did not advance: %d -> %d", a.Seq, stepped.Seq)
	}
	_ = app
}

func TestCommandEnvelopeValidationAndIdempotency(t *testing.T) {
	_, base := newTestServer(t, Options{})
	client := newClient(t)
	initial := getSnapshot(t, client, base)

	first := sendCommand(t, client, base, initial.SessionID, "same-request", map[string]any{"type": "simulation.step"})
	second := sendCommand(t, client, base, initial.SessionID, "same-request", map[string]any{"type": "simulation.step"})
	if first.LogicalTime != second.LogicalTime || first.Seq != second.Seq {
		t.Fatalf("duplicate request executed twice: first=%d/%d second=%d/%d", first.LogicalTime, first.Seq, second.LogicalTime, second.Seq)
	}

	response := commandRequest(t, client, base, map[string]any{
		"sessionId": initial.SessionID,
		"requestId": "bad-speed",
		"command":   map[string]any{"type": "simulation.speed", "speed": 99},
	})
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnprocessableEntity {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("invalid speed status = %d body=%s", response.StatusCode, body)
	}
	var rejected commandResponse
	if err := json.NewDecoder(response.Body).Decode(&rejected); err != nil {
		t.Fatal(err)
	}
	if rejected.Accepted || rejected.RequestID != "bad-speed" || rejected.Reason == "" {
		t.Fatalf("unexpected rejection: %+v", rejected)
	}

	other := newClient(t)
	_ = getSnapshot(t, other, base)
	response = commandRequest(t, other, base, map[string]any{
		"sessionId": initial.SessionID,
		"requestId": "stolen",
		"command":   map[string]any{"type": "simulation.pause"},
	})
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-visitor command status = %d", response.StatusCode)
	}
}

func TestFlatFrontendCommandCompatibility(t *testing.T) {
	_, base := newTestServer(t, Options{})
	client := newClient(t)
	initial := getSnapshot(t, client, base)
	response := commandRequest(t, client, base, map[string]any{"type": "simulation.step"})
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d body=%s", response.StatusCode, body)
	}
	var result commandResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if !result.Accepted || result.Snapshot == nil || result.Snapshot.LogicalTime <= initial.LogicalTime {
		t.Fatalf("unexpected response: %+v", result)
	}
}

func TestWebSocketSnapshotReconnectAndOrigin(t *testing.T) {
	_, base := newTestServer(t, Options{})
	client := newClient(t)
	initial := getSnapshot(t, client, base)
	wsURL := "ws" + strings.TrimPrefix(base, "http") + "/ws?session=" + url.QueryEscape(initial.SessionID)

	connection, response, err := websocket.Dial(context.Background(), wsURL, &websocket.DialOptions{HTTPClient: client})
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		t.Fatalf("dial websocket (status %d): %v", status, err)
	}
	var first serverMessage
	readContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := wsjson.Read(readContext, connection, &first); err != nil {
		t.Fatal(err)
	}
	if first.Type != "snapshot" || first.Snapshot == nil || first.Snapshot.SessionID != initial.SessionID {
		t.Fatalf("unexpected initial websocket message: %+v", first)
	}
	_ = connection.Close(websocket.StatusNormalClosure, "reconnect")

	stepped := sendCommand(t, client, base, initial.SessionID, "ws-step", map[string]any{"type": "simulation.step"})
	connection, _, err = websocket.Dial(context.Background(), wsURL, &websocket.DialOptions{HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	var reconnected serverMessage
	readContext, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := wsjson.Read(readContext, connection, &reconnected); err != nil {
		t.Fatal(err)
	}
	if reconnected.Snapshot == nil || reconnected.Snapshot.Seq != stepped.Seq || reconnected.Snapshot.SessionID != stepped.SessionID {
		t.Fatalf("reconnect was not authoritative: got %+v want %s/%d", reconnected.Snapshot, stepped.SessionID, stepped.Seq)
	}

	headers := http.Header{"Origin": []string{"https://attacker.invalid"}}
	badConnection, response, err := websocket.Dial(context.Background(), wsURL, &websocket.DialOptions{HTTPClient: client, HTTPHeader: headers})
	if badConnection != nil {
		badConnection.CloseNow()
	}
	if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin dial err=%v response=%v", err, response)
	}
}

func TestSessionCapacityAndCleanup(t *testing.T) {
	app, base := newTestServer(t, Options{MaxSessions: 2, SessionTTL: 30 * time.Millisecond, CleanupInterval: 5 * time.Millisecond})
	first := newClient(t)
	second := newClient(t)
	third := newClient(t)
	_ = getSnapshot(t, first, base)
	_ = getSnapshot(t, second, base)
	response, err := third.Get(base + "/api/session")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("capacity status = %d", response.StatusCode)
	}

	time.Sleep(100 * time.Millisecond)
	if snapshot := getSnapshot(t, third, base); snapshot.SessionID == "" {
		t.Fatal("capacity did not recover after cleanup")
	}
	_ = app
}

func TestMultipartImportAndExport(t *testing.T) {
	_, base := newTestServer(t, Options{})
	client := newClient(t)
	snapshot := getSnapshot(t, client, base)
	exported, err := client.Get(base + "/api/export?session=" + url.QueryEscape(snapshot.SessionID))
	if err != nil {
		t.Fatal(err)
	}
	experiment, err := io.ReadAll(exported.Body)
	exported.Body.Close()
	if err != nil || exported.StatusCode != http.StatusOK {
		t.Fatalf("export status=%d err=%v", exported.StatusCode, err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("session", "experiment.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(experiment); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodPost, base+"/api/import", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("import status=%d body=%s", response.StatusCode, payload)
	}
	var imported sim.Snapshot
	if err := json.NewDecoder(response.Body).Decode(&imported); err != nil {
		t.Fatal(err)
	}
	if imported.SessionID != snapshot.SessionID || imported.Mode != "REPLAY" {
		t.Fatalf("unexpected imported snapshot: %+v", imported)
	}
}

func TestSPAAndMissingAsset(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":    {Data: []byte("<h1>MarketLab</h1>")},
		"assets/app.js": {Data: []byte("console.log('ok')")},
	}
	app := New(assets)
	defer app.Close()
	handler := app.Handler()
	for _, target := range []string{"/", "/portfolio"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "MarketLab") {
			t.Fatalf("SPA target %s status=%d body=%s", target, recorder.Code, recorder.Body.String())
		}
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing asset status = %d", recorder.Code)
	}
}

func newTestServer(t *testing.T, options Options) (*Server, string) {
	t.Helper()
	app := NewWithOptions(nil, options)
	httpServer := httptest.NewServer(app.Handler())
	t.Cleanup(func() {
		httpServer.Close()
		app.Close()
	})
	return app, httpServer.URL
}

func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}

func getSnapshot(t *testing.T, client *http.Client, base string) sim.Snapshot {
	t.Helper()
	response, err := client.Get(base + "/api/session")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("session status=%d body=%s", response.StatusCode, body)
	}
	var snapshot sim.Snapshot
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func sendCommand(t *testing.T, client *http.Client, base, sessionID, requestID string, command any) sim.Snapshot {
	t.Helper()
	response := commandRequest(t, client, base, map[string]any{"sessionId": sessionID, "requestId": requestID, "command": command})
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("command status=%d body=%s", response.StatusCode, body)
	}
	var result commandResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if !result.Accepted || result.Snapshot == nil {
		t.Fatalf("command rejected: %+v", result)
	}
	return *result.Snapshot
}

func commandRequest(t *testing.T, client *http.Client, base string, payload any) *http.Response {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodPost, base+"/api/command", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
