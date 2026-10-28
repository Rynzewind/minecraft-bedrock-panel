package proxy
import("testing";"github.com/sandertv/gophertunnel/minecraft/protocol")
func TestSupportsConsole2660(t *testing.T){if protocol.CurrentProtocol!=2225{t.Fatalf("26.60 requires protocol 2225; got %d",protocol.CurrentProtocol)}}
