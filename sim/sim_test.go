package sim

import (
	"testing"

	"opusloss/audio"
)

func makeFrames(t *testing.T) [][]float64 {
	t.Helper()
	src := audio.GenerateSpeechLike(SampleRate, 12*FrameSize, 7)
	return audio.Frames(src, FrameSize)
}

func TestMaskEvents(t *testing.T) {
	m := merge(IsolatedMask(2), BurstMask(5, 3))
	events := m.Events(12)
	want := [][2]int{{2, 2}, {5, 7}}
	if len(events) != len(want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("event %d = %v, want %v", i, events[i], want[i])
		}
	}
}

func TestSilenceChainAndSamples(t *testing.T) {
	frames := makeFrames(t)
	ch := ApplyChannel(BuildPackets(frames), IsolatedMask(3, 8))
	rcv := NewReceiver(ModeSilence)
	out := rcv.Run(ch)
	if rcv.RecoveredSamples() != 2*FrameSize {
		t.Fatalf("recovered samples = %d", rcv.RecoveredSamples())
	}
	for _, idx := range []int{3, 8} {
		for _, v := range out[idx] {
			if v != 0 {
				t.Fatalf("silence frame %d contains non-zero sample", idx)
			}
		}
		if rcv.Chain()[idx].State != StateSilence {
			t.Fatalf("frame %d state = %s", idx, rcv.Chain()[idx].State)
		}
	}
	if err := AuditNoBackfill(out, frames, rcv.Chain()); err != nil {
		t.Fatal(err)
	}
}

func TestPLCColdAndWarmChain(t *testing.T) {
	frames := makeFrames(t)
	ch := ApplyChannel(BuildPackets(frames), merge(IsolatedMask(0), BurstMask(6, 2)))
	rcv := NewReceiver(ModePLC)
	out := rcv.Run(ch)
	if rcv.Chain()[0].State != StatePLCCold {
		t.Fatalf("frame 0 = %s, want PLC_COLD", rcv.Chain()[0].State)
	}
	for _, idx := range []int{6, 7} {
		if rcv.Chain()[idx].State != StatePLC {
			t.Fatalf("frame %d = %s, want PLC", idx, rcv.Chain()[idx].State)
		}
	}
	if rcv.Chain()[8].State != StateFadeIn {
		t.Fatalf("frame 8 = %s, want FADE_IN", rcv.Chain()[8].State)
	}
	if err := AuditNoBackfill(out, frames, rcv.Chain()); err != nil {
		t.Fatal(err)
	}
}

func TestFECIsolatedRepairsAndBurstFallback(t *testing.T) {
	frames := makeFrames(t)
	// Burst reaches the stream end, so no following packet can deliver FEC.
	mask := merge(IsolatedMask(2), BurstMask(8, 4))
	ch := ApplyChannel(BuildPackets(frames), mask)
	rcv := NewReceiver(ModeFEC)
	out := rcv.Run(ch)
	chain := rcv.Chain()
	if chain[2].State != StateFEC {
		t.Fatalf("isolated frame = %s, want FEC", chain[2].State)
	}
	// The next packet of every burst frame is also lost, so in-band FEC can
	// never be delivered; the whole run falls back to PLC transparently.
	wantBurst := []string{StateFECPlc, StateFECPlc, StateFECPlc, StateFECPlc}
	for j, want := range wantBurst {
		if got := chain[8+j].State; got != want {
			t.Fatalf("burst frame %d = %s, want %s", 8+j, got, want)
		}
	}
	if rcv.RecoveredSamples() != 5*FrameSize {
		t.Fatalf("recovered samples = %d, want %d", rcv.RecoveredSamples(), 5*FrameSize)
	}
	if err := AuditNoBackfill(out, frames, chain); err != nil {
		t.Fatal(err)
	}
}

func TestAuditDetectsBackfill(t *testing.T) {
	frames := makeFrames(t)
	ch := ApplyChannel(BuildPackets(frames), IsolatedMask(4))
	rcv := NewReceiver(ModeSilence)
	out := rcv.Run(ch)
	out[4] = append([]float64(nil), frames[4]...) // malicious verbatim copy
	if err := AuditNoBackfill(out, frames, rcv.Chain()); err == nil {
		t.Fatal("expected backfill audit to fail")
	}
}

func TestMetricsFinite(t *testing.T) {
	frames := makeFrames(t)
	ch := ApplyChannel(BuildPackets(frames), IsolatedMask(4))
	rcv := NewReceiver(ModePLC)
	out := rcv.Run(ch)
	m := ErrorMetrics(out, frames, rcv.Chain())
	if m.LostFrames != 1 || m.RecoveredSamples != FrameSize {
		t.Fatalf("metrics = %+v", m)
	}
	if m.MAE <= 0 || m.RMSE <= 0 || m.PeakAbs <= 0 {
		t.Fatalf("non-positive error metrics: %+v", m)
	}
}

func merge(a, b Mask) Mask {
	m := Mask{}
	for k, v := range a {
		m[k] = v
	}
	for k, v := range b {
		m[k] = v
	}
	return m
}
