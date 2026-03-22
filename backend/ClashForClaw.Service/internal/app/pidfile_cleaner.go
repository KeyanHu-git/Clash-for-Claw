package app

import (
	"fmt"
	"log"
	"strconv"
)

func tryRunPIDFileCleaner(args []string) (bool, int) {
	if len(args) == 0 || args[0] != "--pid-file-cleaner" {
		return false, 0
	}

	watchPID, pidFilePath, err := parsePIDFileCleanerArgs(args[1:])
	if err != nil {
		log.Printf("pid file cleaner args invalid: %v", err)
		return true, 2
	}
	if err := runPIDFileCleaner(watchPID, pidFilePath); err != nil {
		log.Printf("pid file cleaner failed: %v", err)
		return true, 1
	}
	return true, 0
}

func parsePIDFileCleanerArgs(args []string) (int, string, error) {
	watchPID := 0
	pidFilePath := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--watch-pid":
			if i+1 >= len(args) {
				return 0, "", fmt.Errorf("missing watch pid")
			}
			pid, err := strconv.Atoi(args[i+1])
			if err != nil || pid <= 0 {
				return 0, "", fmt.Errorf("invalid watch pid")
			}
			watchPID = pid
			i++
		case "--pid-file":
			if i+1 >= len(args) {
				return 0, "", fmt.Errorf("missing pid file")
			}
			pidFilePath = args[i+1]
			i++
		default:
			return 0, "", fmt.Errorf("unknown arg %s", args[i])
		}
	}

	if watchPID <= 0 {
		return 0, "", fmt.Errorf("watch pid required")
	}
	if pidFilePath == "" {
		return 0, "", fmt.Errorf("pid file required")
	}
	return watchPID, pidFilePath, nil
}
