//go:build darwin || linux

package httpapi

import (
	"fmt"
	"runtime"
	"syscall"
)

func processMetrics() string {
	var usage syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &usage) != nil {
		return ""
	}
	rss := usage.Maxrss
	if runtime.GOOS == "linux" {
		rss *= 1024
	}
	return fmt.Sprintf("ods_process_max_rss_bytes %d\n", rss)
}
