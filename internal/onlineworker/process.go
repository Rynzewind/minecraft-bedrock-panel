package onlineworker

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type Process struct {
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
	err    error
}

// Start supplies no token on command lines, environment variables or files.
func Start(ctx context.Context, path string, cfg Bootstrap, cb Callbacks) (*Process, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("Online worker path must be absolute")
	}
	c, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(c, path, "--run")
	prepare(cmd)
	cmd.WaitDelay = time.Second
	in, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, ErrProtocol
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		cancel()
		return nil, ErrProtocol
	}
	if err = cmd.Start(); err != nil {
		in.Close()
		out.Close()
		cancel()
		return nil, ErrProtocol
	}
	release, err := contain(cmd.Process)
	if err != nil {
		cancel()
		in.Close()
		out.Close()
		_ = cmd.Wait()
		return nil, ErrProtocol
	}
	p := &Process{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(p.done)
		defer cancel()
		brokerDone := make(chan error, 1)
		go func() { brokerDone <- Serve(c, out, in, cfg, cb) }()
		waitDone := make(chan error, 1)
		go func() { waitDone <- cmd.Wait() }()
		waited := false
		// Any protocol failure closes the token channel and kills this worker only.
		var e error
		select {
		case <-c.Done():
			e = c.Err()
		case e = <-brokerDone:
		case <-waitDone:
			waited = true
			e = ErrProtocol
		}
		cancel()
		in.Close()
		out.Close()
		release() // Terminate the complete job before waiting for inherited handles.
		if !waited {
			<-waitDone
		}
		p.mu.Lock()
		p.err = e
		p.mu.Unlock()
	}()
	return p, nil
}
func (p *Process) Close() error {
	p.cancel()
	select {
	case <-p.done:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("Online worker did not stop")
	}
}
func (p *Process) Done() <-chan struct{} { return p.done }
func (p *Process) Err() error            { p.mu.Lock(); defer p.mu.Unlock(); return p.err }
