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

func TestVerifySDKAcceptsPlatformManifest(t *testing.T) {
	platforms := map[string]struct {
		bridge string
		libs   []string
	}{
		"darwin": {"libods_d1.dylib", []string{"libods_d1.dylib", "libllama.dylib", "libggml.dylib", "libggml-base.dylib", "libggml-metal.dylib"}},
		"linux":  {"libods_d1.so", []string{"libods_d1.so", "libllama.so", "libggml.so", "libggml-base.so", "libggml-cpu.so", "libllama-common.so", "libmtmd.so", "libggml.so.0.27.0"}},
	}
	for platform, tc := range platforms {
		t.Run(platform, func(t *testing.T) {
			root := t.TempDir()
			for marker, value := range map[string]string{"source.sha256": SourceSHA256, "patch.sha256": PatchSHA256, "bridge.sha256": BridgeSHA256} {
				if err := os.WriteFile(filepath.Join(root, marker), []byte(value+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			lib := filepath.Join(root, "lib")
			if err := os.Mkdir(lib, 0o755); err != nil {
				t.Fatal(err)
			}
			manifest := ""
			for _, name := range tc.libs {
				data := []byte("fake " + name)
				if err := os.WriteFile(filepath.Join(lib, name), data, 0o644); err != nil {
					t.Fatal(err)
				}
				manifest += fmt.Sprintf("%x  %s\n", sha256.Sum256(data), name)
			}
			if err := os.WriteFile(filepath.Join(lib, "SHA256SUMS"), []byte(manifest), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := verifySDK(filepath.Join(lib, tc.bridge)); err != nil {
				t.Fatal(err)
			}
		})
	}
}
