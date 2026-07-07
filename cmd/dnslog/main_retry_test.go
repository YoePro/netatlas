package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"netatlas/internal/model"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type retryTestStore struct {
	err      error
	attempts int
}

func (s *retryTestStore) Close(ctx context.Context) error {
	return nil
}

func (s *retryTestStore) WriteBatch(ctx context.Context, batch []model.DNSEvent) error {
	s.attempts++
	return s.err
}

func TestWriteBatchWithRetryStopsOnNonRetryableNeo4jError(t *testing.T) {
	store := &retryTestStore{
		err: fmt.Errorf("write neo4j batch: %w", &neo4j.Neo4jError{
			Code: "Neo.ClientError.Request.Invalid",
			Msg:  "Illegal value for field params",
		}),
	}

	err := writeBatchWithRetry(context.Background(), store, []model.DNSEvent{{}}, 3, time.Millisecond)
	if err == nil {
		t.Fatal("expected error")
	}
	if store.attempts != 1 {
		t.Fatalf("attempts = %d, want 1", store.attempts)
	}
}

func TestWriteBatchWithRetryKeepsRetryingGenericErrors(t *testing.T) {
	store := &retryTestStore{err: fmt.Errorf("temporary write failure")}

	err := writeBatchWithRetry(context.Background(), store, []model.DNSEvent{{}}, 2, 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if store.attempts != 3 {
		t.Fatalf("attempts = %d, want 3", store.attempts)
	}
}
