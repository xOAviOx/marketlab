package server

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"marketlab/internal/sim"
)

const (
	defaultMaxSessions = 64
	defaultSessionTTL  = 30 * time.Minute
	tickInterval       = 100 * time.Millisecond
	commandQueueSize   = 64
	requestCacheSize   = 256
)

var errSessionCapacity = errors.New("session capacity reached; try again shortly")

type ownerRequest struct {
	requestID string
	mutate    bool
	apply     func(*sim.Simulation) (*sim.Simulation, error)
	result    chan ownerResult
}

type ownerResult struct {
	snapshot sim.Snapshot
	err      error
}

type subscriptionRequest struct {
	add    bool
	queue  chan []byte
	result chan error
}

type cachedResult struct {
	requestID string
	result    ownerResult
}

// Session owns one simulation. The run goroutine is the only code that reads
// or mutates the simulation, including snapshots and exports.
type Session struct {
	id          string
	requests    chan ownerRequest
	subscribe   chan subscriptionRequest
	stop        chan struct{}
	done        chan struct{}
	stopOnce    sync.Once
	lastActive  atomic.Int64
	subscribers atomic.Int64
}

func newSession(id string, seed int64) (*Session, error) {
	state, err := sim.New(id, seed)
	if err != nil {
		return nil, err
	}
	s := &Session{
		id:        id,
		requests:  make(chan ownerRequest, commandQueueSize),
		subscribe: make(chan subscriptionRequest),
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
	s.touch()
	go s.run(state)
	return s, nil
}

func (s *Session) touch() {
	s.lastActive.Store(time.Now().UnixNano())
}

func (s *Session) close() {
	s.stopOnce.Do(func() { close(s.stop) })
}

func (s *Session) run(state *sim.Simulation) {
	defer close(s.done)
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	lastScheduledTick := time.Now()

	clients := make(map[chan []byte]struct{})
	sequence := uint64(1)
	accumulator := 0.0
	dirty := false
	cache := make(map[string]ownerResult)
	cacheOrder := make([]string, 0, requestCacheSize)

	view := func() sim.Snapshot {
		snapshot := state.Snapshot()
		snapshot.Seq = sequence
		return snapshot
	}
	publish := func() {
		if !dirty || len(clients) == 0 {
			dirty = false
			return
		}
		snapshot := view()
		payload, err := json.Marshal(serverMessage{Type: "snapshot", Snapshot: &snapshot})
		if err != nil {
			return
		}
		for queue := range clients {
			select {
			case queue <- payload:
			default:
				select {
				case <-queue:
				default:
				}
				select {
				case queue <- payload:
				default:
				}
			}
		}
		dirty = false
	}

	for {
		select {
		case request := <-s.requests:
			if request.requestID != "" {
				if result, ok := cache[request.requestID]; ok {
					request.result <- result
					continue
				}
			}
			next, err := request.apply(state)
			if next != nil && next != state {
				state = next
				accumulator = 0
			}
			if request.mutate {
				sequence++
				dirty = true
			}
			result := ownerResult{snapshot: view(), err: err}
			if request.requestID != "" {
				cache[request.requestID] = result
				cacheOrder = append(cacheOrder, request.requestID)
				if len(cacheOrder) > requestCacheSize {
					delete(cache, cacheOrder[0])
					cacheOrder = cacheOrder[1:]
				}
			}
			request.result <- result

		case request := <-s.subscribe:
			if request.add {
				clients[request.queue] = struct{}{}
				s.subscribers.Add(1)
				snapshot := view()
				payload, err := json.Marshal(serverMessage{Type: "snapshot", Snapshot: &snapshot})
				if err == nil {
					request.queue <- payload
				}
				request.result <- err
				continue
			}
			if _, ok := clients[request.queue]; ok {
				delete(clients, request.queue)
				s.subscribers.Add(-1)
			}
			request.result <- nil

		case scheduledAt := <-ticker.C:
			periods := int(scheduledAt.Sub(lastScheduledTick) / tickInterval)
			if periods < 1 {
				periods = 1
			}
			lastScheduledTick = lastScheduledTick.Add(time.Duration(periods) * tickInterval)
			if state.Running {
				for range periods {
					accumulator += state.Speed
					for accumulator >= 1 && state.Running {
						state.Step()
						sequence++
						accumulator--
						dirty = true
					}
				}
			}
			publish()

		case <-s.stop:
			for queue := range clients {
				close(queue)
			}
			return
		}
	}
}

func (s *Session) do(ctx context.Context, requestID string, mutate bool, apply func(*sim.Simulation) (*sim.Simulation, error)) (sim.Snapshot, error) {
	result := make(chan ownerResult, 1)
	request := ownerRequest{requestID: requestID, mutate: mutate, apply: apply, result: result}
	select {
	case s.requests <- request:
	case <-s.done:
		return sim.Snapshot{}, errors.New("session expired")
	case <-ctx.Done():
		return sim.Snapshot{}, ctx.Err()
	}
	select {
	case response := <-result:
		s.touch()
		return response.snapshot, response.err
	case <-s.done:
		return sim.Snapshot{}, errors.New("session expired")
	case <-ctx.Done():
		return sim.Snapshot{}, ctx.Err()
	}
}

func (s *Session) addSubscriber(ctx context.Context, queue chan []byte) error {
	result := make(chan error, 1)
	select {
	case s.subscribe <- subscriptionRequest{add: true, queue: queue, result: result}:
	case <-s.done:
		return errors.New("session expired")
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-result:
		s.touch()
		return err
	case <-s.done:
		return errors.New("session expired")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Session) removeSubscriber(queue chan []byte) {
	result := make(chan error, 1)
	select {
	case s.subscribe <- subscriptionRequest{queue: queue, result: result}:
		select {
		case <-result:
		case <-s.done:
		}
	case <-s.done:
	}
}

type manager struct {
	mu              sync.Mutex
	sessions        map[string]*Session
	maxSessions     int
	sessionTTL      time.Duration
	cleanupInterval time.Duration
	stop            chan struct{}
	done            chan struct{}
	closeOnce       sync.Once
}

func newManager(maxSessions int, ttl, cleanupInterval time.Duration) *manager {
	m := &manager{
		sessions:        make(map[string]*Session),
		maxSessions:     maxSessions,
		sessionTTL:      ttl,
		cleanupInterval: cleanupInterval,
		stop:            make(chan struct{}),
		done:            make(chan struct{}),
	}
	go m.cleanup()
	return m
}

func (m *manager) get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[id]
	return session, ok
}

func (m *manager) create() (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sessions) >= m.maxSessions {
		return nil, errSessionCapacity
	}
	id, seed, err := newIdentity()
	if err != nil {
		return nil, err
	}
	session, err := newSession(id, seed)
	if err != nil {
		return nil, err
	}
	m.sessions[id] = session
	return session, nil
}

func (m *manager) cleanup() {
	defer close(m.done)
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C:
			m.mu.Lock()
			for id, session := range m.sessions {
				inactive := now.Sub(time.Unix(0, session.lastActive.Load())) > m.sessionTTL
				if inactive && session.subscribers.Load() == 0 {
					delete(m.sessions, id)
					session.close()
				}
			}
			m.mu.Unlock()
		case <-m.stop:
			m.mu.Lock()
			for id, session := range m.sessions {
				delete(m.sessions, id)
				session.close()
			}
			m.mu.Unlock()
			return
		}
	}
}

func (m *manager) close() {
	m.closeOnce.Do(func() {
		close(m.stop)
		<-m.done
	})
}

func newIdentity() (string, int64, error) {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(buffer[:16]), int64(binary.LittleEndian.Uint64(buffer[16:])), nil
}
