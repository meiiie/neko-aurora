package aurora

import "testing"

func TestPlainMDSnip(t *testing.T) {
	for in, want := range map[string]string{
		"## Giá vàng sáng 23/10":                "Giá vàng sáng 23/10",
		"Tuyệt! Luyện âm **/θ/** nhé [1].":      "Tuyệt! Luyện âm /θ/ nhé.",
		"- Xem [nguồn](https://sjc.com.vn) `x`": "Xem nguồn x",
		"1. Bước đầu":                           "Bước đầu",
	} {
		if got := plainMD(in); got != want {
			t.Errorf("plainMD(%q)=%q want %q", in, got, want)
		}
	}
}
