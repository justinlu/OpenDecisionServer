//go:build !llama || !darwin || !arm64 || !cgo

package chinese

import (
	"strings"
	"testing"
)

func TestOrdinaryBuildExplainsNativeUnavailable(t *testing.T) {
	_, err := New(Options{})
	if err == nil || !strings.Contains(err.Error(), "cgo build") {
		t.Fatalf("unexpected startup error: %v", err)
	}
}
