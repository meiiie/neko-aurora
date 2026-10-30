package aurora

import "testing"

func TestWorkspaceIDPersistsAndSeparatesFreshInstallations(t *testing.T) {
	dir := t.TempDir()
	id, err := loadWorkspaceID(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	again, err := loadWorkspaceID(dir, "")
	if err != nil || id != again {
		t.Fatal("workspace changed on reload", err)
	}
	other, err := loadWorkspaceID(t.TempDir(), "")
	if err != nil || other == id {
		t.Fatal("workspace collision", err)
	}
	configured, err := loadWorkspaceID(dir, "account-test")
	if err != nil || configured != "account-test" {
		t.Fatal("hosted scope", err)
	}
}
