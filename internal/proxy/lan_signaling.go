package proxy

import (
	"context"
	"github.com/df-mc/go-nethernet"
	"strings"
)

// nativeLANSignaling matches identityless native console LAN answers for Xbox compatibility.
// Minecraft login and backend Microsoft authentication are handled separately.
type nativeLANSignaling struct{ nethernet.Signaling }

func (p nativeLANSignaling) Signal(ctx context.Context, signal *nethernet.Signal) error {
	if signal.Type != nethernet.SignalTypeAnswer {
		return p.Signaling.Signal(ctx, signal)
	}
	copySignal := *signal
	lines := strings.Split(signal.Data, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "a=identity:") {
			kept = append(kept, line)
		}
	}
	copySignal.Data = strings.Join(kept, "\n")
	return p.Signaling.Signal(ctx, &copySignal)
}
