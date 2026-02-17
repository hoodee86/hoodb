package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shauntso/hoodb/internal/config"
)

func newTestConfig() *config.Config {
	noSync := true
	return &config.Config{
		NodeID:       "test1",
		HTTPAddr:     ":18001",
		RaftAddr:     "127.0.0.1:19001",
		DataDir:      "./test_data",
		MaxKeySize:   64,
		MaxValueSize: 1024,
		MaxBatchSize: 10,
		NoSync:       &noSync,
	}
}

func newTestConfigWithAPIKey(key string) *config.Config {
	cfg := newTestConfig()
	cfg.APIKey = key
	return cfg
}

func TestAuthMiddleware_NoKey(t *testing.T) {
	cfg := newTestConfig()
	h := &Handler{kv: nil, config: cfg}
	engine := h.SetupRoutes()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	engine.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Error("health should not require auth")
	}
}

func TestAuthMiddleware_RequireKey(t *testing.T) {
	cfg := newTestConfigWithAPIKey("test-secret-key")
	h := &Handler{kv: nil, config: cfg}
	engine := h.SetupRoutes()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/kv/test", nil)
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without API key, got %d", w.Code)
	}

	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/v1/kv/test", nil)
	req2.Header.Set("X-API-Key", "test-secret-key")
	engine.ServeHTTP(w2, req2)
	if w2.Code == http.StatusUnauthorized {
		t.Error("expected non-401 with valid X-API-Key, still got 401")
	}

	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/api/v1/kv/test", nil)
	req3.Header.Set("Authorization", "Bearer test-secret-key")
	engine.ServeHTTP(w3, req3)
	if w3.Code == http.StatusUnauthorized {
		t.Error("expected non-401 with valid Bearer token, still got 401")
	}

	w4 := httptest.NewRecorder()
	req4, _ := http.NewRequest("GET", "/api/v1/kv/test", nil)
	req4.Header.Set("X-API-Key", "wrong-key")
	engine.ServeHTTP(w4, req4)
	if w4.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong API key, got %d", w4.Code)
	}
}

func TestSizeLimits_KeyTooLarge(t *testing.T) {
	cfg := newTestConfig()
	h := &Handler{kv: nil, config: cfg}
	engine := h.SetupRoutes()

	longKey := strings.Repeat("k", 65)
	body := `{"value": "test"}`

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/v1/kv/"+longKey, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for oversized key, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if errMsg, ok := resp["error"].(string); !ok || !strings.Contains(errMsg, "exceeds limit") {
		t.Errorf("expected 'exceeds limit' error, got: %v", resp["error"])
	}
}

func TestSizeLimits_ValueTooLarge(t *testing.T) {
	cfg := newTestConfig()
	h := &Handler{kv: nil, config: cfg}
	engine := h.SetupRoutes()

	longValue := strings.Repeat("v", 1025)
	body := `{"value": "` + longValue + `"}`

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/v1/kv/testkey", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for oversized value, got %d", w.Code)
	}
}

func TestSizeLimits_BatchTooLarge(t *testing.T) {
	cfg := newTestConfig()
	h := &Handler{kv: nil, config: cfg}
	engine := h.SetupRoutes()

	items := make(map[string]string)
	for i := 0; i < 11; i++ {
		items[strings.Repeat("k", i+1)] = "v"
	}
	itemsJSON, _ := json.Marshal(map[string]interface{}{"items": items})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/kv/batch", strings.NewReader(string(itemsJSON)))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for oversized batch, got %d", w.Code)
	}
}

func TestHealthCheckEndpoint(t *testing.T) {
	cfg := newTestConfigWithAPIKey("secret")
	h := &Handler{kv: nil, config: cfg}
	engine := h.SetupRoutes()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	engine.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Error("health check should not require auth")
	}
}

func TestCORSHeaders(t *testing.T) {
	cfg := newTestConfig()
	h := &Handler{kv: nil, config: cfg}
	engine := h.SetupRoutes()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("OPTIONS", "/api/v1/kv/test", nil)
	engine.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected CORS Allow-Origin header")
	}
	if !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "X-API-Key") {
		t.Error("expected X-API-Key in Allow-Headers")
	}
}
