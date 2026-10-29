package onlineworker

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"time"
)

type probe struct {
	Protocol      int    `json:"protocol"`
	Upstream      string `json:"upstream_commit"`
	Fork          string `json:"fork_commit"`
	Direct        bool   `json:"direct_transfer"`
	Authenticated bool   `json:"authenticated_admission"`
	Brokered      bool   `json:"brokered_tokens"`
}
type boundedOutput struct {
	bytes.Buffer
	overflow bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len()+n > 65536 {
		b.overflow = true
		return n, nil
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

// Compatible probes only a local executable. No credentials are supplied.
func Compatible(ctx context.Context, path string) bool {
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(c, path, "--probe")
	prepare(cmd)
	cmd.WaitDelay = time.Second
	var out boundedOutput
	cmd.Stdout = &out
	if cmd.Start() != nil {
		return false
	}
	release, err := contain(cmd.Process)
	if err != nil {
		cancel()
		_ = cmd.Wait()
		return false
	}
	err = cmd.Wait()
	release()
	if err != nil || out.overflow {
		return false
	}
	var p probe
	if json.Unmarshal(out.Bytes(), &p) != nil {
		return false
	}
	return p.Protocol == 2225 && p.Upstream == "bb06c2ddc46fb61b2bbbb886f08d6edf11383a3e" && p.Fork == "f8448d729edd8abed1b04f37566b54da27b50249" && p.Direct && p.Authenticated && p.Brokered
}
