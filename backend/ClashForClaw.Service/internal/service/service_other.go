//go:build !windows

package service

import (
	"errors"

	"github.com/kardianos/service"
)

var errInstallFailed = errors.New("service_install_failed")

func installTask(string, string) error { return errInstallFailed }
func uninstallTask() error             { return nil }
func startTask() error                 { return nil }
func stopTask() error                  { return nil }
func queryTaskRegistration(string) (bool, service.Status, error) {
	return false, service.StatusUnknown, nil
}

func queryServiceRegistration(string) (bool, service.Status, error) {
	return false, service.StatusUnknown, nil
}
