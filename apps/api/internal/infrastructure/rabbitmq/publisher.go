package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Publisher publishes process-job messages.
type Publisher struct {
	conn         *amqp.Connection
	channel      *amqp.Channel
	exchange     string
	queue        string
	routingKey   string
	dlxExchange  string
	dlqName      string
}

type ProcessJobMessage struct {
	JobID      string `json:"job_id"`
	DocumentID string `json:"document_id"`
	EnqueuedAt string `json:"enqueued_at"`
}

func Connect(url, exchange, queue, routingKey string) (*Publisher, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	p := &Publisher{
		conn:        conn,
		channel:     ch,
		exchange:    exchange,
		queue:       queue,
		routingKey:  routingKey,
		dlxExchange: exchange + ".dlx",
		dlqName:     queue + ".dlq",
	}
	if err := p.declare(); err != nil {
		_ = p.Close()
		return nil, err
	}
	return p, nil
}

func (p *Publisher) declare() error {
	if err := p.channel.ExchangeDeclare(p.exchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if err := p.channel.ExchangeDeclare(p.dlxExchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	_, err := p.channel.QueueDeclare(p.dlqName, true, false, false, false, nil)
	if err != nil {
		return err
	}
	if err := p.channel.QueueBind(p.dlqName, p.routingKey, p.dlxExchange, false, nil); err != nil {
		return err
	}
	_, err = p.channel.QueueDeclare(p.queue, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange":    p.dlxExchange,
		"x-dead-letter-routing-key": p.routingKey,
	})
	if err != nil {
		return err
	}
	return p.channel.QueueBind(p.queue, p.routingKey, p.exchange, false, nil)
}

func (p *Publisher) PublishProcessJob(ctx context.Context, jobID, documentID string) error {
	body, err := json.Marshal(ProcessJobMessage{
		JobID:      jobID,
		DocumentID: documentID,
		EnqueuedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return err
	}
	return p.channel.PublishWithContext(ctx, p.exchange, p.routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
		Timestamp:    time.Now().UTC(),
		MessageId:    fmt.Sprintf("%s:%s", jobID, documentID),
	})
}

func (p *Publisher) Close() error {
	if p.channel != nil {
		_ = p.channel.Close()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}
