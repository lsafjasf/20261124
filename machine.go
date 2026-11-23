package eac3sm

import "fmt"

// State is one node of the program-switch state machine.
type State int

const (
	// StateAcquire: no sync yet, hunting for the first valid header.
	StateAcquire State = iota
	// StateAwaitInit: a new program was detected; the machine collects a
	// complete, consistent initial config before it may rebuild the
	// decoder. The old channel mode is never reused for the new program.
	StateAwaitInit
	// StateStreaming: normal decoding with a committed config.
	StateStreaming
	// StateRecoverCRC: CRC failures; frames are concealed from the
	// committed config while waiting for a valid header.
	StateRecoverCRC
	// StateDRCUpdate: transient state for applying a DRC profile change
	// without a decoder rebuild.
	StateDRCUpdate
)

func (s State) String() string {
	return [...]string{"Acquire", "AwaitInit", "Streaming", "RecoverCRC", "DRCUpdate"}[s]
}

// Reason explains why a transition happened; it is what makes each recovery
// path (CRC, sample rate, DRC) distinguishable in the transition log.
type Reason string

const (
	ReasonSyncAcquired      Reason = "sync-acquired"
	ReasonProgramBoundary   Reason = "program-boundary"
	ReasonSampleRateChange  Reason = "sample-rate-change"
	ReasonChannelModeChange Reason = "channel-mode-change"
	ReasonInitComplete      Reason = "init-complete"
	ReasonCRCError          Reason = "crc-error"
	ReasonCRCRecovered      Reason = "crc-recovered"
	ReasonResyncFailed      Reason = "resync-failed"
	ReasonDRCProfileChange  Reason = "drc-profile-change"
	ReasonDRCApplied        Reason = "drc-applied"
)

// Transition is one recorded state migration.
type Transition struct {
	Seq    int
	Frame  int
	From   State
	To     State
	Reason Reason
	Config ProgramConfig
}

func (t Transition) String() string {
	return fmt.Sprintf("#%02d frame=%d %-10s -> %-10s %-20s %s",
		t.Seq, t.Frame, t.From, t.To, t.Reason, t.Config)
}

// EventType classifies what the machine wants the player to do for a frame.
type EventType int

const (
	EventDropped EventType = iota
	EventConceal
	EventAudio
	EventDecoderRebuilt
	EventDRCReconfigured
	EventRecovered
)

func (e EventType) String() string {
	return [...]string{"Dropped", "Conceal", "Audio", "DecoderRebuilt", "DRCReconfigured", "Recovered"}[e]
}

// Event is emitted per fed frame. EventAudio always carries the committed
// config that the samples were decoded with.
type Event struct {
	Type   EventType
	Frame  int
	Config ProgramConfig
	Detail string
}

// Hint carries out-of-band demuxer information for the next frame.
type Hint struct {
	// ProgramBoundary is set on the first frame of a new program, e.g. from
	// a container discontinuity indicator.
	ProgramBoundary bool
}

const (
	// initFramesRequired is how many consecutive valid frames with a
	// consistent core config make up a "complete initial state".
	initFramesRequired = 2
	// maxCRCErrors is the concealment budget before sync is declared lost.
	maxCRCErrors = 3
)

// Machine drives decoder (re)configuration from E-AC-3 syncframe headers.
type Machine struct {
	state        State
	committed    ProgramConfig
	pending      ProgramConfig
	pendingCount int
	crcErrors    int
	frame        int
	transitions  []Transition
	dec          *Decoder
}

func NewMachine() *Machine {
	return &Machine{state: StateAcquire, dec: &Decoder{Muted: true}}
}

func (m *Machine) State() State                   { return m.state }
func (m *Machine) Decoder() *Decoder              { return m.dec }
func (m *Machine) Transitions() []Transition      { return m.transitions }
func (m *Machine) CommittedConfig() ProgramConfig { return m.committed }

func (m *Machine) transition(to State, r Reason, cfg ProgramConfig) {
	m.transitions = append(m.transitions, Transition{
		Seq:    len(m.transitions) + 1,
		Frame:  m.frame,
		From:   m.state,
		To:     to,
		Reason: r,
		Config: cfg,
	})
	m.state = to
}

func (m *Machine) event(t EventType, cfg ProgramConfig, detail string) Event {
	return Event{Type: t, Frame: m.frame, Config: cfg, Detail: detail}
}

// Feed ingests one syncframe and returns the events for it.
func (m *Machine) Feed(frame []byte, hint Hint) []Event {
	m.frame++
	h, err := ParseHeader(frame)
	if err == nil && !ValidateCRC(frame, h) {
		err = ErrCRCMismatch
	}
	if err != nil {
		return m.onInvalidFrame(err)
	}
	return m.onValidFrame(configFromHeader(h), hint)
}

func (m *Machine) onValidFrame(cfg ProgramConfig, hint Hint) []Event {
	switch m.state {
	case StateAcquire:
		m.enterPending(cfg, ReasonSyncAcquired)
		return m.checkInitComplete()

	case StateAwaitInit:
		if hint.ProgramBoundary || !cfg.sameCore(m.pending) {
			m.enterPending(cfg, m.reasonFor(cfg, m.pending, hint))
			return m.checkInitComplete()
		}
		m.pending.DRCProfile = cfg.DRCProfile
		m.pendingCount++
		return m.checkInitComplete()

	case StateRecoverCRC:
		if cfg.sameCore(m.committed) {
			m.crcErrors = 0
			m.transition(StateStreaming, ReasonCRCRecovered, m.committed)
			return []Event{
				m.event(EventRecovered, m.committed, "valid frame after concealment"),
				m.event(EventAudio, m.committed, ""),
			}
		}
		// The stream changed under us while concealing: treat as new program.
		m.enterPending(cfg, m.reasonFor(cfg, m.committed, hint))
		return m.checkInitComplete()

	default: // StateStreaming, StateDRCUpdate
		if hint.ProgramBoundary || !cfg.sameCore(m.committed) {
			// New program: wait for a complete initial state instead of
			// decoding with the stale channel mode.
			m.enterPending(cfg, m.reasonFor(cfg, m.committed, hint))
			return m.checkInitComplete()
		}
		if cfg.DRCProfile != m.committed.DRCProfile {
			return m.applyDRC(cfg)
		}
		return []Event{m.event(EventAudio, m.committed, "")}
	}
}

func (m *Machine) onInvalidFrame(err error) []Event {
	switch m.state {
	case StateAwaitInit:
		// A corrupt frame breaks the run of consecutive valid headers.
		m.pendingCount = 0
		return []Event{m.event(EventDropped, m.pending, err.Error())}
	case StateStreaming, StateDRCUpdate:
		m.crcErrors = 1
		m.transition(StateRecoverCRC, ReasonCRCError, m.committed)
		return []Event{m.event(EventConceal, m.committed, err.Error())}
	case StateRecoverCRC:
		m.crcErrors++
		if m.crcErrors > maxCRCErrors {
			m.crcErrors = 0
			m.dec.Mute()
			m.transition(StateAcquire, ReasonResyncFailed, m.committed)
			return []Event{m.event(EventDropped, m.committed, "resync failed: "+err.Error())}
		}
		return []Event{m.event(EventConceal, m.committed, err.Error())}
	default: // StateAcquire
		return []Event{m.event(EventDropped, ProgramConfig{}, err.Error())}
	}
}

// enterPending starts (or restarts) the collection of a complete initial
// state for a new program. The current frame counts as the first of the set.
func (m *Machine) enterPending(cfg ProgramConfig, r Reason) {
	m.pending = cfg
	m.pendingCount = 1
	m.crcErrors = 0
	m.transition(StateAwaitInit, r, cfg)
}

// checkInitComplete commits the pending config and rebuilds the decoder once
// enough consecutive consistent frames have arrived.
func (m *Machine) checkInitComplete() []Event {
	if m.pendingCount < initFramesRequired {
		return []Event{m.event(EventDropped, m.pending, "await-init: waiting for complete initial state")}
	}
	m.committed = m.pending
	m.dec.Rebuild(m.committed)
	m.transition(StateStreaming, ReasonInitComplete, m.committed)
	return []Event{
		m.event(EventDecoderRebuilt, m.committed, "decoder rebuilt from new program header"),
		m.event(EventAudio, m.committed, ""),
	}
}

// applyDRC handles a DRC-profile-only change: reconfigure in place, no
// decoder rebuild, no audio gap.
func (m *Machine) applyDRC(cfg ProgramConfig) []Event {
	m.transition(StateDRCUpdate, ReasonDRCProfileChange, cfg)
	m.committed.DRCProfile = cfg.DRCProfile
	m.dec.ConfigureDRC(cfg.DRCProfile)
	m.transition(StateStreaming, ReasonDRCApplied, m.committed)
	return []Event{
		m.event(EventDRCReconfigured, m.committed, "drc profile applied without rebuild"),
		m.event(EventAudio, m.committed, ""),
	}
}

// reasonFor classifies a config change so each recovery path gets its own
// recorded reason.
func (m *Machine) reasonFor(newCfg, oldCfg ProgramConfig, hint Hint) Reason {
	switch {
	case hint.ProgramBoundary:
		return ReasonProgramBoundary
	case newCfg.SampleRate != oldCfg.SampleRate:
		return ReasonSampleRateChange
	default:
		return ReasonChannelModeChange
	}
}
