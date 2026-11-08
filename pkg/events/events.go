package events

import (
	"context"
	"time"
	"uuid"
)

type (
	DomainEvent interface {
		EventType() string
	}

	Envelope interface {
		IsReplay() bool
		AggregateId() uuid.UUID
		EventType() string
		Event() DomainEvent
		Time() time.Time
	}
)

type EventHandlerFunc func(ctx context.Context, envelope Envelope) error

type EventHandler interface {
	Handle(ctx context.Context, envelope Envelope) error
}

func (handler EventHandlerFunc) Handle(ctx context.Context, envelope Envelope) error {
	return handler(ctx, envelope)
}

func FilterName(eventType string, handler EventHandler) EventHandlerFunc {
	return func(ctx context.Context, envelope Envelope) error {
		if eventType == envelope.EventType() {
			return handler.Handle(ctx, envelope)
		}
		return nil
	}
}

type Dispatcher struct {
	handlers []EventHandler
}

func NewDispatcher(handlers ...EventHandler) *Dispatcher {
	return &Dispatcher{handlers}
}

func (d *Dispatcher) Dispatch(
	ctx context.Context,
	event Envelope,
) error {
	for _, handler := range d.handlers {
		if err := handler.Handle(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (d *Dispatcher) Register(handler EventHandler) {
	d.handlers = append(d.handlers, handler)
}

func (d *Dispatcher) RegisterFunc(handler EventHandlerFunc) {
	d.handlers = append(d.handlers, handler)
}
