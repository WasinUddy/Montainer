package logging

import (
	"context"
	"time"
)

// Stream identifies where a Bedrock log record originated.
type Stream string

const (
	StreamStdout Stream = "stdout"
	StreamStderr Stream = "stderr"
	StreamSystem Stream = "system"
)

// Record is Montainer's framework-independent representation of a log line.
// Attributes must be treated as immutable after Publish is called.
type Record struct {
	Timestamp         time.Time
	ObservedTimestamp time.Time
	Body              string
	Stream            Stream
	Attributes        map[string]string
}

// NewRecord constructs a record with the standard stream attribute used by
// OpenTelemetry semantic conventions.
func NewRecord(timestamp time.Time, stream Stream, body string) Record {
	return Record{
		Timestamp:         timestamp,
		ObservedTimestamp: timestamp,
		Body:              body,
		Stream:            stream,
		Attributes: map[string]string{
			"log.iostream": string(stream),
		},
	}
}

func (r Record) clone() Record {
	cloned := r
	if r.Attributes != nil {
		cloned.Attributes = make(map[string]string, len(r.Attributes))
		for key, value := range r.Attributes {
			cloned.Attributes[key] = value
		}
	}
	return cloned
}

// Sink receives records from its own bounded Hub queue. A slow or unavailable
// sink therefore cannot block the Bedrock pipe readers or other sinks.
type Sink interface {
	Write(context.Context, Record) error
}

// Flusher is implemented by sinks that can synchronously export buffered data.
type Flusher interface {
	ForceFlush(context.Context) error
}

// Shutdowner is implemented by sinks with resources that need closing.
type Shutdowner interface {
	Shutdown(context.Context) error
}

// Publisher is the small interface the Bedrock supervisor needs from a log
// hub. It keeps process supervision independent from concrete exporters.
type Publisher interface {
	Publish(Record)
}

// PublisherFunc adapts a function to Publisher.
type PublisherFunc func(Record)

func (f PublisherFunc) Publish(record Record) { f(record) }
