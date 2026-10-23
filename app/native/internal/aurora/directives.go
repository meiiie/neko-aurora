package aurora

// normalizeToolLabels rewrites tool calls the model wrote in another shape
// into canonical "TOOL: {json}" lines before parsing: a localized or
// decorated label ("CÔNG CỤ:", "**Tool**:", "tool_call:"), a JSON object in
// a code fence, or a bare JSON line. Only objects that parse as a known,
// valid tool are converted, so prose and example code are left alone
// (live run 5: two "CÔNG CỤ: {…}" calls were silently dropped).

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

var (
	labelRe     = regexp.MustCompile(`(?mi)^[ \t>*_` + "`" + `-]*(?:tool(?:[ _]?call)?|công[ _]cụ|cong[ _]cu|gọi công cụ|function[ _]call)[*_` + "`" + `]*[ \t]*[:：][ \t]*(\{.*\})[ \t*` + "`" + `]*$`)
	fenceToolRe = regexp.MustCompile("(?s)```[a-zA-Z_-]*[ \\t]*\\n(\\s*\\{.*?\\})\\s*\\n?```")
	bareJSONRe  = regexp.MustCompile(`(?m)^[ \t]*(\{"name"[ \t]*:[ \t]*"[a-z_]+".*\})[ \t]*$`)
)

func asTool(js string) (string, bool) {
	var t toolReq
	if json.Unmarshal([]byte(js), &t) != nil || !validTool(t) {
		return "", false
	}
	var b bytes.Buffer
	if json.Compact(&b, []byte(js)) != nil {
		return "", false
	}
	return "TOOL: " + b.String(), true
}

func normalizeToolLabels(text string) string {
	text = fenceToolRe.ReplaceAllStringFunc(text, func(m string) string {
		if line, ok := asTool(strings.TrimSpace(fenceToolRe.FindStringSubmatch(m)[1])); ok {
			return line
		}
		return m
	})
	text = labelRe.ReplaceAllStringFunc(text, func(m string) string {
		if line, ok := asTool(labelRe.FindStringSubmatch(m)[1]); ok {
			return line
		}
		return m
	})
	return bareJSONRe.ReplaceAllStringFunc(text, func(m string) string {
		if line, ok := asTool(strings.TrimSpace(m)); ok {
			return line
		}
		return m
	})
}
