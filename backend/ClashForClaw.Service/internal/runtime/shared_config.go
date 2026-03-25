package runtime

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"clash-for-claw-service/internal/config"
)

// SyncOperationalConfig mirrors the source runtime config into the target
// runtime when the source config is newer. This keeps subscription state,
// active selection, and imported subscription files aligned across hosts.
func SyncOperationalConfig(source Paths, target Paths) error {
	if SameBaseDir(source.BaseDir, target.BaseDir) || samePath(source.ConfigPath, target.ConfigPath) {
		return nil
	}

	sourceInfo, err := os.Stat(source.ConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	targetInfo, err := os.Stat(target.ConfigPath)
	if err == nil {
		// Treat the caller as the authority unless the target is definitely
		// newer. Windows can collapse adjacent writes to the same timestamp,
		// and strict source>target checks would otherwise skip legitimate
		// handoffs between desktop and service mode.
		if targetInfo.ModTime().After(sourceInfo.ModTime()) {
			return nil
		}
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	sourceCfg, err := config.LoadOrInit(source.ConfigPath)
	if err != nil {
		return err
	}

	targetCfg := *sourceCfg
	if sourceCfg.Proxy.Subscriptions != nil {
		targetCfg.Proxy.Subscriptions = append([]config.Subscription(nil), sourceCfg.Proxy.Subscriptions...)
	}
	if err := copySubscriptionFiles(&targetCfg, target); err != nil {
		return err
	}

	return config.Save(target.ConfigPath, &targetCfg)
}

func copySubscriptionFiles(cfg *config.Config, target Paths) error {
	if cfg == nil || len(cfg.Proxy.Subscriptions) == 0 {
		return nil
	}

	destDir := filepath.Join(target.RuntimeDir, "subscriptions")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	for i := range cfg.Proxy.Subscriptions {
		sub := &cfg.Proxy.Subscriptions[i]
		if strings.TrimSpace(sub.FilePath) == "" {
			continue
		}
		if _, err := os.Stat(sub.FilePath); err != nil {
			continue
		}

		name := filepath.Base(sub.FilePath)
		if sub.ID != "" {
			name = sub.ID + "-" + name
		}
		destPath := filepath.Join(destDir, name)
		if err := copyFile(sub.FilePath, destPath); err != nil {
			return fmt.Errorf("copy subscription %s: %w", sub.ID, err)
		}
		sub.FilePath = destPath
	}

	return nil
}

func copyFile(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func samePath(left string, right string) bool {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}
