package decision

import (
	"bytes"
	"encoding/json"
)

// Preserve candidate order on the wire as well as in Winner. Python clients
// resolve ties using dict insertion order, so a sorted Go map changes ties.
type probabilityObject struct {
	options []Option
	values  []float64
}

func (p probabilityObject) MarshalJSON() ([]byte, error) {
	var out bytes.Buffer
	out.WriteByte('{')
	for i, option := range p.options {
		if i > 0 {
			out.WriteByte(',')
		}
		key, err := json.Marshal(option.Key)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(p.values[i])
		if err != nil {
			return nil, err
		}
		out.Write(key)
		out.WriteByte(':')
		out.Write(value)
	}
	out.WriteByte('}')
	return out.Bytes(), nil
}
