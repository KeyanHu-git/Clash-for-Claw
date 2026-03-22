//go:build !windows

package systemproxy

import "clash-for-claw-service/internal/runtime"

type Manager struct{}

func NewManager(_ runtime.Paths) *Manager { return &Manager{} }

func (m *Manager) Enable(_ string) error  { return nil }
func (m *Manager) Disable() error         { return nil }
func (m *Manager) Status() (bool, string) { return false, "" }

