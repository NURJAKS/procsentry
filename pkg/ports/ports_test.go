package ports

import (
	"testing"
)

func TestParseHexIP(t *testing.T) {
	tests := []struct {
		hexIP    string
		expected string
	}{
		{"0100007F", "127.0.0.1"},
		{"00000000", "0.0.0.0"},
		{"invalid", "0.0.0.0"},
	}

	for _, tc := range tests {
		result := parseHexIP(tc.hexIP)
		if result != tc.expected {
			t.Errorf("parseHexIP(%q) = %q; expected %q", tc.hexIP, result, tc.expected)
		}
	}
}

func TestCommonDevPortsCatalog(t *testing.T) {
	knownPorts := []int{3000, 5173, 8000, 5432, 6379, 48888}
	for _, p := range knownPorts {
		if _, ok := commonDevPorts[p]; !ok {
			t.Errorf("expected commonDevPorts to identify port %d", p)
		}
	}
}

func TestGetListeningPortsExecution(t *testing.T) {
	// Should execute without panics on Linux /proc/net/tcp
	ports := GetListeningPorts()
	if ports == nil {
		t.Log("No active listening ports or non-Linux environment")
	}
}
