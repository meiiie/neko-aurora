package providerexec

import (
 "encoding/json"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func witnessFixture(t *testing.T) (WitnessConfig, *bool) {
 t.Helper(); current:=true
 b:=SavedTaskBinding{FixtureOnly:true,AccountID:"account-"+strings.Repeat("a",32),AccountRevision:1,CredentialGeneration:1,Provider:"openai-compatible",Endpoint:"http://127.0.0.1:49152/v1",Model:"fixture-model",RequestID:"fixture-request",AttemptID:"fixture-attempt",Epoch:1,LeaseFence:1,ContextSHA256:strings.Repeat("b",64),ConsentID:"fixture-consent",ConsentRevision:1,ExpiresAtUnixMS:time.Now().UnixMilli()+30000,Limits:SavedTaskLimits{16000,256,2,2,30}}
 cfg:=WitnessConfig{Path:filepath.Join(t.TempDir(),"authority.json"),Binding:b,ServiceGeneration:"executor-boot-"+strings.Repeat("c",32),AuthoritySHA256:strings.Repeat("d",64),RevisionKey:strings.Repeat("6",64),CurrentTask:func(got SavedTaskBinding)bool{return current && got==b}}
 return cfg,&current
}

func TestWitnessDefaultDenyAndDurableRetirement(t *testing.T){
 cfg,live:=witnessFixture(t);cfg.CurrentTask=nil;c,err:=OpenWitness(cfg);if err!=nil{t.Fatal(err)};if _,err=c.Issue();err==nil{t.Fatal("missing authority accepted")}
 cfg,live=witnessFixture(t);c,err=OpenWitness(cfg);if err!=nil{t.Fatal(err)};one,err:=c.Issue();if err!=nil||one.Revision!=1{t.Fatal("initial issuance",err)}
 two,err:=c.Issue();if err!=nil||two.Revision!=2{t.Fatal("heartbeat",err)};*live=false;if _,err=c.Issue();err==nil{t.Fatal("revoked accepted")};*live=true
 reopened,err:=OpenWitness(cfg);if err!=nil{t.Fatal(err)};if _,err=reopened.Issue();err==nil{t.Fatal("retired regrant")}
 raw,_:=os.ReadFile(cfg.Path);if strings.Contains(string(raw),cfg.RevisionKey)||!strings.Contains(string(raw),`"state":"retired"`){t.Fatal("retirement/key boundary")}
}
func TestWitnessPostWriteRevokeAndRetirementIOPoison(t *testing.T){
 cfg,_:=witnessFixture(t);calls:=0;cfg.CurrentTask=func(SavedTaskBinding)bool{calls++;return calls<3};c,err:=OpenWitness(cfg);if err!=nil{t.Fatal(err)};if _,err=c.Issue();err==nil{t.Fatal("postwrite revoke accepted")}
 reopened,err:=OpenWitness(cfg);if err!=nil{t.Fatal(err)};reopened.currentTask=func(SavedTaskBinding)bool{return true};if _,err=reopened.Issue();err==nil{t.Fatal("revoke not durable")}
 cfg,_=witnessFixture(t);c,_=OpenWitness(cfg);if _,err=c.Issue();err!=nil{t.Fatal(err)};os.Remove(cfg.Path);os.Mkdir(cfg.Path,0700);if err=c.Retire();err==nil{t.Fatal("retirement IO expected")};os.Remove(cfg.Path);if _,err=c.Issue();err==nil{t.Fatal("IO poison reset")}
}
func TestWitnessTamperForeignRollbackDeletionExpiryClock(t *testing.T){
 for _,kind:=range []string{"tamper","foreign","rollback","delete","expired","clock"}{t.Run(kind,func(t *testing.T){cfg,_:=witnessFixture(t);c,_:=OpenWitness(cfg);if _,err:=c.Issue();err!=nil{t.Fatal(err)};one,_:=os.ReadFile(cfg.Path);if _,err:=c.Issue();err!=nil{t.Fatal(err)}
 switch kind{case "tamper":os.WriteFile(cfg.Path,[]byte(`{}`),0600);case "foreign":cfg2,_:=witnessFixture(t);cfg2.Path=filepath.Join(filepath.Dir(cfg.Path),"other.json");cfg2.Binding.RequestID="other-request";cfg2.CurrentTask=func(SavedTaskBinding)bool{return true};other,_:=OpenWitness(cfg2);other.Issue();bytes,_:=os.ReadFile(cfg2.Path);os.WriteFile(cfg.Path,bytes,0600);case "rollback":os.WriteFile(cfg.Path,one,0600);case "delete":os.Remove(cfg.Path);case "expired":c.now=func()time.Time{return time.Now().Add(6*time.Second)};case "clock":c.now=func()time.Time{return time.Now().Add(-time.Second)}}
 if _,err:=c.Issue();err==nil{t.Fatal("unsafe renewal",kind)};c.now=time.Now;if _,err:=c.Issue();err==nil{t.Fatal("terminal reset",kind)}
 })}
}
func TestWitnessClosedBindingAndPrivatePath(t *testing.T){
 cfg,_:=witnessFixture(t);raw,_:=json.Marshal(cfg.Binding);if _,err:=ParseSavedTaskBinding(raw);err!=nil{t.Fatal(err)}
 for _,bad:=range []string{strings.Replace(string(raw),`"fixtureOnly":true,`,"",1),strings.Replace(string(raw),`"accountRevision":1`,`"accountRevision":1,"accountRevision":1`,1),strings.Replace(string(raw),`"epoch":1`,`"epoch":0`,1)}{if _,err:=ParseSavedTaskBinding([]byte(bad));err==nil{t.Fatal("binding accepted")}}
 target:=filepath.Join(t.TempDir(),"linked.json");os.WriteFile(target,[]byte(`{}`),0600);if err:=os.Link(target,cfg.Path);err!=nil{t.Fatal(err)};if _,err:=OpenWitness(cfg);err==nil{t.Fatal("hardlink accepted")}
}
func TestWitnessCrossLanguageVector(t *testing.T){
 cfg,_:=witnessFixture(t);cfg.Binding.ExpiresAtUnixMS=2000000030000;hash,err:=SavedTaskBindingHash(cfg.Binding);if err!=nil{t.Fatal(err)}
 body:=WitnessBody{1,"saved_account_authority_v1",hash,cfg.ServiceGeneration,cfg.AuthoritySHA256,1,"active",2000000000000,2000000005000}
 raw,err:=signedWitness(body,cfg.RevisionKey);if err!=nil{t.Fatal(err)}
 if hash!="VECTOR_BINDING"||string(raw)!="VECTOR_RECORD"{t.Fatalf("vector mismatch hash=%s record=%s",hash,raw)}
}
