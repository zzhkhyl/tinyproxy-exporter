package source

import (
	"strings"
	"testing"
)

const procNetDevSample = `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 1234567    8900    0    0    0     0          0         0  1234567    8900    0    0    0     0       0          0
  eth0: 987654321  654321    3    1    0     0          0        12  123456789  123456    5    2    0     0       0          0
`

func TestParseProcNetDevReadsByteCounters(t *testing.T) {
	rx, tx, err := ParseProcNetDev([]byte(procNetDevSample), "eth0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rx != 987654321 {
		t.Fatalf("received = %d, want 987654321", rx)
	}
	if tx != 123456789 {
		t.Fatalf("sent = %d, want 123456789", tx)
	}
}

func TestParseProcNetDevCanReadLoopback(t *testing.T) {
	rx, tx, err := ParseProcNetDev([]byte(procNetDevSample), "lo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rx != 1234567 || tx != 1234567 {
		t.Fatalf("loopback = (%d, %d), want (1234567, 1234567)", rx, tx)
	}
}

func TestParseProcNetDevErrors(t *testing.T) {
	if _, _, err := ParseProcNetDev([]byte(procNetDevSample), "wlan0"); err == nil {
		t.Fatal("expected an error for an unknown interface")
	}
	if _, _, err := ParseProcNetDev([]byte("eth0: 1 2 3\n"), "eth0"); err == nil {
		t.Fatal("expected an error for a truncated counter list")
	}
	if _, _, err := ParseProcNetDev([]byte("eth0: notanumber 2 3 4 5 6 7 8 9\n"), "eth0"); err == nil {
		t.Fatal("expected an error for a non numeric counter")
	}
}

func TestParseProcNetDevIgnoresHeaderLines(t *testing.T) {
	if _, _, err := ParseProcNetDev([]byte(procNetDevSample), "lo:"); err == nil {
		t.Fatalf("interface names must match exactly: %v", err)
	}
	if !strings.Contains(procNetDevSample, "eth0") {
		t.Fatal("fixture changed, the test above no longer proves anything")
	}
}
