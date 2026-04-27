package tsdb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscriber_Subscribe_Unsubscribe(t *testing.T) {
	sub := NewSubscriber()
	ch := make(chan DataPoint, 10)

	unsub := sub.Subscribe([]string{"cpu,host=a"}, ch)
	assert.Equal(t, 1, sub.SubscriberCount("cpu,host=a"))

	// 알림을 보내고 수신 확인
	dp := DataPoint{Timestamp: time.Now(), Fields: map[string]any{"value": 42.0}}
	sub.Notify("cpu,host=a", dp)

	select {
	case got := <-ch:
		assert.Equal(t, dp.Fields["value"], got.Fields["value"])
	case <-time.After(time.Second):
		t.Fatal("알림을 받지 못함")
	}

	// 구독 해제
	unsub()
	assert.Equal(t, 0, sub.SubscriberCount("cpu,host=a"))

	// 구독 해제 후에는 알림이 오지 않아야 함
	sub.Notify("cpu,host=a", dp)
	select {
	case <-ch:
		t.Fatal("구독 해제 후 알림을 받으면 안 됨")
	case <-time.After(50 * time.Millisecond):
		// 정상
	}
}

func TestSubscriber_MultipleKeys(t *testing.T) {
	sub := NewSubscriber()
	ch := make(chan DataPoint, 10)

	unsub := sub.Subscribe([]string{"cpu,host=a", "mem,host=a"}, ch)
	defer unsub()

	assert.Equal(t, 1, sub.SubscriberCount("cpu,host=a"))
	assert.Equal(t, 1, sub.SubscriberCount("mem,host=a"))

	// 각 시리즈에 대한 알림 확인
	dpCPU := DataPoint{Timestamp: time.Now(), Fields: map[string]any{"usage": 80.0}}
	dpMem := DataPoint{Timestamp: time.Now(), Fields: map[string]any{"free": 1024.0}}

	sub.Notify("cpu,host=a", dpCPU)
	sub.Notify("mem,host=a", dpMem)

	got1 := <-ch
	got2 := <-ch
	assert.Contains(t, []any{dpCPU.Fields["usage"], dpMem.Fields["free"]}, got1.Fields[firstKey(got1.Fields)])
	assert.Contains(t, []any{dpCPU.Fields["usage"], dpMem.Fields["free"]}, got2.Fields[firstKey(got2.Fields)])
}

func TestSubscriber_NonBlocking(t *testing.T) {
	sub := NewSubscriber()
	// 버퍼 크기 1인 채널 - 2번째 알림은 드롭되어야 함
	ch := make(chan DataPoint, 1)

	unsub := sub.Subscribe([]string{"cpu"}, ch)
	defer unsub()

	dp := DataPoint{Timestamp: time.Now(), Fields: map[string]any{"value": 1.0}}

	// 채널을 가득 채운다
	sub.Notify("cpu", dp)

	// 두 번째 알림은 블로킹 없이 드롭되어야 함
	done := make(chan struct{})
	go func() {
		sub.Notify("cpu", dp)
		close(done)
	}()

	select {
	case <-done:
		// 정상: 비차단으로 완료됨
	case <-time.After(time.Second):
		t.Fatal("Notify가 블로킹됨")
	}
}

func TestSubscriber_Close(t *testing.T) {
	sub := NewSubscriber()
	ch1 := make(chan DataPoint, 10)
	ch2 := make(chan DataPoint, 10)

	sub.Subscribe([]string{"cpu"}, ch1)
	sub.Subscribe([]string{"cpu", "mem"}, ch2)

	assert.Equal(t, 2, sub.SubscriberCount("cpu"))
	assert.Equal(t, 1, sub.SubscriberCount("mem"))

	sub.Close()

	assert.Equal(t, 0, sub.SubscriberCount("cpu"))
	assert.Equal(t, 0, sub.SubscriberCount("mem"))

	// 채널이 닫혔는지 확인
	_, ok := <-ch1
	assert.False(t, ok, "ch1이 닫혀야 함")
	_, ok = <-ch2
	assert.False(t, ok, "ch2가 닫혀야 함")
}

func TestTSDB_Subscribe_Integration(t *testing.T) {
	cfg := Config{
		MaxSeries:        100,
		MaxQueryPoints:   1000,
		QueryTimeout:     5 * time.Second,
		EvictionInterval: time.Minute,
	}
	db := New(cfg)
	defer db.Close()

	ch := make(chan DataPoint, 10)
	unsub := db.Subscribe([]string{"temp,room=living"}, ch)
	defer unsub()

	// Write 가 구독자에게 알림을 보내는지 확인
	err := db.Write("temp", map[string]string{"room": "living"}, map[string]any{"value": 22.5})
	require.NoError(t, err)

	select {
	case got := <-ch:
		assert.Equal(t, 22.5, got.Fields["value"])
	case <-time.After(time.Second):
		t.Fatal("Write 후 구독자에게 알림이 오지 않음")
	}

	// WriteBatch 도 알림을 보내는지 확인
	err = db.WriteBatch([]WriteRequest{
		{
			Measurement: "temp",
			Tags:        map[string]string{"room": "living"},
			Fields:      map[string]any{"value": 23.0},
		},
	})
	require.NoError(t, err)

	select {
	case got := <-ch:
		assert.Equal(t, 23.0, got.Fields["value"])
	case <-time.After(time.Second):
		t.Fatal("WriteBatch 후 구독자에게 알림이 오지 않음")
	}
}

// firstKey 는 맵에서 첫 번째 키를 반환한다 (테스트 헬퍼).
func firstKey(m map[string]any) string {
	for k := range m {
		return k
	}
	return ""
}
