package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type Config struct {
	Roots               []string
	AutoRoots           bool
	MaxDisplayResults   int
	AutoReindexOnStart  bool
	PreviewWidthPercent int
}

func DefaultConfig() Config {
	return Config{AutoRoots: true, MaxDisplayResults: defaultDisplay, AutoReindexOnStart: false, PreviewWidthPercent: 44}
}

func LoadConfig(path string) Config {
	cfg := DefaultConfig()
	f, err := os.Open(path)
	if err != nil {
		return cfg
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "[") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		switch k {
		case "roots":
			if strings.EqualFold(v, "auto") || v == "" {
				cfg.AutoRoots = true
				cfg.Roots = nil
			} else {
				cfg.AutoRoots = false
				cfg.Roots = nil
				for _, p := range strings.Split(v, ";") {
					if p = strings.TrimSpace(p); p != "" {
						cfg.Roots = append(cfg.Roots, p)
					}
				}
			}
		case "maxdisplayresults":
			if n, err := strconv.Atoi(v); err == nil && n >= 100 && n <= 20000 {
				cfg.MaxDisplayResults = n
			}
		case "autoreindexonstart":
			if b, err := strconv.ParseBool(v); err == nil {
				cfg.AutoReindexOnStart = b
			}
		case "previewwidthpercent":
			if n, err := strconv.Atoi(v); err == nil && n >= 20 && n <= 70 {
				cfg.PreviewWidthPercent = n
			}
		}
	}
	return cfg
}

func EnsureConfig(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := `# xFile_search v0.1.28 configuration
# Portable mode: index/config/log files are kept beside xFile_search.exe when writable.
# Roots=auto indexes local fixed drives. You can also use: Roots=C:\;D:\;S:\
Roots=auto
MaxDisplayResults=2000
AutoReindexOnStart=false
PreviewWidthPercent=44
`
	return os.WriteFile(path, []byte(content), 0o644)
}

func SavePreviewWidthPercent(path string, percent int) error {
	if percent < 20 {
		percent = 20
	}
	if percent > 70 {
		percent = 70
	}
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	key := "PreviewWidthPercent="
	found := false
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trim), strings.ToLower("PreviewWidthPercent=")) {
			lines[i] = key + strconv.Itoa(percent)
			found = true
			break
		}
	}
	if !found {
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, key+strconv.Itoa(percent), "")
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\r\n")), 0o644)
}

var (
	dataPathsOnce    sync.Once
	cachedDataDir    string
	cachedIndexPath  string
	cachedConfigPath string
	cachedLogPath    string
	cachedPortable   bool
)

// DataPaths prefers a transparent portable layout beside xFile_search.exe:
//
//	xFile_search.exe
//	xFile_search.ini
//	Index\xFile_v3.index
//	Logs\xFile.log
//
// If the executable directory is read-only (for example Program Files), it
// safely falls back to %LOCALAPPDATA%\xFile_search.
func DataPaths() (dataDir, indexPath, configPath, logPath string) {
	dataPathsOnce.Do(func() {
		base := ""
		if exe, err := os.Executable(); err == nil {
			dir := filepath.Dir(exe)
			if directoryWritable(dir) {
				base = dir
				cachedPortable = true
			}
		}
		if base == "" {
			base = os.Getenv("LOCALAPPDATA")
			if base == "" {
				base, _ = os.UserConfigDir()
			}
			base = filepath.Join(base, appName)
			_ = os.MkdirAll(base, 0o755)
			cachedPortable = false
		}
		cachedDataDir = base
		cachedIndexPath = filepath.Join(base, "Index", "xFile_v3.index")
		cachedConfigPath = filepath.Join(base, "xFile_search.ini")
		cachedLogPath = filepath.Join(base, "Logs", "xFile.log")
	})
	return cachedDataDir, cachedIndexPath, cachedConfigPath, cachedLogPath
}

func directoryWritable(dir string) bool {
	if dir == "" {
		return false
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	f, err := os.CreateTemp(dir, ".xfile_write_test_*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

func UsingPortableStorage() bool {
	DataPaths()
	return cachedPortable
}

func IndexFolderPath() string {
	_, indexPath, _, _ := DataPaths()
	return filepath.Dir(indexPath)
}

func BackupFolderPath() string {
	dataDir, _, _, _ := DataPaths()
	return filepath.Join(dataDir, "Backup", "Index")
}

func legacyIndexPath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base, _ = os.UserConfigDir()
	}
	if base == "" {
		return ""
	}
	return filepath.Join(base, appName, "Data", "xFile_v3.index")
}

func legacyConfigPath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base, _ = os.UserConfigDir()
	}
	if base == "" {
		return ""
	}
	return filepath.Join(base, appName, "xFile_search.ini")
}

func formatConfigRoots(cfg Config) string {
	if cfg.AutoRoots {
		return "auto"
	}
	return fmt.Sprintf("%v", cfg.Roots)
}

func IndexerProgressPath() string {
	return filepath.Join(IndexFolderPath(), "xFile_v3.index.progress")
}

func DriveStatePath() string {
	return filepath.Join(IndexFolderPath(), "xFile_v3.drives")
}
