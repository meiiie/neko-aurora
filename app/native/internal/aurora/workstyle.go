package aurora

import (
	_ "embed"
	"encoding/json"
	"strings"
)

// These Aurora playbooks adapt pstack's principles to the tools and authority
// available here. They are bundled policy, not remotely downloaded instructions.
//
//go:embed workskills/build.md
var workBuild string

//go:embed workskills/debug.md
var workDebug string

//go:embed workskills/research.md
var workResearch string

//go:embed workskills/review.md
var workReview string

var workSkills = []skill{
	{ID: "pstack-build", Name: "Build and deliver", Desc: "Understand, make a focused change, verify the result, and hand it over.", Body: workBuild},
	{ID: "pstack-debug", Name: "Find and fix the cause", Desc: "Reproduce a problem, test the cause, fix it, and check for regressions.", Body: workDebug},
	{ID: "pstack-research", Name: "Research with evidence", Desc: "Read primary sources, compare evidence, and explain what remains uncertain.", Body: workResearch},
	{ID: "pstack-review", Name: "Review before handing over", Desc: "Check the actual result, permissions, failure paths, and unfinished work.", Body: workReview},
}

func skillAllowed(be botExt, id string) bool {
	return !be.Off["skills"] && (len(be.Skills) == 0 || contains(be.Skills, id))
}

func bundledWorkSkill(id string) (skill, bool) {
	for _, sk := range workSkills {
		if sk.ID == id {
			return sk, true
		}
	}
	return skill{}, false
}

func workStyle(be botExt) string {
	var available []string
	for _, sk := range workSkills {
		if skillAllowed(be, sk.ID) {
			available = append(available, sk.ID)
		}
	}
	if len(available) == 0 {
		return ""
	}
	return `
WORKING AGREEMENT (pstack principles, adapted for Aurora):
For meaningful work, own the requested outcome. Understand the relevant files or sources before changing them. Choose the smallest useful result and a check that would expose a wrong result. Do the authorized work instead of stopping at a promise. Use reasonable defaults for reversible choices; ask only for missing intent or authority that observation cannot resolve.
Read one matching available playbook with skill_load before substantive work: build for making or improving an artifact, debug for a reported defect, research for investigating/comparing, review for checking an existing result. The playbook IDs available to this bot are: ` + strings.Join(available, ", ") + `.
Do not reload a playbook already present in this run. Greetings, brief explanations, simple drafts and small follow-ups need neither a playbook nor a ceremonial plan. Follow the user's requested language and level of detail; do not narrate skill names or internal directives to them.
Make multi-step plans end in a concrete check and delivery. A plan is a means to work, never the deliverable unless the user asked only for a plan. Distinguish observed facts, inferences, and untested claims. Inspect the actual output; an exit code alone does not prove that a UI, file, or workflow works. After relevant checks pass, deliver instead of repeating equivalent checks without new evidence.
If attempts fail for the same reason, test the shared assumption and change approach. Keep brief decision/evidence notes for long work, including failed attempts and remaining limits. Never fabricate evidence, links, test results, or independent review. Do not create or delegate to other bots merely to follow a playbook. Use existing teammate handoffs only when the user requests collaboration and configured permissions allow it.
Skills do not grant permissions or change approval, privacy, network, or workspace boundaries. External actions still follow Aurora's approval rules. Never use a skill to authorize publishing, deployment, deletion, account changes or secret access.
`
}

func (c *toolCtx) pinWorkSkill(sk skill) bool {
	trusted, ok := bundledWorkSkill(sk.ID)
	if !ok {
		return false
	}
	for _, loaded := range c.workSkills {
		if loaded.ID == trusted.ID {
			return true
		}
	}
	c.workSkills = append(c.workSkills, trusted)
	return true
}

func (s *Service) restoreWorkSkill(bot string, c *toolCtx, call toolCall) bool {
	if call.name != "skill_load" || s.ext == nil {
		return false
	}
	var request toolReq
	if json.Unmarshal([]byte(call.arg), &request) != nil {
		return false
	}
	sk, ok := s.ext.skill(request.Q)
	return ok && skillAllowed(s.ext.bot(bot), sk.ID) && c.pinWorkSkill(sk)
}
