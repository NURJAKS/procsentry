package cleaner

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type CacheItem struct {
	Name        string  `json:"name"`
	Path        string  `json:"path"`
	Desc        string  `json:"desc"`
	SizeMB      float64 `json:"size_mb"`
	SizeDisplay string  `json:"size_display"`
}

type cacheDef struct {
	name string
	rel  string
	desc string
}

var targetCaches = []cacheDef{
	{name: "NPM Cache", rel: ".npm", desc: "Кэш загруженных пакетов Node.js"},
	{name: "Yarn Cache", rel: ".cache/yarn", desc: "Кэш пакетов Yarn"},
	{name: "Cargo / Rust Cache", rel: ".cargo/registry/cache", desc: "Архивы пакетов Crates.io"},
	{name: "Pip Cache", rel: ".cache/pip", desc: "Кэш пакетов Python Wheels"},
	{name: "Thumbnail Cache", rel: ".cache/thumbnails", desc: "Миниатюры картинок GNOME"},
	{name: "Chrome / Browser Cache", rel: ".cache/google-chrome", desc: "Кэш страниц и медиа браузера Chrome"},
}

var (
	cachedCaches []CacheItem
	cacheMutex   sync.Mutex
	lastScan     time.Time
)

func getDirSize(path string, maxDepth int) int64 {
	var size int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

// ScanCaches scans developer cache directories with 60s memory caching
func ScanCaches(force bool) []CacheItem {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	if !force && len(cachedCaches) > 0 && time.Since(lastScan) < 60*time.Second {
		return cachedCaches
	}

	home, _ := os.UserHomeDir()
	var results []CacheItem

	for _, target := range targetCaches {
		fullPath := filepath.Join(home, target.rel)
		if fi, err := os.Stat(fullPath); err == nil && fi.IsDir() {
			bytes := getDirSize(fullPath, 4)
			mb := math.Round((float64(bytes)/(1024*1024))*10) / 10
			disp := fmt.Sprintf("%.1f MB", mb)
			if mb >= 1024 {
				disp = fmt.Sprintf("%.2f GB", mb/1024.0)
			}
			results = append(results, CacheItem{
				Name:        target.name,
				Path:        fullPath,
				Desc:        target.desc,
				SizeMB:      mb,
				SizeDisplay: disp,
			})
		}
	}

	cachedCaches = results
	lastScan = time.Now()
	return results
}

// CleanCache removes contents of a target cache folder
func CleanCache(name string) error {
	cacheMutex.Lock()
	lastScan = time.Time{} // invalidate
	cacheMutex.Unlock()

	home, _ := os.UserHomeDir()
	var targetPath string
	for _, tc := range targetCaches {
		if tc.name == name {
			targetPath = filepath.Join(home, tc.rel)
			break
		}
	}
	if targetPath == "" {
		return fmt.Errorf("кэш %s не найден", name)
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		_ = os.RemoveAll(filepath.Join(targetPath, entry.Name()))
	}
	return nil
}
