package tokens

import "testing"

func TestEstimatorCount(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"single short word", "hello", 2}, // ceil(5/4)
		{"tiny word", "a", 1},             // at least one token
		{"two words, single space is free", "hello world", 4},
		{"long word splits", "internationalization", 5}, // 20 runes / 4
		{"punctuation costs one each", "a, b.", 4},      // a , b .
		{"newline costs one", "a\nb", 3},
		{"indentation is cheap", "a\n    b", 4}, // a, \n, 3 extra spaces -> 1, b
		{"cjk one per rune", "日本語", 3},
		{"emoji one per rune", "🙂🙂", 2},
		{"code line", "func main() {", 5}, // func, main, (, ), {
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewEstimator().Count(tt.in); got != tt.want {
				t.Errorf("Count(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

// TestEstimatorProseRatio pins the estimator near the ~4 characters per token
// ratio that real BPE tokenizers show on English prose.
func TestEstimatorProseRatio(t *testing.T) {
	const prose = "The linter reads every instruction file in the repository and " +
		"reports the ones that have grown past their token budget, so a reviewer " +
		"can see the cost of the change before it lands on the default branch."

	got := NewEstimator().Count(prose)
	ratio := float64(len(prose)) / float64(got)
	if ratio < 3.2 || ratio > 5.0 {
		t.Errorf("Count = %d tokens for %d chars (%.2f chars/token), want a ratio in [3.2, 5.0]",
			got, len(prose), ratio)
	}
}

// TestEstimatorMonotonic guards the property the budgets rely on: more text is
// never fewer tokens.
func TestEstimatorMonotonic(t *testing.T) {
	e := NewEstimator()
	prev := 0
	text := ""
	for i := 0; i < 50; i++ {
		text += "another sentence of filler prose. "
		got := e.Count(text)
		if got < prev {
			t.Fatalf("Count dropped from %d to %d at iteration %d", prev, got, i)
		}
		prev = got
	}
}

func TestEstimatorImplementsCounter(t *testing.T) {
	var _ Counter = NewEstimator()
}
