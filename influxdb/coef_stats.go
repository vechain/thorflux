package influxdb

import (
	"github.com/vechain/thor/v2/api/blocks"
	"sort"
)

type coefStats struct {
	Average   float64
	Max       float64
	Min       float64
	Mode      float64
	Median    float64
	Total     int
	coefs     []float64
	coefCount map[float64]int
	sum       int
}

func (s *coefStats) processTx(t *blocks.JSONEmbeddedTx) {
	coef := *t.GasPriceCoef
	s.coefs = append(s.coefs, float64(coef))
	s.coefCount[float64(coef)]++
	s.sum += int(coef)
}

func (s *coefStats) finalizeCalc() {
	if len(s.coefs) > 0 {
		sort.Slice(s.coefs, func(i, j int) bool { return s.coefs[i] < s.coefs[j] })
		s.Min = s.coefs[0]
		s.Max = s.coefs[len(s.coefs)-1]
		s.Median = s.coefs[len(s.coefs)/2]
		s.Average = float64(s.sum) / float64(len(s.coefs))

		mode := float64(0)
		maxCount := 0
		for coef, count := range s.coefCount {
			if count > maxCount {
				mode = float64(coef)
				maxCount = count
			}
		}
		s.Mode = mode
	}
}
