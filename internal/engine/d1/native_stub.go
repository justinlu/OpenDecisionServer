//go:build !llama || !cgo || ((!darwin || !arm64) && !linux)

package d1

import "fmt"

func nativeBuildError() error {
	return fmt.Errorf("d1-2B requires cgo on darwin/arm64 or linux: see Dockerfile or scripts/build-d1.sh")
}
func openNative(Options) (native, error) { return nil, nativeBuildError() }
