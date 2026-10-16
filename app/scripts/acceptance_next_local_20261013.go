package ledger

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func independentNextGroupInput(c Channel, nonce string) CollaborationInput {
	return CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Compare the authorized evidence", MaxStages: 3, ClientNonce: nonce}
}

func independentNextContainsSource(w Work, id string) bool {
	for _, source := range w.Context.Sources {
		if source.RequestID == id { return true }
	}
	return false
}

func TestIndependentNextLocalRequiredPeerHistoryOff(t *testing.T) {
	s, c, _ := reportFixture(t, false)
	ambient, _, err := s.AcceptProduct(Input{BotID:"bot-davin", ClientNonce:"ind-next-ambient", Text:"Optional ambient context", Lane:"foreground"}, SubmissionScope{ChannelID:c.ID})
	if err != nil { t.Fatal(err) }
	completeChannelWork(t,s,ambient,"AMBIENT_CANARY_NOT_REQUIRED")
	for _, id := range []string{"bot-meiiie","bot-davin"} {
		if _, _, err=s.SetBotProfile(id,BotProfileInput{Instructions:"Use the explicit group dependency", IncludeHistory:false, ClientNonce:"ind-next-profile-"+id}); err!=nil { t.Fatal(err) }
	}
	g,_,err:=s.CreateCollaboration(independentNextGroupInput(c,"ind-next-historyoff-group"));if err!=nil { t.Fatal(err) }
	_, first:=advanceGroup(t,s,g.ID)
	first=completeChannelWork(t,s,first,"REQUIRED_PEER_RESULT_001")
	if err=s.AdvanceCollaborations();err!=nil { t.Fatal(err) }
	current:=groupState(t,s,g.ID)
	if len(current.RequestIDs)!=2 { t.Fatalf("required peer result unavailable with historyoff: status=%s reason=%s stages=%d proposalBytes=%d",current.Status,current.Reason,len(current.RequestIDs),len(first.Reply)) }
	reviewer,err:=s.Get(current.RequestIDs[1]);if err!=nil { t.Fatal(err) }
	if reviewer.BotProfile==nil||reviewer.BotProfile.IncludeHistory||!independentNextContainsSource(reviewer,first.RequestID)||independentNextContainsSource(reviewer,ambient.RequestID)||strings.Contains(reviewer.RuntimePrompt(),"AMBIENT_CANARY_NOT_REQUIRED") { t.Fatal("required dependency/optional ambient profile boundary changed") }
	t.Logf("historyoff preserved; required peer source included; ambient excluded; stages=%d",len(current.RequestIDs))
}

func TestIndependentNextLocalRequiredPeerEncodedControlsGate(t *testing.T) {
	s,c,_:=reportFixture(t,false)
	g,_,err:=s.CreateCollaboration(independentNextGroupInput(c,"ind-next-controls-group"));if err!=nil { t.Fatal(err) }
	_,first:=advanceGroup(t,s,g.ID)
	first=completeChannelWork(t,s,first,"CONTROL_PREFIX_"+strings.Repeat("\x01",1400))
	encoded,_:=json.Marshal([]ContextSource{{RequestID:first.RequestID,MessageID:first.SourceMessageID,Sequence:first.CreatedSequence,Epoch:first.Epoch,Text:first.Text,Reply:first.Reply,AccessRevision:accessRevision(first),BotID:first.BotID}})
	if len(first.Reply)>=8000||len(encoded)<=8000 { t.Fatal("fixture fails raw-versus-encoded byte distinction") }
	if err=s.AdvanceCollaborations();err!=nil { t.Fatal(err) }
	current:=groupState(t,s,g.ID)
	if current.Status!="needs_owner"||current.Reason!="stage_context_unavailable"||len(current.RequestIDs)!=1 { t.Fatal("encoded required source exceeded 8KB but reviewer was launched") }
	t.Logf("rawReplyBytes=%d encodedSourcesBytes=%d gate=%s/%s",len(first.Reply),len(encoded),current.Status,current.Reason)
}

func TestIndependentNextLocalRequiredPeerBeforeOptionalReportBytes(t *testing.T) {
	s,c,_:=reportFixture(t,false)
	source,_,err:=s.Accept(Input{BotID:"bot-meiiie",ClientNonce:"ind-next-large-report-source",Text:"A private report request",Lane:"foreground"});if err!=nil { t.Fatal(err) }
	completeChannelWork(t,s,source,strings.Repeat("Public optional evidence. ",250))
	delivery:=reportSubmission(c,"ind-next-large-report-delivery");delivery.Text="@Meiiie gửi báo cáo mới nhất ở cuộc trò chuyện riêng vào kênh này"
	if _,_,err=s.DeliverReport(delivery);err!=nil { t.Fatal(err) }
	g,_,err:=s.CreateCollaboration(independentNextGroupInput(c,"ind-next-priority-group"));if err!=nil { t.Fatal(err) }
	if len(g.PublishedReportContext.Pins)!=1 { t.Fatal("optional report fixture was not originally pinned") }
	_,first:=advanceGroup(t,s,g.ID)
	first=completeChannelWork(t,s,first,strings.Repeat("Required proposal point. ",70))
	required:=[]ContextSource{{RequestID:first.RequestID,MessageID:first.SourceMessageID,Sequence:first.CreatedSequence,Epoch:first.Epoch,Text:first.Text,Reply:first.Reply,AccessRevision:accessRevision(first),BotID:first.BotID}}
	rawRequired,_:=json.Marshal(required)
	combined,_:=json.Marshal(struct{Sources []ContextSource;Reports []PublishedReportPin}{required,g.PublishedReportContext.Pins})
	if len(rawRequired)>=8000||len(combined)<=8000 { t.Fatalf("fixture budget distinction invalid: required=%d combined=%d",len(rawRequired),len(combined)) }
	if err=s.AdvanceCollaborations();err!=nil { t.Fatal(err) }
	current:=groupState(t,s,g.ID)
	if len(current.RequestIDs)!=2 { t.Fatalf("optional reports starved required peer source: requiredBytes=%d combinedBytes=%d status=%s reason=%s stages=%d",len(rawRequired),len(combined),current.Status,current.Reason,len(current.RequestIDs)) }
	reviewer,err:=s.Get(current.RequestIDs[1]);if err!=nil { t.Fatal(err) }
	actual,_:=json.Marshal(struct{Sources []ContextSource;Reports []PublishedReportPin}{reviewer.Context.Sources,reviewer.Context.PublishedReports})
	if !independentNextContainsSource(reviewer,first.RequestID)||len(actual)>8000 { t.Fatal("reviewer lost required source or widened byte bound") }
	t.Logf("requiredBytes=%d oldCombinedBytes=%d admittedBytes=%d reportsSelected=%d",len(rawRequired),len(combined),len(actual),len(reviewer.Context.PublishedReports))
}

func TestIndependentNextLocalUnselectedReportHasNoTaskAuthority(t *testing.T) {
	s,c,original:=reportFixture(t,true)
	selected:=reportSubmission(c,"ind-next-selected-file-report");selected.Text="@Meiiie gửi báo cáo tệp ở cuộc trò chuyện riêng vào kênh này"
	firstDelivery,_,err:=s.DeliverReport(selected);if err!=nil { t.Fatal(err) }
	g,_,err:=s.CreateCollaboration(independentNextGroupInput(c,"ind-next-unselected-group"));if err!=nil { t.Fatal(err) }
	_,task:=advanceGroup(t,s,g.ID)
	later,_,err:=s.Accept(Input{BotID:"bot-meiiie",ClientNonce:"ind-next-later-source",Text:"An independently completed later report",Lane:"foreground"});if err!=nil { t.Fatal(err) }
	later=startPermissionWork(t,s,later)
	if _,err=s.ExecuteTool(later.RequestID,later.Epoch,invocation("LATER_UNSELECTED_REPORT_FILE"));err!=nil { t.Fatal(err) }
	if err=s.AppendText(later.RequestID,later.Epoch,"LATER_UNSELECTED_REPORT_RESULT");err!=nil { t.Fatal(err) }
	if err=s.Finish(later.RequestID,later.Epoch,"completed","");err!=nil { t.Fatal(err) }
	later,err=s.Get(later.RequestID);if err!=nil { t.Fatal(err) }
	lateDeliveryInput:=reportSubmission(c,"ind-next-later-file-delivery");lateDeliveryInput.Text="@Meiiie gửi báo cáo tệp mới nhất ở cuộc trò chuyện riêng vào kênh này"
	lateDelivery,_,err:=s.DeliverReport(lateDeliveryInput);if err!=nil||lateDelivery.Source==nil||lateDelivery.Source.RequestID!=later.RequestID||len(lateDelivery.Artifacts)!=1 { t.Fatal("later publication fixture",err) }
	s.mu.Lock();_,selectedBytes,selectedErr:=s.readPublishedArtifactLocked(task,firstDelivery.ID,original.Artifacts[0].ID);_,lateBytes,lateErr:=s.readPublishedArtifactLocked(task,lateDelivery.ID,later.Artifacts[0].ID);s.mu.Unlock()
	if selectedErr!=nil||len(selectedBytes)==0||!errors.Is(lateErr,ErrNotFound)||len(lateBytes)!=0 { t.Fatalf("unselected delivery obtained task authority: selected=%v later=%v laterBytes=%d",selectedErr,lateErr,len(lateBytes)) }
	if _,_,err=s.SetSourceAccess(AccessInput{RequestID:later.RequestID,Granted:false,ClientNonce:"ind-next-unselected-revoke"});err!=nil { t.Fatal(err) }
	if err=s.AdvanceCollaborations();err!=nil { t.Fatal(err) }
	current:=groupState(t,s,g.ID)
	if current.Status!="running"||!reflect.DeepEqual(current.PublishedReportContext,g.PublishedReportContext) { t.Fatal("unselected publication or its revocation retargeted selected authority") }
	t.Log("later public report read denied task-locally; its unrelated revocation did not suspend original selected group")
}

func TestIndependentNextLocalLateChannelRevisionFencesClaim(t *testing.T) {
	s,c,_:=reportFixture(t,false)
	g,_,err:=s.CreateCollaboration(independentNextGroupInput(c,"ind-next-channel-revoke-group"));if err!=nil { t.Fatal(err) }
	_,task:=advanceGroup(t,s,g.ID)
	if _,_,err=s.SaveChannel(ChannelInput{ID:c.ID,Name:c.Name,ExpectedRevision:c.Revision,BotIDs:[]string{"bot-meiiie"},ClientNonce:"ind-next-channel-member-remove"});err!=nil { t.Fatal(err) }
	if err=s.AdvanceCollaborations();err!=nil { t.Fatal(err) }
	current:=groupState(t,s,g.ID)
	if current.Status!="suspended"||current.Reason!="authority_changed"||len(current.RequestIDs)!=1||!reflect.DeepEqual(current.PublishedReportContext,g.PublishedReportContext) { t.Fatal("late audience revision refreshed group or admitted stage") }
	if _,err=s.Claim(task.RequestID,task.Epoch);!errors.Is(err,ErrConflict) { t.Fatal("original task claim succeeded after audience revision",err) }
}

func TestIndependentNextLocalLateUploadedFileRevokeFencesClaim(t *testing.T) {
	s,c,p:=productFixture(t)
	file,_,err:=s.UploadFile(UploadInput{ProjectID:p.ID,ProjectRevision:p.Revision,ChannelID:c.ID,ClientNonce:"ind-next-selected-upload",Filename:"evidence.md",Content:[]byte("SELECTED_PUBLIC_FILE_EVIDENCE")});if err!=nil { t.Fatal(err) }
	in:=independentNextGroupInput(c,"ind-next-file-revoke-group");in.ProjectID=p.ID;in.ProjectRevision=p.Revision;in.FileIDs=[]string{file.ID}
	g,_,err:=s.CreateCollaboration(in);if err!=nil { t.Fatal(err) }
	_,task:=advanceGroup(t,s,g.ID)
	if err=s.SetFileAccess(FileAccessInput{FileID:file.ID,ExpectedRevision:file.AccessRevision,Granted:false,ClientNonce:"ind-next-selected-file-revoke"});err!=nil { t.Fatal(err) }
	if err=s.AdvanceCollaborations();err!=nil { t.Fatal(err) }
	current:=groupState(t,s,g.ID)
	if current.Status!="suspended"||current.Reason!="authority_changed"||len(current.RequestIDs)!=1||!reflect.DeepEqual(current.Resources,g.Resources) { t.Fatal("late file revoke refreshed or replaced frozen resource pin") }
	if _,err=s.Claim(task.RequestID,task.Epoch);!errors.Is(err,ErrConflict) { t.Fatal("queued task claim succeeded after selected file revoke",err) }
	if _,raw,err:=s.ReadProjectFile(p.ID,c.ID,file.ID);!errors.Is(err,ErrConflict)||len(raw)!=0 { t.Fatal("revoked uploaded source remained readable",err) }
}
