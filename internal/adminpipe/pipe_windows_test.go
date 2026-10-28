package adminpipe

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPipeSecurityDescriptorIsRestricted(t *testing.T) {
	s, err := CurrentUserSDDL()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s, ";;;WD") || strings.Contains(s, ";;;AU") {
		t.Fatalf("broad ACL: %s", s)
	}
	if !strings.Contains(s, ";;;SY") {
		t.Fatalf("SYSTEM missing: %s", s)
	}
}
func TestCurrentUserCanConnect(t *testing.T) {
	name := DefaultName + `.test`
	l, err := ListenCurrentUser(name)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	accepted := make(chan error, 1)
	go func() {
		c, e := l.Accept()
		if e == nil {
			c.Close()
		}
		accepted <- e
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c, err := DialCurrentUser(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if err = <-accepted; err != nil {
		t.Fatal(err)
	}
}
