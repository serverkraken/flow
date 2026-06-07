package usecase_test

import (
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// fakeSyncWriteQueue is a hand-rolled double for ports.WriteQueue.
type fakeSyncWriteQueue struct {
	entries []ports.WriteQueueEntry
	peekErr error
}

func (f *fakeSyncWriteQueue) Enqueue(_, _ string, _ []byte, _ int64) (int64, error) {
	return 0, errors.New("not implemented in fake")
}

func (f *fakeSyncWriteQueue) Peek(_ int) ([]ports.WriteQueueEntry, error) {
	if f.peekErr != nil {
		return nil, f.peekErr
	}
	return f.entries, nil
}

func (f *fakeSyncWriteQueue) Remove(_ int64) error { return nil }

func (f *fakeSyncWriteQueue) SetError(_ int64, _ string) error { return nil }

func (f *fakeSyncWriteQueue) SetRetry(_ int64, _ string, _ string) error { return nil }

// fakeSyncWatermarkStore is a hand-rolled double for ports.SyncWatermarkStore.
type fakeSyncWatermarkStore struct {
	watermarks map[string]int64
	getErr     error
}

func (f *fakeSyncWatermarkStore) Get(resource string) (int64, error) {
	if f.getErr != nil {
		return 0, f.getErr
	}
	return f.watermarks[resource], nil
}

func (f *fakeSyncWatermarkStore) Set(_ string, _ int64) error { return nil }

func TestSyncStatus_Status_QueueLen(t *testing.T) {
	entries := []ports.WriteQueueEntry{
		{Seq: 1, Resource: "sessions"},
		{Seq: 2, Resource: "active_sessions"},
		{Seq: 3, Resource: "active_sessions"},
	}
	uc := usecase.NewSyncStatus(
		&fakeSyncWriteQueue{entries: entries},
		&fakeSyncWatermarkStore{watermarks: map[string]int64{}},
	)

	st, err := uc.Status()
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if st.QueueLen != len(entries) {
		t.Errorf("QueueLen = %d, want %d", st.QueueLen, len(entries))
	}
}

func TestSyncStatus_Status_WatermarksPopulated(t *testing.T) {
	wm := map[string]int64{
		"projects":        42,
		"sessions":        7,
		"active_sessions": 0,
		"users":           1,
		"repos":           0,
		"repo_notes":      5,
	}
	uc := usecase.NewSyncStatus(
		&fakeSyncWriteQueue{entries: nil},
		&fakeSyncWatermarkStore{watermarks: wm},
	)

	st, err := uc.Status()
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if len(st.Watermarks) != len(wm) {
		t.Errorf("watermarks len = %d, want %d", len(st.Watermarks), len(wm))
	}
	for k, want := range wm {
		if got := st.Watermarks[k]; got != want {
			t.Errorf("watermarks[%q] = %d, want %d", k, got, want)
		}
	}
}

func TestSyncStatus_Status_LastPullAtEmpty(t *testing.T) {
	uc := usecase.NewSyncStatus(
		&fakeSyncWriteQueue{},
		&fakeSyncWatermarkStore{watermarks: map[string]int64{}},
	)
	st, err := uc.Status()
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if st.LastPullAt != "" {
		t.Errorf("LastPullAt = %q, want empty string", st.LastPullAt)
	}
	if st.LastPullError != "" {
		t.Errorf("LastPullError = %q, want empty string", st.LastPullError)
	}
}

func TestSyncStatus_Status_QueuePeekError(t *testing.T) {
	peekErr := errors.New("db error")
	uc := usecase.NewSyncStatus(
		&fakeSyncWriteQueue{peekErr: peekErr},
		&fakeSyncWatermarkStore{watermarks: map[string]int64{}},
	)
	_, err := uc.Status()
	if !errors.Is(err, peekErr) {
		t.Errorf("expected peekErr, got %v", err)
	}
}

func TestSyncStatus_ForcePull_ReturnsNotWired(t *testing.T) {
	uc := usecase.NewSyncStatus(
		&fakeSyncWriteQueue{},
		&fakeSyncWatermarkStore{watermarks: map[string]int64{}},
	)
	err := uc.ForcePull()
	if !errors.Is(err, usecase.ErrSyncWorkerNotWired) {
		t.Errorf("ForcePull() = %v, want ErrSyncWorkerNotWired", err)
	}
}

func TestSyncStatus_AcceptServerVersion_ReturnsNotWired(t *testing.T) {
	uc := usecase.NewSyncStatus(
		&fakeSyncWriteQueue{},
		&fakeSyncWatermarkStore{watermarks: map[string]int64{}},
	)
	if err := uc.AcceptServerVersion(1); !errors.Is(err, usecase.ErrSyncWorkerNotWired) {
		t.Errorf("AcceptServerVersion() = %v, want ErrSyncWorkerNotWired", err)
	}
}

func TestSyncStatus_OverwriteServerVersion_ReturnsNotWired(t *testing.T) {
	uc := usecase.NewSyncStatus(
		&fakeSyncWriteQueue{},
		&fakeSyncWatermarkStore{watermarks: map[string]int64{}},
	)
	if err := uc.OverwriteServerVersion(1); !errors.Is(err, usecase.ErrSyncWorkerNotWired) {
		t.Errorf("OverwriteServerVersion() = %v, want ErrSyncWorkerNotWired", err)
	}
}
