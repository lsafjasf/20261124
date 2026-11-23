package subtitle

import (
	"sort"
	"strings"
	"time"
)

// Decider 保存轨道、用户偏好与最近一次播放环境，
// 在音轨切换或窗口尺寸变化时基于缓存状态重新计算决策。
type Decider struct {
	tracks []Track
	prefs  Preferences
	last   PlaybackState
	has    bool
}

// NewDecider 用可用轨道与用户偏好构造决策器。
func NewDecider(tracks []Track, prefs Preferences) *Decider {
	cloned := append([]Track(nil), tracks...)
	return &Decider{tracks: cloned, prefs: prefs}
}

// SetPreferences 更新偏好并返回基于最近状态的新决策。
func (d *Decider) SetPreferences(prefs Preferences) Decision {
	d.prefs = prefs
	return d.recompute()
}

// ChangeAudio 切换音轨：更新音轨语言并重新计算。
func (d *Decider) ChangeAudio(audioLanguage string) Decision {
	d.last.AudioLanguage = audioLanguage
	return d.recompute()
}

// Resize 修改窗口可读空间并重新计算。
func (d *Decider) Resize(space ReadableSpace) Decision {
	d.last.Readable = space
	return d.recompute()
}

// Decide 针对给定播放状态做出决策，并缓存该状态供后续重算。
func (d *Decider) Decide(state PlaybackState) Decision {
	d.last = state
	d.has = true
	decision := Decide(d.tracks, d.prefs, state)
	return decision
}

func (d *Decider) recompute() Decision {
	if !d.has {
		return Decision{Reason: ReasonNone}
	}
	return Decide(d.tracks, d.prefs, d.last)
}

type regularCandidate struct {
	index int
	track *Track
	cue   *Cue
	fit   CueFit
	ok    bool
}

// Decide 是无状态的纯函数决策入口，规则与 Decider.Decide 完全一致。
func Decide(tracks []Track, prefs Preferences, state PlaybackState) Decision {
	regulars := collectRegulars(tracks, prefs, state)
	if prefs.SubtitlesEnabled && len(regulars) > 0 {
		if picked := pickRegular(regulars, prefs); picked != nil {
			reason := ReasonLanguageFallback
			if prefRank(picked.track.Language, prefs) >= 0 {
				reason = ReasonPreferredLanguage
			}
			if picked.track.Kind == KindSDH && prefs.SDHMode {
				reason = ReasonSDHPreferred
			}
			return Decision{
				Track: picked.track, Cue: picked.cue, Kind: picked.track.Kind,
				Fits: picked.fit.Fits, Overflow: picked.fit.Overflow, Reason: reason,
			}
		}
		// 存在当前时刻命中的常规轨时，强制轨不接管；
		// 即使所有常规轨都放不下，也保留排序最高者并标记溢出。
		return Decision{Reason: ReasonNone}
	}

	if forced := pickForced(tracks, prefs, state); forced != nil {
		reason := ReasonForcedOverride
		if !prefs.SubtitlesEnabled {
			reason = ReasonForcedOnly
		}
		return Decision{
			Track: forced.track, Cue: forced.cue, Kind: KindForced,
			Fits: forced.fit.Fits, Overflow: forced.fit.Overflow, Reason: reason,
		}
	}

	return Decision{Reason: ReasonNone}
}

func collectRegulars(tracks []Track, prefs Preferences, state PlaybackState) []regularCandidate {
	var candidates []regularCandidate
	for i := range tracks {
		track := &tracks[i]
		if track.Kind != KindFull && track.Kind != KindSDH {
			continue
		}
		cue := cueAt(track, state.At)
		if cue == nil {
			continue
		}
		fit := FitCue(*cue, state.Readable)
		candidates = append(candidates, regularCandidate{
			index: i, track: track, cue: cue, fit: fit, ok: fit.Fits,
		})
	}
	return candidates
}

func pickRegular(candidates []regularCandidate, prefs Preferences) *regularCandidate {
	sort.SliceStable(candidates, func(i, j int) bool {
		return regularLess(candidates[i], candidates[j], prefs)
	})
	for i := range candidates {
		if candidates[i].ok {
			return &candidates[i]
		}
	}
	return &candidates[0]
}

func regularLess(a, b regularCandidate, prefs Preferences) bool {
	ar, br := prefRank(a.track.Language, prefs), prefRank(b.track.Language, prefs)
	if (ar >= 0) != (br >= 0) {
		return ar >= 0
	}
	if ar != br {
		return ar < br
	}
	ak, bk := kindRank(a.track.Kind, prefs), kindRank(b.track.Kind, prefs)
	if ak != bk {
		return ak < bk
	}
	return a.index < b.index
}

func kindRank(kind Kind, prefs Preferences) int {
	if prefs.SDHMode {
		if kind == KindSDH {
			return 0
		}
		if kind == KindFull {
			return 1
		}
	} else {
		if kind == KindFull {
			return 0
		}
		if kind == KindSDH {
			return 1
		}
	}
	return 2
}

func prefRank(language string, prefs Preferences) int {
	base := langBase(language)
	for i, preferred := range prefs.PreferredLanguages {
		if langBase(preferred) == base {
			return i
		}
	}
	return -1
}

type forcedCandidate struct {
	track *Track
	cue   *Cue
	fit   CueFit
	index int
}

func pickForced(tracks []Track, prefs Preferences, state PlaybackState) *forcedCandidate {
	var candidates []forcedCandidate
	for i := range tracks {
		track := &tracks[i]
		if track.Kind != KindForced {
			continue
		}
		if track.AudioLanguage != "" && !langMatches(track.AudioLanguage, state.AudioLanguage) {
			continue
		}
		cue := cueAt(track, state.At)
		if cue == nil {
			continue
		}
		candidates = append(candidates, forcedCandidate{
			track: track, cue: cue, index: i,
			fit: FitCue(*cue, state.Readable),
		})
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		ri, rj := prefRank(candidates[i].track.Language, prefs), prefRank(candidates[j].track.Language, prefs)
		if (ri >= 0) != (rj >= 0) {
			return ri >= 0
		}
		if ri != rj {
			return ri < rj
		}
		return candidates[i].index < candidates[j].index
	})
	// 强制叙述不可省略：即使可读空间不足也必须返回，仅通过 Fits/Overflow 标记溢出。
	return &candidates[0]
}

func cueAt(track *Track, at time.Duration) *Cue {
	for i := range track.Cues {
		cue := &track.Cues[i]
		if at >= cue.Start && at < cue.End {
			return cue
		}
	}
	return nil
}

func langBase(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if i := strings.IndexByte(tag, '-'); i >= 0 {
		tag = tag[:i]
	}
	return tag
}

func langMatches(a, b string) bool {
	return langBase(a) == langBase(b)
}
