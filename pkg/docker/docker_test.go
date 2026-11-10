package docker

import (
	"encoding/binary"
	"testing"
)

func TestStripANSI(t *testing.T) {
	input := "\x1b[32m[Nest] 1  - \x1b[39m11/10/2026 LOG\x1b[0m"
	expected := "[Nest] 1  - 11/10/2026 LOG"
	actual := stripANSI(input)
	if actual != expected {
		t.Errorf("stripANSI failed: expected %q, got %q", expected, actual)
	}
}

func TestDemuxDockerSocket(t *testing.T) {
	// Construct simulated multiplexed frames
	// Frame 1: stdout, "Hello World\n" (12 bytes)
	f1Text := "Hello World\n"
	f1 := make([]byte, 8+len(f1Text))
	f1[0] = 1 // stdout
	binary.BigEndian.PutUint32(f1[4:8], uint32(len(f1Text)))
	copy(f1[8:], []byte(f1Text))

	// Frame 2: stderr, "Error occurred\n" (15 bytes)
	f2Text := "Error occurred\n"
	f2 := make([]byte, 8+len(f2Text))
	f2[0] = 2 // stderr
	binary.BigEndian.PutUint32(f2[4:8], uint32(len(f2Text)))
	copy(f2[8:], []byte(f2Text))

	combined := append(f1, f2...)
	demuxed := demuxDockerSocket(combined)
	expected := "Hello World\nError occurred\n"
	if demuxed != expected {
		t.Errorf("demuxDockerSocket failed: expected %q, got %q", expected, demuxed)
	}
}
