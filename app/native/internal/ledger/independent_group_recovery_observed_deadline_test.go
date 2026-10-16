package ledger
import("errors";"testing";"time")
func TestIndependentGroupRecoveryObservedDeadlineCannotRewind(t *testing.T){
 s,_,g,w,_:=groupRecoveryFixture(t,1);deadline,e:=time.Parse(time.RFC3339Nano,g.DeadlineAt);if e!=nil{t.Fatal(e)};in:=groupRecoverInput(g,w,"independent-observed-group-deadline");s.now=func()time.Time{return deadline};if _,_,e=s.RecoverCollaboration(g.ID,in);!errors.Is(e,ErrConflict){t.Fatal("positive original deadline denial",e)};if e=s.AdvanceCollaborations();e!=nil{t.Fatal("observe finite deadline during stopped group",e)};s.now=func()time.Time{return deadline.Add(-time.Nanosecond)};if _,_,e=s.RecoverCollaboration(g.ID,in);!errors.Is(e,ErrConflict){t.Fatal("already-observed original five-minute group expiry revives stopped group when clock rewinds inside saved window",e)}
}
