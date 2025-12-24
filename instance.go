/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Instance management for SFU and Election instances.
 * Uses sync.Map for thread-safe access from multiple goroutines.
 */
package main

import (
	"sync"
	"sync/atomic"

	"github.com/maiguangyang/relay_core/pkg/election"
	"github.com/maiguangyang/relay_core/pkg/sfu"
)

var (
	// SFU instances: id -> *sfu.SFU
	sfuInstances sync.Map
	sfuIDCounter int64

	// Election instances: roomID -> *election.Elector
	electors sync.Map
)

// registerSFUInstance registers a new SFU instance and returns its ID
func registerSFUInstance(s *sfu.SFU) int64 {
	id := atomic.AddInt64(&sfuIDCounter, 1)
	sfuInstances.Store(id, s)
	return id
}

// getSFUInstance returns an SFU instance by ID
func getSFUInstance(id int64) *sfu.SFU {
	if v, ok := sfuInstances.Load(id); ok {
		return v.(*sfu.SFU)
	}
	return nil
}

// unregisterSFUInstance removes an SFU instance
func unregisterSFUInstance(id int64) {
	sfuInstances.Delete(id)
}

// registerElector registers an elector for a room
func registerElector(roomID string, e *election.Elector) {
	// Close existing elector if any
	if existing, ok := electors.Load(roomID); ok {
		existing.(*election.Elector).Close()
	}
	electors.Store(roomID, e)
}

// getElector returns an elector by room ID
func getElector(roomID string) *election.Elector {
	if v, ok := electors.Load(roomID); ok {
		return v.(*election.Elector)
	}
	return nil
}

// unregisterElector removes an elector
func unregisterElector(roomID string) {
	electors.Delete(roomID)
}

// cleanupAllInstances closes all SFU instances and electors
func cleanupAllInstances() {
	sfuInstances.Range(func(key, value interface{}) bool {
		if s, ok := value.(*sfu.SFU); ok {
			s.Close()
		}
		sfuInstances.Delete(key)
		return true
	})

	electors.Range(func(key, value interface{}) bool {
		if e, ok := value.(*election.Elector); ok {
			e.Close()
		}
		electors.Delete(key)
		return true
	})
}
