package system

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// SubscriptionID is a unique identifier for an event subscription.
type SubscriptionID string

// EventHandler is the function called when an event is delivered.
type EventHandler func(event Event)

// Event represents a published event.
type Event struct {
	Topic     string      // Dot-separated topic name
	Data      interface{} // Event payload
	Timestamp time.Time   // Time the event was emitted
	Source    string      // ID of the emitting agent
}

// EventEmitter defines the pub/sub interface for internal events.
type EventEmitter interface {
	Emit(topic string, data interface{}) error
	Subscribe(topic string, handler EventHandler) (SubscriptionID, error)
	SubscribePattern(pattern string, handler EventHandler) (SubscriptionID, error)
	Unsubscribe(id SubscriptionID) error
	Topics() []string
	SubscriberCount(topic string) int
}

// eventSubscription holds a single subscription's metadata.
type eventSubscription struct {
	id      SubscriptionID
	topic   string // exact topic or pattern
	pattern bool   // true if this is a pattern subscription
	handler EventHandler
	ch      chan Event
}

// eventStats holds atomic counters for event agent statistics.
type eventStats struct {
	eventsEmitted       atomic.Int64
	eventsDelivered     atomic.Int64
	eventsDropped       atomic.Int64
	activeSubscriptions atomic.Int64
	activeTopics        atomic.Int64
}

// EventAgentImpl is the system event pub/sub agent.
// It embeds *agent.BaseAgent and implements agent.SystemAgent.
type EventAgentImpl struct {
	*agent.BaseAgent
	bufferSize int

	mu            sync.RWMutex
	subscribers   map[string][]*eventSubscription    // topic -> subscriptions (exact)
	patterns      []*eventSubscription               // pattern subscriptions
	subsIndex     map[SubscriptionID]*eventSubscription // fast lookup by ID
	closed        bool
	eventStats    eventStats
	nextSubID     atomic.Int64
	wg            sync.WaitGroup
}

// Compile-time interface check.
var _ agent.SystemAgent = (*EventAgentImpl)(nil)
var _ EventEmitter = (*EventAgentImpl)(nil)

// defaultEventBufferSize is the default channel buffer size per subscriber.
const defaultEventBufferSize = 256

// NewEventAgent creates a new EventAgentImpl with an optional buffer size.
// If no buffer size is provided, defaults to 256.
func NewEventAgent(bufferSize ...int) *EventAgentImpl {
	bs := defaultEventBufferSize
	if len(bufferSize) > 0 && bufferSize[0] > 0 {
		bs = bufferSize[0]
	}

	return &EventAgentImpl{
		BaseAgent:  agent.NewBaseAgent(),
		bufferSize: bs,
	}
}

// IsSystem returns true because EventAgent is a system agent.
func (e *EventAgentImpl) IsSystem() bool { return true }

// RequiresTransport returns false because EventAgent uses in-process channels.
func (e *EventAgentImpl) RequiresTransport() bool { return false }

// Init initializes the event agent by calling BaseAgent.Init and setting up
// internal data structures.
func (e *EventAgentImpl) Init(config agent.AgentConfig) error {
	if err := e.BaseAgent.Init(config); err != nil {
		return err
	}

	e.mu.Lock()
	e.subscribers = make(map[string][]*eventSubscription)
	e.patterns = make([]*eventSubscription, 0)
	e.subsIndex = make(map[SubscriptionID]*eventSubscription)
	e.closed = false
	e.mu.Unlock()

	return nil
}

// Stop shuts down the event agent, cleans up all subscribers and waits for
// in-flight deliveries to complete.
func (e *EventAgentImpl) Stop(ctx context.Context) error {
	e.mu.Lock()
	e.closed = true
	// Close all subscription channels to stop delivery goroutines
	for _, sub := range e.subsIndex {
		close(sub.ch)
	}
	e.subscribers = make(map[string][]*eventSubscription)
	e.patterns = nil
	e.subsIndex = make(map[SubscriptionID]*eventSubscription)
	e.eventStats.activeSubscriptions.Store(0)
	e.eventStats.activeTopics.Store(0)
	e.mu.Unlock()

	// Wait for all in-flight handler goroutines
	e.wg.Wait()

	return e.BaseAgent.Stop(ctx)
}

// Emit publishes an event to all matching subscribers.
func (e *EventAgentImpl) Emit(topic string, data interface{}) error {
	if topic == "" {
		return ErrTopicEmpty
	}

	e.mu.RLock()
	if e.closed {
		e.mu.RUnlock()
		return ErrEventAgentClosed
	}

	evt := Event{
		Topic:     topic,
		Data:      data,
		Timestamp: time.Now(),
		Source:    e.ID(),
	}

	// Collect exact subscribers
	subs := make([]*eventSubscription, 0, len(e.subscribers[topic]))
	subs = append(subs, e.subscribers[topic]...)

	// Collect pattern subscribers
	for _, ps := range e.patterns {
		if matchPattern(ps.topic, topic) {
			subs = append(subs, ps)
		}
	}
	e.mu.RUnlock()

	e.eventStats.eventsEmitted.Add(1)

	// Deliver to each subscriber
	for _, sub := range subs {
		select {
		case sub.ch <- evt:
			// delivered to channel
		default:
			// buffer full, drop event
			e.eventStats.eventsDropped.Add(1)
		}
	}

	return nil
}

// Subscribe registers a handler for an exact topic.
func (e *EventAgentImpl) Subscribe(topic string, handler EventHandler) (SubscriptionID, error) {
	if topic == "" {
		return "", ErrTopicEmpty
	}
	if handler == nil {
		return "", ErrHandlerNil
	}

	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return "", ErrEventAgentClosed
	}

	id := e.generateSubID()
	ch := make(chan Event, e.bufferSize)
	sub := &eventSubscription{
		id:      id,
		topic:   topic,
		pattern: false,
		handler: handler,
		ch:      ch,
	}

	isNewTopic := len(e.subscribers[topic]) == 0
	e.subscribers[topic] = append(e.subscribers[topic], sub)
	e.subsIndex[id] = sub
	e.eventStats.activeSubscriptions.Add(1)
	if isNewTopic {
		e.eventStats.activeTopics.Add(1)
	}
	e.mu.Unlock()

	// Start delivery goroutine
	e.wg.Add(1)
	go e.deliverLoop(sub)

	return id, nil
}

// SubscribePattern registers a handler for topics matching a pattern.
// Patterns use dots as segment separators:
//   - "*" matches exactly one segment
//   - "**" matches one or more segments
func (e *EventAgentImpl) SubscribePattern(pattern string, handler EventHandler) (SubscriptionID, error) {
	if pattern == "" {
		return "", ErrInvalidPattern
	}
	if handler == nil {
		return "", ErrHandlerNil
	}

	// Validate pattern has at least one segment
	if !isValidPattern(pattern) {
		return "", ErrInvalidPattern
	}

	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return "", ErrEventAgentClosed
	}

	id := e.generateSubID()
	ch := make(chan Event, e.bufferSize)
	sub := &eventSubscription{
		id:      id,
		topic:   pattern,
		pattern: true,
		handler: handler,
		ch:      ch,
	}

	e.patterns = append(e.patterns, sub)
	e.subsIndex[id] = sub
	e.eventStats.activeSubscriptions.Add(1)
	e.mu.Unlock()

	// Start delivery goroutine
	e.wg.Add(1)
	go e.deliverLoop(sub)

	return id, nil
}

// Unsubscribe removes a subscription by its ID.
func (e *EventAgentImpl) Unsubscribe(id SubscriptionID) error {
	e.mu.Lock()
	sub, ok := e.subsIndex[id]
	if !ok {
		e.mu.Unlock()
		return ErrEventSubNotFound
	}

	delete(e.subsIndex, id)
	e.eventStats.activeSubscriptions.Add(-1)

	if sub.pattern {
		// Remove from patterns slice
		for i, ps := range e.patterns {
			if ps.id == id {
				e.patterns = append(e.patterns[:i], e.patterns[i+1:]...)
				break
			}
		}
	} else {
		// Remove from topic subscribers
		subs := e.subscribers[sub.topic]
		for i, s := range subs {
			if s.id == id {
				e.subscribers[sub.topic] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		if len(e.subscribers[sub.topic]) == 0 {
			delete(e.subscribers, sub.topic)
			e.eventStats.activeTopics.Add(-1)
		}
	}

	// Close channel to stop delivery goroutine
	close(sub.ch)
	e.mu.Unlock()

	return nil
}

// Topics returns a list of all topics with active exact subscriptions.
func (e *EventAgentImpl) Topics() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	topics := make([]string, 0, len(e.subscribers))
	for t := range e.subscribers {
		topics = append(topics, t)
	}
	return topics
}

// SubscriberCount returns the number of exact subscribers for a given topic.
func (e *EventAgentImpl) SubscriberCount(topic string) int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.subscribers[topic])
}

// deliverLoop runs in a goroutine, reading events from the subscription
// channel and calling the handler asynchronously.
func (e *EventAgentImpl) deliverLoop(sub *eventSubscription) {
	defer e.wg.Done()
	for evt := range sub.ch {
		e.eventStats.eventsDelivered.Add(1)
		sub.handler(evt)
	}
}

// generateSubID creates a unique subscription ID.
func (e *EventAgentImpl) generateSubID() SubscriptionID {
	n := e.nextSubID.Add(1)
	return SubscriptionID(strings.Join([]string{"sub", itoa(n)}, "-"))
}

// itoa converts an int64 to string without importing strconv.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// matchPattern checks if a topic matches a dot-separated pattern.
// "*" matches exactly one segment, "**" matches one or more segments.
func matchPattern(pattern, topic string) bool {
	patParts := strings.Split(pattern, ".")
	topParts := strings.Split(topic, ".")
	return matchParts(patParts, topParts)
}

// matchParts recursively matches pattern parts against topic parts.
func matchParts(patParts, topParts []string) bool {
	pi, ti := 0, 0
	for pi < len(patParts) && ti < len(topParts) {
		switch patParts[pi] {
		case "**":
			// "**" must match at least one segment
			if pi == len(patParts)-1 {
				// Last pattern part, matches all remaining segments
				return true
			}
			// Try matching remaining pattern from each position
			for k := ti + 1; k <= len(topParts); k++ {
				if matchParts(patParts[pi+1:], topParts[k:]) {
					return true
				}
			}
			return false
		case "*":
			// Match exactly one segment
			pi++
			ti++
		default:
			if patParts[pi] != topParts[ti] {
				return false
			}
			pi++
			ti++
		}
	}
	return pi == len(patParts) && ti == len(topParts)
}

// isValidPattern checks if a pattern string is structurally valid.
func isValidPattern(pattern string) bool {
	parts := strings.Split(pattern, ".")
	if len(parts) == 0 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
	}
	return true
}

