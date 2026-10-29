package proxy

import (
	"context"
	"log/slog"
)

// Protocol libraries may attach full SDP offers to an error. SDP contains
// client login assertions: neither raw messages nor attributes cross this gate.
type gatewayProtocolHandler struct{ parent *slog.Logger }

func safeGatewayProtocolLog(parent *slog.Logger) *slog.Logger {
	if parent == nil {
		parent = slog.Default()
	}
	return slog.New(gatewayProtocolHandler{parent: parent})
}
func (h gatewayProtocolHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= slog.LevelWarn && h.parent.Enabled(ctx, level)
}
func (h gatewayProtocolHandler) Handle(ctx context.Context, r slog.Record) error {
	stage := "transport_error"
	switch r.Message {
	case "error negotiating connection":
		stage = "negotiation_failed"
	case "error parsing offer":
		stage = "invalid_offer"
	case "client authentication failed":
		stage = "authentication_failed"
	}
	h.parent.Log(ctx, r.Level, "Online gateway transport diagnostic", "stage", stage)
	return nil
}
func (h gatewayProtocolHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h gatewayProtocolHandler) WithGroup(string) slog.Handler      { return h }
