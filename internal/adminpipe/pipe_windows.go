package adminpipe

import (
	"context"
	"fmt"
	winio "github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"net"
	"time"
)

const DefaultName = `\\.\pipe\NetherNetMultiServer.Admin.v1`

func CurrentUserSDDL() (string, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("D:P(A;;GA;;;SY)(A;;GA;;;%s)", u.User.Sid.String()), nil
}
func ListenCurrentUser(name string) (net.Listener, error) {
	if name == "" {
		name = DefaultName
	}
	sddl, err := CurrentUserSDDL()
	if err != nil {
		return nil, err
	}
	return winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: sddl, MessageMode: false, InputBufferSize: 65536, OutputBufferSize: 65536})
}
func DialCurrentUser(ctx context.Context, name string) (net.Conn, error) {
	if name == "" {
		name = DefaultName
	}
	return winio.DialPipeContext(ctx, name)
}
func DialTimeout(name string, d time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return DialCurrentUser(ctx, name)
}
