package processor

import (
	"log"
	"math"

	"gonum.org/v1/gonum/stat"
)

type StatisticalAnalysis struct {
	Mean      float64 `json:"mean"`
	StdDev    float64 `json:"stdDev"`
	Variance  float64 `json:"variance"`
	ZScore    float64 `json:"zScore"`
	IsAnomaly bool    `json:"isAnomaly"`
}

// AnalyzeMetric performs statistical variance analysis and Z-score anomaly calculation.
func AnalyzeMetric(source, metricType string, values []float64, currentVal float64) StatisticalAnalysis {
	if len(values) == 0 {
		return StatisticalAnalysis{
			Mean:      currentVal,
			StdDev:    0,
			Variance:  0,
			ZScore:    0,
			IsAnomaly: false,
		}
	}

	mean := stat.Mean(values, nil)
	variance := stat.Variance(values, nil)
	stdDev := math.Sqrt(variance)

	zScore := 0.0
	if stdDev > 0 {
		zScore = (currentVal - mean) / stdDev
	}

	// Anomaly condition: Z-Score magnitude greater than 2.5
	isAnomaly := math.Abs(zScore) >= 2.5

	log.Printf("[INFO] [StatProcessor] Analyzed '%s:%s' -> CurrentVal: %.2f | Mean: %.2f | StdDev: %.2f | Z-Score: %.2f | Anomaly: %v",
		source, metricType, currentVal, mean, stdDev, zScore, isAnomaly)

	return StatisticalAnalysis{
		Mean:      mean,
		StdDev:    stdDev,
		Variance:  variance,
		ZScore:    zScore,
		IsAnomaly: isAnomaly,
	}
}
