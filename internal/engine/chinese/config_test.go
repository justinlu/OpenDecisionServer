package chinese

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRejectWrongPinnedArtifacts(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "decision_config.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(Options{ModelPath: filepath.Join(dir, "wrong.gguf"), ConfigDir: dir}); err == nil {
		t.Fatal("wrong artifacts accepted")
	}
}
