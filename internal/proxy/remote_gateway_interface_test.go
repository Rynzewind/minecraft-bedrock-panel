package proxy

import (
	"net"
	"testing"
)

func TestGatewayMuxBindsOnlySelectedInterface(t *testing.T) {
	a, e := gatewayUDPAddress("0.0.0.0:19141", net.ParseIP("192.0.2.10"))
	if e != nil || !a.IP.Equal(net.ParseIP("192.0.2.10")) || a.Port != 19141 {
		t.Fatal(a, e)
	}
	for _, tc := range []struct{ bind, ip string }{{"0.0.0.0:19141", ""}, {"127.0.0.1:19141", "192.0.2.10"}, {"0.0.0.0:19141", "::1"}, {"0.0.0.0:19141", "0.0.0.0"}} {
		if _, e := gatewayUDPAddress(tc.bind, net.ParseIP(tc.ip)); e == nil {
			t.Fatal("ambiguous or mismatched interface accepted", tc)
		}
	}
}
