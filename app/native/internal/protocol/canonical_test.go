package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSharedCanonicalFixtures(t *testing.T) {
	b, err := os.ReadFile("../../../contracts/canonical-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name      string
		Input     string
		Canonical string
		Digest    string
		Valid     bool
	}
	if err = json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			canonical, err := Canonical([]byte(c.Input))
			if !c.Valid {
				if err == nil {
					t.Fatal("invalid frame accepted")
				}
				return
			}
			if err != nil || string(canonical) != c.Canonical {
				t.Fatal("canonical bytes mismatch", err)
			}
			digest := sha256.Sum256(canonical)
			if hex.EncodeToString(digest[:]) != c.Digest {
				t.Fatal("digest mismatch")
			}
		})
	}
}
func TestBounds(t *testing.T) {
	for _, b := range [][]byte{[]byte(strings.Repeat(" ", MaxFrame+1)), {0xff}, []byte(strings.Repeat("[", 34) + "0" + strings.Repeat("]", 34))} {
		if _, err := Canonical(b); err == nil {
			t.Fatal("bounds ignored")
		}
	}
}
