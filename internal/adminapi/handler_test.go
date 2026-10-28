package adminapi

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"testing"
)

type controller struct{}

func (controller) Handle(_ context.Context, m string, _ json.RawMessage) (any, error) {
	if m == "fail" {
		return nil, errors.New("secret refresh-token")
	}
	if m == "detail" {
		return nil, errors.New("install account store: access denied")
	}
	return map[string]bool{"ok": true}, nil
}

func TestOrdinaryLocalErrorDetailsAreReturned(t *testing.T) {
	var in, out bytes.Buffer
	_ = WriteRequest(&in, Request{ID: "1", Method: "detail"})
	_ = ServeConnection(context.Background(), struct {
		io.Reader
		io.Writer
	}{&in, &out}, controller{})
	resp, err := ReadResponse(&out)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil || resp.Error.Message != "install account store: access denied" {
		t.Fatalf("%+v", resp.Error)
	}
}
func TestServeConnectionRejectsFrameOverOneMiB(t *testing.T) {
	var in bytes.Buffer
	_ = binary.Write(&in, binary.LittleEndian, uint32(MaxFrameSize+1))
	err := ServeConnection(context.Background(), struct {
		io.Reader
		io.Writer
	}{&in, io.Discard}, controller{})
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("got %v", err)
	}
}
func TestErrorsContainingTokenMaterialAreRedacted(t *testing.T) {
	var in, out bytes.Buffer
	_ = WriteRequest(&in, Request{ID: "1", Method: "fail"})
	ctx, cancel := context.WithCancel(context.Background())
	rw := struct {
		io.Reader
		io.Writer
	}{&in, &out}
	_ = ServeConnection(ctx, rw, controller{})
	cancel()
	resp, err := ReadResponse(&out)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil || bytes.Contains(out.Bytes(), []byte("refresh-token")) {
		t.Fatalf("response not redacted: %s", out.String())
	}
}
