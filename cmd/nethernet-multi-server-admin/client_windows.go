package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"nethernet-multi-server/internal/adminapi"
	"nethernet-multi-server/internal/adminpipe"
	"sync/atomic"
	"time"
)

type pipeClient struct{ next atomic.Uint64 }

func (c *pipeClient) Call(ctx context.Context, method string, params, target any) error {
	conn, err := adminpipe.DialCurrentUser(ctx, adminpipe.DefaultName)
	if err != nil {
		return fmt.Errorf("connect to service: %w", err)
	}
	defer conn.Close()
	stopCancellation := bindCallContext(ctx, conn)
	defer stopCancellation()
	b, err := json.Marshal(params)
	if err != nil {
		return err
	}
	id := fmt.Sprint(c.next.Add(1))
	if err = adminapi.WriteRequest(conn, adminapi.Request{ID: id, Method: method, Params: b}); err != nil {
		return err
	}
	resp, err := adminapi.ReadResponse(conn)
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}
	if target != nil {
		raw, err := json.Marshal(resp.Result)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(raw, target); err != nil {
			return err
		}
	}
	return nil
}

func bindCallContext(ctx context.Context, conn net.Conn) func() {
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	return func() { stop() }
}
func timeoutContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}
