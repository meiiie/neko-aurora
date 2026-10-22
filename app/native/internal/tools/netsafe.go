package tools

import (
	"net"
	"regexp"
	"strings"
)

// isPublicAddress and RequestedURLs are ported from CopilotKit/OpenDots
// src/browser/security.ts and src/server/research.ts (MIT, Copyright (c)
// Atai Barkai). They refuse special-purpose ranges beyond Go's IsPrivate
// (CGNAT 100.64/10, benchmarking 198.18/15, TEST-NETs, non-global IPv6) and
// extract owner-typed URLs without trailing punctuation or unbalanced brackets.
func isPublicAddress(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		a, b := int(v4[0]), int(v4[1])
		return !(a == 0 || a == 10 || a == 127 || a >= 224 ||
			(a == 100 && b >= 64 && b <= 127) ||
			(a == 169 && b == 254) ||
			(a == 172 && b >= 16 && b <= 31) ||
			(a == 192 && (b == 168 || b == 0 || b == 2)) ||
			(a == 198 && (b == 18 || b == 19 || b == 51)) ||
			(a == 203 && b == 0))
	}
	if len(ip) != net.IPv6len {
		return false
	}
	first := int(ip[0])<<8 | int(ip[1])
	second := int(ip[2])<<8 | int(ip[3])
	if first == 0x2001 && second < 0x200 || first == 0x3fff && second < 0x1000 {
		return false
	}
	if first>>12 != 2 && first>>12 != 3 { // only global unicast 2000::/3
		return false
	}
	return !(first == 0x2001 && (second == 0 || second == 0xdb8 || second == 0x10 || second == 0x20) || first == 0x2002)
}

var urlRe = regexp.MustCompile(`(?i)https?://[^\s<>"']+`)

var closers = map[byte]byte{')': '(', ']': '['}

func RequestedURLs(prompt string) []string {
	text := strings.ReplaceAll(prompt, "](", "] (")
	seen := map[string]bool{}
	var out []string
	for _, u := range urlRe.FindAllString(text, -1) {
		for len(u) > 0 {
			last := u[len(u)-1]
			open, isCloser := closers[last]
			unbalanced := isCloser && strings.Count(u, string(last)) > strings.Count(u, string(open))
			if strings.IndexByte(".,;!?:", last) >= 0 || unbalanced {
				u = u[:len(u)-1]
				continue
			}
			break
		}
		if u != "" && !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}
