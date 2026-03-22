package mihomo

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	adapterRuntime "clash-for-claw-service/internal/runtime"
)

const mihomoExecutableName = "mihomo.exe"

var errMihomoBinaryNotFound = errors.New("mihomo_binary_not_found")

type binaryResolution struct {
	Path          string
	CopyToManaged bool
}

func resolveBinaryPath(paths adapterRuntime.Paths, executablePath string, getenv func(string) string) (binaryResolution, error) {
	if path, err := resolveConfiguredBinary(getenv); err != nil {
		return binaryResolution{}, err
	} else if path != "" {
		return binaryResolution{Path: path}, nil
	}

	managedPath := filepath.Join(paths.BinDir, mihomoExecutableName)
	if isUsableBinary(managedPath) {
		return binaryResolution{Path: managedPath}, nil
	}

	for _, candidate := range sidecarCandidates(executablePath) {
		if sameFilePath(candidate, managedPath) {
			continue
		}
		if isUsableBinary(candidate) {
			return binaryResolution{
				Path:          candidate,
				CopyToManaged: true,
			}, nil
		}
	}

	return binaryResolution{}, errMihomoBinaryNotFound
}

func resolveConfiguredBinary(getenv func(string) string) (string, error) {
	for _, name := range []string{"CLASH_FOR_CLAW_MIHOMO_PATH", "OPENCLAW_ADAPTER_MIHOMO_PATH"} {
		raw := strings.TrimSpace(getenv(name))
		if raw == "" {
			continue
		}
		path := filepath.Clean(raw)
		if !isUsableBinary(path) {
			return "", fmt.Errorf("mihomo_binary_invalid:%s", name)
		}
		return path, nil
	}
	return "", nil
}

func sidecarCandidates(executablePath string) []string {
	if strings.TrimSpace(executablePath) == "" {
		return nil
	}

	exeDir := filepath.Dir(executablePath)
	return uniquePaths(
		filepath.Join(exeDir, "bin", mihomoExecutableName),
		filepath.Join(exeDir, mihomoExecutableName),
	)
}

func ensureManagedCopy(sourcePath string, managedPath string) (string, error) {
	if sameFilePath(sourcePath, managedPath) {
		return managedPath, nil
	}
	if isUsableBinary(managedPath) {
		return managedPath, nil
	}
	if err := os.MkdirAll(filepath.Dir(managedPath), 0o755); err != nil {
		return "", err
	}
	if err := copyFile(sourcePath, managedPath); err != nil {
		return "", err
	}
	return managedPath, nil
}

func copyFile(sourcePath string, targetPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer target.Close()

	if _, err := io.Copy(target, source); err != nil {
		return err
	}
	return target.Close()
}

func currentExecutablePath() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	return path
}

func isUsableBinary(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Size() > minMihomoSize
}

func sameFilePath(left string, right string) bool {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func uniquePaths(values ...string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		normalized := filepath.Clean(value)
		key := strings.ToLower(normalized)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, normalized)
	}
	return out
}
