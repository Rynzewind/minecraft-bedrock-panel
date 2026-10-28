package accountstore

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

type DPAPIProtector struct{ Entropy []byte }
type dataBlob struct {
	size uint32
	data *byte
}

var crypt32 = windows.NewLazySystemDLL("crypt32.dll")
var procProtect = crypt32.NewProc("CryptProtectData")
var procUnprotect = crypt32.NewProc("CryptUnprotectData")
var kernel32 = windows.NewLazySystemDLL("kernel32.dll")
var procLocalFree = kernel32.NewProc("LocalFree")

func replaceFile(source, destination string) error {
	src, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	dst, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(src, dst, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

func blob(b []byte) dataBlob {
	if len(b) == 0 {
		return dataBlob{}
	}
	return dataBlob{uint32(len(b)), &b[0]}
}
func blobBytes(b dataBlob) []byte {
	if b.size == 0 {
		return nil
	}
	return append([]byte(nil), unsafe.Slice(b.data, b.size)...)
}
func (p DPAPIProtector) Protect(plain []byte) ([]byte, error) {
	in, entropy := blob(plain), blob(p.Entropy)
	var out dataBlob
	r, _, e := procProtect.Call(uintptr(unsafe.Pointer(&in)), 0, uintptr(unsafe.Pointer(&entropy)), 0, 0, 1, uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return nil, fmt.Errorf("CryptProtectData: %w", e)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.data)))
	return blobBytes(out), nil
}
func (p DPAPIProtector) Unprotect(cipher []byte) ([]byte, error) {
	in, entropy := blob(cipher), blob(p.Entropy)
	var out dataBlob
	r, _, e := procUnprotect.Call(uintptr(unsafe.Pointer(&in)), 0, uintptr(unsafe.Pointer(&entropy)), 0, 0, 1, uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return nil, fmt.Errorf("CryptUnprotectData: %w", e)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.data)))
	return blobBytes(out), nil
}
