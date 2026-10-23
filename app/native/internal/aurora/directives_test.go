package aurora

import "testing"

func TestToolLabelVariantsParse(t *testing.T) {
	for _, in := range []string{
		`CÔNG CỤ: {"name":"computer_write_file","path":"make_video.py","content":"print(1)"}`,
		`Công cụ: {"name":"computer_run","cmd":"python3 a.py"}`,
		`**Tool**: {"name":"computer_run","cmd":"python3 a.py"}`,
		`tool_call: {"name":"web_search","q":"Hg-198"}`,
		"Mình ghi file:\n```json\n{\n  \"name\": \"computer_write_file\",\n  \"path\": \"a.py\",\n  \"content\": \"print(1)\"\n}\n```",
		"{\"name\":\"computer_run\",\"cmd\":\"ls\"}",
	} {
		if d := ParseDirectives(in); len(d.Tools) != 1 {
			t.Errorf("not parsed: %q", in)
		} else if d.Text != "" && d.Text != "Mình ghi file:" {
			t.Errorf("leftover %q", d.Text)
		}
	}
	for _, in := range []string{
		"```json\n{\"slides\": 8}\n```",
		`Ví dụ: {"name":"Hg","z":80}`,
		"```python\nprint({'name':'x'})\n```",
	} {
		if d := ParseDirectives(in); len(d.Tools) != 0 || d.Text == "" {
			t.Errorf("false tool from %q: %+v", in, d)
		}
	}
}
