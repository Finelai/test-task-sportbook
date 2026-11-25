package domain

import (
	"errors"
	"math"
	"time"
)

type Odds1X2 [3]int64

func Probabilities(sources [2][3]float64) ([3]float64, error) {
	var result [3]float64
	for _, source := range sources {
		var inverse [3]float64
		var sum float64
		for i, odd := range source {
			if odd <= 0 || math.IsNaN(odd) || math.IsInf(odd, 0) {
				return result, errors.New("invalid odds")
			}
			inverse[i] = 1 / odd
			sum += inverse[i]
		}
		if math.IsInf(sum, 0) || sum == 0 {
			return [3]float64{}, errors.New("invalid odds")
		}
		for i := range result {
			result[i] += inverse[i] / sum * 1.05 / 2
		}
	}
	return result, nil
}
func Price(sources [2][3]float64) (Odds1X2, error) {
	p, err := Probabilities(sources)
	if err != nil {
		return Odds1X2{}, err
	}
	var odds Odds1X2
	for i, v := range p {
		value := 100 / v
		if math.IsInf(value, 0) || value > float64(math.MaxInt64) {
			return Odds1X2{}, errors.New("odds out of range")
		}
		odds[i] = int64(math.Round(value))
	}
	return odds, nil
}

type Market struct {
	Sources [2][3]float64
	Updated [2]time.Time
}

func (m *Market) Update(source int, odds [3]float64, now time.Time) error {
	if source < 0 || source >= 2 {
		return errors.New("invalid source")
	}
	if _, err := Probabilities([2][3]float64{odds, odds}); err != nil {
		return err
	}
	m.Sources[source] = odds
	m.Updated[source] = now
	return nil
}
func (m Market) Open(now time.Time) bool {
	for _, updated := range m.Updated {
		if updated.IsZero() || now.Sub(updated) >= 15*time.Second {
			return false
		}
	}
	return true
}
