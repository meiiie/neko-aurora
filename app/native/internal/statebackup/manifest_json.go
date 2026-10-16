package statebackup

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

func decodeManifestJSON(raw []byte, target any) error {
	if int64(len(raw)) > maxManifest || !utf8.Valid(raw) {
		return ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if validateManifestValue(decoder, 0) != nil {
		return ErrInvalid
	}
	if _, err := decoder.Token(); err != io.EOF {
		return ErrInvalid
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return ErrInvalid
	}
	return nil
}

func validateManifestValue(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return ErrInvalid
	}
	token, err := decoder.Token()
	if err != nil {
		return ErrInvalid
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			keys := map[string]bool{}
			for decoder.More() {
				token, err = decoder.Token()
				name, ok := token.(string)
				if err != nil || !ok || strings.ContainsRune(name, '\uFFFD') || keys[strings.ToLower(name)] {
					return ErrInvalid
				}
				keys[strings.ToLower(name)] = true
				if validateManifestValue(decoder, depth+1) != nil {
					return ErrInvalid
				}
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim('}') {
				return ErrInvalid
			}
		case '[':
			for decoder.More() {
				if validateManifestValue(decoder, depth+1) != nil {
					return ErrInvalid
				}
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim(']') {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
	case json.Number:
		if strings.ContainsAny(string(value), ".eE") || string(value) == "-0" {
			return ErrInvalid
		}
		n, err := strconv.ParseInt(string(value), 10, 64)
		if err != nil || n < -9007199254740991 || n > 9007199254740991 {
			return ErrInvalid
		}
	case string:
		if strings.ContainsRune(value, '\uFFFD') {
			return ErrInvalid
		}
	case nil, bool:
	default:
		return ErrInvalid
	}
	return nil
}
