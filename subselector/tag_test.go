package subselector

import "testing"

func TestParseTag(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		lang    string
		script  string
		region  string
		wantErr bool
	}{
		{"en", "en", "en", "", "", false},
		{"zh-hant-tw", "zh-Hant-TW", "zh", "Hant", "TW", false},
		{"zh-TW", "zh-Hant-TW", "zh", "Hant", "TW", false},
		{"zh-CN", "zh-Hans-CN", "zh", "Hans", "CN", false},
		{"zh-HK", "zh-Hant-HK", "zh", "Hant", "HK", false},
		{"pt-BR", "pt-BR", "pt", "", "BR", false},
		{"en-GB", "en-GB", "en", "", "GB", false},
		{"es-419", "es-419", "es", "", "419", false},
		{"en-US-x-private", "en-US", "en", "", "US", false},
		{"", "", "", "", "", false},
		{"123", "", "", "", "", true},
		{"en-GB-GB", "", "", "", "", true},
		{"en-abc-def", "", "", "", "", true},
	}
	for _, c := range cases {
		got, err := ParseTag(c.in)
		if (err != nil) != c.wantErr {
			t.Fatalf("ParseTag(%q) err=%v, wantErr=%v", c.in, err, c.wantErr)
		}
		if c.wantErr {
			continue
		}
		if got.String() != c.want || got.Language != c.lang ||
			got.Script != c.script || got.Region != c.region {
			t.Errorf("ParseTag(%q) = %+v, want %q lang=%q script=%q region=%q",
				c.in, got, c.want, c.lang, c.script, c.region)
		}
	}
}

func TestMatchScore(t *testing.T) {
	mustTag := func(s string) Tag {
		tag, err := ParseTag(s)
		if err != nil {
			t.Fatalf("unexpected parse error %q: %v", s, err)
		}
		return tag
	}

	cases := []struct {
		request string
		cand    string
		ok      bool
		exact   bool
	}{
		{"en-US", "en-US", true, true},
		{"en", "en-US", true, false},
		{"en-US", "en-GB", true, false},
		{"zh-TW", "zh-Hant", true, false},
		{"zh-CN", "zh-Hant", false, false}, // simplified vs traditional
		{"zh-Hans", "zh-Hant", false, false},
		{"en", "fr", false, false},
		{"pt-BR", "pt", true, false},
		{"pt", "pt-BR", true, false},
	}
	for _, c := range cases {
		score := MatchScore(mustTag(c.request), mustTag(c.cand))
		if c.ok && score < 0 {
			t.Errorf("MatchScore(%q,%q)=%d, expected compatible",
				c.request, c.cand, score)
		}
		if !c.ok && score >= 0 {
			t.Errorf("MatchScore(%q,%q)=%d, expected incompatible",
				c.request, c.cand, score)
		}
		if c.exact && score != 200 {
			t.Errorf("MatchScore(%q,%q)=%d, expected exact 200",
				c.request, c.cand, score)
		}
	}
}

func TestRegionBeatsLanguageOnly(t *testing.T) {
	req, _ := ParseTag("en-GB")
	gb, _ := ParseTag("en-GB")
	us, _ := ParseTag("en-US")
	plain, _ := ParseTag("en")
	if MatchScore(req, gb) <= MatchScore(req, us) {
		t.Error("en-GB should outrank en-US for request en-GB")
	}
	// A regionless generic track is a safer fallback than a track pinned
	// to the wrong region.
	if MatchScore(req, plain) <= MatchScore(req, us) {
		t.Error("regionless en should outrank the wrong-region en-US")
	}
	if MatchScore(req, gb) <= MatchScore(req, plain) {
		t.Error("the exact regional tag should outrank the regionless one")
	}
}
