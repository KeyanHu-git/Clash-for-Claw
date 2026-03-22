//go:build !windows

package mihomo

func attachKillOnCloseJob(int) (uintptr, error) { return 0, nil }

func closeJobHandle(uintptr) error { return nil }

func cleanupManagedPortOwners(string, []int, int) error { return nil }

func startPIDFileCleanerProcess(int, string) error { return nil }

func portsOwnedByPID(int, []int) (bool, string) {
	return true, ""
}

func isProcessRunning(int) bool { return true }
