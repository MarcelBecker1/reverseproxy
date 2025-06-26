package connection

import "sync"

type Manager struct {
	mu          *sync.RWMutex
	connections uint16
	capacity    uint16
}

func NewManager(capacity uint16) *Manager {
	return &Manager{
		connections: 0,
		capacity:    capacity,
	}
}

func (m *Manager) Increment() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connections++
}

func (m *Manager) Decrement() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.connections > 0 {
		m.connections--
	}
}

func (m *Manager) HasCapacity() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connections < m.capacity
}

func (m *Manager) Count() uint16 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connections
}
