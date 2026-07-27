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
	SignalsCollection = "business_signals"
	SignalTTL         = 90 * 24 * time.Hour // 90-day data retention window for streaming signals
)

type mongoRepository struct {
	client     *mongo.Client
	database   *mongo.Database
	collection *mongo.Collection
}

// NewMongoRepository initializes and returns a MongoDB repository instance.
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
		client:     client,
		database:   db,
		collection: db.Collection(SignalsCollection),
	}

	if err := repo.EnsureIndexes(ctx); err != nil {
		log.Printf("[WARN] Error setting up MongoDB indexes: %v", err)
	}

	return repo, nil
}

// EnsureIndexes creates performance compound indexes and TTL data retention index.
func (r *mongoRepository) EnsureIndexes(ctx context.Context) error {
	indexModels := []mongo.IndexModel{
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

	names, err := r.collection.Indexes().CreateMany(ctx, indexModels)
	if err != nil {
		return fmt.Errorf("failed to create MongoDB indexes: %w", err)
	}

	log.Printf("[INFO] MongoDB indexes ensured for collection '%s': %v", SignalsCollection, names)
	return nil
}

// SaveSignals performs batch insertion of signals into MongoDB.
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

	_, err := r.collection.InsertMany(ctx, docs)
	if err != nil {
		return fmt.Errorf("failed to batch insert %d signals: %w", len(signals), err)
	}

	log.Printf("[INFO] Successfully saved %d signals to MongoDB", len(signals))
	return nil
}

// GetRecentSignals retrieves the most recent signals sorted by timestamp descending.
func (r *mongoRepository) GetRecentSignals(ctx context.Context, limit int) ([]model.Signal, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "timestamp", Value: -1}}).
		SetLimit(int64(limit))

	cursor, err := r.collection.Find(ctx, bson.D{}, opts)
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

// Close gracefully closes the MongoDB connection.
func (r *mongoRepository) Close(ctx context.Context) error {
	return r.client.Disconnect(ctx)
}
