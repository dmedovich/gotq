package query

import (
	"context"
	"errors"
	"time"
)

// QueryPhase identifies one phase of Engine.List or Session.List.
type QueryPhase string

const (
	PhaseParse    QueryPhase = "parse"
	PhaseValidate QueryPhase = "validate"
	PhaseCount    QueryPhase = "count"
	PhaseData     QueryPhase = "data"
)

// Observation contains bounded metadata, never query text, SQL, cursor keys,
// database errors, or identities. ErrorCode is empty on success. Duration does
// not include time spent in the observer itself.
type Observation struct {
	Phase     QueryPhase
	Duration  time.Duration
	ErrorCode ErrorCode
}

// ObserverFunc receives synchronous List phase observations. It must be safe
// for concurrent use, return promptly, and not panic. The context is the
// caller's context (including cancellation). Parse and Apply do not emit events.
type ObserverFunc func(context.Context, Observation)

func (e *Engine[T]) observationStart() time.Time {
	if e.config.Observe == nil {
		return time.Time{}
	}
	return time.Now()
}

func (e *Engine[T]) observe(ctx context.Context, phase QueryPhase, start time.Time, err error) {
	if e.config.Observe == nil {
		return
	}
	event := Observation{Phase: phase, Duration: time.Since(start)}
	if err != nil {
		event.ErrorCode = CodeExecutionFailed
		var queryError *Error
		if errors.As(err, &queryError) {
			event.ErrorCode = queryError.Code
		}
	}
	e.config.Observe(ctx, event)
}
