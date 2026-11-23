package rate

import "testing"

func TestResolve(t *testing.T) {
	cases := []struct {
		in        string
		nominal   int
		drop      bool
		shouldErr bool
	}{
		{"24", 24, false, false},
		{"25", 25, false, false},
		{"30", 30, false, false},
		{"29.97", 30, true, false},
		{"30000/1001", 30, true, false},
		{"59.94", 60, true, false},
		{"60000/1001", 60, true, false},
		{"23.976", 24, false, false},
		{"24000/1001", 24, false, false},
		{"60", 60, false, false},
		{"", 0, false, true},
		{"abc", 0, false, true},
		{"0", 0, false, true},
		{"100000", 0, false, true},
	}
	for _, c := range cases {
		r, err := Resolve(c.in)
		if c.shouldErr {
			if err == nil {
				t.Fatalf("%q: expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%q: unexpected error %v", c.in, err)
		}
		if r.NominalFPS != c.nominal || r.DropCapable != c.drop {
			t.Fatalf("%q: got nominal=%d drop=%v, want %d/%v",
				c.in, r.NominalFPS, r.DropCapable, c.nominal, c.drop)
		}
	}
}
