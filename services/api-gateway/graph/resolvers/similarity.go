package resolvers

import (
	"hash/fnv"
	"strings"
	"unicode"
)

// Code similarity for the integrity report, in the manner of MOSS
// (Schleimer, Wilkerson & Aiken, "Winnowing", SIGMOD 2003).
//
// Two submissions are compared by the structure of their code, not its text:
//   - comments and whitespace are dropped;
//   - every identifier becomes the same token, so renaming variables does not
//     hide a copy; numbers and strings likewise collapse to one token each;
//   - keywords and operators are kept, since they carry the structure.
//
// The token stream is cut into overlapping k-grams, each hashed, and winnowing
// keeps the minimum hash of every window of w consecutive k-grams. Those
// fingerprints are the code's signature. Fingerprints of the problem's starter
// code are removed first, so the boilerplate every candidate begins from is
// not mistaken for copying.

const (
	simK = 5 // tokens per k-gram: long enough that ordinary idioms rarely match
	simW = 4 // winnowing window: guarantees any shared run of k+w-1 tokens is caught
	// Below this many fingerprints a solution is too short to call a copy —
	// two correct three-line answers are supposed to look alike.
	simMinPrints = 8
)

// fingerprint is the winnowed signature of one piece of code.
type fingerprint map[uint64]struct{}

// Only structural keywords. Words that are also everyday variable names in
// some language (val, str, list, map, set, len, range, print…) are left out:
// keeping them would let a rename to one of them change the structure.
var simKeywords = func() map[string]bool {
	words := `
		if else for while do switch case default break continue return
		func function def class struct interface new delete this self super
		try catch except finally throw throws raise with import from package
		public private protected static final const let var void int long
		float double char bool boolean true false nil null None True False
		and or not in is go defer select chan yield lambda async await
		using namespace template typename auto
		`
	m := map[string]bool{}
	for _, w := range strings.Fields(words) {
		m[w] = true
	}
	return m
}()

// simTokens turns source into a normalised token stream.
func simTokens(code, language string) []string {
	hashComments := language == "python" || language == "sql"
	rs := []rune(code)
	var out []string
	for i := 0; i < len(rs); {
		c := rs[i]
		switch {
		case unicode.IsSpace(c):
			i++
		// Line comments: // everywhere, # in Python (and SQL's --).
		case c == '/' && i+1 < len(rs) && rs[i+1] == '/',
			c == '#' && hashComments,
			c == '-' && language == "sql" && i+1 < len(rs) && rs[i+1] == '-':
			for i < len(rs) && rs[i] != '\n' {
				i++
			}
		// C preprocessor lines (#include …) are boilerplate, not logic.
		case c == '#' && !hashComments:
			for i < len(rs) && rs[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(rs) && rs[i+1] == '*':
			i += 2
			for i+1 < len(rs) && !(rs[i] == '*' && rs[i+1] == '/') {
				i++
			}
			i += 2
		case c == '"' || c == '\'' || c == '`':
			q := c
			i++
			// Python docstrings ("""…""") are comments in practice.
			triple := i+1 < len(rs) && rs[i] == q && rs[i+1] == q
			if triple {
				i += 2
				for i+2 < len(rs) && !(rs[i] == q && rs[i+1] == q && rs[i+2] == q) {
					i++
				}
				i += 3
				continue
			}
			for i < len(rs) && rs[i] != q && rs[i] != '\n' {
				if rs[i] == '\\' {
					i++
				}
				i++
			}
			i++
			out = append(out, "S")
		case unicode.IsDigit(c):
			for i < len(rs) && (unicode.IsDigit(rs[i]) || rs[i] == '.' || rs[i] == '_' || unicode.IsLetter(rs[i])) {
				i++
			}
			out = append(out, "N")
		case unicode.IsLetter(c) || c == '_' || c == '$':
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == '_' || rs[j] == '$') {
				j++
			}
			w := string(rs[i:j])
			if simKeywords[w] {
				out = append(out, w)
			} else {
				out = append(out, "I")
			}
			i = j
		default:
			out = append(out, string(c))
			i++
		}
	}
	return out
}

// simFingerprint winnows the k-gram hashes of the code.
func simFingerprint(code, language string) fingerprint {
	toks := simTokens(code, language)
	fp := fingerprint{}
	if len(toks) < simK {
		return fp
	}
	hashes := make([]uint64, 0, len(toks)-simK+1)
	for i := 0; i+simK <= len(toks); i++ {
		h := fnv.New64a()
		for _, t := range toks[i : i+simK] {
			_, _ = h.Write([]byte(t))
			_, _ = h.Write([]byte{0})
		}
		hashes = append(hashes, h.Sum64())
	}
	if len(hashes) <= simW {
		for _, h := range hashes {
			fp[h] = struct{}{}
		}
		return fp
	}
	for i := 0; i+simW <= len(hashes); i++ {
		min := hashes[i]
		for _, h := range hashes[i+1 : i+simW] {
			if h < min {
				min = h
			}
		}
		fp[min] = struct{}{}
	}
	return fp
}

// without drops the prints also found in base (the starter code).
func (f fingerprint) without(base fingerprint) fingerprint {
	if len(base) == 0 {
		return f
	}
	out := fingerprint{}
	for h := range f {
		if _, ok := base[h]; !ok {
			out[h] = struct{}{}
		}
	}
	return out
}

// simScore is the share of the smaller signature found in the larger one, as
// a percentage — so pasting someone's solution into a longer file still
// scores high. ok is false when either side is too short to judge.
func simScore(a, b fingerprint) (pct int, ok bool) {
	if len(a) < simMinPrints || len(b) < simMinPrints {
		return 0, false
	}
	small, large := a, b
	if len(b) < len(a) {
		small, large = b, a
	}
	shared := 0
	for h := range small {
		if _, ok := large[h]; ok {
			shared++
		}
	}
	return shared * 100 / len(small), true
}
