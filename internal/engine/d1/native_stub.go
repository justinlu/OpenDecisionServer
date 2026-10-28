//go:build !llama || !darwin || !arm64 || !cgo

package d1

import "fmt"

func nativeBuildError() error {
	return fmt.Errorf("d1-2B requires darwin/arm64 cgo: run scripts/build-d1.sh")
}
func openNative(Options) (native, error) { return nil, nativeBuildError() }
