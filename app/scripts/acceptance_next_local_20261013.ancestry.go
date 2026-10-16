package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type independentAncestryFixture struct {
	s *Store
	c Channel
	file UploadedFile
	source Work
	delivery ReportDelivery
	g Collaboration
	first Work
	reviewer Work
}

func independentAncestryCreate(t *testing.T) independentAncestryFixture {
	t.Helper()
	s,err:=Open(t.TempDir());if err!=nil { t.Fatal(err) };t.Cleanup(func(){s.Close()})
	c:=sharedChannel(t,s,"Independent ancestral destination","ind-ancestry-channel")
	private:="conversation-bot-meiiie"
	p,_,err:=s.SaveProject(ProjectInput{Name:"Original grounding project",Goal:"Bound original grounding",ChannelIDs:[]string{c.ID,private},ClientNonce:"ind-ancestry-project"});if err!=nil { t.Fatal(err) }
	file,_,err:=s.UploadFile(UploadInput{ProjectID:p.ID,ProjectRevision:p.Revision,ChannelID:private,Filename:"grounding.md",Content:[]byte("ORIGINAL_PRIVATE_GROUNDING_FILE"),ClientNonce:"ind-ancestry-upload"});if err!=nil { t.Fatal(err) }
	source,_,err:=s.AcceptProduct(Input{BotID:"bot-meiiie",ClientNonce:"ind-ancestry-private-source",Text:"PRIVATE_REQUEST_NOT_PUBLISHED",Lane:"foreground"},SubmissionScope{ProjectID:p.ID,ProjectRevision:p.Revision,FileIDs:[]string{file.ID}});if err!=nil { t.Fatal(err) }
	source=startPermissionWork(t,s,source)
	if _,err=s.ExecuteTool(source.RequestID,source.Epoch,invocation("Original published output file"));err!=nil { t.Fatal(err) }
	if err=s.AppendText(source.RequestID,source.Epoch,strings.Repeat("Report ancestry evidence. ",210));err!=nil { t.Fatal(err) }
	if err=s.Finish(source.RequestID,source.Epoch,"completed","");err!=nil { t.Fatal(err) };source,err=s.Get(source.RequestID);if err!=nil { t.Fatal(err) }
	input:=reportSubmission(c,"ind-ancestry-delivery");input.Text="@Meiiie gửi báo cáo tệp mới nhất ở cuộc trò chuyện riêng vào kênh này"
	delivery,_,err:=s.DeliverReport(input);if err!=nil||len(delivery.Artifacts)!=1 { t.Fatal("selected publication fixture",err) }
	if _,_,err=s.SetBotProfile("bot-davin",BotProfileInput{Instructions:"Retain explicit group dependencies; exclude ambient history",IncludeHistory:false,ClientNonce:"ind-ancestry-reviewer-profile"});err!=nil { t.Fatal(err) }
	g,_,err:=s.CreateCollaboration(CollaborationInput{ChannelID:c.ID,ExpectedChannelRevision:c.Revision,Participants:[]string{"bot-meiiie","bot-davin"},CoordinatorBotID:"bot-meiiie",Goal:"Compare the published result",MaxStages:3,ClientNonce:"ind-ancestry-group"});if err!=nil||g.PublishedReportContext.Version!=3||len(g.PublishedReportContext.Pins)!=1||len(g.PublishedReportContext.Pins[0].Files)!=1 { t.Fatal("V3 frozen report fixture",err) }
	return independentAncestryFixture{s:s,c:c,file:file,source:source,delivery:delivery,g:g}
}

func independentAncestryReviewer(t *testing.T, f *independentAncestryFixture) {
	t.Helper()
	_,f.first=advanceGroup(t,f.s,f.g.ID)
	if len(f.first.Context.PublishedReports)!=1 { t.Fatal("first stage did not capture original report") }
	f.first=completeChannelWork(t,f.s,f.first,strings.Repeat("Mandatory peer observation. ",75))
	current,w:=advanceGroup(t,f.s,f.g.ID);f.reviewer=w
	if len(current.RequestIDs)!=2||len(w.Context.PublishedReports)!=0||len(w.Context.Sources)!=1||w.Context.Sources[0].RequestID!=f.first.RequestID||w.Context.Omitted<1 { t.Fatal("reviewer fixture did not wholly omit report for required peer") }
	if w.Context.Profile==nil||w.Context.Profile.IncludeHistory||w.BotProfile==nil||w.BotProfile.IncludeHistory { t.Fatal("omission widened reviewer history preference") }
	raw,_:=json.Marshal(struct{Sources []ContextSource;Reports []PublishedReportPin}{w.Context.Sources,w.Context.PublishedReports})
	if len(raw)>8000||strings.Contains(w.RuntimePrompt(),"PRIVATE_REQUEST_NOT_PUBLISHED") { t.Fatal("reviewer envelope widened or imported private request") }
	t.Logf("reviewerBytes=%d directReports=%d requiredPeers=%d omitted=%d",len(raw),len(w.Context.PublishedReports),len(w.Context.Sources),w.Context.Omitted)
}

func independentAncestryFence(t *testing.T,f independentAncestryFixture,kind string) {
	t.Helper()
	var err error
	switch kind {
	case "source":_,_,err=f.s.SetSourceAccess(AccessInput{RequestID:f.source.RequestID,Granted:false,ClientNonce:"ind-ancestry-source-revoke"})
	case "report":_,_,err=f.s.ControlBot(BotControlInput{BotID:"bot-meiiie",Action:"defaults",ExpectedRevision:0,DefaultModel:"local-preference",ClientNonce:"ind-ancestry-report-source-revision"})
	case "file":err=f.s.SetFileAccess(FileAccessInput{FileID:f.file.ID,ExpectedRevision:f.file.AccessRevision,Granted:false,ClientNonce:"ind-ancestry-file-revoke"})
	case "plugin":_,_,err=f.s.ControlPlugin(PluginInput{Version:1,ID:"neko-published-reports",Action:"disable",ClientNonce:"ind-ancestry-reader-disable"})
	default:t.Fatal("unknown fence")
	}
	if err!=nil { t.Fatal("fence",kind,err) }
}

func TestIndependentNextAncestryWholeOmissionReaderAndRestart(t *testing.T) {
	f:=independentAncestryCreate(t);independentAncestryReviewer(t,&f)
	f.s.mu.Lock();_,firstBytes,firstErr:=f.s.readPublishedArtifactLocked(f.first,f.delivery.ID,f.source.Artifacts[0].ID);_,reviewBytes,reviewErr:=f.s.readPublishedArtifactLocked(f.reviewer,f.delivery.ID,f.source.Artifacts[0].ID);f.s.mu.Unlock()
	if firstErr!=nil||len(firstBytes)==0||reviewErr==nil||len(reviewBytes)!=0 { t.Fatalf("omitted report gained direct reader: first=%v reviewer=%v",firstErr,reviewErr) }
	before:=groupState(t,f.s,f.g.ID);beforeWork:=copyWork(f.reviewer);dir:=f.s.dir;f.s.Close()
	reopened,err:=Open(dir);if err!=nil { t.Fatal("replay",err) };defer reopened.Close()
	got,duplicate,err:=reopened.CreateCollaboration(f.g.CollaborationInput);if err!=nil||!duplicate||!reflect.DeepEqual(before,got) { t.Fatal("replay nonce changed frozen group selection",err) }
	restored,err:=reopened.Get(f.reviewer.RequestID);if err!=nil||restored.Context.SHA256!=beforeWork.Context.SHA256||!reflect.DeepEqual(restored.Context,beforeWork.Context) { t.Fatal("replay changed reviewer selection",err) }
	reopened.mu.Lock();_,read,err:=reopened.readPublishedArtifactLocked(restored,f.delivery.ID,f.source.Artifacts[0].ID);reopened.mu.Unlock()
	if err==nil||len(read)!=0 { t.Fatal("replay restored direct reader for omitted report") }
	t.Logf("stableReviewerSHA256=%s frozenReportVersion=%d",restored.Context.SHA256,got.PublishedReportContext.Version)
}

func TestIndependentNextAncestryOmittedReportRevocationFencesQueuedWork(t *testing.T) {
	for _,kind:=range []string{"source","report","file","plugin"} { t.Run(kind,func(t *testing.T){
		f:=independentAncestryCreate(t);independentAncestryReviewer(t,&f)
		frozen:=copyPublishedReportContext(f.g.PublishedReportContext);pinSHA:=f.reviewer.Context.SHA256
		independentAncestryFence(t,f,kind)
		f.s.mu.Lock();allowed:=f.s.contextAllowed(f.s.work[f.reviewer.RequestID]);storedSHA:=f.s.work[f.reviewer.RequestID].Context.SHA256;f.s.mu.Unlock()
		if allowed||storedSHA!=pinSHA||f.s.RuntimeAllowed(f.reviewer.RequestID,f.reviewer.Epoch) { t.Fatal("omission erased inherited authority fence or reselected context") }
		if _,err:=f.s.Claim(f.reviewer.RequestID,f.reviewer.Epoch);!errors.Is(err,ErrConflict) { t.Fatal("revoked inherited dependency remained claimable",err) }
		explanation,err:=f.s.ExplainSources(f.reviewer.RequestID,f.c.ID,"");if err!=nil||explanation.Usable { t.Fatal("source metadata did not report inherited unusability",err) }
		if err=f.s.AdvanceCollaborations();err!=nil { t.Fatal(err) };current:=groupState(t,f.s,f.g.ID)
		if current.Status!="suspended"||current.Reason!="authority_changed"||len(current.RequestIDs)!=2||!reflect.DeepEqual(current.PublishedReportContext,frozen) { t.Fatal("revocation refreshed group authority or stage map") }
		dir:=f.s.dir;f.s.Close();reopened,err:=Open(dir);if err!=nil { t.Fatal("revoked replay",err) };defer reopened.Close()
		if _,err=reopened.Claim(f.reviewer.RequestID,f.reviewer.Epoch);!errors.Is(err,ErrConflict) { t.Fatal("restart revived revoked inherited dependency",err) }
		if after:=groupState(t,reopened,f.g.ID);!reflect.DeepEqual(after,current) { t.Fatal("restart changed suspended group/frozen pins") }
	}) }
}

func TestIndependentNextAncestryCompletedReplyMasksAfterRevocation(t *testing.T) {
	for _,kind:=range []string{"source","report","file","plugin"} { t.Run(kind,func(t *testing.T){
		f:=independentAncestryCreate(t);independentAncestryReviewer(t,&f)
		f.reviewer=completeChannelWork(t,f.s,f.reviewer,"DEPENDENT_PUBLIC_REPLY_CANARY")
		if f.s.PublicWork(f.reviewer).Reply!="DEPENDENT_PUBLIC_REPLY_CANARY" { t.Fatal("completed dependent result was not initially public") }
		independentAncestryFence(t,f,kind)
		public:=f.s.PublicWork(f.reviewer);if public.Reply!=""||len(public.Artifacts)!=0||public.Context!=nil { t.Fatal("completed reply retained content after omitted ancestor revoke") }
		dir:=f.s.dir;f.s.Close();reopened,err:=Open(dir);if err!=nil { t.Fatal("completed revoked replay",err) };defer reopened.Close()
		for _,w:=range reopened.Snapshot().Work { if w.RequestID==f.reviewer.RequestID&&(w.Reply!=""||len(w.Artifacts)!=0||w.Context!=nil) { t.Fatal("restart projection revived completed dependent reply") } }
	}) }
}

func TestIndependentNextAncestryTypedV1V2ReplayKeepsHistoricalGate(t *testing.T) {
	for _,version:=range []int{1,2} { t.Run(string(rune('0'+version)),func(t *testing.T){
		f:=independentAncestryCreate(t)
		probe:=Work{BotID:f.g.CoordinatorBotID,OwnerUserID:f.g.OwnerUserID,SpaceID:f.g.SpaceID,ConversationID:f.g.ChannelID,ToolPolicyVersion:f.g.ToolPolicyVersion,PluginPins:f.g.PluginPins}
		f.s.pinBotProfile(&probe);legacy:=f.s.capturePublishedReportsVersion(probe,f.g.PublishedReportContext.AtSequence,version)
		if !pinnedTool(probe,"report.read_artifact") { stripPublishedFiles(legacy.Pins) }
		dir:=f.s.dir;f.s.Close();journal:=filepath.Join(dir,"transactions.jsonl");raw,err:=os.ReadFile(journal);if err!=nil { t.Fatal(err) }
		lines:=bytes.Split(bytes.TrimSuffix(raw,[]byte("\n")),[]byte("\n"));var last record;if err=json.Unmarshal(lines[len(lines)-1],&last);err!=nil { t.Fatal(err) }
		if last.Event.Type!="collaboration.created" { t.Fatal("typed legacy fixture is not a created record") };last.Event.Collaboration.PublishedReportContext=legacy;last.Digest,err=digestRecord(last);if err!=nil { t.Fatal(err) };lines[len(lines)-1],err=json.Marshal(last);if err!=nil { t.Fatal(err) }
		if err=os.WriteFile(journal,append(bytes.Join(lines,[]byte("\n")),'\n'),0600);err!=nil { t.Fatal(err) }
		reopened,err:=Open(dir);if err!=nil { t.Fatal("typed legacy replay",err) };defer reopened.Close()
		got,duplicate,err:=reopened.CreateCollaboration(f.g.CollaborationInput);if err!=nil||!duplicate||got.PublishedReportContext.Version!=version||!reflect.DeepEqual(got.PublishedReportContext,legacy) { t.Fatal("typed legacy policy changed on replay/nonce",err) }
		_,first:=advanceGroup(t,reopened,got.ID);first=completeChannelWork(t,reopened,first,strings.Repeat("Mandatory peer observation. ",75))
		if err=reopened.AdvanceCollaborations();err!=nil { t.Fatal(err) };current:=groupState(t,reopened,got.ID)
		if current.Status!="needs_owner"||current.Reason!="stage_context_unavailable"||len(current.RequestIDs)!=1||current.PublishedReportContext.Version!=version||!reflect.DeepEqual(current.PublishedReportContext,legacy) { t.Fatal("old policy was silently upgraded to V3 selection") }
		t.Logf("typedHistoricalVersion=%d remains=%s/%s; genuine old-binary journal export is separate evidence",version,current.Status,current.Reason)
	}) }
}
