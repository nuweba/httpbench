package concurrency

import (
	"testing"
	"time"
)

const (
	ConcurrentLimit = 5
)

func TestNew(t *testing.T) {
	manager := New(ConcurrentLimit)

	if manager.ConcurrentCount.Counter() != 0 {
		t.Error("Newly created counter should be 0")
	}
}

func TestManager_AddConcurrent(t *testing.T) {
	manager := New(ConcurrentLimit)
	manager.AddConcurrent()

	if manager.ConcurrentCount.Counter() != 1 {
		t.Error("Add concurrent should increase counter by 1")
	}
}

func TestManager_DecConcurrent(t *testing.T) {
	manager := New(ConcurrentLimit)
	manager.AddConcurrent()
	manager.AcquireConcurrencySlot()
	manager.DecConcurrent()

	if manager.ConcurrentCount.Counter() != 0 {
		t.Error("dec concurrent should decrease counter by 1")
	}
}

func TestManager_IsConcurrencyUnlimited(t *testing.T) {
	manager := New(ConcurrentLimit)

	if manager.IsConcurrencyUnlimited() {
		t.Error("Newly created manager with limit isn't unlimited")
	}

	manager2 := New(0)

	if !manager2.IsConcurrencyUnlimited() {
		t.Error("Newly created manager with limit is unlimited")
	}
}

func TestManager_AcquireConcurrencySlot(t *testing.T) {
	manager := New(ConcurrentLimit)
	order := make(chan int, 3)

	for i := 0; i < ConcurrentLimit; i++ {
		manager.AcquireConcurrencySlot()
	}

	go func() {
		manager.AcquireConcurrencySlot() // The ConcurrentLimit + 1 acquire should wait.
		order <- 2
	}()

	time.Sleep(time.Millisecond)
	order <- 1
	manager.DecConcurrent()

	a := <-order
	b := <-order

	if a > b {
		t.Error("manager acquire didn't work correctly")
	}
}
