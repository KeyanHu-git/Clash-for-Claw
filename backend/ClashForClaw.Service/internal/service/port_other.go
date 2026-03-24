//go:build !windows

package service

func ensureServicePortFree(string, string) error {
	return nil
}
