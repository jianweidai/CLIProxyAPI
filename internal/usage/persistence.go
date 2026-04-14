package usage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	log "github.com/sirupsen/logrus"
)

type persistencePayload struct {
	Version    int                `json:"version"`
	ExportedAt time.Time          `json:"exported_at"`
	Usage      StatisticsSnapshot `json:"usage"`
}

type persistenceManager struct {
	mu       sync.Mutex
	path     string
	interval time.Duration
	stopCh   chan struct{}
	running  bool
	retain   time.Duration
}

var usagePersistence = &persistenceManager{}

// ConfigurePersistence enables or disables usage stats persistence based on config.
// It is safe to call multiple times; redundant calls are no-ops.
func ConfigurePersistence(cfg *config.Config, configFilePath string) {
	if cfg == nil {
		usagePersistence.stop()
		return
	}
	if !cfg.UsageStatisticsEnabled {
		usagePersistence.stop()
		return
	}
	path := resolveUsageStatisticsPath(strings.TrimSpace(cfg.UsageStatisticsFile), configFilePath)
	if path == "" {
		usagePersistence.stop()
		return
	}
	interval := time.Duration(cfg.UsageStatisticsFlushIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = time.Minute
	}
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	retain := time.Duration(cfg.UsageStatisticsRetentionDays) * 24 * time.Hour
	if retain < 0 {
		retain = 0
	}
	usagePersistence.start(path, interval, retain)
}

func resolveUsageStatisticsPath(path string, configFilePath string) string {
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

func (p *persistenceManager) start(path string, interval time.Duration, retain time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running && p.path == path && p.interval == interval && p.retain == retain {
		return
	}
	p.stopLocked()
	p.path = path
	p.interval = interval
	p.retain = retain
	p.stopCh = make(chan struct{})
	p.running = true
	go p.loop(path, interval, retain, p.stopCh)
}

func (p *persistenceManager) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
}

func (p *persistenceManager) stopLocked() {
	if !p.running {
		return
	}
	close(p.stopCh)
	p.running = false
	p.stopCh = nil
	p.path = ""
	p.retain = 0
}

func (p *persistenceManager) loop(path string, interval time.Duration, retain time.Duration, stop <-chan struct{}) {
	if err := loadSnapshot(path, retain); err != nil {
		log.WithError(err).Warn("usage: failed to load persisted statistics")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := persistSnapshot(path, retain); err != nil {
				log.WithError(err).Warn("usage: failed to persist statistics")
			}
		case <-stop:
			return
		}
	}
}

func loadSnapshot(path string, retain time.Duration) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var payload persistencePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	if payload.Version != 0 && payload.Version != 1 {
		return errors.New("unsupported usage snapshot version")
	}
	if payload.Usage.TotalRequests == 0 && len(payload.Usage.APIs) == 0 {
		return nil
	}
	stats := GetRequestStatistics()
	if stats == nil {
		return nil
	}
	stats.MergeSnapshot(payload.Usage)
	if retain > 0 {
		stats.PruneOlderThan(time.Now().Add(-retain))
	}
	return nil
}

func persistSnapshot(path string, retain time.Duration) error {
	stats := GetRequestStatistics()
	if stats == nil {
		return nil
	}
	if retain > 0 {
		stats.PruneOlderThan(time.Now().Add(-retain))
	}
	snapshot := stats.Snapshot()
	payload := persistencePayload{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		Usage:      snapshot,
	}
	data, err := json.Marshal(payload)
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
