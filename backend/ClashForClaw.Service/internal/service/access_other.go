//go:build !windows

package service

import "clash-for-claw-service/internal/runtime"

func ensureServiceAccess(paths runtime.Paths) error {
	_ = paths
	return nil
}

