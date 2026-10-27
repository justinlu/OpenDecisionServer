//go:build !llama || !darwin || !arm64 || !cgo

package chinese

import "fmt"

func nativeBuildError() error {
	return fmt.Errorf("Chinese Decision requires the darwin/arm64 cgo build: run scripts/build-llama.sh")
}
func openNative(Options) (native, error) { return nil, nativeBuildError() }
