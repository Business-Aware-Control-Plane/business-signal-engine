package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type TimeWindow struct {
	Start time.Time `bson:"start" json:"start"`
	End   time.Time `bson:"end" json:"end"`
}

// BusinessEvent represents a high-confidence, standardized business event published to RabbitMQ.
type BusinessEvent struct {
	ID                primitive.ObjectID     `bson:"_id,omitempty" json:"id,omitempty"`
	EventID           string                 `bson:"eventId" json:"eventId"`                   // Unique UUID
	EventType         string                 `bson:"eventType" json:"eventType"`               // e.g. "ExpectedDemandIncrease", "ViralCampaignSpike"
	Category          string                 `bson:"category" json:"category"`                 // e.g. "Marketing", "Environmental", "Operational", "Business Calendar"
	Severity          string                 `bson:"severity" json:"severity"`                 // "Low", "Medium", "High", "Critical"
	Confidence        float64                `bson:"confidence" json:"confidence"`             // Overall confidence score 0.0 to 1.0
	TimeWindow        TimeWindow             `bson:"timeWindow" json:"timeWindow"`             // Impact time window
	SupportingSignals []string               `bson:"supportingSignals" json:"supportingSignals"` // Sources of contributing signals
	AISummary         string                 `bson:"aiSummary,omitempty" json:"aiSummary,omitempty"` // AI-synthesized narrative
	Metadata          map[string]interface{} `bson:"metadata,omitempty" json:"metadata,omitempty"`   // Contextual payload
	Timestamp         time.Time              `bson:"timestamp" json:"timestamp"`
	CreatedAt         time.Time              `bson:"createdAt" json:"createdAt"`
}
