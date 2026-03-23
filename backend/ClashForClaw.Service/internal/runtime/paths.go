package runtime

import (
	"os"
	"path/filepath"
)

const (
	AppFolderName       = "ClashForClaw"
	legacyAppFolderName = "OpenClawAdapter"
	BaseDirEnvVar       = "CLASH_FOR_CLAW_BASE_DIR"
	LogDirEnvVar        = "CLASH_FOR_CLAW_LOG_DIR"
)

type Paths struct {
	BaseDir    string
	ConfigPath string
	RuntimeDir string
	BinDir     string
	LogsDir    string
	MihomoDir  string
}

func ResolvePaths(baseOverride string) (Paths, error) {
	return resolvePaths(baseOverride, false)
}

func ResolveServicePaths(baseOverride string) (Paths, error) {
	return resolvePaths(baseOverride, true)
}

func PeekPaths(baseOverride string) Paths {
	return buildPaths(resolveBaseDir(baseOverride, false))
}

func PeekServicePaths(baseOverride string) Paths {
	return buildPaths(resolveBaseDir(baseOverride, true))
}

func resolvePaths(baseOverride string, serviceMode bool) (Paths, error) {
	base := resolveBaseDir(baseOverride, serviceMode)
	purgeLegacyBaseDir(serviceMode)
	paths := buildPaths(base)
	if err := os.MkdirAll(paths.RuntimeDir, 0o755); err != nil {
		return Paths{}, err
	}
	_ = os.MkdirAll(paths.BinDir, 0o755)
	_ = os.MkdirAll(paths.LogsDir, 0o755)
	_ = os.MkdirAll(paths.MihomoDir, 0o755)
	return paths, nil
}

func buildPaths(base string) Paths {
	logsDir := filepath.Join(base, "logs")
	if envLogs := os.Getenv(LogDirEnvVar); envLogs != "" {
		if trimmed := filepath.Clean(envLogs); trimmed != "." {
			logsDir = trimmed
		}
	}

	paths := Paths{
		BaseDir:    base,
		ConfigPath: filepath.Join(base, "config.json"),
		RuntimeDir: filepath.Join(base, "runtime"),
		BinDir:     filepath.Join(base, "bin"),
		LogsDir:    logsDir,
		MihomoDir:  filepath.Join(base, "mihomo"),
	}
	return paths
}

func resolveBaseDir(baseOverride string, serviceMode bool) string {
	if trimmed := filepath.Clean(baseOverride); baseOverride != "" && trimmed != "." {
		return trimmed
	}
	if envBase := os.Getenv(BaseDirEnvVar); envBase != "" {
		if trimmed := filepath.Clean(envBase); trimmed != "." {
			return trimmed
		}
	}
	if serviceMode {
		programData := os.Getenv("ProgramData")
		if trimmed := filepath.Clean(programData); programData != "" && trimmed != "." {
			return filepath.Join(trimmed, AppFolderName)
		}
	}
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = "."
	}
	return filepath.Join(base, AppFolderName)
}

func purgeLegacyBaseDir(serviceMode bool) {
	legacy := legacyBaseDir(serviceMode)
	if legacy == "" {
		return
	}
	_ = os.RemoveAll(legacy)
}

func legacyBaseDir(serviceMode bool) string {
	if serviceMode {
		programData := os.Getenv("ProgramData")
		if trimmed := filepath.Clean(programData); programData != "" && trimmed != "." {
			return filepath.Join(trimmed, legacyAppFolderName)
		}
	}
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = "."
	}
	return filepath.Join(base, legacyAppFolderName)
}
