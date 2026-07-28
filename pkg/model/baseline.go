package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// BaselineProfile represents historical seasonal metric baselines stored in MongoDB `business_baselines`.
type BaselineProfile struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	MetricKey   string             `bson:"metricKey" json:"metricKey"`   // e.g. "google_analytics:active_users"
	DayOfWeek   int                `bson:"dayOfWeek" json:"dayOfWeek"`   // 0=Sunday..6=Saturday
	HourOfDay   int                `bson:"hourOfDay" json:"hourOfDay"`   // 0..23
	MeanValue   float64            `bson:"meanValue" json:"meanValue"`
	StdDevValue float64            `bson:"stdDevValue" json:"stdDevValue"`
	SampleCount int64              `bson:"sampleCount" json:"sampleCount"`
	LastUpdated time.Time          `bson:"lastUpdated" json:"lastUpdated"`
}
