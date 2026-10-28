package decision

import (
	"context"
	"encoding/json"
	"testing"
)

type officialD1 struct{}

func (officialD1) Decide(context.Context, []Input) ([]Result, error) {
	return []Result{{Answer: json.RawMessage(`{"type":"score","score":2,"probabilities":{"0":0.2,"1":0.2,"2":0.6},"legend":{"0":"a","1":"b","2":"c"},"confidence":0.6}`), InputTokens: 12}}, nil
}
func TestD1OfficialScoreIsPreserved(t *testing.T) {
	s := Service{Model: D1Model(), Engine: officialD1{}}
	r, err := s.Decide(context.Background(), Request{Model: "d1-2b", Questions: []Input{{ID: "q", Type: Score}}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(r.Answers["q"])
	var answer map[string]any
	json.Unmarshal(b, &answer)
	if answer["score"] != float64(2) || r.Usage.InputTokens != 12 {
		t.Fatalf("%s %+v", b, r.Usage)
	}
}
