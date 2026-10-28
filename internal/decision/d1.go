package decision

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Text-only policy. The fixed official C++ compiler performs the model-specific
// validation too; Go rejects unsupported modalities before scheduling native work.
func parseD1(body []byte, maxQuestions int) (Request, error) {
	var req Request
	if _, err := ChineseText(body); err != nil {
		return req, bad([]any{"body"}, err.Error(), "value_error")
	}
	root, err := orderedObject(body)
	if err != nil {
		return req, err
	}
	for _, key := range []string{"files", "images", "videos"} {
		if raw, ok := value(root, key); ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("[]")) && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return req, bad([]any{"body", key}, "d1-2b service currently accepts text only", "value_error")
		}
	}
	var media any
	if err = json.Unmarshal(body, &media); err != nil {
		return req, err
	}
	var containsMedia func(any) bool
	containsMedia = func(v any) bool {
		switch x := v.(type) {
		case map[string]any:
			if typ, ok := x["type"].(string); ok && (typ == "image_url" || typ == "input_audio" || typ == "image" || typ == "audio" || typ == "video_url" || typ == "video") {
				return true
			}
			for _, item := range x {
				if containsMedia(item) {
					return true
				}
			}
		case []any:
			for _, item := range x {
				if containsMedia(item) {
					return true
				}
			}
		}
		return false
	}
	if containsMedia(media) {
		return req, bad([]any{"body"}, "media inputs have not been enabled for d1-2b", "value_error")
	}
	state, ok := value(root, "state")
	if !ok {
		return req, bad([]any{"body", "state"}, "Field required", "missing")
	}
	req.Model = "d1-2b"
	if raw, ok := value(root, "model"); ok {
		if json.Unmarshal(raw, &req.Model) != nil || req.Model == "" {
			return req, bad([]any{"body", "model"}, "model must be a nonempty string", "value_error")
		}
	}
	raw, _ := value(root, "questions")
	questions, err := orderedObject(raw)
	if err != nil || len(questions) < 1 || len(questions) > maxQuestions {
		return req, bad([]any{"body", "questions"}, fmt.Sprintf("requires 1-%d questions", maxQuestions), "value_error")
	}
	for _, q := range questions {
		fields, err := orderedObject(q.raw)
		loc := []any{"body", "questions", q.key}
		if err != nil || q.key == "" {
			return req, bad(loc, "invalid question", "value_error")
		}
		for _, key := range []string{"multiple", "permutation", "permutations"} {
			if raw, ok := value(fields, key); ok && string(raw) != "false" {
				return req, bad(loc, "unsupported decision option: "+key, "value_error")
			}
		}
		kind, _ := value(fields, "type")
		var typ QuestionType
		if json.Unmarshal(kind, &typ) != nil {
			return req, bad(loc, "invalid question type", "value_error")
		}
		instructions, ok := value(fields, "instructions")
		if !ok || bytes.Equal(bytes.TrimSpace(instructions), []byte("null")) {
			return req, bad(loc, "requires non-null instructions", "value_error")
		}
		input := Input{ID: q.key, Type: typ, State: state, Instructions: instructions, RawQuestion: q.raw}
		criteria, _ := value(fields, "criteria")
		switch typ {
		case Noul:
			input.Options = []Option{{Key: "false"}, {Key: "true"}}
		case Choice:
			options, e := orderedObject(criteria)
			if e != nil {
				return req, bad(loc, "choice criteria must be an object", "value_error")
			}
			for _, o := range options {
				input.Options = append(input.Options, Option{Key: o.key, Raw: o.raw})
			}
		case Score:
			var options []json.RawMessage
			if json.Unmarshal(criteria, &options) != nil || jsonKind(criteria) != '[' || len(options) > 10 {
				return req, bad(loc, "score requires 2-10 levels", "value_error")
			}
			for i, o := range options {
				input.Options = append(input.Options, Option{Key: fmt.Sprint(i), Raw: o})
			}
		default:
			return req, bad(loc, "type must be choice, noul or score", "value_error")
		}
		if len(input.Options) < 2 || len(input.Options) > 255 {
			return req, bad(loc, "requires 2-255 candidates", "value_error")
		}
		req.Questions = append(req.Questions, input)
	}
	return req, nil
}
