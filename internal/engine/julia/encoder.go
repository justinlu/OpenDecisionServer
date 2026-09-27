package julia

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/opendecisionserver/ods/internal/decision"
	sp "github.com/tggo/goSentencePiece"
)

const (
	padID    = 0
	eosID    = 1
	bosID    = 2
	markerID = 4
)

type Encoded struct {
	IDs     []int64
	Markers []int64
	QType   int64
}

type Encoder struct {
	tokenizer  *sp.Tokenizer
	MaxLength  int
	HeadLength int
}

func NewEncoder(tokenizerPath string, maxLength, headLength int) (*Encoder, error) {
	if maxLength <= 0 || headLength+4 >= maxLength {
		return nil, fmt.Errorf("invalid Julia sequence budget")
	}
	tok, err := sp.NewTokenizerFromJSON(tokenizerPath)
	if err != nil {
		return nil, err
	}
	if tok.Model().BosID() != bosID || tok.Model().EosID() != eosID || tok.Model().PadID() != padID {
		return nil, fmt.Errorf("unexpected Julia tokenizer special tokens")
	}
	return &Encoder{tokenizer: tok, MaxLength: maxLength, HeadLength: headLength}, nil
}

func (e *Encoder) tokenize(s string) ([]int64, error) {
	ids, err := e.tokenizer.Encode(s)
	if err != nil {
		return nil, err
	}
	out := make([]int64, len(ids))
	for i, v := range ids {
		out[i] = int64(v)
	}
	return out, nil
}

func stateText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("missing state")
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", err
	}
	switch v.(type) {
	case string:
		return v.(string), nil
	case map[string]any, []any:
		b, err := json.Marshal(v)
		return string(b), err
	default:
		return "", fmt.Errorf("state must be string, object, or array")
	}
}

// Encode ports the strict Julia-1 encoder's sequence layout to Go. The
// tokenizer must be verified against the pinned upstream parity fixtures.
func (e *Encoder) Encode(row decision.Input) (Encoded, error) {
	var result Encoded
	switch row.Type {
	case decision.Choice:
		result.QType = 0
	case decision.Score:
		result.QType = 1
	case decision.Noul:
		result.QType = 2
	default:
		return result, fmt.Errorf("invalid question type")
	}
	if len(row.Options) < 2 || len(row.Options) > 20 || (row.Type == decision.Noul && len(row.Options) != 2) {
		return result, fmt.Errorf("options must contain 2-20 entries")
	}
	state, err := stateText(row.State)
	if err != nil {
		return result, err
	}
	if strings.Contains(state, "<mask>") || strings.Contains(row.Question, "<mask>") {
		return result, fmt.Errorf("reserved model marker in request")
	}
	head, err := e.tokenize(string(row.Type) + " question: " + row.Question)
	if err != nil {
		return result, err
	}
	options := make([][]int64, len(row.Options))
	originalLengths := make([]int, len(row.Options))
	totalOptions := 0
	for i, option := range row.Options {
		if option.Text == "" || strings.Contains(option.Text, "<mask>") {
			return result, fmt.Errorf("invalid option text")
		}
		ids, err := e.tokenize(" " + option.Text)
		if err != nil {
			return result, err
		}
		if len(ids) > 48 {
			return result, fmt.Errorf("option exceeds 48-token model contract")
		}
		originalLengths[i] = len(ids) + 1
		options[i] = append([]int64{markerID}, ids...)
		totalOptions += len(options[i])
	}
	budget := e.HeadLength - totalOptions
	if budget < 16 {
		cap := (e.HeadLength - 16) / len(options)
		if cap < 4 {
			cap = 4
		}
		for i := range options {
			if len(options[i]) > cap {
				options[i] = options[i][:cap]
			}
		}
		budget = e.HeadLength
		for _, option := range options {
			budget -= len(option)
		}
	}
	if len(head) > budget {
		return result, fmt.Errorf("question/options exceed lossless head budget")
	}
	// Strict encoding disallows loss of any option tokens.
	for i := range options {
		if len(options[i]) != originalLengths[i] {
			return result, fmt.Errorf("question/options exceed lossless head budget")
		}
	}
	result.IDs = make([]int64, 0, e.MaxLength)
	result.IDs = append(result.IDs, bosID)
	result.IDs = append(result.IDs, head...)
	result.IDs = append(result.IDs, eosID)
	for _, option := range options {
		result.Markers = append(result.Markers, int64(len(result.IDs)))
		result.IDs = append(result.IDs, option...)
	}
	result.IDs = append(result.IDs, eosID)
	stateIDs, err := e.tokenize(state)
	if err != nil {
		return result, err
	}
	room := e.MaxLength - len(result.IDs) - 1
	if room < 1 || len(stateIDs) > room {
		return result, fmt.Errorf("state exceeds lossless context budget")
	}
	result.IDs = append(result.IDs, stateIDs...)
	result.IDs = append(result.IDs, eosID)
	return result, nil
}
