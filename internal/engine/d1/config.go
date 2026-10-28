package d1

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const BridgeSHA256 = "ba062f71247cb7f163e428dc2efd03b631dd03b5c169d913ebba5f11ac712ac7"

const ModelSHA256 = "073313d92b53b0ac8193db6e906cb3c78a98183d244b15b7b8f5426115c6738c"
const SourceSHA256 = "65b13ebdf4e03cefe58f27096e9af698dc3d41ec6f63f6cd9b4a32ce02e525cf"
const PatchSHA256 = "b57f2ca9d79d57af1aa425c4cf0f308ae2d317a89f5085cb3d9ca527f50b79ae"

type Options struct {
	ModelPath, LibraryPath                string
	Threads, MaxWaiting, MaxRequestTokens int
}

func hash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func verifySDK(path string) error {
	dir := filepath.Dir(path)
	for marker, want := range map[string]string{"source.sha256": SourceSHA256, "patch.sha256": PatchSHA256, "bridge.sha256": BridgeSHA256} {
		got, err := os.ReadFile(filepath.Join(dir, "..", marker))
		if err != nil || strings.TrimSpace(string(got)) != want {
			return fmt.Errorf("d1 SDK %s mismatch", marker)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(dir, "SHA256SUMS"))
	if err != nil {
		return err
	}
	found := false
	seen := map[string]bool{}
	for _, line := range strings.Split(string(manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 || filepath.Base(fields[1]) != fields[1] || seen[fields[1]] {
			return fmt.Errorf("invalid d1 SDK manifest")
		}
		seen[fields[1]] = true
		if fields[1] == filepath.Base(path) {
			found = true
		}
		got, err := hash(filepath.Join(dir, fields[1]))
		if err != nil || got != fields[0] {
			return fmt.Errorf("d1 SDK library hash mismatch: %s", fields[1])
		}
	}
	for _, name := range []string{"libods_d1.dylib", "libllama.dylib", "libggml.dylib", "libggml-base.dylib", "libggml-metal.dylib"} {
		if !seen[name] {
			return fmt.Errorf("d1 SDK manifest missing %s", name)
		}
	}
	if !found {
		return fmt.Errorf("d1 bridge missing from manifest")
	}
	return nil
}
func New(opts Options) (*Engine, error) {
	if err := nativeBuildError(); err != nil {
		return nil, err
	}
	if opts.Threads < 1 || opts.MaxWaiting < 0 || opts.MaxWaiting > 1024 || opts.MaxRequestTokens < 1 || opts.MaxRequestTokens > 64*8192 {
		return nil, fmt.Errorf("invalid d1 limits")
	}
	model, err := hash(opts.ModelPath)
	if err != nil || model != ModelSHA256 {
		return nil, fmt.Errorf("d1-2B Q8_0 model hash mismatch")
	}
	if err = verifySDK(opts.LibraryPath); err != nil {
		return nil, err
	}
	n, err := openNative(opts)
	if err != nil {
		return nil, err
	}
	return newEngine(n, opts.MaxWaiting, opts.MaxRequestTokens), nil
}
