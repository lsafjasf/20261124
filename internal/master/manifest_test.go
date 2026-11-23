package master

import (
	"encoding/json"
	"testing"
)

func TestConfigPartialKeepsDitherDefault(t *testing.T) {
	var cfg Config
	if err := json.Unmarshal([]byte(`{"sdr_peak_nits":120}`), &cfg); err != nil {
		t.Fatal(err)
	}
	merged, err := mergeConfig(DefaultConfig(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !merged.Dither || merged.SDRPeakNits != 120 {
		t.Fatalf("merged config: %+v", merged)
	}
}

func TestConfigCanDisableDitherExplicitly(t *testing.T) {
	var cfg Config
	if err := json.Unmarshal([]byte(`{"dither":false}`), &cfg); err != nil {
		t.Fatal(err)
	}
	merged, err := mergeConfig(DefaultConfig(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Dither {
		t.Fatal("explicit dither=false was ignored")
	}
}
