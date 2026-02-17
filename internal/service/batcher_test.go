package service

import (
	"testing"
)

func TestWriteBatcher_Stop(t *testing.T) {
	// Test that batcher can be stopped cleanly even without a KVService
	// We just test the stop mechanism without actual Raft
	wb := &writeBatcher{
		reqCh:     make(chan *writeRequest, 100),
		maxBatch:  200,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}

	// Start the run loop
	go wb.run()

	// Stop should not hang
	wb.Stop()

	// Double stop should be safe
	wb.Stop()
}

func TestWriteBatcher_StopReturnsError(t *testing.T) {
	wb := &writeBatcher{
		reqCh:     make(chan *writeRequest, 100),
		maxBatch:  200,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}

	go wb.run()
	wb.Stop()

	// Submit after stop should return error
	err := wb.Submit("key", "val")
	if err == nil {
		t.Error("expected error from Submit after Stop")
	}
}
