package julia

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/opendecisionserver/ods/internal/decision"
)

func TestOfficialEncoderGolden(t *testing.T) {
	path := filepath.Join("..", "..", "..", "models", "julia-1", "tokenizer.json")
	if _, err := os.Stat(path); err != nil {
		t.Skip("Julia tokenizer not downloaded")
	}
	encoder, err := NewEncoder(path, 1024, 256)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := encoder.Encode(decision.Input{
		Type: decision.Choice, State: json.RawMessage(`"The customer asks to reset their password."`),
		Question: "Which team should handle this?", Options: []decision.Option{{Text: "Account support"}, {Text: "Billing"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []int64{2, 6241, 2872, 235292, 12236, 2970, 1412, 6589, 736, 235336, 1, 4, 10741, 2676, 4, 97222, 1, 714, 6650, 19619, 577, 13846, 1024, 8003, 235265, 1}
	if !reflect.DeepEqual(actual.IDs, wantIDs) {
		t.Fatalf("token IDs differ from official reference:\ngot %v\nwant %v", actual.IDs, wantIDs)
	}
	if !reflect.DeepEqual(actual.Markers, []int64{11, 14}) || actual.QType != 0 {
		t.Fatalf("markers or qtype differ: %+v", actual)
	}
}
