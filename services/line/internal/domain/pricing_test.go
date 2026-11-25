package domain

import (
	"math"
	"testing"
	"time"
)

func TestPrice(t *testing.T) {
	sources := [2][3]float64{{4, 4, 4}, {4, 4, 4}}
	got, err := Price(sources)
	if err != nil || got != (Odds1X2{286, 286, 286}) {
		t.Fatalf("Price = %v, %v", got, err)
	}
	p, err := Probabilities(sources)
	if err != nil || math.Abs(p[0]+p[1]+p[2]-1.05) > 1e-9 {
		t.Fatalf("probabilities = %v, %v", p, err)
	}
	for i := range sources {
		changed := sources
		changed[i][0] = 2
		next, err := Price(changed)
		if err != nil || next == got {
			t.Fatalf("source %d did not affect price", i)
		}
	}
	for _, v := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		invalid := sources
		invalid[0][1] = v
		if _, err := Price(invalid); err == nil {
			t.Fatalf("accepted %v", v)
		}
	}
}

func TestStaleSourceClosesMarket(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var m Market
	valid := [3]float64{4, 4, 4}
	if err := m.Update(0, valid, now); err != nil {
		t.Fatal(err)
	}
	if err := m.Update(1, valid, now); err != nil {
		t.Fatal(err)
	}
	if !m.Open(now.Add(15*time.Second-time.Nanosecond)) || m.Open(now.Add(15*time.Second)) {
		t.Fatal("freshness boundary")
	}
	if err := m.Update(0, [3]float64{2, 0, 3}, now.Add(time.Second)); err == nil {
		t.Fatal("accepted partial source")
	}
	if !m.Open(now.Add(14 * time.Second)) {
		t.Fatal("partial update discarded last complete source")
	}
	if err := m.Update(0, valid, now.Add(15*time.Second)); err != nil {
		t.Fatal(err)
	}
	if m.Open(now.Add(15 * time.Second)) {
		t.Fatal("second source stale")
	}
	if err := m.Update(1, valid, now.Add(15*time.Second)); err != nil {
		t.Fatal(err)
	}
	if !m.Open(now.Add(15 * time.Second)) {
		t.Fatal("not recovered")
	}
}
