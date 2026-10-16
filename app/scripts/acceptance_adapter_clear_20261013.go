package ledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type independentAdapterClearFixture struct {
	s *Store
	c Channel
	a AdapterSession
	f UploadedFile
	w Work
	input AdapterMessageInput
	consumed AdapterPage
}

func independentAdapterClearCreate(t *testing.T, partial bool) independentAdapterClearFixture {
	t.Helper()
	s,c,p:=productFixture(t)
	if _,_,err:=s.SetBotProfile("bot-meiiie",BotProfileInput{IncludeHistory:false,ClientNonce:"ind-adapter-no-ambient"});err!=nil { t.Fatal(err) }
	a,_,err:=s.RegisterAdapter(AdapterSessionInput{SourceApp:"core-tui",SourceSessionID:"independent-source-session",SourceThreadID:"independent-source-thread",ChannelID:c.ID,ExpectedChannelRevision:c.Revision,ClientNonce:"independent-adapter-session"});if err!=nil { t.Fatal(err) }
	f,_,err:=s.UploadFile(UploadInput{ProjectID:p.ID,ProjectRevision:p.Revision,ChannelID:c.ID,Filename:"owned-source.md",Content:[]byte("INDEPENDENT_SELECTED_SOURCE"),ClientNonce:"ind-independent-selected-file"});if err!=nil { t.Fatal(err) }
	in:=AdapterMessageInput{Input:Input{BotID:"bot-meiiie",ClientNonce:"independent-external-message",Text:"Use only the selected local source",Lane:"foreground"},SourceMessageID:"independent-external-id",ProjectID:p.ID,ProjectRevision:p.Revision,FileIDs:[]string{f.ID}}
	w,_,err:=s.AcceptAdapter(a.ID,in);if err!=nil { t.Fatal(err) };w=startPermissionWork(t,s,w)
	if !partial { if _,err=s.ExecuteTool(w.RequestID,w.Epoch,invocation("INDEPENDENT_DERIVED_ARTIFACT"));err!=nil { t.Fatal(err) } }
	text:="INDEPENDENT_COMPLETED_DERIVED_REPLY";if partial { text="INDEPENDENT_PARTIAL_DERIVED_REPLY" }
	if err=s.AppendText(w.RequestID,w.Epoch,text);err!=nil { t.Fatal(err) }
	if !partial { if err=s.Finish(w.RequestID,w.Epoch,"completed","");err!=nil { t.Fatal(err) } };w,err=s.Get(w.RequestID);if err!=nil { t.Fatal(err) }
	page,err:=s.AdapterReplies(a.ID,0);if err!=nil||len(page.Replies)!=1||page.Replies[0].Text!=text||page.Replies[0].SourceMessageID!=in.SourceMessageID { t.Fatal("consumed adapter fixture",err) }
	return independentAdapterClearFixture{s:s,c:c,a:a,f:f,w:w,input:in,consumed:page}
}

func independentAdapterClearRevoke(t *testing.T,f independentAdapterClearFixture,kind string) uint64 {
	t.Helper()
	before:=uint64(len(f.s.events));var err error
	if kind=="file" { err=f.s.SetFileAccess(FileAccessInput{FileID:f.f.ID,ExpectedRevision:f.f.AccessRevision,Granted:false,ClientNonce:"independent-adapter-file-revoke"}) } else { _,_,err=f.s.ControlPlugin(PluginInput{Version:1,ID:"neko-project-sources",Action:"disable",ClientNonce:"independent-adapter-plugin-disable"}) }
	if err!=nil { t.Fatal(err) }
	for _,e:=range f.s.events[before:] { if kind=="file"&&e.Type=="file.access"||kind=="plugin"&&e.Plugin!=nil { return e.Sequence } }
	t.Fatal("missing causal authority event");return 0
}

func independentAdapterStateDigest(s *Store) string {
	raw,_:=json.Marshal(struct{Work map[string]Work;MessageNonces map[string]string;AdapterNonces map[string]Event;ProductNonces map[string]Event;PluginNonces map[string]Event;ReportNonces map[string]string;TaskNonces map[string]Event;CoreNonces map[string]Event}{s.work,s.nonces,s.adapterNonces,s.productNonces,s.pluginNonces,s.reportDeliveryNonces,s.taskControlNonces,s.coreProfileNonces})
	digest:=sha256.Sum256(raw);return hex.EncodeToString(digest[:])
}

func independentAdapterAssertClear(t *testing.T,f independentAdapterClearFixture,page AdapterPage,after,causal uint64) {
	t.Helper()
	if len(page.Replies)!=1 { t.Fatalf("expected one exact clear, got=%d",len(page.Replies)) };clear:=page.Replies[0];old:=f.consumed.Replies[0]
	if clear.ReplyID!=old.ReplyID||clear.RequestID!=old.RequestID||clear.SourceMessageID!=old.SourceMessageID||clear.BotID!=old.BotID||clear.Role!=old.Role||clear.Text!=""||len(clear.Artifacts)!=0||clear.Sequence<=after||clear.Sequence<causal||clear.Sequence>page.Cursor||clear.Sequence>uint64(len(f.s.events)) { t.Fatal("clear lacked durable greater sequence or changed target identity") }
}

func TestIndependentAdapterClearIncremental(t *testing.T) {
	for _,scenario:=range []struct{name,kind string;partial bool}{{"completed-file","file",false},{"completed-plugin","plugin",false},{"partial-file","file",true},{"partial-plugin","plugin",true}} { t.Run(scenario.name,func(t *testing.T){
		f:=independentAdapterClearCreate(t,scenario.partial);causal:=independentAdapterClearRevoke(t,f,scenario.kind);before:=uint64(len(f.s.events));journal:=filepath.Join(f.s.dir,"transactions.jsonl");bytesBefore,_:=os.ReadFile(journal)
		delta,err:=f.s.AdapterReplies(f.a.ID,f.consumed.Cursor);if err!=nil { t.Fatal(err) };independentAdapterAssertClear(t,f,delta,f.consumed.Cursor,causal)
		retry,err:=f.s.AdapterReplies(f.a.ID,f.consumed.Cursor);if err!=nil||!reflect.DeepEqual(retry,delta) { t.Fatal("same cursor retry changed projection",err) }
		next,err:=f.s.AdapterReplies(f.a.ID,delta.Cursor);if err!=nil||len(next.Replies)!=0||next.Cursor!=delta.Cursor||uint64(len(f.s.events))!=before { t.Fatal("consumed incremental clear blocked next poll or wrote journal",err) }
		bytesAfter,_:=os.ReadFile(journal);if !bytes.Equal(bytesBefore,bytesAfter) { t.Fatal("incremental read rewrote journal") }
		t.Logf("consumedCursor=%d causalAuthority=%d clearSequence=%d clearCursor=%d",f.consumed.Cursor,causal,delta.Replies[0].Sequence,delta.Cursor)
	}) }
}

func TestIndependentAdapterClearOpenRepair(t *testing.T) {
	for _,scenario:=range []struct{name,kind string;partial bool}{{"completed-file","file",false},{"partial-plugin","plugin",true}} { t.Run(scenario.name,func(t *testing.T){
		f:=independentAdapterClearCreate(t,scenario.partial);causal:=independentAdapterClearRevoke(t,f,scenario.kind);legacyCursor:=uint64(len(f.s.events));beforeState:=independentAdapterStateDigest(f.s);beforeWork:=copyWork(f.s.work[f.w.RequestID]);dir:=f.s.dir;f.s.Close()
		reopened,err:=Open(dir);if err!=nil { t.Fatal("repair open",err) };defer reopened.Close();f.s=reopened
		if uint64(len(reopened.events))!=legacyCursor+1||independentAdapterStateDigest(reopened)!=beforeState||!reflect.DeepEqual(reopened.work[f.w.RequestID],beforeWork) { t.Fatal("repair did not add only one greater-sequence checkpoint or rewrote Work/nonces") }
		checkpoint:=reopened.events[len(reopened.events)-1]
		if !reflect.DeepEqual(checkpoint,Event{Type:"adapter.reply_clear_checkpoint",Sequence:legacyCursor+1,Work:Work{RequestID:f.w.RequestID},Nonce:adapterReplyClearNonce(beforeWork,causal)}) { t.Fatal("repair event contained full Work or incorrect causal key") }
		delta,err:=reopened.AdapterReplies(f.a.ID,legacyCursor);if err!=nil { t.Fatal(err) };independentAdapterAssertClear(t,f,delta,legacyCursor,causal)
		if delta.Replies[0].Sequence!=checkpoint.Sequence { t.Fatal("repair clear did not refer to actual journal checkpoint") }
		ack,duplicate,err:=reopened.AcceptAdapter(f.a.ID,f.input);if err!=nil||!duplicate||!reflect.DeepEqual(ack,beforeWork) { t.Fatal("repair altered original adapter nonce ACK",err) }
		bytesAfterRepair,_:=os.ReadFile(filepath.Join(dir,"transactions.jsonl"));next,err:=reopened.AdapterReplies(f.a.ID,delta.Cursor);if err!=nil||len(next.Replies)!=0||next.Cursor!=delta.Cursor { t.Fatal("next poll after repaired clear was blocked",err) };reopened.Close()
		restarted,err:=Open(dir);if err!=nil { t.Fatal("second restart",err) };defer restarted.Close();again,err:=restarted.AdapterReplies(f.a.ID,legacyCursor)
		bytesAfterRestart,_:=os.ReadFile(filepath.Join(dir,"transactions.jsonl"));if err!=nil||!reflect.DeepEqual(again,delta)||!bytes.Equal(bytesAfterRepair,bytesAfterRestart)||independentAdapterStateDigest(restarted)!=beforeState { t.Fatal("repair repeated, changed Work/nonces, or changed clear on second restart",err) }
		t.Logf("legacyConsumedAuthorityCursor=%d repairSequence=%d; fullWorkAndNonceMapsUnchanged=true",legacyCursor,checkpoint.Sequence)
	}) }
}

func TestIndependentAdapterClearReadOnly(t *testing.T) {
	f:=independentAdapterClearCreate(t,false);independentAdapterClearRevoke(t,f,"file");legacyCursor:=uint64(len(f.s.events));dir:=f.s.dir;f.s.Close();journal:=filepath.Join(dir,"transactions.jsonl");before,_:=os.ReadFile(journal)
	readonly,err:=OpenReadOnly(dir);if err!=nil { t.Fatal(err) };defer readonly.Close()
	if _,err=readonly.AdapterReplies(f.a.ID,legacyCursor);!errors.Is(err,ErrConflict) { t.Fatal("read-only consumed authority cursor was acknowledged without durable repair",err) }
	if err=readonly.recoverAdapterReplyClears();err!=nil||uint64(len(readonly.events))!=legacyCursor { t.Fatal("read-only recovery mutated sequence",err) }
	readonly.Close();after,_:=os.ReadFile(journal);if !bytes.Equal(before,after) { t.Fatal("read-only open/poll/recovery modified journal bytes") }
}

func TestIndependentAdapterClearMalformedAndCollision(t *testing.T) {
	f:=independentAdapterClearCreate(t,false);causal:=independentAdapterClearRevoke(t,f,"file");current:=copyWork(f.s.work[f.w.RequestID]);nonce:=adapterReplyClearNonce(current,causal)
	canonical:=Event{Type:"adapter.reply_clear_checkpoint",Work:Work{RequestID:f.w.RequestID},Nonce:nonce};before:=uint64(len(f.s.events));stateBefore:=independentAdapterStateDigest(f.s);journal:=filepath.Join(f.s.dir,"transactions.jsonl");bytesBefore,_:=os.ReadFile(journal)
	mutations:=[]func(*Event){func(e *Event){e.Work.Reply="FORBIDDEN_FULL_WORK_BODY"},func(e *Event){e.Work.RequestID="req-unknown-target"},func(e *Event){e.Work.Adapter=&AdapterPin{AdapterSessionID:"adapter-wrong-target"}},func(e *Event){e.Nonce="independent-wrong-checkpoint-key"},func(e *Event){e.Channel=&Channel{ID:f.c.ID}}}
	for i,mutate:=range mutations { e:=canonical;mutate(&e);if err:=f.s.commit(e);!errors.Is(err,ErrInvalid)||uint64(len(f.s.events))!=before||f.s.poisoned||independentAdapterStateDigest(f.s)!=stateBefore { t.Fatalf("malformed checkpoint %d changed journal/state or global availability: %v",i,err) } }
	bytesAfter,_:=os.ReadFile(journal);if !bytes.Equal(bytesBefore,bytesAfter) { t.Fatal("rejected checkpoints changed journal bytes") }
	registration:=AdapterSessionInput{SourceApp:"core-tui",SourceSessionID:"independent-other-session",SourceThreadID:"independent-other-thread",ChannelID:f.c.ID,ExpectedChannelRevision:f.c.Revision,ClientNonce:nonce}
	other,_,err:=f.s.RegisterAdapter(registration);if err!=nil { t.Fatal("valid client nonce collision setup",err) };collisionState:=independentAdapterStateDigest(f.s);prior:=uint64(len(f.s.events))
	if err=f.s.recoverAdapterReplyClears();err!=nil||uint64(len(f.s.events))!=prior+1||f.s.poisoned||independentAdapterStateDigest(f.s)!=collisionState||f.s.adapterNonces[nonce].AdapterSession.ID!=other.ID||f.s.adapterReplyClearCheckpoints()[nonce]!=prior+1 { t.Fatal("client adapter nonce collision blocked internal clear, overwrote maps or globally denied",err) }
	if err=f.s.commit(canonical);!errors.Is(err,ErrInvalid)||uint64(len(f.s.events))!=prior+1||f.s.poisoned { t.Fatal("duplicate checkpoint committed or poisoned store",err) }
	registered,duplicate,err:=f.s.RegisterAdapter(registration);if err!=nil||!duplicate||registered.ID!=other.ID { t.Fatal("internal repair changed valid client registration nonce",err) }
	independent,_,err:=f.s.AcceptAdapter(other.ID,AdapterMessageInput{Input:Input{BotID:"bot-meiiie",ClientNonce:"independent-other-message",Text:"Independent unbound request",Lane:"foreground"},SourceMessageID:"independent-other-source-id"});if err!=nil { t.Fatal("unrelated adapter intake globally denied",err) };independent=completeChannelWork(t,f.s,independent,"INDEPENDENT_UNRELATED_REPLY")
	page,err:=f.s.AdapterReplies(other.ID,0);if err!=nil||len(page.Replies)!=1||page.Replies[0].RequestID!=independent.RequestID||page.Replies[0].Text!=independent.Reply { t.Fatal("affected adapter globally denied or retargeted unrelated session",err) }
}
