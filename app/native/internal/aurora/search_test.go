package aurora

import "testing"

func TestSnippetFold(t *testing.T) {
	s, ok := snippet("Mình đã tóm tắt bài báo về Đà Nẵng", foldText("tom tat"))
	if !ok || s != "Mình đã tóm tắt bài báo về Đà Nẵng" {
		t.Fatal(s)
	}
	if _, ok := snippet("abc", "zz"); ok {
		t.Fatal("false hit")
	}
	if s, _ := snippet(string(make([]rune, 0))+"đà nẵng", foldText("Da Nang")); s != "đà nẵng" {
		t.Fatal(s)
	}
}
