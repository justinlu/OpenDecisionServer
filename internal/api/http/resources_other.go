//go:build !darwin && !linux

package httpapi

func processMetrics() string { return "" }
