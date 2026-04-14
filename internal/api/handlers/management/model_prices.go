package management

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

type modelPrice struct {
	Prompt     float64 `json:"prompt"`
	Completion float64 `json:"completion"`
	Cache      float64 `json:"cache"`
}

type modelPricesPayload struct {
	Version   int                   `json:"version"`
	UpdatedAt time.Time             `json:"updated_at"`
	Prices    map[string]modelPrice `json:"prices"`
}

// GetModelPrices returns persisted model prices for the management UI.
func (h *Handler) GetModelPrices(c *gin.Context) {
	path := resolveModelPricesPath(h.cfg, h.configFilePath)
	if path == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "model prices persistence not configured"})
		return
	}
	h.modelPricesMu.Lock()
	defer h.modelPricesMu.Unlock()

	prices, err := readModelPrices(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"prices": prices})
}

// PutModelPrices persists model prices for the management UI.
func (h *Handler) PutModelPrices(c *gin.Context) {
	path := resolveModelPricesPath(h.cfg, h.configFilePath)
	if path == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "model prices persistence not configured"})
		return
	}

	prices, err := parseModelPricesRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.modelPricesMu.Lock()
	defer h.modelPricesMu.Unlock()

	if errWrite := writeModelPrices(path, prices); errWrite != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": errWrite.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"prices": prices})
}

func resolveModelPricesPath(cfg *config.Config, configFilePath string) string {
	if cfg == nil {
		return ""
	}
	path := strings.TrimSpace(cfg.ModelPricesFile)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	base := strings.TrimSpace(configFilePath)
	if base == "" {
		return path
	}
	info, err := os.Stat(base)
	if err == nil && !info.IsDir() {
		base = filepath.Dir(base)
	}
	if base == "" {
		return path
	}
	return filepath.Join(base, path)
}

func readModelPrices(path string) (map[string]modelPrice, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]modelPrice{}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return map[string]modelPrice{}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return map[string]modelPrice{}, nil
	}
	if pricesRaw, ok := raw["prices"]; ok {
		if pricesMap, ok := pricesRaw.(map[string]any); ok {
			return normalizeModelPrices(pricesMap), nil
		}
	}
	if pricesRaw, ok := raw["model-prices"]; ok {
		if pricesMap, ok := pricesRaw.(map[string]any); ok {
			return normalizeModelPrices(pricesMap), nil
		}
	}
	return normalizeModelPrices(raw), nil
}

func writeModelPrices(path string, prices map[string]modelPrice) error {
	if prices == nil {
		prices = map[string]modelPrice{}
	}
	payload := modelPricesPayload{
		Version:   1,
		UpdatedAt: time.Now().UTC(),
		Prices:    prices,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func parseModelPricesRequest(c *gin.Context) (map[string]modelPrice, error) {
	var raw map[string]any
	if err := c.ShouldBindJSON(&raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return map[string]modelPrice{}, nil
	}
	if pricesRaw, ok := raw["prices"]; ok {
		if pricesMap, ok := pricesRaw.(map[string]any); ok {
			return normalizeModelPrices(pricesMap), nil
		}
	}
	if pricesRaw, ok := raw["model-prices"]; ok {
		if pricesMap, ok := pricesRaw.(map[string]any); ok {
			return normalizeModelPrices(pricesMap), nil
		}
	}
	return normalizeModelPrices(raw), nil
}

func normalizeModelPrices(raw map[string]any) map[string]modelPrice {
	out := make(map[string]modelPrice)
	for name, value := range raw {
		model := strings.TrimSpace(name)
		if model == "" {
			continue
		}
		obj, ok := value.(map[string]any)
		if !ok {
			continue
		}
		prompt, okPrompt := parsePrice(obj["prompt"])
		completion, okCompletion := parsePrice(obj["completion"])
		cache, okCache := parsePrice(obj["cache"])
		if !okPrompt && !okCompletion && !okCache {
			continue
		}
		if prompt < 0 {
			prompt = 0
		}
		if completion < 0 {
			completion = 0
		}
		if cache < 0 {
			cache = 0
		}
		if !okCache {
			if okPrompt {
				cache = prompt
				okCache = true
			} else {
				cache = 0
			}
		}
		if !okPrompt {
			prompt = 0
		}
		if !okCompletion {
			completion = 0
		}
		out[model] = modelPrice{
			Prompt:     prompt,
			Completion: completion,
			Cache:      cache,
		}
	}
	return out
}

func parsePrice(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		if !isFinite(v) {
			return 0, false
		}
		return v, true
	case float32:
		f := float64(v)
		if !isFinite(f) {
			return 0, false
		}
		return f, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint64:
		return float64(v), true
	case uint32:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		if err != nil || !isFinite(f) {
			return 0, false
		}
		return f, true
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(text, 64)
		if err != nil || !isFinite(f) {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func isFinite(v float64) bool {
	return !math.IsInf(v, 0) && !math.IsNaN(v)
}
