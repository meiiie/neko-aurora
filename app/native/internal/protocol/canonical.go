package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

var ErrFrame = errors.New("invalid_protocol_frame")

const MaxFrame = 65536
const safeInteger = int64(9007199254740991)

func Canonical(frame []byte) ([]byte, error) {
	if len(frame) > MaxFrame || !utf8.Valid(frame) {
		return nil, ErrFrame
	}
	dec := json.NewDecoder(bytes.NewReader(frame))
	dec.UseNumber()
	value, err := parse(dec, 0)
	if err != nil {
		return nil, ErrFrame
	}
	if _, err = dec.Token(); err != io.EOF {
		return nil, ErrFrame
	}
	var out bytes.Buffer
	if err = encode(&out, value); err != nil {
		return nil, ErrFrame
	}
	return out.Bytes(), nil
}
func parse(dec *json.Decoder, depth int) (any, error) {
	if depth > 32 {
		return nil, ErrFrame
	}
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch v := token.(type) {
	case json.Delim:
		switch v {
		case '{':
			out := map[string]any{}
			for dec.More() {
				key, err := dec.Token()
				if err != nil {
					return nil, err
				}
				name, ok := key.(string)
				if !ok || strings.ContainsRune(name, '\uFFFD') {
					return nil, ErrFrame
				}
				if _, ok = out[name]; ok {
					return nil, ErrFrame
				}
				value, err := parse(dec, depth+1)
				if err != nil {
					return nil, err
				}
				out[name] = value
			}
			end, err := dec.Token()
			if err != nil || end != json.Delim('}') {
				return nil, ErrFrame
			}
			return out, nil
		case '[':
			out := []any{}
			for dec.More() {
				value, err := parse(dec, depth+1)
				if err != nil {
					return nil, err
				}
				out = append(out, value)
			}
			end, err := dec.Token()
			if err != nil || end != json.Delim(']') {
				return nil, ErrFrame
			}
			return out, nil
		}
		return nil, ErrFrame
	case json.Number:
		if strings.ContainsAny(string(v), ".eE") || string(v) == "-0" {
			return nil, ErrFrame
		}
		n, err := strconv.ParseInt(string(v), 10, 64)
		if err != nil || n < -safeInteger || n > safeInteger {
			return nil, ErrFrame
		}
		return n, nil
	case string:
		if strings.ContainsRune(v, '\uFFFD') {
			return nil, ErrFrame
		}
		return v, nil
	case nil, bool:
		return v, nil
	}
	return nil, ErrFrame
}
func quote(out *bytes.Buffer, s string) {
	out.WriteByte('"')
	for _, c := range s {
		switch c {
		case '"', '\\':
			out.WriteByte('\\')
			out.WriteRune(c)
		case '\b':
			out.WriteString("\\b")
		case '\f':
			out.WriteString("\\f")
		case '\n':
			out.WriteString("\\n")
		case '\r':
			out.WriteString("\\r")
		case '\t':
			out.WriteString("\\t")
		default:
			if c < 32 {
				const hex = "0123456789abcdef"
				out.WriteString("\\u00")
				out.WriteByte(hex[(c>>4)&15])
				out.WriteByte(hex[c&15])
			} else {
				out.WriteRune(c)
			}
		}
	}
	out.WriteByte('"')
}
func encode(out *bytes.Buffer, value any) error {
	switch v := value.(type) {
	case nil:
		out.WriteString("null")
	case bool:
		if v {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	case int64:
		out.WriteString(strconv.FormatInt(v, 10))
	case string:
		quote(out, v)
	case []any:
		out.WriteByte('[')
		for i, e := range v {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := encode(out, e); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			quote(out, k)
			out.WriteByte(':')
			if err := encode(out, v[k]); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	default:
		return ErrFrame
	}
	return nil
}
