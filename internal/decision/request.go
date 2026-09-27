package decision

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type member struct {
	key string
	raw json.RawMessage
}

// orderedObject retains wire order, which is significant for Julia option scores.
func orderedObject(raw []byte) ([]member, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		return nil, fmt.Errorf("expected object")
	}
	var out []member
	seen := make(map[string]bool)
	for d.More() {
		t, err := d.Token()
		if err != nil {
			return nil, err
		}
		key := t.(string)
		if seen[key] {
			return nil, fmt.Errorf("duplicate key %q", key)
		}
		seen[key] = true
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return nil, err
		}
		out = append(out, member{key, value})
	}
	if _, err := d.Token(); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing JSON content")
	}
	return out, nil
}

func value(members []member, key string) (json.RawMessage, bool) {
	for _, m := range members {
		if m.key == key {
			return m.raw, true
		}
	}
	return nil, false
}

func bad(loc []any, msg string, kind string) error {
	return FieldError{Location: loc, Message: msg, Type: kind}
}

func jsonKind(raw []byte) byte {
	for _, b := range raw {
		if b != ' ' && b != '\n' && b != '\r' && b != '\t' {
			return b
		}
	}
	return 0
}

// ContentText converts Jev structured content to deterministic compact JSON.
// String content remains plain text, as in Julia's native API.
func ContentText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", nil
	}
	if jsonKind(raw) == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}
		return s, nil
	}
	if jsonKind(raw) != '{' && jsonKind(raw) != '[' {
		return "", fmt.Errorf("content must be string, object, array, or null")
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", err
	}
	b, err := json.Marshal(v)
	return string(b), err
}

func ParseRequest(body []byte, maxQuestions int) (Request, error) {
	var req Request
	root, err := orderedObject(body)
	if err != nil {
		return req, bad([]any{"body"}, "Invalid JSON object", "value_error")
	}
	state, ok := value(root, "state")
	if !ok {
		return req, bad([]any{"body", "state"}, "Field required", "missing")
	}
	if jsonKind(state) != '"' && jsonKind(state) != '{' && jsonKind(state) != '[' {
		return req, bad([]any{"body", "state"}, "state must be string, object, or array", "value_error")
	}
	model, ok := value(root, "model")
	if !ok {
		return req, bad([]any{"body", "model"}, "Field required", "missing")
	}
	if err := json.Unmarshal(model, &req.Model); err != nil || req.Model == "" {
		return req, bad([]any{"body", "model"}, "model must be a nonempty string", "value_error")
	}
	questions, ok := value(root, "questions")
	if !ok {
		return req, bad([]any{"body", "questions"}, "Field required", "missing")
	}
	entries, err := orderedObject(questions)
	if err != nil {
		return req, bad([]any{"body", "questions"}, "questions must be an object with unique IDs", "value_error")
	}
	if len(entries) == 0 || len(entries) > maxQuestions {
		return req, bad([]any{"body", "questions"}, fmt.Sprintf("questions must contain 1-%d entries", maxQuestions), "value_error")
	}
	for _, entry := range entries {
		loc := []any{"body", "questions", entry.key}
		if strings.TrimSpace(entry.key) == "" {
			return req, bad(loc, "question ID must be nonempty", "value_error")
		}
		fields, err := orderedObject(entry.raw)
		if err != nil {
			return req, bad(loc, "question must be an object", "value_error")
		}
		typeRaw, ok := value(fields, "type")
		if !ok {
			return req, bad(append(loc, "type"), "Field required", "missing")
		}
		var typ QuestionType
		if err := json.Unmarshal(typeRaw, &typ); err != nil || (typ != Noul && typ != Choice && typ != Score) {
			return req, bad(append(loc, "type"), "type must be noul, choice, or score", "value_error")
		}
		instruction, _ := value(fields, "instructions")
		question, err := ContentText(instruction)
		if err != nil {
			return req, bad(append(loc, "instructions"), err.Error(), "value_error")
		}
		input := Input{ID: entry.key, Type: typ, State: state, Question: question}
		criteria, hasCriteria := value(fields, "criteria")
		switch typ {
		case Noul:
			texts := []string{"false", "true"}
			if hasCriteria && !bytes.Equal(bytes.TrimSpace(criteria), []byte("null")) {
				items, err := orderedObject(criteria)
				if err != nil {
					return req, bad(append(loc, "criteria"), "noul criteria must be an object", "value_error")
				}
				for _, item := range items {
					idx := -1
					if item.key == "false" {
						idx = 0
					} else if item.key == "true" {
						idx = 1
					}
					if idx < 0 {
						return req, bad(append(loc, "criteria", item.key), "unknown noul criterion", "value_error")
					}
					text, err := ContentText(item.raw)
					if err != nil {
						return req, bad(append(loc, "criteria", item.key), err.Error(), "value_error")
					}
					if text != "" {
						texts[idx] = text
					}
				}
			}
			input.Options = []Option{{Key: "false", Text: texts[0]}, {Key: "true", Text: texts[1]}}
		case Choice:
			if !hasCriteria {
				return req, bad(append(loc, "criteria"), "Field required", "missing")
			}
			items, err := orderedObject(criteria)
			if err != nil {
				return req, bad(append(loc, "criteria"), "choice criteria must be an object", "value_error")
			}
			if len(items) < 2 || len(items) > 20 {
				return req, bad(append(loc, "criteria"), "choice requires 2-20 options", "value_error")
			}
			for _, item := range items {
				if item.key == "" {
					return req, bad(append(loc, "criteria"), "option key must be nonempty", "value_error")
				}
				text, err := ContentText(item.raw)
				if err != nil {
					return req, bad(append(loc, "criteria", item.key), err.Error(), "value_error")
				}
				if text == "" {
					text = item.key
				}
				input.Options = append(input.Options, Option{Key: item.key, Text: text, Raw: item.raw})
			}
		case Score:
			if !hasCriteria {
				return req, bad(append(loc, "criteria"), "Field required", "missing")
			}
			var items []json.RawMessage
			if err := json.Unmarshal(criteria, &items); err != nil {
				return req, bad(append(loc, "criteria"), "score criteria must be an array", "value_error")
			}
			if len(items) < 2 || len(items) > 10 {
				return req, bad(append(loc, "criteria"), "score requires 2-10 levels", "value_error")
			}
			for i, item := range items {
				text, err := ContentText(item)
				if err != nil || text == "" {
					return req, bad(append(loc, "criteria", i), "score level must be nonempty content", "value_error")
				}
				input.Options = append(input.Options, Option{Key: fmt.Sprint(i), Text: text, Raw: item})
			}
		}
		req.Questions = append(req.Questions, input)
	}
	return req, nil
}
