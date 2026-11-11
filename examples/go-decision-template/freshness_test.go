package freshness

import (
	"testing"
	"time"
)

func TestCheck(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	ttl := 30 * time.Second
	bad := -time.Second
	at := func(ago time.Duration) *time.Time { v := now.Add(-ago); return &v }
	var zero time.Time
	for _, test := range []struct {
		name     string
		evidence Evidence
		want     Verdict
		left     time.Duration
	}{
		{"no ttl is always fresh", Evidence{ObservedAt: at(time.Hour)}, Fresh, 0},
		{"invalid ttl wins over a missing timestamp", Evidence{TTL: &bad}, TTLInvalid, 0},
		{"never observed", Evidence{TTL: &ttl}, TimestampMissing, 0},
		{"zero timestamp", Evidence{TTL: &ttl, ObservedAt: &zero}, TimestampInvalid, 0},
		{"inside the ttl", Evidence{TTL: &ttl, ObservedAt: at(10 * time.Second)}, Fresh, 20 * time.Second},
		{"exactly at the ttl is stale", Evidence{TTL: &ttl, ObservedAt: at(30 * time.Second)}, Stale, 0},
		{"past the ttl", Evidence{TTL: &ttl, ObservedAt: at(time.Minute)}, Stale, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, left := Check(test.evidence, now)
			if got != test.want || left != test.left {
				t.Fatalf("Check = %v, %v; want %v, %v", got, left, test.want, test.left)
			}
		})
	}
}
