package mihomo

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	adapterRuntime "clash-for-claw-service/internal/runtime"
)

const (
	defaultReleaseAPIURL = "https://api.github.com/repos/MetaCubeX/mihomo/releases/latest"
	downloadUserAgent    = "ClashForClaw/1.0"
)

type releaseResponse struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func ensureDownloadedBinary(paths adapterRuntime.Paths, getenv func(string) string) (string, error) {
	if isAutoDownloadDisabled(getenv) {
		return "", errMihomoBinaryNotFound
	}

	managedPath := filepath.Join(paths.BinDir, mihomoExecutableName)
	if isUsableBinary(managedPath) {
		return managedPath, nil
	}

	assetURL, err := resolveReleaseAssetURL(getenv)
	if err != nil {
		return "", err
	}

	return downloadAndExtractBinary(paths, assetURL)
}

func isAutoDownloadDisabled(getenv func(string) string) bool {
	value := strings.TrimSpace(getenv("CLASH_FOR_CLAW_DISABLE_MIHOMO_AUTO_DOWNLOAD"))
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func resolveReleaseAssetURL(getenv func(string) string) (string, error) {
	override := strings.TrimSpace(getenv("CLASH_FOR_CLAW_MIHOMO_DOWNLOAD_URL"))
	if override != "" {
		return override, nil
	}

	release, err := fetchLatestRelease(getenv)
	if err != nil {
		return "", err
	}

	asset, ok := selectReleaseAsset(release.Assets, runtime.GOARCH)
	if !ok {
		return "", fmt.Errorf("mihomo_release_asset_missing:%s", runtime.GOARCH)
	}
	if strings.TrimSpace(asset.BrowserDownloadURL) == "" {
		return "", errors.New("mihomo_release_asset_missing_url")
	}

	return asset.BrowserDownloadURL, nil
}

func fetchLatestRelease(getenv func(string) string) (releaseResponse, error) {
	url := strings.TrimSpace(getenv("CLASH_FOR_CLAW_MIHOMO_RELEASE_API_URL"))
	if url == "" {
		url = defaultReleaseAPIURL
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return releaseResponse{}, fmt.Errorf("mihomo_release_query_failed:%w", err)
	}
	req.Header.Set("User-Agent", downloadUserAgent)
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return releaseResponse{}, fmt.Errorf("mihomo_release_query_failed:%w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return releaseResponse{}, fmt.Errorf("mihomo_release_query_status_%d", resp.StatusCode)
	}

	var release releaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return releaseResponse{}, fmt.Errorf("mihomo_release_parse_failed:%w", err)
	}
	return release, nil
}

func selectReleaseAsset(assets []releaseAsset, goarch string) (releaseAsset, bool) {
	preferred := preferredAssetPrefixes(goarch)
	if len(preferred) == 0 {
		return releaseAsset{}, false
	}

	for _, prefix := range preferred {
		for _, asset := range assets {
			name := strings.ToLower(strings.TrimSpace(asset.Name))
			if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".zip") {
				return asset, true
			}
		}
	}

	return releaseAsset{}, false
}

func preferredAssetPrefixes(goarch string) []string {
	switch goarch {
	case "amd64":
		return []string{
			"mihomo-windows-amd64-compatible-",
			"mihomo-windows-amd64-",
			"mihomo-windows-amd64-v1-",
		}
	case "386":
		return []string{"mihomo-windows-386-"}
	case "arm64":
		return []string{"mihomo-windows-arm64-"}
	default:
		return nil
	}
}

func downloadAndExtractBinary(paths adapterRuntime.Paths, assetURL string) (string, error) {
	if err := os.MkdirAll(paths.BinDir, 0o755); err != nil {
		return "", err
	}

	workDir := filepath.Join(paths.RuntimeDir, "downloads")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return "", err
	}

	zipPath := filepath.Join(workDir, "mihomo.zip")
	if err := downloadFile(assetURL, zipPath); err != nil {
		return "", err
	}
	defer os.Remove(zipPath)

	targetPath := filepath.Join(paths.BinDir, mihomoExecutableName)
	if err := extractZipExecutable(zipPath, targetPath); err != nil {
		return "", err
	}
	return targetPath, nil
}

func downloadFile(url string, targetPath string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("mihomo_download_failed:%w", err)
	}
	req.Header.Set("User-Agent", downloadUserAgent)

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("mihomo_download_failed:%w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mihomo_download_status_%d", resp.StatusCode)
	}

	file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("mihomo_download_failed:%w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("mihomo_download_failed:%w", err)
	}
	return file.Close()
}

func extractZipExecutable(zipPath string, targetPath string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("mihomo_extract_failed:%w", err)
	}
	defer reader.Close()

	for _, file := range reader.File {
		name := strings.ToLower(filepath.Base(file.Name))
		if !strings.HasSuffix(name, ".exe") {
			continue
		}

		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("mihomo_extract_failed:%w", err)
		}

		tmpPath := targetPath + ".tmp"
		out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			rc.Close()
			return fmt.Errorf("mihomo_extract_failed:%w", err)
		}

		_, copyErr := io.Copy(out, rc)
		closeErr := out.Close()
		rc.Close()
		if copyErr != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("mihomo_extract_failed:%w", copyErr)
		}
		if closeErr != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("mihomo_extract_failed:%w", closeErr)
		}

		if !isUsableBinary(tmpPath) {
			_ = os.Remove(tmpPath)
			return errors.New("mihomo_extract_invalid_binary")
		}

		if err := os.Rename(tmpPath, targetPath); err != nil {
			_ = os.Remove(targetPath)
			if err := os.Rename(tmpPath, targetPath); err != nil {
				_ = os.Remove(tmpPath)
				return fmt.Errorf("mihomo_extract_failed:%w", err)
			}
		}
		return nil
	}

	return errors.New("mihomo_extract_missing_executable")
}
