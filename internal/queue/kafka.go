package queue

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/segmentio/kafka-go"

	"coordinate-validator/internal/config"
	"coordinate-validator/internal/model"
)

type KafkaProducer struct {
	refinementWriter *kafka.Writer
	learningWriter   *kafka.Writer
	cfg              *config.KafkaConfig
}

func NewKafkaProducer(cfg *config.KafkaConfig) *KafkaProducer {
	refinementWriter := &kafka.Writer{
		Addr:     kafka.TCP(cfg.Brokers...),
		Topic:    cfg.RefinementTopic,
		Balancer: &kafka.LeastBytes{},
		Async:    true,
	}

	learningWriter := &kafka.Writer{
		Addr:     kafka.TCP(cfg.Brokers...),
		Topic:    cfg.LearningTopic,
		Balancer: &kafka.LeastBytes{},
		Async:    true,
	}

	return &KafkaProducer{
		refinementWriter: refinementWriter,
		learningWriter:   learningWriter,
		cfg:              cfg,
	}
}

func (p *KafkaProducer) Close() error {
	if err := p.refinementWriter.Close(); err != nil {
		return err
	}
	return p.learningWriter.Close()
}

// ============================================
// Refinement Events
// ============================================

func (p *KafkaProducer) SendRefinementEvent(ctx context.Context, event *model.RefinementEvent) error {
	event.EventTime = time.Now()

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	msg := kafka.Message{
		Key:   []byte(event.DeviceID),
		Value: data,
	}

	log.Printf("[Kafka] Sending refinement event: device=%s, result=%s", event.DeviceID, event.Result)

	return p.refinementWriter.WriteMessages(ctx, msg)
}

// ============================================
// Learning Events
// ============================================

func (p *KafkaProducer) SendLearningEvent(ctx context.Context, event *model.LearningEvent) error {
	event.EventTime = time.Now()

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	msg := kafka.Message{
		Key:   []byte(event.ObjectID),
		Value: data,
	}

	log.Printf("[Kafka] Sending learning event: object=%s, companion=%v", event.ObjectID, event.IsCompanion)

	return p.learningWriter.WriteMessages(ctx, msg)
}

// ============================================
// Batch Operations
// ============================================

func (p *KafkaProducer) SendRefinementBatch(ctx context.Context, events []model.RefinementEvent) error {
	if len(events) == 0 {
		return nil
	}

	msgs := make([]kafka.Message, len(events))
	for i, e := range events {
		e.EventTime = time.Now()
		data, _ := json.Marshal(e)
		msgs[i] = kafka.Message{
			Key:   []byte(e.DeviceID),
			Value: data,
		}
	}

	log.Printf("[Kafka] Sending batch of %d refinement events", len(events))

	return p.refinementWriter.WriteMessages(ctx, msgs...)
}

func (p *KafkaProducer) SendLearningBatch(ctx context.Context, events []model.LearningEvent) error {
	if len(events) == 0 {
		return nil
	}

	msgs := make([]kafka.Message, len(events))
	for i, e := range events {
		e.EventTime = time.Now()
		data, _ := json.Marshal(e)
		msgs[i] = kafka.Message{
			Key:   []byte(e.ObjectID),
			Value: data,
		}
	}

	log.Printf("[Kafka] Sending batch of %d learning events", len(events))

	return p.learningWriter.WriteMessages(ctx, msgs...)
}
