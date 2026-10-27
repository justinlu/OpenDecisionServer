package decision

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

func parseChinese(body []byte, maxQuestions int) (Request, error) {
	var req Request
	if _, err := ChineseText(body); err != nil {
		return req, bad([]any{"body"}, err.Error(), "value_error")
	}
	root, err := orderedObject(body)
	if err != nil {
		return req, bad([]any{"body"}, err.Error(), "value_error")
	}
	state, ok := value(root, "state")
	if !ok {
		return req, bad([]any{"body", "state"}, "Field required", "missing")
	}
	model, ok := value(root, "model")
	if !ok || json.Unmarshal(model, &req.Model) != nil || req.Model == "" {
		return req, bad([]any{"body", "model"}, "model must be a nonempty string", "value_error")
	}
	questions, _ := value(root, "questions")
	entries, err := orderedObject(questions)
	if err != nil || len(entries) < 1 || len(entries) > maxQuestions {
		return req, bad([]any{"body", "questions"}, fmt.Sprintf("questions requires 1-%d entries", maxQuestions), "value_error")
	}
	for _, entry := range entries {
		loc := []any{"body", "questions", entry.key}
		fields, err := orderedObject(entry.raw)
		if err != nil || entry.key == "" {
			return req, bad(loc, "invalid question", "value_error")
		}
		kind, _ := value(fields, "type")
		var typ QuestionType
		if json.Unmarshal(kind, &typ) != nil {
			return req, bad(loc, "invalid question type", "value_error")
		}
		instructions, exists := value(fields, "instructions")
		if !exists || bytes.Equal(bytes.TrimSpace(instructions), []byte("null")) {
			return req, bad(append(loc, "instructions"), "requires non-null instructions", "value_error")
		}
		text, _ := ChineseText(instructions)
		input := Input{ID: entry.key, Type: typ, State: state, Instructions: instructions, Question: text}
		criteria, exists := value(fields, "criteria")
		option := func(key string, raw json.RawMessage) {
			text, _ := ChineseText(raw)
			input.Options = append(input.Options, Option{Key: key, Text: text, Raw: raw})
		}
		switch typ {
		case Noul:
			var items []member
			if exists && !bytes.Equal(bytes.TrimSpace(criteria), []byte("null")) {
				items, err = orderedObject(criteria)
				if err != nil {
					return req, bad(append(loc, "criteria"), "noul criteria must be an object", "value_error")
				}
			}
			for i, key := range []string{"false", "true"} {
				raw, ok := value(items, key)
				if !ok {
					if i == 0 {
						raw = json.RawMessage(`"否"`)
					} else {
						raw = json.RawMessage(`"是"`)
					}
				}
				option(key, raw)
			}
		case Choice:
			items, err := orderedObject(criteria)
			if err != nil {
				return req, bad(append(loc, "criteria"), "choice criteria must be an object", "value_error")
			}
			for _, item := range items {
				option(item.key, item.raw)
			}
		case Score:
			var items []json.RawMessage
			if json.Unmarshal(criteria, &items) != nil || jsonKind(criteria) != '[' {
				return req, bad(append(loc, "criteria"), "score criteria must be an array", "value_error")
			}
			for i, item := range items {
				option(strconv.Itoa(i), item)
			}
		default:
			return req, bad(append(loc, "type"), "invalid question type", "value_error")
		}
		if len(input.Options) < 2 || len(input.Options) > 26 {
			return req, bad(append(loc, "criteria"), "requires 2-26 candidates", "value_error")
		}
		req.Questions = append(req.Questions, input)
	}
	return req, nil
}
