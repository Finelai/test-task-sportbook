package main

import (
	"testing"
	"time"
)

func TestGenerationInterval(t *testing.T) {
	for _, tc := range []struct {
		value   string
		want    time.Duration
		invalid bool
	}{
		{"", 5 * time.Second, false}, {"1h", time.Hour, false}, {"0s", 0, true}, {"-1s", 0, true}, {"wrong", 0, true},
	} {
		got, err := generationInterval(tc.value)
		if (err != nil) != tc.invalid || got != tc.want {
			t.Errorf("%q: got %s, error %v", tc.value, got, err)
		}
	}
}
