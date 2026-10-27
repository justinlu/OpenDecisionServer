package decision

import (
	"context"
	"encoding/json"
)

type QuestionType string

const (
	Noul   QuestionType = "noul"
	Choice QuestionType = "choice"
	Score  QuestionType = "score"
)

type Option struct {
	Key  string
	Text string
	Raw  json.RawMessage
}

type Input struct {
	ID           string
	Type         QuestionType
	State        json.RawMessage
	Question     string
	Instructions json.RawMessage
	Options      []Option
}

type Result struct {
	Logits        []float32
	Probabilities []float64
	Winner        int
	InputTokens   int
}

type Engine interface {
	Decide(context.Context, []Input) ([]Result, error)
}

type Request struct {
	Model     string
	Questions []Input
}

type FieldError struct {
	Location []any  `json:"loc"`
	Message  string `json:"msg"`
	Type     string `json:"type"`
}

func (e FieldError) Error() string { return e.Message }
