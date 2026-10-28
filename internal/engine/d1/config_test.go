package d1

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestBridgeSourceIdentity(t *testing.T) {
	var source []byte
	for _, name := range []string{"bridge.cpp", "bridge.h", "CMakeLists.txt"} {
		data, err := os.ReadFile(filepath.Join("../../../native/d1", name))
		if err != nil {
			t.Fatal(err)
		}
		source = append(source, data...)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != BridgeSHA256 {
		t.Fatalf("bridge marker must be updated: %s", got)
	}
}
