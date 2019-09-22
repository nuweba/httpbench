package concurrency

import (
	"github.com/nuweba/counter"
	"github.com/nuweba/semaphore"
)

type Manager struct {
	ConcurrentCount  *counter.Counter
	ConcurrencySem   *semaphore.Semaphore
	concurrencyLimit uint64
}

func New(concurrencyLimit uint64) *Manager {
	return &Manager{
		ConcurrentCount:  new(counter.Counter),
		ConcurrencySem:   semaphore.NewSemaphore(concurrencyLimit),
		concurrencyLimit: concurrencyLimit,
	}
}

func (m *Manager) AddConcurrent() {
	m.ConcurrentCount.Inc()
}

func (m *Manager) DecConcurrent() {
	m.ConcurrentCount.Dec()
	m.ReleaseConcurrencySlot()
}

func (m *Manager) AcquireConcurrencySlot() {
	if !m.IsConcurrencyUnlimited() {
		m.ConcurrencySem.Acquire()
	}
}

func (m *Manager) IsConcurrencyUnlimited() bool {
	if m.concurrencyLimit == 0 {
		return true
	}

	return false
}

func (m *Manager) ReleaseConcurrencySlot() {
	if !m.IsConcurrencyUnlimited() {
		m.ConcurrencySem.Release()
	}
}
