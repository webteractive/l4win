package version

import "testing"

func TestConstraints(t *testing.T) {
	cases := []struct {
		constraint string
		version    string
		want       bool
	}{
		{"^8.2", "8.2.0", true},
		{"^8.2", "8.5.12", true},
		{"^8.2", "8.1.34", false},
		{"^8.2", "9.0.0", false},
		// Composer's tilde differs from npm's: ~8.2 allows 8.x from 8.2 up.
		{"~8.2", "8.4.1", true},
		{"~8.2", "9.0.0", false},
		{"~8.2.3", "8.2.9", true},
		{"~8.2.3", "8.3.0", false},
		{"8.3.*", "8.3.36", true},
		{"8.3.*", "8.4.0", false},
		{"8.*", "8.5.0", true},
		{"8.3", "8.3.36", true}, // bare partial version means 8.3.*
		{"8.3", "8.4.0", false},
		{"8.3.36", "8.3.36", true},
		{"8.3.36", "8.3.35", false},
		{">=8.1 <8.4", "8.3.0", true},
		{">=8.1 <8.4", "8.4.0", false},
		{">=8.1,<8.4", "8.0.30", false},
		{">= 8.1", "8.1.0", true},
		{"^8.2|^9.0", "9.1.0", true},
		{"^8.2 || ^9.0", "8.2.1", true},
		{"^8.2 || ^9.0", "8.1.0", false},
		{"8.1 - 8.3", "8.3.20", true},
		{"8.1 - 8.3", "8.4.0", false},
		{"8.1 - 8.3.5", "8.3.6", false},
		{"*", "8.4.0", true},
		{"^8.2@dev", "8.3.0", true},
		{"!=8.3.0", "8.3.0", false},
		{"v24", "24.22.3", true},
	}
	for _, tc := range cases {
		c, err := ParseConstraint(tc.constraint)
		if err != nil {
			t.Fatalf("ParseConstraint(%q): %v", tc.constraint, err)
		}
		v, err := Parse(tc.version)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.version, err)
		}
		if got := c.Check(v); got != tc.want {
			t.Errorf("%q.Check(%s) = %v, want %v", tc.constraint, tc.version, got, tc.want)
		}
	}
}

func TestParse(t *testing.T) {
	for in, want := range map[string]string{
		"8.4.27-1": "8.4.27", // Windows rebuild suffix
		"v24.22.3": "24.22.3",
		"8.2.0.0":  "8.2.0",
		"8.4":      "8.4.0",
	} {
		v, err := Parse(in)
		if err != nil || v.String() != want {
			t.Errorf("Parse(%q) = %v, %v; want %s", in, v, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "8.x.1"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) should fail", bad)
		}
	}
	if _, err := ParseConstraint("^"); err == nil {
		t.Error(`ParseConstraint("^") should fail`)
	}
}
