//go:build windows

package fileio

import (
	"errors"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const shareAll = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE

// OpenFile opens a file as os.OpenFile does, but sharing delete access, so the file can be renamed
// or deleted while it's open (POSIX-like: the name goes at once and the handle reads on). Folders
// are opened by os.OpenFile (Readdir needs its own kind of File).
func OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	if name == "" {
		return nil, &os.PathError{Op: "open", Path: name, Err: windows.ERROR_FILE_NOT_FOUND}
	}
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	var access uint32
	switch flag & (os.O_RDONLY | os.O_WRONLY | os.O_RDWR) {
	case os.O_RDONLY:
		access = windows.GENERIC_READ
	case os.O_WRONLY:
		access = windows.GENERIC_WRITE
	case os.O_RDWR:
		access = windows.GENERIC_READ | windows.GENERIC_WRITE
	}
	readOnly := access == windows.GENERIC_READ && flag&os.O_CREATE == 0
	if flag&os.O_CREATE != 0 {
		access |= windows.GENERIC_WRITE
	}
	if flag&os.O_APPEND != 0 { // as syscall.Open: append data, not write it anywhere
		if flag&os.O_TRUNC == 0 {
			access &^= windows.GENERIC_WRITE
		}
		access |= windows.FILE_APPEND_DATA | windows.FILE_WRITE_ATTRIBUTES | windows.FILE_WRITE_EA |
			windows.STANDARD_RIGHTS_WRITE | windows.SYNCHRONIZE
	}
	attrs := uint32(windows.FILE_ATTRIBUTE_NORMAL)
	if perm&0o200 == 0 && flag&os.O_CREATE != 0 {
		attrs = windows.FILE_ATTRIBUTE_READONLY
	}
	if readOnly {
		attrs |= windows.FILE_FLAG_BACKUP_SEMANTICS // so a folder opens, to be handed to os.OpenFile
	}
	var mode uint32
	switch {
	case flag&(os.O_CREATE|os.O_EXCL) == os.O_CREATE|os.O_EXCL:
		mode = windows.CREATE_NEW
		attrs |= windows.FILE_FLAG_OPEN_REPARSE_POINT
	case flag&os.O_CREATE != 0:
		mode = windows.OPEN_ALWAYS
	default:
		mode = windows.OPEN_EXISTING
	}
	h, err := windows.CreateFile(p, access, shareAll, nil, mode, attrs, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	if readOnly {
		var info windows.ByHandleFileInformation
		if windows.GetFileInformationByHandle(h, &info) == nil && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
			windows.CloseHandle(h)
			return os.OpenFile(name, flag, perm)
		}
	}
	f := os.NewFile(uintptr(h), name)
	if flag&os.O_TRUNC != 0 && access&(windows.GENERIC_WRITE) != 0 {
		if err := f.Truncate(0); err != nil {
			f.Close()
			return nil, &os.PathError{Op: "open", Path: name, Err: err}
		}
	}
	return f, nil
}

// fileRenameInfo is FILE_RENAME_INFO with FileRenameInfoEx's flags.
type fileRenameInfo struct {
	Flags          uint32
	RootDirectory  windows.Handle
	FileNameLength uint32
	FileName       [1]uint16
}

// Rename renames a file, replacing what's at newpath, with POSIX semantics: it works while either
// file is open with delete access shared (as OpenFile opens them), which os.Rename's MoveFileEx
// refuses for the one replaced. Where the file system has no POSIX renames (FAT), it's os.Rename.
func Rename(oldpath, newpath string) error {
	err := posixRename(oldpath, newpath)
	if err == nil {
		return nil
	}
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) || errors.Is(err, windows.ERROR_NOT_SUPPORTED) ||
		errors.Is(err, windows.ERROR_INVALID_FUNCTION) {
		return os.Rename(oldpath, newpath)
	}
	return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: err}
}

func posixRename(oldpath, newpath string) error {
	target, err := filepath.Abs(newpath)
	if err != nil {
		return err
	}
	name, err := windows.UTF16FromString(target)
	if err != nil {
		return err
	}
	from, err := windows.UTF16PtrFromString(oldpath)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(from, windows.DELETE|windows.SYNCHRONIZE, shareAll, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	var info fileRenameInfo
	size := int(unsafe.Offsetof(info.FileName)) + len(name)*2
	buf := make([]uint64, (size+7)/8) // 8-byte aligned, as the struct needs
	ri := (*fileRenameInfo)(unsafe.Pointer(&buf[0]))
	ri.Flags = windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS
	ri.FileNameLength = uint32((len(name) - 1) * 2)
	copy(unsafe.Slice(&ri.FileName[0], len(name)), name)
	return windows.SetFileInformationByHandle(h, windows.FileRenameInfoEx, (*byte)(unsafe.Pointer(&buf[0])), uint32(size))
}

func crossDevice(err error) bool {
	return errors.Is(err, windows.ERROR_NOT_SAME_DEVICE)
}

func inUse(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}

// ownerOnly sets path's ACL to full control for this process's user, SYSTEM and Administrators,
// protected from its folder's (inherited by a folder's contents). An ACL already that is left alone,
// so a big folder isn't walked again at every start.
func ownerOnly(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	allowed, err := ownerSIDs()
	if err != nil {
		return err
	}
	if sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION); err == nil && onlyAllowed(sd, allowed, true) {
		return nil
	}
	inherit := uint32(windows.NO_INHERITANCE)
	if info.IsDir() {
		inherit = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}
	var entries []windows.EXPLICIT_ACCESS
	for i, sid := range allowed {
		kind := windows.TRUSTEE_IS_GROUP
		if i == 0 {
			kind = windows.TRUSTEE_IS_USER
		}
		entries = append(entries, windows.EXPLICIT_ACCESS{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.SET_ACCESS,
			Inheritance:       inherit,
			Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_TYPE(kind),
				TrusteeValue: windows.TrusteeValueFromSID(sid)},
		})
	}
	acl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}

// onlyAllowed tells whether sd's DACL grants access only to the allowed SIDs, and is protected
// from its folder's if protected (a file inside an owner-only folder isn't: it inherits that ACL).
func onlyAllowed(sd *windows.SECURITY_DESCRIPTOR, allowed []*windows.SID, protected bool) bool {
	control, _, err := sd.Control()
	if err != nil || protected && control&windows.SE_DACL_PROTECTED == 0 {
		return false
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return false // no DACL: everyone has access
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			return false
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue // a deny takes access away
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		known := false
		for _, a := range allowed {
			if sid.Equals(a) {
				known = true
			}
		}
		if !known {
			return false
		}
	}
	return true
}

func isOwnerOnly(path string) (bool, error) {
	allowed, err := ownerSIDs()
	if err != nil {
		return false, err
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false, err
	}
	return onlyAllowed(sd, allowed, false), nil
}

// ownerSIDs: this process's user, SYSTEM and Administrators.
func ownerSIDs() ([]*windows.SID, error) {
	me, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return nil, err
	}
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return nil, err
	}
	return []*windows.SID{me.User.Sid, system, admins}, nil
}
