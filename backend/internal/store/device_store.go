package store

import (
	"sort"
	"sync"
	"time"
)

// DeviceState holds the latest telemetry snapshot received from a single endpoint.
type DeviceState struct {
	DeviceID         string    `json:"deviceId"`
	Timestamp        string    `json:"timestamp"`
	ServiceName      string    `json:"serviceName"`
	ServiceStatus    string    `json:"serviceStatus"`
	NetworkReachable bool      `json:"networkReachable"`
	CPUUsage         float64   `json:"cpuUsage"`
	MemoryUsage      float64   `json:"memoryUsage"`
	RecentLogs       []string  `json:"recentLogs"`
	Heartbeat        bool      `json:"heartbeat"`
	LastSeenAt       time.Time `json:"lastSeenAt"`
}

// DeviceStore is a thread-safe in-memory map of device ID to latest DeviceState.
type DeviceStore struct {
	mu      sync.RWMutex
	devices map[string]*DeviceState
}

// NewDeviceStore returns an initialised, empty DeviceStore.
func NewDeviceStore() *DeviceStore {
	return &DeviceStore{
		devices: make(map[string]*DeviceState),
	}
}

// Upsert writes or replaces the stored state for the device identified by
// state.DeviceID and stamps LastSeenAt with the current UTC time.
func (s *DeviceStore) Upsert(state DeviceState) {
	state.LastSeenAt = time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devices[state.DeviceID] = &state
}

// Get returns a copy of the latest state for the given device ID.
// The second return value is false when no state has been recorded yet.
// A copy is returned (not the internal pointer) to prevent data races after
// the lock is released.
func (s *DeviceStore) Get(deviceID string) (DeviceState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.devices[deviceID]
	if !ok {
		return DeviceState{}, false
	}
	return *p, true
}

// List returns a stable copy of all device states sorted ascending by DeviceID.
func (s *DeviceStore) List() []DeviceState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]DeviceState, 0, len(s.devices))
	for _, p := range s.devices {
		result = append(result, *p)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].DeviceID < result[j].DeviceID
	})
	return result
}
