package testrt

import (
	"context"
	"errors"
	"testing"

	"gofx/rt"
)

func TestRunSuspendsAndCapturesFailures(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value any
		err   error
		want  bool
	}{
		{"true", true, nil, true},
		{"false", false, nil, false},
		{"IO error", nil, errors.New("broken"), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			created, executed := 0, 0
			action := Run("testExample", func() any {
				created++
				return rt.IO(func(context.Context) (any, error) {
					executed++
					return tt.value, tt.err
				})
			}).(rt.IO)
			if created != 0 || executed != 0 {
				t.Fatalf("test ran eagerly: created=%d executed=%d", created, executed)
			}
			value, err := action(context.Background())
			if err != nil || value != tt.want || created != 1 || executed != 1 {
				t.Fatalf("value=%v error=%v created=%d executed=%d, want %v, nil, 1, 1", value, err, created, executed, tt.want)
			}
		})
	}
}

func TestRunCancellationDoesNotConstructAction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	action := Run("cancelled", func() any {
		t.Fatal("cancelled harness constructed its action")
		return rt.Pure(true)
	}).(rt.IO)
	value, err := action(ctx)
	failure, ok := err.(*rt.Failure)
	if value != nil || !ok || failure.Kind != rt.Cancelled || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled harness = (%v, %#v), want classified Cancelled", value, err)
	}
}
