//go:build windows

package winproc

import "testing"

func TestParseNetstatPortOwnersFiltersListeningTCPPorts(t *testing.T) {
	raw := "" +
		"  TCP    127.0.0.1:13000        0.0.0.0:0              LISTENING       54792\r\n" +
		"  TCP    127.0.0.1:7890         0.0.0.0:0              LISTENING       11223\r\n" +
		"  TCP    127.0.0.1:13000        127.0.0.1:58123        ESTABLISHED     54792\r\n" +
		"  UDP    0.0.0.0:5353          *:*                                    2048\r\n"

	owners := parseNetstatPortOwners(raw, []int{13000, 7890, 9999})

	if got := owners[13000]; got != 54792 {
		t.Fatalf("owner for 13000 = %d, want %d", got, 54792)
	}
	if got := owners[7890]; got != 11223 {
		t.Fatalf("owner for 7890 = %d, want %d", got, 11223)
	}
	if _, ok := owners[9999]; ok {
		t.Fatal("unexpected owner recorded for unlisted port 9999")
	}
}

func TestSameExecutablePathNormalizesWindowsPrefixes(t *testing.T) {
	actual := `\\?\C:\ProgramData\ClashForClaw\bin\mihomo.exe`
	expected := `c:\ProgramData\ClashForClaw\bin\mihomo.exe`

	if !SameExecutablePath(actual, expected) {
		t.Fatalf("SameExecutablePath(%q, %q) = false, want true", actual, expected)
	}
}
