package online

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
)

func privateDirectory(path string) error {
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		if st, e := os.Lstat(current); e == nil && st.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return fmt.Errorf("linked Online directories are not supported")
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	if e := os.MkdirAll(path, 0700); e != nil {
		return e
	}
	u, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		return e
	}
	sd, e := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;" + u.User.Sid.String() + ")")
	if e != nil {
		return e
	}
	acl, _, e := sd.DACL()
	if e != nil {
		return e
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}
