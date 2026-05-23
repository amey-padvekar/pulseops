package store_test

import (
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/store"
)

func TestDeviceStore_UpsertAndGet(t *testing.T) {
	s := store.NewDeviceStore()

	_, ok := s.Get("LAPTOP-22")
	if ok {
		t.Fatal("expected empty store to return false for unknown device")
	}

	before := time.Now().UTC()
	s.Upsert(store.DeviceState{
		DeviceID:      "LAPTOP-22",
		ServiceName:   "OpenVPNService",
		ServiceStatus: "running",
		Heartbeat:     true,
	})

	got, ok := s.Get("LAPTOP-22")
	if !ok {
		t.Fatal("expected state to be found after upsert")
	}
	if got.DeviceID != "LAPTOP-22" {
		t.Errorf("DeviceID: got %q, want %q", got.DeviceID, "LAPTOP-22")
	}
	if got.ServiceStatus != "running" {
		t.Errorf("ServiceStatus: got %q, want %q", got.ServiceStatus, "running")
	}
	if got.LastSeenAt.Before(before) {
		t.Errorf("LastSeenAt %v is before the upsert call at %v", got.LastSeenAt, before)
	}
}

func TestDeviceStore_UpsertOverwrites(t *testing.T) {
	s := store.NewDeviceStore()

	s.Upsert(store.DeviceState{DeviceID: "LAPTOP-22", ServiceStatus: "running"})
	s.Upsert(store.DeviceState{DeviceID: "LAPTOP-22", ServiceStatus: "stopped"})

	got, _ := s.Get("LAPTOP-22")
	if got.ServiceStatus != "stopped" {
		t.Errorf("expected stopped after second upsert, got %q", got.ServiceStatus)
	}
}

func TestDeviceStore_GetReturnsCopy(t *testing.T) {
	s := store.NewDeviceStore()
	s.Upsert(store.DeviceState{DeviceID: "LAPTOP-22", ServiceStatus: "running"})

	got, _ := s.Get("LAPTOP-22")
	got.ServiceStatus = "mutated"

	// internal state must not be affected by mutating the returned copy
	original, _ := s.Get("LAPTOP-22")
	if original.ServiceStatus != "running" {
		t.Errorf("store was mutated via returned copy: got %q", original.ServiceStatus)
	}
}

func TestDeviceStore_ListSortedAndCopied(t *testing.T) {
	s := store.NewDeviceStore()
	s.Upsert(store.DeviceState{DeviceID: "ZEBRA-1", ServiceStatus: "running"})
	s.Upsert(store.DeviceState{DeviceID: "ALPHA-1", ServiceStatus: "stopped"})
	s.Upsert(store.DeviceState{DeviceID: "MANGO-1", ServiceStatus: "running"})

	list := s.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(list))
	}
	if list[0].DeviceID != "ALPHA-1" || list[1].DeviceID != "MANGO-1" || list[2].DeviceID != "ZEBRA-1" {
		t.Errorf("list is not sorted: %v", []string{list[0].DeviceID, list[1].DeviceID, list[2].DeviceID})
	}

	// mutating the list must not affect the store
	list[0].ServiceStatus = "mutated"
	fresh := s.List()
	if fresh[0].ServiceStatus != "stopped" {
		t.Errorf("store was mutated via List copy: got %q", fresh[0].ServiceStatus)
	}
}

func TestDeviceStore_ListEmpty(t *testing.T) {
	s := store.NewDeviceStore()
	list := s.List()
	if list == nil {
		t.Error("List() should return an empty slice, not nil")
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d entries", len(list))
	}
}
