package chinese

import (
	"fmt"
	"github.com/opendecisionserver/ods/internal/decision"
	"strings"
)

// Prompt is the pinned hmm-zh-v1 single-user, thinking-disabled template.
func Prompt(row decision.Input) (string, error) {
	if len(row.Options) < 2 || len(row.Options) > 26 {
		return "", fmt.Errorf("requires 2-26 candidates")
	}
	state, err := decision.ChineseText(row.State)
	if err != nil {
		return "", err
	}
	question := row.Question
	if len(row.Instructions) > 0 {
		question, err = decision.ChineseText(row.Instructions)
		if err != nil {
			return "", err
		}
	}
	var options []string
	for i, o := range row.Options {
		options = append(options, fmt.Sprintf("%c: %s — %s", 'A'+i, o.Key, o.Text))
	}
	return "<|im_start|>user\n待判断的上下文：\n" + state + "\n\n问题：\n" + question + "\n\n候选项：\n" + strings.Join(options, "\n") + "\n只返回候选项对应的字母。<|im_end|>\n<|im_start|>assistant\n<think>\n\n</think>\n\n", nil
}
