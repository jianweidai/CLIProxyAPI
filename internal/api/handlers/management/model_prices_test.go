package management

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestGetModelPrices_NotConfigured(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	h := &Handler{
		cfg: &config.Config{},
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/model-prices", nil)

	h.GetModelPrices(c)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestPutAndGetModelPrices_Persists(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	path := filepath.Join(dir, "model-prices.json")

	h := &Handler{
		cfg: &config.Config{
			ModelPricesFile: path,
		},
	}

	body := []byte(`{"prices":{"gpt-4":{"prompt":0.03,"completion":0.06,"cache":0.01}}}`)
	recPut := httptest.NewRecorder()
	ctxPut, _ := gin.CreateTestContext(recPut)
	ctxPut.Request = httptest.NewRequest(http.MethodPut, "/v0/management/model-prices", bytes.NewReader(body))
	ctxPut.Request.Header.Set("Content-Type", "application/json")

	h.PutModelPrices(ctxPut)

	if recPut.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want %d; body=%s", recPut.Code, http.StatusOK, recPut.Body.String())
	}

	recGet := httptest.NewRecorder()
	ctxGet, _ := gin.CreateTestContext(recGet)
	ctxGet.Request = httptest.NewRequest(http.MethodGet, "/v0/management/model-prices", nil)

	h.GetModelPrices(ctxGet)

	if recGet.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d; body=%s", recGet.Code, http.StatusOK, recGet.Body.String())
	}

	var resp struct {
		Prices map[string]modelPrice `json:"prices"`
	}
	if err := json.Unmarshal(recGet.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	price, ok := resp.Prices["gpt-4"]
	if !ok {
		t.Fatalf("missing gpt-4 price in response")
	}
	if price.Prompt != 0.03 || price.Completion != 0.06 || price.Cache != 0.01 {
		t.Fatalf("unexpected price values: %+v", price)
	}
}
