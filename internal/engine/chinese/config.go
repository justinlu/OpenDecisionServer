package chinese

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const SourceSHA256 = "832db0699007f1be95a7e41ef12e88926b02ba836461e36a36372db2760c1a2e"
const ModelSHA256 = "4f902f73781f87cc8388617bc7e2e99eb14fdeb8551f7ebb48ffbde69394cc02"

type Options struct {
	ModelPath, ConfigDir, LibraryPath    string
	Threads, MaxWaiting, MaxQueuedTokens int
}
type config struct {
	Temperature   float64 `json:"temperature"`
	ModelName     string  `json:"model_name"`
	PromptVersion string  `json:"prompt_version"`
	MaxLength     int     `json:"max_length"`
	MaxOptions    int     `json:"max_options"`
}

var pinned = map[string]string{
	"decision_config.json":  "7e5dacec4c914dfb13c6582d23f423272a24bc631788d591f6585a5a7f16b7d9",
	"chat_template.jinja":   "273d8e0e683b885071fb17e08d71e5f2a5ddfb5309756181681de4f5a1822d80",
	"tokenizer.json":        "a5cd9732badce41de57e6efce8302930ded1c1188c5f81feb2bd6c24c4a1941f",
	"tokenizer_config.json": "fb6a894aa514307c68e32d1481080013ad389c038bd1c8781720deae114eccb8",
}

func fileHash(path string) (string, error) {
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
func checkHash(path, want string) error {
	got, err := fileHash(path)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("SHA-256 mismatch for %s", path)
	}
	return nil
}
func loadConfig(opts Options) (config, error) {
	var c config
	for name, hash := range pinned {
		if err := checkHash(filepath.Join(opts.ConfigDir, name), hash); err != nil {
			return c, err
		}
	}
	data, err := os.ReadFile(filepath.Join(opts.ConfigDir, "decision_config.json"))
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(data, &c); err != nil {
		return c, err
	}
	if c.ModelName != "chinese-decision-2b" || c.PromptVersion != "hmm-zh-v1" || c.MaxLength != 16384 || c.MaxOptions != 26 {
		return c, fmt.Errorf("unsupported decision configuration")
	}
	if err = checkHash(opts.ModelPath, ModelSHA256); err != nil {
		return c, err
	}
	return c, nil
}
func New(opts Options) (*Engine, error) {
	if err := nativeBuildError(); err != nil {
		return nil, err
	}
	if opts.Threads == 0 {
		opts.Threads = 4
	}
	if opts.MaxQueuedTokens == 0 {
		opts.MaxQueuedTokens = 65536
	}
	if opts.Threads < 1 || opts.MaxWaiting < 0 || opts.MaxQueuedTokens < 1 {
		return nil, fmt.Errorf("invalid Chinese Decision configuration")
	}
	if opts.ConfigDir == "" {
		opts.ConfigDir = filepath.Dir(opts.ModelPath)
	}
	c, err := loadConfig(opts)
	if err != nil {
		return nil, fmt.Errorf("verify Chinese Decision artifacts: %w", err)
	}
	n, err := openNative(opts)
	if err != nil {
		return nil, err
	}
	for i := 0; i < 26; i++ {
		ids, err := n.Tokenize(string(rune('A' + i)))
		if err != nil || len(ids) != 1 || ids[0] != int32(32+i) {
			n.Close()
			return nil, fmt.Errorf("candidate tokenizer mismatch for %c", 'A'+i)
		}
	}
	return newEngine(n, c.Temperature, opts.MaxWaiting, opts.MaxQueuedTokens), nil
}
