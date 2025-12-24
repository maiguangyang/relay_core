/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Instance management for Election instances.
 * Uses sync.Map for thread-safe access from multiple goroutines.
 */
package main

import (
	"sync"

	"github.com/maiguangyang/relay_core/pkg/election"
)

var (
	// Election instances: roomID -> *election.Elector
	electors sync.Map
)

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

// cleanupAllElectors closes all electors
func cleanupAllElectors() {
	electors.Range(func(key, value interface{}) bool {
		if e, ok := value.(*election.Elector); ok {
			e.Close()
		}
		electors.Delete(key)
		return true
	})
}
