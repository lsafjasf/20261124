package eac3sm

import "testing"

func intPtr(v int) *int { return &v }

func feed(m *Machine, specs ...Spec) []Event {
	var out []Event
	for _, s := range specs {
		out = append(out, m.Feed(SynthFrame(s), Hint{})...)
	}
	return out
}

func eventTypes(events []Event) []EventType {
	types := make([]EventType, len(events))
	for i, e := range events {
		types[i] = e.Type
	}
	return types
}

func audioWithMode(events []Event, mode string) bool {
	for _, e := range events {
		if e.Type == EventAudio && e.Config.ChannelMode == mode {
			return true
		}
	}
	return false
}

func transitionsWithReason(m *Machine, r Reason) []Transition {
	var out []Transition
	for _, t := range m.Transitions() {
		if t.Reason == r {
			out = append(out, t)
		}
	}
	return out
}

func startStreaming(t *testing.T, m *Machine, s Spec) {
	t.Helper()
	feed(m, s, s)
	if m.State() != StateStreaming {
		t.Fatalf("expected Streaming, got %s", m.State())
	}
}

// The reported bug: switching programs must not decode the new program with
// the old channel mode. The machine must wait for a complete initial state
// and rebuild the decoder from the new header.
func TestProgramSwitchWaitsForCompleteInit(t *testing.T) {
	m := NewMachine()
	stereo := Spec{FSCod: 0, Acmod: 2}              // 2/0
	surround := Spec{FSCod: 0, Acmod: 7, LFE: true} // 3/2+LFE
	startStreaming(t, m, stereo)

	// First frame of the new program arrives with a boundary hint.
	events := m.Feed(SynthFrame(surround), Hint{ProgramBoundary: true})
	if m.State() != StateAwaitInit {
		t.Fatalf("expected AwaitInit after boundary, got %s", m.State())
	}
	if audioWithMode(events, "2/0") {
		t.Fatal("stale channel mode 2/0 used after program boundary")
	}
	if audioWithMode(events, "3/2") {
		t.Fatal("audio emitted before complete initial state")
	}
	if got := transitionsWithReason(m, ReasonProgramBoundary); len(got) != 1 {
		t.Fatalf("expected 1 program-boundary transition, got %d", len(got))
	}

	// Second consistent frame completes the initial state.
	events = feed(m, surround)
	if m.State() != StateStreaming {
		t.Fatalf("expected Streaming after init complete, got %s", m.State())
	}
	if !audioWithMode(events, "3/2") {
		t.Fatal("expected audio decoded with new channel mode 3/2")
	}
	if cfg := m.Decoder().Config; cfg.ChannelMode != "3/2" || !cfg.LFE {
		t.Fatalf("decoder rebuilt with wrong config: %s", cfg)
	}
	if m.Decoder().Rebuilds != 2 {
		t.Fatalf("expected 2 rebuilds, got %d", m.Decoder().Rebuilds)
	}
	if got := transitionsWithReason(m, ReasonInitComplete); len(got) != 2 {
		t.Fatalf("expected 2 init-complete transitions, got %d", len(got))
	}
}

// A corrupt frame during init must restart the consecutive-valid count.
func TestInitInterruptedByCRCError(t *testing.T) {
	m := NewMachine()
	surround := Spec{FSCod: 0, Acmod: 7}
	m.Feed(SynthFrame(surround), Hint{ProgramBoundary: true})
	feed(m, Spec{FSCod: 0, Acmod: 7, Corrupt: true})
	if m.State() != StateAwaitInit {
		t.Fatalf("corrupt frame must not leave AwaitInit, got %s", m.State())
	}
	feed(m, surround)
	if m.State() != StateAwaitInit {
		t.Fatalf("one valid frame after corruption is not a complete init, got %s", m.State())
	}
	feed(m, surround)
	if m.State() != StateStreaming {
		t.Fatalf("expected Streaming after two consecutive valid frames, got %s", m.State())
	}
}

func TestCRCErrorRecovery(t *testing.T) {
	m := NewMachine()
	stereo := Spec{FSCod: 0, Acmod: 2}
	startStreaming(t, m, stereo)

	events := feed(m, Spec{FSCod: 0, Acmod: 2, Corrupt: true})
	if m.State() != StateRecoverCRC {
		t.Fatalf("expected RecoverCRC, got %s", m.State())
	}
	if got := eventTypes(events); got[0] != EventConceal {
		t.Fatalf("expected concealment, got %v", got)
	}

	events = feed(m, stereo)
	if m.State() != StateStreaming {
		t.Fatalf("expected recovery to Streaming, got %s", m.State())
	}
	if got := eventTypes(events); got[0] != EventRecovered {
		t.Fatalf("expected Recovered event, got %v", got)
	}
	if got := transitionsWithReason(m, ReasonCRCRecovered); len(got) != 1 {
		t.Fatalf("expected crc-recovered transition, got %d", len(got))
	}
}

func TestCRCBurstLosesSync(t *testing.T) {
	m := NewMachine()
	stereo := Spec{FSCod: 0, Acmod: 2}
	startStreaming(t, m, stereo)

	bad := Spec{FSCod: 0, Acmod: 2, Corrupt: true}
	for i := 0; i < maxCRCErrors+1; i++ {
		feed(m, bad)
	}
	if m.State() != StateAcquire {
		t.Fatalf("expected Acquire after %d CRC errors, got %s", maxCRCErrors+1, m.State())
	}
	if !m.Decoder().Muted {
		t.Fatal("decoder must be muted after resync failure")
	}
	if got := transitionsWithReason(m, ReasonResyncFailed); len(got) != 1 {
		t.Fatalf("expected resync-failed transition, got %d", len(got))
	}
}

func TestSampleRateChangeRebuildsDecoder(t *testing.T) {
	m := NewMachine()
	startStreaming(t, m, Spec{FSCod: 0, Acmod: 2}) // 48 kHz

	rate44 := Spec{FSCod: 1, Acmod: 2} // 44.1 kHz, same layout
	feed(m, rate44)
	if m.State() != StateAwaitInit {
		t.Fatalf("sample-rate change must enter AwaitInit, got %s", m.State())
	}
	if got := transitionsWithReason(m, ReasonSampleRateChange); len(got) != 1 {
		t.Fatalf("expected sample-rate-change transition, got %d", len(got))
	}
	feed(m, rate44)
	if cfg := m.Decoder().Config; cfg.SampleRate != 44100 {
		t.Fatalf("decoder not rebuilt for 44.1kHz: %s", cfg)
	}
	if m.Decoder().Rebuilds != 2 {
		t.Fatalf("expected full rebuild on sample-rate change, got %d rebuilds", m.Decoder().Rebuilds)
	}
}

func TestDRCProfileChangeReconfiguresOnly(t *testing.T) {
	m := NewMachine()
	startStreaming(t, m, Spec{FSCod: 0, Acmod: 2, Compr: intPtr(10)})
	rebuilds := m.Decoder().Rebuilds

	events := feed(m, Spec{FSCod: 0, Acmod: 2, Compr: intPtr(20)})
	if m.State() != StateStreaming {
		t.Fatalf("DRC change must return to Streaming, got %s", m.State())
	}
	if got := eventTypes(events); got[0] != EventDRCReconfigured {
		t.Fatalf("expected DRCReconfigured, got %v", got)
	}
	if m.Decoder().Rebuilds != rebuilds {
		t.Fatal("DRC change must not rebuild the decoder")
	}
	if m.Decoder().DRCUpdates != 1 {
		t.Fatalf("expected 1 DRC update, got %d", m.Decoder().DRCUpdates)
	}
	if m.Decoder().Config.DRCProfile != "compr/20" {
		t.Fatalf("wrong DRC profile: %s", m.Decoder().Config.DRCProfile)
	}
	if got := transitionsWithReason(m, ReasonDRCProfileChange); len(got) != 1 {
		t.Fatalf("expected drc-profile-change transition, got %d", len(got))
	}
}

func TestHalfRateSampleRateParsing(t *testing.T) {
	m := NewMachine()
	startStreaming(t, m, Spec{FSCod: 3, FSCod2: 0, Acmod: 1}) // 24 kHz mono
	if got := m.Decoder().Config.SampleRate; got != 24000 {
		t.Fatalf("expected 24000 Hz, got %d", got)
	}
}

func TestTransitionLogIsSequential(t *testing.T) {
	m := NewMachine()
	startStreaming(t, m, Spec{FSCod: 0, Acmod: 2})
	for i, tr := range m.Transitions() {
		if tr.Seq != i+1 {
			t.Fatalf("transition seq broken at %d: %+v", i, tr)
		}
	}
}
