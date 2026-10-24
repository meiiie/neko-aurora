package computer

// Vietnamese-capable fonts for the bot's sandbox (DejaVu, Bitstream Vera /
// public-domain licence). Sandboxes boot from a stock image without a bold
// face, so slides and video frames fell back to bitmap or missing glyphs;
// the fonts are put in ~/.fonts once per home and synced with it.

import (
	"embed"
	"os"
	"path/filepath"
)

//go:embed fonts/*.ttf
var fontFS embed.FS

const FontDir = ".fonts"

func ensureFonts(home string) {
	ents, _ := fontFS.ReadDir("fonts")
	for _, e := range ents {
		dst := filepath.Join(home, FontDir, e.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		b, err := fontFS.ReadFile("fonts/" + e.Name())
		if err != nil {
			continue
		}
		_ = os.MkdirAll(filepath.Dir(dst), 0o700)
		_ = os.WriteFile(dst, b, 0o600)
	}
}
