package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Signal represents a standardized business, environmental, or external event metric.
type Signal struct {
	ID         primitive.ObjectID     `bson:"_id,omitempty" json:"id,omitempty"`
	SignalID   string                 `bson:"signalId" json:"signalId"`
	Source     string                 `bson:"source" json:"source"`                 // e.g., "google_analytics", "meta_business", "weather", "calendar"
	Type       string                 `bson:"type" json:"type"`                     // e.g., "active_users", "ad_ctr", "temperature", "public_holiday"
	Value      float64                `bson:"value" json:"value"`                   // Metric numeric value
	Unit       string                 `bson:"unit,omitempty" json:"unit,omitempty"` // e.g., "celsius", "count", "percentage", "currency"
	Confidence float64                `bson:"confidence" json:"confidence"`         // Score between 0.0 and 1.0 indicating data reliability
	Metadata   map[string]interface{} `bson:"metadata,omitempty" json:"metadata,omitempty"` // Provider-specific contextual payload
	Timestamp  time.Time              `bson:"timestamp" json:"timestamp"`           // Time of metric measurement
	CreatedAt  time.Time              `bson:"createdAt" json:"createdAt"`           // Extraction ingestion timestamp
}
