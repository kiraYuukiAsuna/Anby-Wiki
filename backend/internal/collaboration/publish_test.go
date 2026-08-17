package collaboration

import "testing"

func TestSameSequence(t *testing.T) {
	t.Parallel()

	current := int64(7)
	stale := int64(6)
	negative := int64(-1)

	for _, test := range []struct {
		name     string
		expected *int64
		actual   int64
		want     bool
	}{
		{name: "equal", expected: &current, actual: 7, want: true},
		{name: "missing", expected: nil, actual: 7, want: false},
		{name: "stale", expected: &stale, actual: 7, want: false},
		{name: "negative", expected: &negative, actual: 7, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := sameSequence(test.expected, test.actual); got != test.want {
				t.Fatalf("sameSequence(%v, %d) = %t, want %t",
					test.expected, test.actual, got, test.want)
			}
		})
	}
}
