// Package freshness is an ordinary Go package whose one decision is written
// in go4t. Everything a caller sees is Go: time.Duration and time.Time in, a
// Go enum out. The go4t types stay inside this file.
package freshness

import (
	"time"

	"example.com/freshness/decide"
)

// Verdict is what the caller acts on.
type Verdict int

const (
	Fresh Verdict = iota
	Stale
	TTLInvalid
	TimestampMissing
	TimestampInvalid
)

// Evidence is an observation as Go code usually holds it: optional values and
// values that may be malformed. The zero value means "no TTL, never observed".
type Evidence struct {
	TTL        *time.Duration // nil: no TTL configured
	ObservedAt *time.Time     // nil: never observed
}

// Check reports whether the evidence is still fresh at now, and for how long.
//
// The projection below is the only place that turns Go's nil/zero conventions
// into the three named cases of each input; the decision itself, and the laws
// about it, are in decide/decide.gt.
func Check(e Evidence, now time.Time) (Verdict, time.Duration) {
	switch result := decide.FreshnessFn(ttl(e.TTL), timestamp(e.ObservedAt), now.UnixNano()).(type) {
	case decide.FreshnessFresh:
		return Fresh, time.Duration(result.RemainingNanos)
	case decide.FreshnessStale:
		return Stale, 0
	case decide.FreshnessTtlInvalid:
		return TTLInvalid, 0
	case decide.FreshnessTimestampMissing:
		return TimestampMissing, 0
	case decide.FreshnessTimestampInvalid:
		return TimestampInvalid, 0
	default:
		panic("unreachable: decide.Freshness has five constructors")
	}
}

func ttl(d *time.Duration) decide.Ttl {
	switch {
	case d == nil:
		return decide.TtlNoTtl{}
	case *d <= 0:
		return decide.TtlInvalidTtl{}
	default:
		return decide.TtlTtlNanos{Nanos: d.Nanoseconds()}
	}
}

func timestamp(t *time.Time) decide.Timestamp {
	switch {
	case t == nil:
		return decide.TimestampNoTimestamp{}
	case t.IsZero():
		return decide.TimestampInvalidTimestamp{}
	default:
		return decide.TimestampAtNanos{Nanos: t.UnixNano()}
	}
}
