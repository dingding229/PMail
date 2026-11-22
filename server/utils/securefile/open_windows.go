package securefile

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

func openPrivate(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.READ_CONTROL|windows.WRITE_DAC, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open private file", Path: path, Err: err}
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("private file %q must not be a reparse point or directory", path)
	}
	return os.NewFile(uintptr(h), path), nil
}

func restrictPrivate(f *os.File, path string) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("private file %q is not a regular file", path)
	}
	h := windows.Handle(f.Fd())
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	if !owner.Equals(user.User.Sid) {
		return fmt.Errorf("private file %q must be owned by the service", path)
	}
	want, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + owner.String() + ")")
	if err != nil {
		return err
	}
	have, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	if have.String() == want.String() {
		return nil
	}
	dacl, _, err := want.DACL()
	if err != nil {
		return err
	}
	if err = windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		return err
	}
	log.Warnf("tightened permissions on private file %q to a protected owner-only ACL", path)
	return nil
}

// A new temporary file contains no secret until this succeeds. The default
// handle from os.CreateTemp does not request WRITE_DAC.
func restrictNew(f *os.File) error {
	other, err := openPrivate(f.Name())
	if err != nil {
		return err
	}
	defer other.Close()
	return restrictPrivate(other, f.Name())
}
