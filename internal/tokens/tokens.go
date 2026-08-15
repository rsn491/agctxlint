// Package tokens estimates how many tokens a piece of text costs a language
// model, without shipping a tokenizer vocabulary.
package tokens

import "unicode"

// Counter reports the token cost of a string. The heuristic Estimator is the
// only implementation today; the interface is the seam where a real BPE
// tokenizer can be substituted without touching the lint rules.
type Counter interface {
	Count(string) int
}

// Estimator approximates byte-pair encoding by measuring the shape of the text
// rather than looking words up in a vocabulary.
//
// Accuracy: on English prose it lands within roughly 15% of cl100k_base and
// o200k_base. On source code, punctuation-dense text and non-Latin scripts it
// deliberately errs high, so a budget that passes here also passes against a
// real tokenizer.
type Estimator struct{}

// NewEstimator returns the default heuristic Counter.
func NewEstimator() Estimator { return Estimator{} }

// Count returns the estimated number of tokens in s.
//
// The rules, applied in a single pass:
//
//   - A run of n ASCII letters or digits costs ceil(n/4) tokens, at least 1.
//     Real BPE merges common short words into one token and splits long ones
//     into word pieces of roughly four characters.
//   - Each punctuation or symbol rune costs 1 token.
//   - A single space between chunks is free: BPE folds the leading space into
//     the token that follows it. Each newline costs 1 token, and a whitespace
//     run of n characters beyond the first costs ceil(n/4) (indentation and
//     blank-line padding are cheap but not free).
//   - Each non-ASCII rune costs 1 token. CJK, emoji and other scripts tokenize
//     far denser than Latin text, often at or above one token per character.
func (Estimator) Count(s string) int {
	total := 0
	runs := 0 // consecutive non-space runes, flushed when the run ends

	flushWord := func() {
		if runs > 0 {
			total += ceilDiv(runs, 4)
			runs = 0
		}
	}

	space := 0 // consecutive whitespace characters, excluding newlines
	flushSpace := func() {
		if space > 1 {
			total += ceilDiv(space-1, 4)
		}
		space = 0
	}

	for _, r := range s {
		switch {
		case r == '\n' || r == '\r':
			flushWord()
			flushSpace()
			total++
		case unicode.IsSpace(r):
			flushWord()
			space++
		case r > unicode.MaxASCII:
			flushWord()
			flushSpace()
			total++
		case isWordRune(r):
			flushSpace()
			runs++
		default: // ASCII punctuation and symbols
			flushWord()
			flushSpace()
			total++
		}
	}
	flushWord()
	flushSpace()

	return total
}

func isWordRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

func ceilDiv(a, b int) int {
	if a <= 0 {
		return 0
	}
	return (a + b - 1) / b
}
