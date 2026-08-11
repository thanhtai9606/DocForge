package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Consumer reads process-job messages from RabbitMQ.
type Consumer struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	queue   string
}

func ConnectConsumer(url, exchange, queue, routingKey string) (*Consumer, error) {
	pub, err := Connect(url, exchange, queue, routingKey)
	if err != nil {
		return nil, err
	}
	return &Consumer{conn: pub.conn, channel: pub.channel, queue: queue}, nil
}

func (c *Consumer) Close() error {
	if c.channel != nil {
		_ = c.channel.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Handler processes one job message. Return error to nack/requeue according to retryable policy.
type Handler func(ctx context.Context, msg ProcessJobMessage) error

// Serve consumes messages until ctx is cancelled.
func (c *Consumer) Serve(ctx context.Context, handler Handler) error {
	if err := c.channel.Qos(1, 0, false); err != nil {
		return err
	}
	deliveries, err := c.channel.Consume(c.queue, "docforge-worker", false, false, false, false, nil)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("rabbitmq channel closed")
			}
			var msg ProcessJobMessage
			if err := json.Unmarshal(d.Body, &msg); err != nil {
				_ = d.Nack(false, false) // poison → DLQ
				continue
			}
			if err := handler(ctx, msg); err != nil {
				_ = d.Nack(false, true)
				continue
			}
			_ = d.Ack(false)
		}
	}
}
