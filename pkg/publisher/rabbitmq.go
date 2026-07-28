package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	amqp "github.com/rabbitmq/amqp091-go"
)

type EventPublisher interface {
	PublishBusinessEvent(ctx context.Context, event *model.BusinessEvent) error
	Close() error
}

type rabbitMQPublisher struct {
	cfg     *config.Config
	conn    *amqp.Connection
	channel *amqp.Channel
}

func NewRabbitMQPublisher(cfg *config.Config) EventPublisher {
	if cfg.RabbitMQURI == "" {
		log.Printf("[INFO] [RabbitMQ] RABBITMQ_URI is not set. RabbitMQ publishing fallback active.")
		return &noopPublisher{}
	}

	conn, err := amqp.Dial(cfg.RabbitMQURI)
	if err != nil {
		log.Printf("[WARN] [RabbitMQ] Could not connect to RabbitMQ at %s: %v. Publishing fallback active.", cfg.RabbitMQURI, err)
		return &noopPublisher{}
	}

	ch, err := conn.Channel()
	if err != nil {
		log.Printf("[WARN] [RabbitMQ] Could not open AMQP channel: %v. Publishing fallback active.", err)
		_ = conn.Close()
		return &noopPublisher{}
	}

	// Declare exchange and queue
	err = ch.ExchangeDeclare(
		cfg.RabbitMQExchange, // name
		"topic",              // type
		true,                 // durable
		false,                // auto-deleted
		false,                // internal
		false,                // no-wait
		nil,                  // arguments
	)
	if err != nil {
		log.Printf("[WARN] [RabbitMQ] Failed to declare exchange '%s': %v", cfg.RabbitMQExchange, err)
	}

	_, err = ch.QueueDeclare(
		cfg.RabbitMQQueue, // name
		true,              // durable
		false,             // delete when unused
		false,             // exclusive
		false,             // no-wait
		nil,               // arguments
	)
	if err != nil {
		log.Printf("[WARN] [RabbitMQ] Failed to declare queue '%s': %v", cfg.RabbitMQQueue, err)
	}

	log.Printf("[INFO] [RabbitMQ] Connected to RabbitMQ at %s (Exchange: '%s', Queue: '%s')", cfg.RabbitMQURI, cfg.RabbitMQExchange, cfg.RabbitMQQueue)

	return &rabbitMQPublisher{
		cfg:     cfg,
		conn:    conn,
		channel: ch,
	}
}

func (p *rabbitMQPublisher) PublishBusinessEvent(ctx context.Context, event *model.BusinessEvent) error {
	if event == nil {
		return nil
	}

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to JSON serialize BusinessEvent: %w", err)
	}

	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	routingKey := fmt.Sprintf("business.event.%s", event.Category)

	err = p.channel.PublishWithContext(
		pubCtx,
		p.cfg.RabbitMQExchange, // exchange
		routingKey,            // routing key
		false,                 // mandatory
		false,                 // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Timestamp:   event.Timestamp,
			MessageId:   event.EventID,
			Body:        body,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish AMQP message to RabbitMQ: %w", err)
	}

	log.Printf("[INFO] [RabbitMQ] Successfully published BusinessEvent '%s' (ID: %s, RoutingKey: %s) to RabbitMQ", event.EventType, event.EventID, routingKey)
	return nil
}

func (p *rabbitMQPublisher) Close() error {
	if p.channel != nil {
		_ = p.channel.Close()
	}
	if p.conn != nil {
		_ = p.conn.Close()
	}
	return nil
}

type noopPublisher struct{}

func (n *noopPublisher) PublishBusinessEvent(ctx context.Context, event *model.BusinessEvent) error {
	if event != nil {
		log.Printf("[INFO] [Publisher-Fallback] Formatted BusinessEvent JSON Payload ready: EventID='%s', Type='%s', Severity='%s', Category='%s'", event.EventID, event.EventType, event.Severity, event.Category)
	}
	return nil
}

func (n *noopPublisher) Close() error {
	return nil
}
