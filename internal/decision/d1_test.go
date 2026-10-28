package decision

import (
	"fmt"
	"strings"
	"testing"
)

func TestD1TextPolicy(t *testing.T) {
	m := D1Model()
	req, err := ParseForModel([]byte(`{"state":null,"questions":{"q":{"type":"noul","instructions":"可以吗？"}}}`), 64, m)
	if err != nil || req.Model != "d1-2b" || len(req.Questions) != 1 || len(req.Questions[0].RawQuestion) == 0 {
		t.Fatalf("default/null policy: %+v %v", req, err)
	}
	entries := []string{}
	for i := 0; i < 40; i++ {
		entries = append(entries, fmt.Sprintf(`"k%d":"选项%d"`, i, i))
	}
	b := []byte(`{"model":"d1-2b","state":"test","questions":{"q":{"type":"choice","instructions":"pick","criteria":{` + strings.Join(entries, ",") + `}}}}`)
	if _, err = ParseForModel(b, 64, m); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{"state":"x","videos":["data:video/mp4;base64,AAAA"],"questions":{"q":{"type":"noul","instructions":"x"}}}`,
		`{"state":[{"type":"video","url":"x"}],"questions":{"q":{"type":"noul","instructions":"x"}}}`,
		`{"state":"x","files":["data:audio/wav;base64,AAAA"],"questions":{"q":{"type":"noul","instructions":"x"}}}`,
		`{"state":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"x"}}]}],"questions":{"q":{"type":"noul","instructions":"x"}}}`,
		`{"state":"x","questions":{"q":{"type":"choice","instructions":"x","multiple":true,"criteria":{"a":"a","b":"b"}}}}`,
	} {
		if _, err = ParseForModel([]byte(body), 64, m); err == nil {
			t.Fatalf("accepted unsupported input: %s", body)
		}
	}
}
