// Package player models a playback pipeline whose subtitle renderer can be
// hot-swapped without rebuilding the video pipeline or moving the
// playback position.
package player

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"subselector/subselector"
)

// Cue is one timed subtitle line.
type Cue struct {
	Start time.Duration
	End   time.Duration
	Text  string
}

// Renderer renders one track's cues. It is the only component replaced
// during a language switch.
type Renderer struct {
	TrackID string
	Off     bool
	cues    []Cue
}

// CueAt returns the active cue at time t, or "" when none applies.
func (r *Renderer) CueAt(t time.Duration) string {
	if r == nil || r.Off {
		return ""
	}
	for _, c := range r.cues {
		if t >= c.Start && t < c.End {
			return c.Text
		}
	}
	return ""
}

// Player owns the simulated A/V pipeline.
type Player struct {
	mu sync.RWMutex

	selector *subselector.Selector
	logger   *slog.Logger

	// Pipeline state that must survive subtitle switches.
	position time.Duration
	playing  bool

	renderer *Renderer

	// Diagnostics counters.
	rendererSwaps   int
	pipelineRebuild int
}

// NewPlayer constructs a player around an existing selector.
func NewPlayer(sel *subselector.Selector, logger *slog.Logger) *Player {
	if logger == nil {
		logger = slog.Default()
	}
	return &Player{selector: sel, logger: logger}
}

// SwitchLanguage runs the fallback chain and applies the decision as a
// pure renderer hot-swap. Video output and playback position are untouched.
func (p *Player) SwitchLanguage(ctx context.Context,
	prefs []subselector.Preference, subtitlesOff bool) (subselector.Decision, error) {

	decision := p.selector.Select(ctx, prefs, subtitlesOff)
	if err := p.ApplyDecision(ctx, decision); err != nil {
		return decision, err
	}
	return decision, nil
}

// ApplyDecision replaces only the subtitle renderer. Re-applying the track
// already shown is a logged no-op so UI retries cannot cause flapping.
func (p *Player) ApplyDecision(ctx context.Context,
	decision subselector.Decision) error {

	p.mu.Lock()
	defer p.mu.Unlock()

	current := ""
	if p.renderer != nil {
		current = p.renderer.TrackID
	}
	if current == decision.TrackID &&
		(p.renderer != nil && p.renderer.Off == decision.Off) {
		p.logger.InfoContext(ctx, "subtitle switch skipped: already active",
			"track_id", decision.TrackID, "position_ms",
			p.position.Milliseconds())
		return nil
	}

	p.renderer = &Renderer{TrackID: decision.TrackID, Off: decision.Off}
	p.rendererSwaps++

	// Deliberately no change to p.position, p.playing and no call to any
	// pipeline teardown: the video decoder and audio keep running.
	p.logger.InfoContext(ctx, "subtitle renderer hot-swapped",
		"previous_track", current,
		"new_track", decision.TrackID,
		"off", decision.Off,
		"reason", decision.Reason,
		"position_ms", p.position.Milliseconds(),
		"renderer_swaps", p.rendererSwaps)
	return nil
}

// LoadCues attaches decoded cues to the active renderer.
func (p *Player) LoadCues(trackID string, cues []Cue) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.renderer == nil || p.renderer.TrackID != trackID {
		return fmt.Errorf("track %q is not the active subtitle track", trackID)
	}
	p.renderer.cues = append(p.renderer.cues[:0:0], cues...)
	return nil
}

// Play starts or resumes playback without touching subtitles.
func (p *Player) Play() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.playing = true
}

// Pause keeps position and subtitles intact.
func (p *Player) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.playing = false
}

// Stop tears down the whole pipeline, the only path that may reset video.
func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.playing = false
	p.position = 0
	p.renderer = nil
	p.pipelineRebuild++
}

// Tick advances the playback clock while playing.
func (p *Player) Tick(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.playing {
		p.position += d
	}
}

// Position returns the current playback position.
func (p *Player) Position() time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.position
}

// IsPlaying reports playback state.
func (p *Player) IsPlaying() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.playing
}

// ActiveTrackID returns the renderer's track, or "" when subtitles are off.
func (p *Player) ActiveTrackID() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.renderer == nil || p.renderer.Off {
		return ""
	}
	return p.renderer.TrackID
}

// CueAt exposes the renderer output at the current position.
func (p *Player) CueAt() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.renderer == nil {
		return ""
	}
	return p.renderer.CueAt(p.position)
}

// RendererSwaps counts subtitle hot-swaps.
func (p *Player) RendererSwaps() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.rendererSwaps
}

// PipelineRebuilds counts full pipeline teardowns (must stay zero across
// language switches).
func (p *Player) PipelineRebuilds() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.pipelineRebuild
}
