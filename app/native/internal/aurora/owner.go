package aurora

import "strings"

// ownerName is who the bots work for. Prompts are written for the original
// owner ("Darren"); hosted workspaces swap in their own user's name.
func (s *Service) ownerName() string {
	if s.owner != nil {
		if n := strings.TrimSpace(s.owner()); n != "" {
			return n
		}
		return "bạn"
	}
	return "Darren"
}

func (s *Service) personalize(msgs []ChatMsg) []ChatMsg {
	if s.owner == nil {
		return msgs
	}
	n := s.ownerName()
	r := strings.NewReplacer("Darren (HoLiLiHu)", n, "DARREN", strings.ToUpper(n), "Darren", n)
	out := make([]ChatMsg, len(msgs))
	for i, m := range msgs {
		out[i] = ChatMsg{Role: m.Role, Content: r.Replace(m.Content)}
	}
	return out
}
