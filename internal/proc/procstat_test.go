package proc

import "testing"

func TestShouldParseProcStatWithParensInComm(t *testing.T) {
	// given a comm with spaces and parentheses
	stat := "4242 (my (odd) proc) S 4100 4242 4100 34819 4242 4194304 ...\n"

	// when
	comm, ppid, ttyNr, err := parseStat(stat)

	// then
	if err != nil || comm != "my (odd) proc" || ppid != 4100 || ttyNr != 34819 {
		t.Fatalf("comm=%q ppid=%d tty=%d err=%v", comm, ppid, ttyNr, err)
	}
}

func TestShouldRejectTruncatedProcStat(t *testing.T) {
	if _, _, _, err := parseStat("4242 (x"); err == nil {
		t.Fatal("expected error")
	}
}

func TestShouldNameLinuxTTYs(t *testing.T) {
	for nr, want := range map[int]string{0: "", 34819: "pts/3", 34816 + 1<<20 + 5: "pts/261", 1025: "tty1"} {
		if got := ttyFromNr(nr); got != want {
			t.Errorf("%d → %q, want %q", nr, got, want)
		}
	}
}

func TestShouldSplitNulSeparatedEnviron(t *testing.T) {
	env := parseEnviron([]byte("HOME=/home/m\x00HERDR_PANE_ID=w1:p1\x00BROKEN\x00"))
	if len(env) != 2 || env["HERDR_PANE_ID"] != "w1:p1" || env["HOME"] != "/home/m" {
		t.Fatalf("%v", env)
	}
}

func TestShouldStripDeletedSuffixFromExe(t *testing.T) {
	if got := exeLink("/usr/bin/kitty (deleted)"); got != "/usr/bin/kitty" {
		t.Fatalf("got %q", got)
	}
	if got := exeLink("/usr/bin/kitty"); got != "/usr/bin/kitty" {
		t.Fatalf("got %q", got)
	}
}
