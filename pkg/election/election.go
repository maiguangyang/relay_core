/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 */
package election

import (
	"sort"
	"sync"
	"time"
)

// Candidate represents a node that can become a proxy
type Candidate struct {
	PeerID     string
	Score      float64 // Higher is better
	Bandwidth  int64   // Available bandwidth in bytes/sec
	Latency    int64   // Average latency in ms
	PacketLoss float64 // Packet loss ratio (0-1)
	IsProxy    bool    // Currently acting as proxy
	LastUpdate time.Time
}

// ElectionResult represents the result of a proxy election
type ElectionResult struct {
	ProxyID   string
	Score     float64
	Reason    string
	Timestamp time.Time
}

// ElectionCallback is called when election completes
type ElectionCallback func(result ElectionResult)

// Elector manages proxy election for a room
type Elector struct {
	mu           sync.RWMutex
	roomID       string
	candidates   map[string]*Candidate
	currentProxy string

	// Configuration
	minCandidates    int           // Minimum candidates needed for election
	scoreThreshold   float64       // Minimum score to become proxy
	electionInterval time.Duration // How often to re-evaluate

	// Callbacks
	onElection ElectionCallback

	// State
	closed bool
	stopCh chan struct{}
}

// ElectorConfig holds election configuration
type ElectorConfig struct {
	MinCandidates    int
	ScoreThreshold   float64
	ElectionInterval time.Duration
}

// DefaultElectorConfig returns default election configuration
func DefaultElectorConfig() ElectorConfig {
	return ElectorConfig{
		MinCandidates:    2,
		ScoreThreshold:   0.5,
		ElectionInterval: 10 * time.Second,
	}
}

// NewElector creates a new proxy elector
func NewElector(roomID string, config ElectorConfig) *Elector {
	return &Elector{
		roomID:           roomID,
		candidates:       make(map[string]*Candidate),
		minCandidates:    config.MinCandidates,
		scoreThreshold:   config.ScoreThreshold,
		electionInterval: config.ElectionInterval,
		stopCh:           make(chan struct{}),
	}
}

// SetOnElection sets the election callback
func (e *Elector) SetOnElection(fn ElectionCallback) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onElection = fn
}

// UpdateCandidate updates or adds a candidate
func (e *Elector) UpdateCandidate(candidate Candidate) {
	e.mu.Lock()
	defer e.mu.Unlock()

	candidate.LastUpdate = time.Now()
	candidate.Score = e.calculateScore(&candidate)
	e.candidates[candidate.PeerID] = &candidate
}

// RemoveCandidate removes a candidate
func (e *Elector) RemoveCandidate(peerID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	delete(e.candidates, peerID)

	// If removed candidate was the proxy, trigger new election
	if e.currentProxy == peerID {
		e.currentProxy = ""
		e.triggerElection("proxy_left")
	}
}

// calculateScore calculates the candidate score based on network metrics
// Score formula: bandwidth_weight * bandwidth - latency_weight * latency - packetloss_weight * packetloss
func (e *Elector) calculateScore(c *Candidate) float64 {
	// Normalize values
	bandwidthScore := float64(c.Bandwidth) / 1000000.0 // Normalize to Mbps
	latencyScore := 1.0 - float64(c.Latency)/1000.0    // Lower latency is better
	if latencyScore < 0 {
		latencyScore = 0
	}
	packetLossScore := 1.0 - c.PacketLoss // Lower packet loss is better

	// Weighted combination
	score := 0.4*bandwidthScore + 0.3*latencyScore + 0.3*packetLossScore
	return score
}

// triggerElection runs the election algorithm
func (e *Elector) triggerElection(reason string) {
	if len(e.candidates) < e.minCandidates {
		return
	}

	// Find best candidate
	var bestCandidate *Candidate
	for _, c := range e.candidates {
		if c.Score >= e.scoreThreshold {
			if bestCandidate == nil || c.Score > bestCandidate.Score {
				bestCandidate = c
			}
		}
	}

	if bestCandidate == nil {
		return
	}

	// Only trigger callback if proxy changed
	if bestCandidate.PeerID != e.currentProxy {
		e.currentProxy = bestCandidate.PeerID

		result := ElectionResult{
			ProxyID:   bestCandidate.PeerID,
			Score:     bestCandidate.Score,
			Reason:    reason,
			Timestamp: time.Now(),
		}

		if e.onElection != nil {
			go e.onElection(result)
		}
	}
}

// Elect manually triggers an election
func (e *Elector) Elect() *ElectionResult {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.candidates) < e.minCandidates {
		return nil
	}

	// Sort candidates by score
	sorted := make([]*Candidate, 0, len(e.candidates))
	for _, c := range e.candidates {
		sorted = append(sorted, c)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Score > sorted[j].Score
	})

	if len(sorted) == 0 || sorted[0].Score < e.scoreThreshold {
		return nil
	}

	bestCandidate := sorted[0]
	e.currentProxy = bestCandidate.PeerID

	return &ElectionResult{
		ProxyID:   bestCandidate.PeerID,
		Score:     bestCandidate.Score,
		Reason:    "manual_election",
		Timestamp: time.Now(),
	}
}

// GetCurrentProxy returns the current proxy peer ID
func (e *Elector) GetCurrentProxy() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentProxy
}

// GetCandidates returns all candidates sorted by score
func (e *Elector) GetCandidates() []Candidate {
	e.mu.RLock()
	defer e.mu.RUnlock()

	candidates := make([]Candidate, 0, len(e.candidates))
	for _, c := range e.candidates {
		candidates = append(candidates, *c)
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	return candidates
}

// Start begins periodic election evaluation
func (e *Elector) Start() {
	go func() {
		ticker := time.NewTicker(e.electionInterval)
		defer ticker.Stop()

		for {
			select {
			case <-e.stopCh:
				return
			case <-ticker.C:
				e.mu.Lock()
				e.triggerElection("periodic")
				e.mu.Unlock()
			}
		}
	}()
}

// Close stops the elector
func (e *Elector) Close() {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	e.closed = true
	e.mu.Unlock()

	close(e.stopCh)
}
