package storage

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

const (
	SignalsCollection  = "business_signals"
	TimelineCollection = "business_timeline"
	SignalTTL          = 90 * 24 * time.Hour // 90-day data retention window for streaming signals
)

type mongoRepository struct {
	client             *mongo.Client
	database           *mongo.Database
	signalsCollection  *mongo.Collection
	timelineCollection *mongo.Collection
}

func NewMongoRepository(ctx context.Context, cfg *config.Config) (SignalRepository, error) {
	if cfg.MongoURI == "" {
		return nil, fmt.Errorf("MongoDB URI is required")
	}

	connCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	clientOpts := options.Client().ApplyURI(cfg.MongoURI)
	client, err := mongo.Connect(connCtx, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	if err := client.Ping(connCtx, readpref.Primary()); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB at %s: %w", cfg.MongoURI, err)
	}

	log.Printf("[INFO] Connected to MongoDB database '%s' at %s", cfg.MongoDatabase, cfg.MongoURI)

	db := client.Database(cfg.MongoDatabase)
	repo := &mongoRepository{
		client:             client,
		database:           db,
		signalsCollection:  db.Collection(SignalsCollection),
		timelineCollection: db.Collection(TimelineCollection),
	}

	if err := repo.EnsureIndexes(ctx); err != nil {
		log.Printf("[WARN] Error setting up MongoDB indexes: %v", err)
	}

	return repo, nil
}

func (r *mongoRepository) EnsureIndexes(ctx context.Context) error {
	signalIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "source", Value: 1},
				{Key: "type", Value: 1},
				{Key: "timestamp", Value: -1},
			},
			Options: options.Index().SetName("idx_source_type_time"),
		},
		{
			Keys: bson.D{
				{Key: "timestamp", Value: 1},
			},
			Options: options.Index().SetExpireAfterSeconds(int32(SignalTTL.Seconds())).SetName("idx_signal_ttl"),
		},
	}

	timelineIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "eventType", Value: 1},
				{Key: "timestamp", Value: -1},
			},
			Options: options.Index().SetName("idx_event_type_time"),
		},
	}

	_, err := r.signalsCollection.Indexes().CreateMany(ctx, signalIndexes)
	if err != nil {
		return fmt.Errorf("failed to create signals collection indexes: %w", err)
	}

	_, err = r.timelineCollection.Indexes().CreateMany(ctx, timelineIndexes)
	if err != nil {
		return fmt.Errorf("failed to create timeline collection indexes: %w", err)
	}

	log.Printf("[INFO] MongoDB indexes ensured for collections '%s' and '%s'", SignalsCollection, TimelineCollection)
	return nil
}

func (r *mongoRepository) SaveSignals(ctx context.Context, signals []model.Signal) error {
	if len(signals) == 0 {
		return nil
	}

	docs := make([]interface{}, len(signals))
	now := time.Now()
	for i, s := range signals {
		if s.CreatedAt.IsZero() {
			s.CreatedAt = now
		}
		docs[i] = s
	}

	_, err := r.signalsCollection.InsertMany(ctx, docs)
	if err != nil {
		return fmt.Errorf("failed to batch insert %d signals: %w", len(signals), err)
	}

	log.Printf("[INFO] Successfully saved %d signals to MongoDB", len(signals))
	return nil
}

func (r *mongoRepository) GetRecentSignals(ctx context.Context, limit int) ([]model.Signal, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "timestamp", Value: -1}}).
		SetLimit(int64(limit))

	cursor, err := r.signalsCollection.Find(ctx, bson.D{}, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to query signals: %w", err)
	}
	defer cursor.Close(ctx)

	var signals []model.Signal
	if err := cursor.All(ctx, &signals); err != nil {
		return nil, fmt.Errorf("failed to decode signals: %w", err)
	}

	return signals, nil
}

func (r *mongoRepository) SaveBusinessEvent(ctx context.Context, event *model.BusinessEvent) error {
	if event == nil {
		return nil
	}

	now := time.Now()
	if event.CreatedAt.IsZero() {
		event.CreatedAt = now
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = now
	}

	_, err := r.timelineCollection.InsertOne(ctx, event)
	if err != nil {
		return fmt.Errorf("failed to save BusinessEvent to timeline collection: %w", err)
	}

	log.Printf("[INFO] Saved BusinessEvent '%s' (ID: %s) to MongoDB business_timeline", event.EventType, event.EventID)
	return nil
}

func (r *mongoRepository) GetBusinessTimeline(ctx context.Context, limit int) ([]model.BusinessEvent, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "timestamp", Value: -1}}).
		SetLimit(int64(limit))

	cursor, err := r.timelineCollection.Find(ctx, bson.D{}, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to query business timeline: %w", err)
	}
	defer cursor.Close(ctx)

	var events []model.BusinessEvent
	if err := cursor.All(ctx, &events); err != nil {
		return nil, fmt.Errorf("failed to decode business timeline events: %w", err)
	}

	return events, nil
}

func (r *mongoRepository) Close(ctx context.Context) error {
	return r.client.Disconnect(ctx)
}
