package proxy

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestGatewayProtocolLogsNeverIncludeSDPOrRawErrors(t *testing.T) {
	var output bytes.Buffer
	log := safeGatewayProtocolLog(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	log.With("offer", "SECRET_SDP").WithGroup("nested").Error("error negotiating connection", "signal", "SECRET_JWT", "error", "SECRET_ERROR")
	log.Error("untrusted message SECRET_RAW", "body", "SECRET_BODY")
	if strings.Contains(output.String(), "SECRET") || !strings.Contains(output.String(), "negotiation_failed") {
		t.Fatal("unsafe or absent diagnostic:", output.String())
	}
}
