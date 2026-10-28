package supervisor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"nethernet-multi-server/internal/config"
)

type WindowsFactory struct{}
type windowsProcess struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	log   *os.File
}

func (p *windowsProcess) PID() int                    { return p.cmd.Process.Pid }
func (p *windowsProcess) Write(b []byte) (int, error) { return p.stdin.Write(b) }
func (p *windowsProcess) Wait() error {
	err := p.cmd.Wait()
	_ = p.stdin.Close()
	_ = p.log.Close()
	return err
}

func (WindowsFactory) Start(ctx context.Context, cfg config.Process, recent io.Writer) (Process, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.LogDirectory, 0o750); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	name := time.Now().Format("20060102-150405") + "-bedrock.log"
	logFile, err := os.OpenFile(filepath.Join(cfg.LogDirectory, name), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create BDS log: %w", err)
	}
	cmd := exec.Command(cfg.Executable)
	cmd.Dir = cfg.WorkingDirectory
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = logFile.Close()
		return nil, err
	}
	w := io.MultiWriter(logFile, recent)
	cmd.Stdout = w
	cmd.Stderr = w
	if err = cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = logFile.Close()
		return nil, err
	}
	return &windowsProcess{cmd: cmd, stdin: stdin, log: logFile}, nil
}
