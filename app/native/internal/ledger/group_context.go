package ledger

import (
	"encoding/json"
	"reflect"
	"sort"
)

func groupPublishedReportIdentity(p PublishedReportPin) string {
	p.DeliveryID = ""
	p.MessageID = ""
	p.Sequence = 0
	return publishedReportDigest(p)
}

func (s *Store) captureGroupPublishedReports(w Work, at uint64) *PublishedReportContext {
	return s.capturePublishedReportsVersion(w, at, 3)
}

func groupContextEnvelopeBytes(sources []ContextSource, reports []PublishedReportPin) int {
	raw, _ := json.Marshal(struct {
		Sources []ContextSource
		Reports []PublishedReportPin
	}{sources, reports})
	return len(raw)
}

func groupContextSource(w Work) ContextSource {
	return ContextSource{RequestID: w.RequestID, MessageID: w.SourceMessageID, Sequence: w.CreatedSequence, Epoch: w.Epoch, Text: w.Text, Reply: w.Reply, Artifacts: append([]Artifact(nil), w.Artifacts...), AccessRevision: accessRevision(w), BotID: w.BotID}
}

func (s *Store) groupPluginSelectionWork(w Work) (Work, bool) {
	if w.Collaboration == nil {
		return w, true
	}
	g, ok := s.collaborations[w.Collaboration.ID]
	if !ok {
		return Work{}, false
	}
	if g.PublishedReportContext == nil || g.PublishedReportContext.Version == 1 || g.PublishedReportContext.Version == 2 {
		return w, true
	}
	if g.PublishedReportContext.Version != 3 || w.ToolPolicyVersion != g.ToolPolicyVersion || !reflect.DeepEqual(w.PluginSelection, g.PluginSelection) || !reflect.DeepEqual(w.PluginPins, g.PluginPins) || w.OwnerUserID != g.OwnerUserID || w.SpaceID != g.SpaceID || w.ConversationID != g.ChannelID || !contains(g.Participants, w.BotID) {
		return Work{}, false
	}
	for _, p := range g.PublishedReportContext.Pins {
		if p.Source == nil || !s.publishedReportAllowed(w, p, g.PublishedReportContext.AtSequence) {
			return Work{}, false
		}
	}
	w.Context = &ContextPin{Version: 5, PublishedReports: copyPublishedReports(g.PublishedReportContext.Pins)}
	return w, true
}

func (s *Store) groupContextSourceEligible(w, prior Work) bool {
	return prior.OwnerUserID == w.OwnerUserID && prior.SpaceID == w.SpaceID && prior.ConversationID == w.ConversationID && prior.Status == "completed" && prior.CreatedSequence < w.CreatedSequence && (prior.Resources == nil || w.Resources != nil && prior.Resources.Project.ID == w.Resources.Project.ID && prior.Resources.Project.Revision == w.Resources.Project.Revision) && operationsComplete(prior) && s.contextAllowed(prior)
}

func (s *Store) captureGroupContextVersion(w Work, version int, g Collaboration) *ContextPin {
	pin := &ContextPin{Version: version, AtSequence: uint64(len(s.events)), OwnerUserID: w.OwnerUserID, SpaceID: w.SpaceID, BotID: w.BotID, ConversationID: w.ConversationID, BotConfigRevision: w.BotConfigRevision, Sources: []ContextSource{}, ToolPolicyVersion: w.ToolPolicyVersion, PluginPins: append([]PluginPin(nil), w.PluginPins...), Resources: copyResources(w.Resources), Profile: copyBotProfile(w.BotProfile), Omitted: g.PublishedReportContext.Omitted}
	for _, bot := range s.bots {
		if bot.ID == w.BotID {
			pin.BotName = bot.Name
			break
		}
	}
	selected := map[string]bool{}
	complete := true
	for stage := 1; stage < w.Collaboration.Stage; stage++ {
		if stage > len(g.RequestIDs) {
			complete = false
			break
		}
		id := g.RequestIDs[stage-1]
		selected[id] = true
		prior, ok := s.work[id]
		if !ok || prior.Collaboration == nil || prior.Collaboration.ID != g.ID || prior.Collaboration.Stage != stage || !s.groupContextSourceEligible(w, prior) {
			pin.Omitted++
			complete = false
			continue
		}
		sources := append(append([]ContextSource{}, pin.Sources...), groupContextSource(prior))
		if len(sources) > 8 || groupContextEnvelopeBytes(sources, nil) > 8000 {
			pin.Omitted++
			complete = false
			continue
		}
		pin.Sources = sources
	}
	if complete {
		reports := copyPublishedReports(g.PublishedReportContext.Pins)
		if !pinnedTool(w, "report.read_artifact") {
			stripPublishedFiles(reports)
		}
		for i := len(reports) - 1; i >= 0; i-- {
			candidate := append(append([]PublishedReportPin{}, pin.PublishedReports...), reports[i])
			if len(pin.Sources)+len(candidate) > 8 || groupContextEnvelopeBytes(pin.Sources, candidate) > 8000 {
				pin.Omitted++
				continue
			}
			pin.PublishedReports = candidate
		}
		for i, j := 0, len(pin.PublishedReports)-1; i < j; i, j = i+1, j-1 {
			pin.PublishedReports[i], pin.PublishedReports[j] = pin.PublishedReports[j], pin.PublishedReports[i]
		}
		if pin.Profile == nil || pin.Profile.IncludeHistory {
			eligible := []Work{}
			for _, prior := range s.work {
				if !selected[prior.RequestID] && s.groupContextSourceEligible(w, prior) {
					eligible = append(eligible, prior)
				}
			}
			sort.Slice(eligible, func(i, j int) bool { return eligible[i].CreatedSequence > eligible[j].CreatedSequence })
			for _, prior := range eligible {
				sources := append(append([]ContextSource{}, pin.Sources...), groupContextSource(prior))
				if len(sources)+len(pin.PublishedReports) > 8 || groupContextEnvelopeBytes(sources, pin.PublishedReports) > 8000 {
					pin.Omitted++
					continue
				}
				pin.Sources = sources
			}
		}
	}
	sort.Slice(pin.Sources, func(i, j int) bool { return pin.Sources[i].Sequence < pin.Sources[j].Sequence })
	raw, _ := json.Marshal(pin)
	pin.SHA256 = manifestHash(string(raw))
	return pin
}
