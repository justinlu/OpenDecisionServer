package decision

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ChineseText mirrors Python json.dumps(ensure_ascii=False,separators=(',',':')).
// It walks ordered objects and distinguishes arbitrary-precision integers from
// binary64 floats instead of converting every number through interface{}.
func ChineseText(raw json.RawMessage) (string, error) {
	if !utf8.Valid(raw) {
		return "", fmt.Errorf("invalid UTF-8")
	}
	// encoding/json replaces isolated UTF-16 surrogates. Reject instead of silently
	// changing the model input; paired surrogates and literal backslashes are valid.
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			break
		}
		if raw[i] != 'u' {
			continue
		}
		if i+4 >= len(raw) {
			return "", fmt.Errorf("invalid unicode escape")
		}
		u, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil {
			return "", err
		}
		i += 4
		if u >= 0xd800 && u <= 0xdbff {
			if i+6 >= len(raw) || string(raw[i+1:i+3]) != "\\u" {
				return "", fmt.Errorf("unpaired surrogate")
			}
			v, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
			if err != nil || v < 0xdc00 || v > 0xdfff {
				return "", fmt.Errorf("unpaired surrogate")
			}
			i += 6
		} else if u >= 0xdc00 && u <= 0xdfff {
			return "", fmt.Errorf("unpaired surrogate")
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var out strings.Builder
	if err := writeChineseJSON(d, &out, true); err != nil {
		return "", err
	}
	if _, err := d.Token(); err != io.EOF {
		return "", fmt.Errorf("trailing content")
	}
	return out.String(), nil
}

func quotePython(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString("\\\"")
		case '\\':
			b.WriteString("\\\\")
		case '\b':
			b.WriteString("\\b")
		case '\f':
			b.WriteString("\\f")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		default:
			if r < 32 {
				fmt.Fprintf(&b, "\\u%04x", r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func pythonFloat(raw string) (string, error) {
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return "", fmt.Errorf("number is not a finite binary64 float")
	}
	s := strconv.FormatFloat(f, 'e', -1, 64)
	parts := strings.Split(s, "e")
	exponent, _ := strconv.Atoi(parts[1])
	digits := strings.ReplaceAll(parts[0], ".", "")
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign = "-"
		digits = digits[1:]
	}
	if exponent < -4 || exponent >= 16 {
		return s, nil
	}
	point := exponent + 1
	if point <= 0 {
		return sign + "0." + strings.Repeat("0", -point) + digits, nil
	}
	if point >= len(digits) {
		return sign + digits + strings.Repeat("0", point-len(digits)) + ".0", nil
	}
	return sign + digits[:point] + "." + digits[point:], nil
}

func writeChineseJSON(d *json.Decoder, b *strings.Builder, plainString bool) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	switch v := token.(type) {
	case string:
		if plainString {
			b.WriteString(v)
		} else {
			b.WriteString(quotePython(v))
		}
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(v))
	case json.Number:
		s := v.String()
		if strings.ContainsAny(s, ".eE") {
			s, err = pythonFloat(s)
			if err != nil {
				return err
			}
		} else if s == "-0" {
			s = "0"
		}
		b.WriteString(s)
	case json.Delim:
		if v != '{' && v != '[' {
			return fmt.Errorf("unexpected delimiter")
		}
		b.WriteByte(byte(v))
		first := true
		seen := map[string]bool{}
		for d.More() {
			if !first {
				b.WriteByte(',')
			}
			first = false
			if v == '{' {
				key, err := d.Token()
				if err != nil {
					return err
				}
				name := key.(string)
				if seen[name] {
					return fmt.Errorf("duplicate key %q", name)
				}
				seen[name] = true
				b.WriteString(quotePython(name))
				b.WriteByte(':')
			}
			if err := writeChineseJSON(d, b, false); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil {
			return err
		}
		if v == '{' && end != json.Delim('}') || v == '[' && end != json.Delim(']') {
			return fmt.Errorf("invalid delimiter")
		}
		b.WriteByte(byte(end.(json.Delim)))
	default:
		return fmt.Errorf("invalid JSON content")
	}
	return nil
}
