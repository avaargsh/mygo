package xerj

import (
	"context"
	"errors"
	"testing"
)

func TestRebuildPublishesOnlyAfterVerification(t *testing.T) {
	m := NewManager("http://127.0.0.1:9200")
	built := false
	err := m.Rebuild(context.Background(), "a", "a-v1", func(_ context.Context, target BuildTarget) error {
		if target.Workspace != "a" || target.Index != "a-v1" || target.Generation != 1 || target.Endpoint != "http://127.0.0.1:9200" {
			t.Fatalf("target %+v", target)
		}
		built = true
		return nil
	}, func(_ context.Context, target BuildTarget) error {
		if !built || target.Generation != 1 || m.Status("a").State != IndexBuilding {
			t.Fatal("verification must precede publication")
		}
		return nil
	})
	if err != nil || m.Status("a").State != IndexReady {
		t.Fatalf("status=%+v err=%v", m.Status("a"), err)
	}
}

func TestRebuildFailureAndCancellation(t *testing.T) {
	boom := errors.New("fixture failure")
	for _, stage := range []string{"build", "verify", "cancel"} {
		t.Run(stage, func(t *testing.T) {
			m := NewManager("http://127.0.0.1:9200")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			verified := false
			err := m.Rebuild(ctx, "a", "a-v1", func(context.Context, BuildTarget) error {
				if stage == "build" {
					return boom
				}
				if stage == "cancel" {
					cancel()
				}
				return nil
			}, func(context.Context, BuildTarget) error {
				verified = true
				return boom
			})
			want := boom
			if stage == "cancel" {
				want = context.Canceled
			}
			if !errors.Is(err, want) || m.Status("a").State != IndexFailed || verified != (stage == "verify") {
				t.Fatalf("stage=%s status=%+v err=%v verified=%t", stage, m.Status("a"), err, verified)
			}
		})
	}
}

func TestRebuildSupersessionDoesNotVerifyOrOverwrite(t *testing.T) {
	m := NewManager("http://127.0.0.1:9200")
	entered, release := make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	verified := false
	go func() {
		result <- m.Rebuild(context.Background(), "a", "a-v1", func(context.Context, BuildTarget) error {
			close(entered)
			<-release
			return nil
		}, func(context.Context, BuildTarget) error { verified = true; return nil })
	}()
	<-entered
	second, err := m.Begin("a", "a-v2")
	if err == nil {
		err = m.Publish(second)
	}
	close(release)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, ErrStaleGeneration) || verified {
		t.Fatalf("superseded build: %v verified=%t", err, verified)
	}
	if got := m.Status("a"); got.State != IndexReady || got.Generation != 2 {
		t.Fatalf("new binding overwritten: %+v", got)
	}
}

func TestRebuildInvalidInputDoesNotFence(t *testing.T) {
	m := NewManager("http://127.0.0.1:9200")
	ticket, err := m.Begin("a", "a-v1")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Publish(ticket); err != nil {
		t.Fatal(err)
	}
	noop := func(context.Context, BuildTarget) error { return nil }
	if err := m.Rebuild(context.Background(), "a", "a-v2", noop, nil); err == nil {
		t.Fatal("missing verifier accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.Rebuild(ctx, "a", "a-v2", noop, noop); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v", err)
	}
	if got := m.Status("a"); got.State != IndexReady || got.Generation != 1 {
		t.Fatalf("binding changed: %+v", got)
	}
}
