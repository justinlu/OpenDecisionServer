package decision

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestChineseContentMatchesPython(t *testing.T) {
	data, err := os.ReadFile("testdata/chinese-golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Raw  json.RawMessage
		Text string
	}
	if err = json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		got, err := ChineseText(f.Raw)
		if err != nil || got != f.Text {
			t.Fatalf("raw %s: got %q, want %q, error %v", f.Raw, got, f.Text, err)
		}
	}
}

func TestChineseRequestPolicy(t *testing.T) {
	req, err := ParseForModel([]byte(`{"model":"chinese-decision-2b","state":null,"questions":{"n":{"type":"noul","instructions":"判断"},"c":{"type":"choice","instructions":{"z":2,"a":1},"criteria":{"z":null,"a":""}},"s":{"type":"score","instructions":false,"criteria":[null,{"z":9007199254740993,"a":1.0}]}}}`), 64, ChineseModel())
	if err != nil {
		t.Fatal(err)
	}
	if req.Questions[0].Options[0].Text != "否" || req.Questions[0].Options[1].Text != "是" {
		t.Fatal(req)
	}
	c := req.Questions[1]
	if c.Question != `{"z":2,"a":1}` || c.Options[0].Key != "z" || c.Options[0].Text != "null" || c.Options[1].Text != "" {
		t.Fatal(c)
	}
	s := req.Questions[2]
	if s.Question != "false" || s.Options[1].Text != `{"z":9007199254740993,"a":1.0}` {
		t.Fatal(s)
	}
	if string(s.Instructions) != "false" {
		t.Fatal("raw instructions missing")
	}
}

func TestChineseRejectsAmbiguousOrInvalidContent(t *testing.T) {
	for _, raw := range []string{`{"a":{"x":1,"x":2}}`, `1e999`, `"\ud800"`, `"\udc00"`} {
		if _, err := ChineseText([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, q := range []string{`{"type":"noul"}`, `{"type":"noul","instructions":null}`} {
		if _, err := ParseForModel([]byte(`{"model":"chinese-decision-2b","state":"x","questions":{"q":`+q+`}}`), 64, ChineseModel()); err == nil {
			t.Fatalf("accepted %s", q)
		}
	}
}

func TestChineseCandidateLimits(t *testing.T) {
	for _, size := range []int{2, 26, 27} {
		var options []string
		for i := 0; i < size; i++ {
			options = append(options, fmt.Sprintf(`"k%d":""`, i))
		}
		body := `{"model":"chinese-decision-2b","state":true,"questions":{"q":{"type":"choice","instructions":"选择","criteria":{` + strings.Join(options, ",") + `}}}}`
		req, err := ParseForModel([]byte(body), 64, ChineseModel())
		if size <= 26 {
			if err != nil || len(req.Questions[0].Options) != size {
				t.Fatalf("%d: %v", size, err)
			}
		} else if err == nil {
			t.Fatal("27 candidates accepted")
		}
	}
}
