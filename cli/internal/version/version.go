// Package version parses versions and Composer-style version constraints.
package version

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is a PHP or Node version as major, minor, patch.
type Version [3]int

// Parse reads "8.4.27", "8.4.27-1" (a Windows rebuild) or "8.4".
func Parse(s string) (Version, error) {
	v, _, err := parsePartial(s)
	return v, err
}

func (v Version) String() string { return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2]) }

// Minor returns "8.4".
func (v Version) Minor() string { return fmt.Sprintf("%d.%d", v[0], v[1]) }

// Compare returns -1, 0 or 1.
func (v Version) Compare(o Version) int {
	for i := range v {
		if v[i] != o[i] {
			if v[i] < o[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// parsePartial parses a version with 1–3 parts and reports how many were given.
func parsePartial(s string) (Version, int, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 { // "8.4.27-1", "8.4.0-dev"
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) == 0 || len(parts) > 4 || parts[0] == "" {
		return Version{}, 0, fmt.Errorf("invalid version %q", s)
	}
	var v Version
	n := 0
	for i, p := range parts {
		if i >= 3 {
			break // ignore a 4th part ("8.2.0.0")
		}
		num, err := strconv.Atoi(p)
		if err != nil || num < 0 {
			return Version{}, 0, fmt.Errorf("invalid version %q", s)
		}
		v[i] = num
		n++
	}
	return v, n, nil
}

// Constraint is a Composer version constraint such as "^8.2", "~8.2.0",
// ">=8.1 <8.4", "8.3.*" or "^8.2 || ^8.3".
type Constraint struct {
	groups [][]bound // OR of ANDs
	raw    string
}

type bound struct {
	op string // ">=", ">", "<=", "<", "==", "!="
	v  Version
}

func (b bound) ok(v Version) bool {
	c := v.Compare(b.v)
	switch b.op {
	case ">=":
		return c >= 0
	case ">":
		return c > 0
	case "<=":
		return c <= 0
	case "<":
		return c < 0
	case "!=":
		return c != 0
	default:
		return c == 0
	}
}

func (c Constraint) String() string { return c.raw }

// Check reports whether v satisfies the constraint.
func (c Constraint) Check(v Version) bool {
	for _, group := range c.groups {
		ok := true
		for _, b := range group {
			if !b.ok(v) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// ParseConstraint parses Composer's constraint syntax (the parts that make
// sense for a PHP version). A bare partial version such as "8.2" is read
// as "8.2.*", which is what people mean when they write it for PHP.
func ParseConstraint(s string) (Constraint, error) {
	c := Constraint{raw: s}
	normalized := strings.ReplaceAll(s, "||", "|")
	for _, alt := range strings.Split(normalized, "|") {
		group, err := parseGroup(alt)
		if err != nil {
			return Constraint{}, fmt.Errorf("constraint %q: %w", s, err)
		}
		c.groups = append(c.groups, group)
	}
	return c, nil
}

func parseGroup(s string) ([]bound, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty constraint")
	}
	// Hyphen range: "8.1 - 8.3".
	if lo, hi, ok := strings.Cut(s, " - "); ok {
		from, _, err := parsePartial(lo)
		if err != nil {
			return nil, err
		}
		to, n, err := parsePartial(hi)
		if err != nil {
			return nil, err
		}
		if n == 3 {
			return []bound{{">=", from}, {"<=", to}}, nil
		}
		return []bound{{">=", from}, {"<", bump(to, n-1)}}, nil
	}

	// Join "op version" written with a space (">= 8.1") before splitting.
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
	var terms []string
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if strings.Trim(f, "<>=!") == "" && i+1 < len(fields) {
			f += fields[i+1]
			i++
		}
		terms = append(terms, f)
	}

	var group []bound
	for _, t := range terms {
		bs, err := parseTerm(t)
		if err != nil {
			return nil, err
		}
		group = append(group, bs...)
	}
	return group, nil
}

func parseTerm(t string) ([]bound, error) {
	if i := strings.Index(t, "@"); i >= 0 { // stability flag: "^8.2@dev"
		t = t[:i]
	}
	if t == "*" || t == "x" || t == "" {
		return nil, nil
	}

	for _, op := range []string{">=", "<=", "!=", "==", "<>", ">", "<", "="} {
		if rest, ok := strings.CutPrefix(t, op); ok {
			v, _, err := parsePartial(rest)
			if err != nil {
				return nil, err
			}
			switch op {
			case "=":
				op = "=="
			case "<>":
				op = "!="
			}
			return []bound{{op, v}}, nil
		}
	}

	if rest, ok := strings.CutPrefix(t, "^"); ok {
		v, n, err := parsePartial(rest)
		if err != nil {
			return nil, err
		}
		// ^8.2 → >=8.2.0 <9.0.0; ^0.3 → <0.4.0 (not relevant for PHP, but correct).
		pos := 0
		for pos < n-1 && v[pos] == 0 {
			pos++
		}
		return []bound{{">=", v}, {"<", bump(v, pos)}}, nil
	}

	if rest, ok := strings.CutPrefix(t, "~"); ok {
		v, n, err := parsePartial(rest)
		if err != nil {
			return nil, err
		}
		// Composer: ~8.2 → <9.0; ~8.2.3 → <8.3.0; ~8 → <9.
		pos := n - 2
		if pos < 0 {
			pos = 0
		}
		return []bound{{">=", v}, {"<", bump(v, pos)}}, nil
	}

	// Wildcards "8.3.*", "8.*" and bare partial versions "8.3".
	trimmed := strings.TrimSuffix(strings.TrimSuffix(t, ".*"), ".x")
	v, n, err := parsePartial(trimmed)
	if err != nil {
		return nil, err
	}
	if n == 3 && trimmed == t {
		return []bound{{"==", v}}, nil
	}
	return []bound{{">=", v}, {"<", bump(v, n-1)}}, nil
}

// bump increments part pos and zeroes the parts after it.
func bump(v Version, pos int) Version {
	v[pos]++
	for i := pos + 1; i < len(v); i++ {
		v[i] = 0
	}
	return v
}
