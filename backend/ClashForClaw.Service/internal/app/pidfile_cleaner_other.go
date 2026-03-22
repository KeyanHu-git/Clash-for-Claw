//go:build !windows

package app

func runPIDFileCleaner(int, string) error { return nil }
