package edl

import "testing"

func TestParseEventsAndComments(t *testing.T) {
	raw := "TITLE: DEMO\n" +
		"FCM: DROP FRAME\n" +
		"\n" +
		"001  REEL_A V     C        00:00:59;29 00:01:00;02 00:00:59;29 00:01:00;02\n" +
		"* FROM CLIP NAME: foo.mov\n" +
		"002  REEL_A A1    C        00:01:00:00 00:02:00:00 00:01:00:00 00:02:00:00\n"
	doc := Parse(raw)
	if len(doc.Events) != 2 {
		t.Fatalf("events = %d, want 2", len(doc.Events))
	}
	ev := doc.Events[0]
	if ev.EventNum != "001" || ev.Reel != "REEL_A" || ev.FCM != FCMDrop {
		t.Fatalf("event header parsed wrong: %+v", ev)
	}
	if len(ev.Tokens) != 4 {
		t.Fatalf("tokens = %d", len(ev.Tokens))
	}
	fields := []string{"SRC_IN", "SRC_OUT", "REC_IN", "REC_OUT"}
	for i, ref := range ev.Tokens {
		if ref.Field != fields[i] {
			t.Fatalf("token %d field = %q", i, ref.Field)
		}
	}
	if doc.Events[1].FCM != FCMDrop {
		t.Fatal("FCM section must stay active across events")
	}
	if doc.Lines[4].Text != "* FROM CLIP NAME: foo.mov" {
		t.Fatal("comment line not preserved")
	}
}

func TestNonDropFCM(t *testing.T) {
	doc := Parse("FCM: NON-DROP FRAME\n001 R V C 00:00:00:00 00:00:01:00 00:00:00:00 00:00:01:00\n")
	if doc.Events[0].FCM != FCMNonDrop {
		t.Fatalf("fcm = %v", doc.Events[0].FCM)
	}
}

func TestReplaceTokenKeepsAlignment(t *testing.T) {
	line := "001  REEL_A V     C        00:01:00:00 00:02:00:00 00:01:00:00 00:02:00:00"
	doc := Parse(line + "\n")
	ev := doc.Events[0]
	out := line
	for _, ref := range ev.Tokens {
		out = ReplaceToken(out, ref, "00:01:00;02")
	}
	if len(out) != len(line) {
		t.Fatalf("length changed: %d vs %d", len(out), len(line))
	}
	expected := "001  REEL_A V     C        00:01:00;02 00:01:00;02 00:01:00;02 00:01:00;02"
	if out != expected {
		t.Fatalf("replacement wrong:\n%s", out)
	}
}

func TestGluedDigitsAreNotTokens(t *testing.T) {
	// A 12-digit run must not be misread as a timecode.
	doc := Parse("* NOTE 123456789012 trailing\n")
	if len(doc.Events) != 0 {
		t.Fatal("digit run misdetected as event")
	}
}
