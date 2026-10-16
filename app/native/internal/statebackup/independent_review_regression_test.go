package statebackup

import (
 "errors"
 "os"
 "path/filepath"
 "testing"
)

func TestIndependentRestoreRejectsTrailingDotBackupAlias(t *testing.T){
 source,_:=fixture(t);backup:=filepath.Join(t.TempDir(),"backup-original");if _,err:=Create(source,backup,"0680b80897e53bb101c96af8781cd622263866d9");err!=nil{t.Fatal(err)}
 before,err:=os.ReadFile(filepath.Join(backup,"manifest.json"));if err!=nil{t.Fatal(err)}
 target:=backup+"."+string(filepath.Separator)+"nested-restored"
 if _,err=RestoreDryRun(backup,target);err==nil{
  t.Error("restore accepted a trailing-dot alias inside its source backup, violating disjoint target and modifying original backup")
  if _,e:=Verify(backup);e==nil{t.Error("control: modified backup unexpectedly still valid")}else if !errors.Is(e,ErrInvalid){t.Error("verification error",e)}
  after,_:=os.ReadFile(filepath.Join(backup,"manifest.json"));if string(after)!=string(before){t.Error("manifest bytes changed as well")}
  if _,e:=os.Stat(filepath.Join(backup,"nested-restored","restore-readonly.json"));e!=nil{t.Error("control: restore not actually nested in original backup",e)}
 }
}
