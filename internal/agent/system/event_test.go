package system

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

// newTestEventAgent creates an initialized EventAgentImpl for testing.
func newTestEventAgent(t *testing.T) *EventAgentImpl {
	t.Helper()
	ea := NewEventAgent()
	cfg := agent.AgentConfig{
		ID:   "event-test",
		Name: "event-test-agent",
		Type: string(agent.SystemAgentEvent),
	}
	require.NoError(t, ea.Init(cfg))
	t.Cleanup(func() {
		_ = ea.Stop(context.Background())
	})
	return ea
}

// ---------------------------------------------------------------------------
// Construction tests
// ---------------------------------------------------------------------------

func TestEventAgent_NewEventAgent_DefaultBuffer(t *testing.T) {
	ea := NewEventAgent()
	assert.NotNil(t, ea)
	assert.Equal(t, lifecycle.StateCreated, ea.CurrentState())
}

func TestEventAgent_NewEventAgent_CustomBuffer(t *testing.T) {
	ea := NewEventAgent(512)
	assert.NotNil(t, ea)
}

// ---------------------------------------------------------------------------
// SystemAgent interface compliance
// ---------------------------------------------------------------------------

func TestEventAgent_IsSystem(t *testing.T) {
	ea := NewEventAgent()
	assert.True(t, ea.IsSystem())
}

func TestEventAgent_RequiresTransport(t *testing.T) {
	ea := NewEventAgent()
	assert.False(t, ea.RequiresTransport())
}

// ---------------------------------------------------------------------------
// Lifecycle tests
// ---------------------------------------------------------------------------

func TestEventAgent_Init_TransitionsToRunning(t *testing.T) {
	ea := NewEventAgent()
	cfg := agent.AgentConfig{
		ID:   "event-init",
		Name: "event-init-agent",
		Type: string(agent.SystemAgentEvent),
	}
	err := ea.Init(cfg)
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, ea.CurrentState())
	_ = ea.Stop(context.Background())
}

func TestEventAgent_Init_InvalidConfig(t *testing.T) {
	ea := NewEventAgent()
	cfg := agent.AgentConfig{} // missing ID and Name
	err := ea.Init(cfg)
	assert.Error(t, err)
}

func TestEventAgent_Stop_TransitionsToStopped(t *testing.T) {
	ea := NewEventAgent()
	cfg := agent.AgentConfig{
		ID:   "event-stop",
		Name: "event-stop-agent",
		Type: string(agent.SystemAgentEvent),
	}
	require.NoError(t, ea.Init(cfg))

	err := ea.Stop(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopped, ea.CurrentState())
}

// ---------------------------------------------------------------------------
// Emit and Subscribe tests
// ---------------------------------------------------------------------------

func TestEventAgent_Emit_EmptyTopic(t *testing.T) {
	ea := newTestEventAgent(t)
	err := ea.Emit("", "data")
	assert.ErrorIs(t, err, ErrTopicEmpty)
}

func TestEventAgent_Emit_Closed(t *testing.T) {
	ea := NewEventAgent()
	cfg := agent.AgentConfig{
		ID:   "event-emit-closed",
		Name: "event-emit-closed-agent",
		Type: string(agent.SystemAgentEvent),
	}
	require.NoError(t, ea.Init(cfg))
	require.NoError(t, ea.Stop(context.Background()))

	err := ea.Emit("topic", "data")
	assert.ErrorIs(t, err, ErrEventAgentClosed)
}

func TestEventAgent_Subscribe_NilHandler(t *testing.T) {
	ea := newTestEventAgent(t)
	_, err := ea.Subscribe("topic", nil)
	assert.ErrorIs(t, err, ErrHandlerNil)
}

func TestEventAgent_Subscribe_EmptyTopic(t *testing.T) {
	ea := newTestEventAgent(t)
	handler := func(_ Event) {}
	_, err := ea.Subscribe("", handler)
	assert.ErrorIs(t, err, ErrTopicEmpty)
}

func TestEventAgent_Subscribe_Closed(t *testing.T) {
	ea := NewEventAgent()
	cfg := agent.AgentConfig{
		ID:   "event-sub-closed",
		Name: "event-sub-closed-agent",
		Type: string(agent.SystemAgentEvent),
	}
	require.NoError(t, ea.Init(cfg))
	require.NoError(t, ea.Stop(context.Background()))

	_, err := ea.Subscribe("topic", func(_ Event) {})
	assert.ErrorIs(t, err, ErrEventAgentClosed)
}

func TestEventAgent_EmitAndReceive(t *testing.T) {
	ea := newTestEventAgent(t)

	var received atomic.Value
	done := make(chan struct{})

	_, err := ea.Subscribe("test.topic", func(evt Event) {
		received.Store(evt)
		close(done)
	})
	require.NoError(t, err)

	err = ea.Emit("test.topic", "hello")
	require.NoError(t, err)

	select {
	case <-done:
		evt := received.Load().(Event)
		assert.Equal(t, "test.topic", evt.Topic)
		assert.Equal(t, "hello", evt.Data)
		assert.NotZero(t, evt.Timestamp)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestEventAgent_EmitToMultipleSubscribers(t *testing.T) {
	ea := newTestEventAgent(t)

	var count atomic.Int32
	done := make(chan struct{})
	const numSubs = 3

	for i := 0; i < numSubs; i++ {
		_, err := ea.Subscribe("multi.topic", func(_ Event) {
			if count.Add(1) == int32(numSubs) {
				close(done)
			}
		})
		require.NoError(t, err)
	}

	err := ea.Emit("multi.topic", "broadcast")
	require.NoError(t, err)

	select {
	case <-done:
		assert.Equal(t, int32(numSubs), count.Load())
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for all subscribers")
	}
}

func TestEventAgent_EmitNoSubscribers(t *testing.T) {
	ea := newTestEventAgent(t)
	err := ea.Emit("no.listeners", "data")
	assert.NoError(t, err)
}

func TestEventAgent_EmitDifferentTopic(t *testing.T) {
	ea := newTestEventAgent(t)

	called := make(chan struct{}, 1)
	_, err := ea.Subscribe("topic.a", func(_ Event) {
		called <- struct{}{}
	})
	require.NoError(t, err)

	err = ea.Emit("topic.b", "data")
	require.NoError(t, err)

	select {
	case <-called:
		t.Fatal("handler should not be called for different topic")
	case <-time.After(200 * time.Millisecond):
		// expected: no call
	}
}

// ---------------------------------------------------------------------------
// Unsubscribe tests
// ---------------------------------------------------------------------------

func TestEventAgent_Unsubscribe(t *testing.T) {
	ea := newTestEventAgent(t)

	var callCount atomic.Int32
	subID, err := ea.Subscribe("unsub.topic", func(_ Event) {
		callCount.Add(1)
	})
	require.NoError(t, err)

	// Emit first event, should be received
	err = ea.Emit("unsub.topic", "first")
	require.NoError(t, err)
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, int32(1), callCount.Load())

	// Unsubscribe
	err = ea.Unsubscribe(subID)
	require.NoError(t, err)

	// Emit second event, should NOT be received
	err = ea.Emit("unsub.topic", "second")
	require.NoError(t, err)
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, int32(1), callCount.Load())
}

func TestEventAgent_Unsubscribe_NotFound(t *testing.T) {
	ea := newTestEventAgent(t)
	err := ea.Unsubscribe(SubscriptionID("nonexistent"))
	assert.ErrorIs(t, err, ErrEventSubNotFound)
}

// ---------------------------------------------------------------------------
// Pattern subscription tests
// ---------------------------------------------------------------------------

func TestEventAgent_SubscribePattern_SingleWildcard(t *testing.T) {
	ea := newTestEventAgent(t)

	var received atomic.Value
	done := make(chan struct{})

	_, err := ea.SubscribePattern("flow.*", func(evt Event) {
		received.Store(evt)
		close(done)
	})
	require.NoError(t, err)

	err = ea.Emit("flow.started", "data")
	require.NoError(t, err)

	select {
	case <-done:
		evt := received.Load().(Event)
		assert.Equal(t, "flow.started", evt.Topic)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pattern event")
	}
}

func TestEventAgent_SubscribePattern_SingleWildcard_NoDeepMatch(t *testing.T) {
	ea := newTestEventAgent(t)

	called := make(chan struct{}, 1)
	_, err := ea.SubscribePattern("flow.*", func(_ Event) {
		called <- struct{}{}
	})
	require.NoError(t, err)

	// flow.* should NOT match flow.a.b (multi-segment)
	err = ea.Emit("flow.a.b", "data")
	require.NoError(t, err)

	select {
	case <-called:
		t.Fatal("single wildcard should not match multi-segment topic")
	case <-time.After(200 * time.Millisecond):
		// expected
	}
}

func TestEventAgent_SubscribePattern_DoubleWildcard(t *testing.T) {
	ea := newTestEventAgent(t)

	var events sync.Map
	var count atomic.Int32
	done := make(chan struct{})

	_, err := ea.SubscribePattern("system.**", func(evt Event) {
		events.Store(evt.Topic, true)
		if count.Add(1) == 3 {
			close(done)
		}
	})
	require.NoError(t, err)

	require.NoError(t, ea.Emit("system.a", "1"))
	require.NoError(t, ea.Emit("system.a.b", "2"))
	require.NoError(t, ea.Emit("system.a.b.c", "3"))

	select {
	case <-done:
		_, ok1 := events.Load("system.a")
		_, ok2 := events.Load("system.a.b")
		_, ok3 := events.Load("system.a.b.c")
		assert.True(t, ok1)
		assert.True(t, ok2)
		assert.True(t, ok3)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for double wildcard events")
	}
}

func TestEventAgent_SubscribePattern_InvalidPattern(t *testing.T) {
	ea := newTestEventAgent(t)
	_, err := ea.SubscribePattern("", func(_ Event) {})
	assert.ErrorIs(t, err, ErrInvalidPattern)
}

func TestEventAgent_SubscribePattern_NilHandler(t *testing.T) {
	ea := newTestEventAgent(t)
	_, err := ea.SubscribePattern("test.*", nil)
	assert.ErrorIs(t, err, ErrHandlerNil)
}

// ---------------------------------------------------------------------------
// Topics and SubscriberCount tests
// ---------------------------------------------------------------------------

func TestEventAgent_Topics(t *testing.T) {
	ea := newTestEventAgent(t)
	handler := func(_ Event) {}

	_, err := ea.Subscribe("topic.a", handler)
	require.NoError(t, err)
	_, err = ea.Subscribe("topic.b", handler)
	require.NoError(t, err)

	topics := ea.Topics()
	assert.Contains(t, topics, "topic.a")
	assert.Contains(t, topics, "topic.b")
}

func TestEventAgent_SubscriberCount(t *testing.T) {
	ea := newTestEventAgent(t)
	handler := func(_ Event) {}

	_, err := ea.Subscribe("count.topic", handler)
	require.NoError(t, err)
	_, err = ea.Subscribe("count.topic", handler)
	require.NoError(t, err)

	assert.Equal(t, 2, ea.SubscriberCount("count.topic"))
	assert.Equal(t, 0, ea.SubscriberCount("nonexistent"))
}

// ---------------------------------------------------------------------------
// Stats tests
// ---------------------------------------------------------------------------

func TestEventAgent_Stats_EventsEmitted(t *testing.T) {
	ea := newTestEventAgent(t)

	require.NoError(t, ea.Emit("stats.topic", "data1"))
	require.NoError(t, ea.Emit("stats.topic", "data2"))

	assert.Equal(t, int64(2), ea.eventStats.eventsEmitted.Load())
}

func TestEventAgent_Stats_EventsDelivered(t *testing.T) {
	ea := newTestEventAgent(t)

	done := make(chan struct{})
	_, err := ea.Subscribe("deliver.topic", func(_ Event) {
		close(done)
	})
	require.NoError(t, err)

	require.NoError(t, ea.Emit("deliver.topic", "data"))

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}

	// Give time for stats to update
	time.Sleep(50 * time.Millisecond)
	assert.GreaterOrEqual(t, ea.eventStats.eventsDelivered.Load(), int64(1))
}

func TestEventAgent_Stats_ActiveSubscriptions(t *testing.T) {
	ea := newTestEventAgent(t)
	handler := func(_ Event) {}

	sub1, err := ea.Subscribe("sub.topic", handler)
	require.NoError(t, err)
	assert.Equal(t, int64(1), ea.eventStats.activeSubscriptions.Load())

	_, err = ea.Subscribe("sub.topic2", handler)
	require.NoError(t, err)
	assert.Equal(t, int64(2), ea.eventStats.activeSubscriptions.Load())

	require.NoError(t, ea.Unsubscribe(sub1))
	assert.Equal(t, int64(1), ea.eventStats.activeSubscriptions.Load())
}

func TestEventAgent_Stats_ActiveTopics(t *testing.T) {
	ea := newTestEventAgent(t)
	handler := func(_ Event) {}

	sub1, err := ea.Subscribe("topicA", handler)
	require.NoError(t, err)
	assert.Equal(t, int64(1), ea.eventStats.activeTopics.Load())

	_, err = ea.Subscribe("topicB", handler)
	require.NoError(t, err)
	assert.Equal(t, int64(2), ea.eventStats.activeTopics.Load())

	// Unsubscribe the only subscriber of topicA
	require.NoError(t, ea.Unsubscribe(sub1))
	assert.Equal(t, int64(1), ea.eventStats.activeTopics.Load())
}

// ---------------------------------------------------------------------------
// Concurrency stress test
// ---------------------------------------------------------------------------

func TestEventAgent_Concurrent_EmitSubscribe(t *testing.T) {
	ea := newTestEventAgent(t)
	const goroutines = 20
	const ops = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			handler := func(_ Event) {}
			for j := 0; j < ops; j++ {
				topic := "concurrent.topic"
				if j%2 == 0 {
					_, _ = ea.Subscribe(topic, handler)
				} else {
					_ = ea.Emit(topic, id)
				}
			}
		}(i)
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// Event source field test
// ---------------------------------------------------------------------------

func TestEventAgent_Emit_EventSource(t *testing.T) {
	ea := newTestEventAgent(t)

	var received atomic.Value
	done := make(chan struct{})
	_, err := ea.Subscribe("source.test", func(evt Event) {
		received.Store(evt)
		close(done)
	})
	require.NoError(t, err)

	require.NoError(t, ea.Emit("source.test", "data"))

	select {
	case <-done:
		evt := received.Load().(Event)
		assert.Equal(t, "event-test", evt.Source) // agent ID
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
}

// ---------------------------------------------------------------------------
// Stop cleans up subscribers
// ---------------------------------------------------------------------------

func TestEventAgent_Stop_CleansSubscribers(t *testing.T) {
	ea := NewEventAgent()
	cfg := agent.AgentConfig{
		ID:   "event-cleanup",
		Name: "event-cleanup-agent",
		Type: string(agent.SystemAgentEvent),
	}
	require.NoError(t, ea.Init(cfg))

	_, err := ea.Subscribe("cleanup.topic", func(_ Event) {})
	require.NoError(t, err)
	assert.Equal(t, 1, ea.SubscriberCount("cleanup.topic"))

	require.NoError(t, ea.Stop(context.Background()))

	assert.Equal(t, 0, ea.SubscriberCount("cleanup.topic"))
	assert.Empty(t, ea.Topics())
}
