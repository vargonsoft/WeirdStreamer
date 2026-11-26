package fileio

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func privateNow(t *testing.T, path string) bool {
	t.Helper()
	ok, err := IsOwnerOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

// A folder made owner-only keeps out everyone else (Users, Authenticated Users, Everyone), and so
// do files and folders made in it later, which inherit its ACL; a file is the same on its own.
func TestOwnerOnlyWindows(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "data")
	os.Mkdir(dir, 0o700)
	existing := filepath.Join(dir, "users.db")
	os.WriteFile(existing, []byte("x"), 0o600)
	// A folder open to everyone, as one under C:\ProgramData or C:\ws is.
	everyone, _ := windows.CreateWellKnownSid(windows.WinWorldSid)
	acl, _ := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{AccessPermissions: windows.GENERIC_ALL,
		AccessMode: windows.GRANT_ACCESS, Inheritance: windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
		Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
			TrusteeValue: windows.TrusteeValueFromSID(everyone)}}}, nil)
	if err := windows.SetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	if privateNow(t, dir) {
		t.Fatal("the folder starts private: the test shows nothing")
	}

	if err := OwnerOnly(dir); err != nil {
		t.Fatal(err)
	}
	if !privateNow(t, dir) || !privateNow(t, existing) {
		t.Errorf("after OwnerOnly: folder %v, its file %v", privateNow(t, dir), privateNow(t, existing))
	}
	later := filepath.Join(dir, "users.db-wal")
	os.WriteFile(later, []byte("x"), 0o600)
	sub := filepath.Join(dir, "sub")
	os.Mkdir(sub, 0o700)
	if !privateNow(t, later) || !privateNow(t, sub) {
		t.Errorf("made later: file %v, folder %v", privateNow(t, later), privateNow(t, sub))
	}
	if err := OwnerOnly(dir); err != nil { // already so: left alone
		t.Fatal(err)
	}

	key := filepath.Join(root, "local.key")
	os.WriteFile(key, []byte("secret"), 0o600)
	if privateNow(t, key) {
		t.Fatal("the file starts private")
	}
	if err := OwnerOnly(key); err != nil {
		t.Fatal(err)
	}
	if !privateNow(t, key) {
		t.Error("the file isn't private")
	}
	if b, err := ReadFile(key); err != nil || string(b) != "secret" {
		t.Errorf("the owner can't read it: %q %v", b, err)
	}
}
