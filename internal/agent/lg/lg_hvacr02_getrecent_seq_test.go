// lg_hvacr02_getrecent_seq_test.go — get_recent 가 last_seq 를 반환하여 노드의
// 델타 추적이 동작함을 검증(이미 보낸 프레임 재전송 방지).
//
// 증상: processGetRecent 가 last_seq 를 반환하지 않아 노드 lastSeq 가 0 에 머물고,
// 매 조회마다 ring 백로그가 통째로 재전송됐다("전송된 메시지를 다시 전송").
package lg

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type getRecentResp struct {
	Count   int               `json:"count"`
	Frames  []json.RawMessage `json:"frames"`
	LastSeq int64             `json:"last_seq"`
}

func callGetRecent(t *testing.T, a *Hvacr02Agent, lastSeq int64) getRecentResp {
	t.Helper()
	cmd, err := json.Marshal(map[string]any{
		"command":  "get_recent",
		"count":    32,
		"last_seq": lastSeq,
	})
	require.NoError(t, err)
	resp, err := a.Process(cmd)
	require.NoError(t, err)
	var out getRecentResp
	require.NoError(t, json.Unmarshal(resp, &out))
	return out
}

func TestProcessGetRecent_ReturnsLastSeq_NoReSend(t *testing.T) {
	a := newControllerMergeTestAgent(t)
	ts := time.Now()

	// 두 디바이스 상태 emit → ring 에 push (최초관측 report).
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("OFF")}, ts)
	a.updateDeviceState("44550065", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("OFF")}, ts.Add(time.Millisecond))

	// 1차 조회(lastSeq=0): 백로그 반환 + last_seq>0.
	r1 := callGetRecent(t, a, 0)
	require.Greater(t, len(r1.Frames), 0, "최초 조회는 백로그를 반환한다")
	require.Greater(t, r1.LastSeq, int64(0), "get_recent 는 last_seq 를 반환해야 한다")

	// 2차 조회(lastSeq=r1.LastSeq): 새 프레임 없음 → 재전송 없음.
	r2 := callGetRecent(t, a, r1.LastSeq)
	assert.Equal(t, 0, len(r2.Frames), "이미 보낸 프레임은 재전송되면 안 된다")
	assert.Equal(t, r1.LastSeq, r2.LastSeq, "새 프레임이 없으면 last_seq 는 유지(에코)된다")

	// 새 상태 변경 발생 → 1건만 새로 반환.
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("ON"), IndoorTempC: f64Ptr(24.0)}, ts.Add(time.Second))
	r3 := callGetRecent(t, a, r2.LastSeq)
	assert.Equal(t, 1, len(r3.Frames), "변경분 1건만 새로 반환되어야 한다")
	assert.Greater(t, r3.LastSeq, r2.LastSeq, "last_seq 가 전진해야 한다")
}
