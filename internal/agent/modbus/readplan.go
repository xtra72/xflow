package modbus

import (
	"sort"
	"time"
)

// ---------------------------------------------------------------------------
// SPEC-MODBUS-013 REQ-02/REQ-03 — 읽기 계획(블록 병합)
// ---------------------------------------------------------------------------
//
// 레지스터 그룹을 레지스터 단위로 잘게 쪼개면 그룹마다 개별 설명을 붙일 수 있지만,
// "그룹 = 물리 읽기 1회" 구조에서는 트랜잭션 수가 그대로 폭증한다.
// 읽기 계획은 이 둘을 분리한다: 병합은 **트랜스포트 계층에서만** 일어나고,
// 캐시 갱신·변경 감지·메시지 방출은 여전히 **그룹 단위**로 수행된다.
// 따라서 다운스트림(노드·대시보드·플로우) 계약은 전혀 바뀌지 않는다.

// DefaultMaxBlockRegisters 는 블록당 최대 레지스터 수의 기본값이다(REQ-03).
const DefaultMaxBlockRegisters uint16 = 32

// ReadBlock 은 물리 읽기 1회의 단위이다. Members 는 이 블록이 대표하는 원래
// 레지스터 그룹들이며, 읽기 결과는 멤버 경계로 다시 잘려 그룹 단위로 처리된다.
type ReadBlock struct {
	FunctionCode byte
	StartAddress uint16
	Quantity     uint16
	// PollInterval 은 이 블록이 속한 케이던스 코호트이다. 0 이면 에이전트 기본 주기.
	PollInterval time.Duration
	// Members 는 주소 오름차순으로 정렬된 원래 그룹들이다(최소 1개).
	Members []RegisterGroupConfig
}

// clampMaxBlock 은 블록 상한을 [1, MaxRegistersRead] 로 제한한다(REQ-03 / AC-13).
// 0(미지정)은 기본값으로 대체한다.
func clampMaxBlock(v uint16) uint16 {
	if v == 0 {
		return DefaultMaxBlockRegisters
	}
	if v > MaxRegistersRead {
		return MaxRegistersRead
	}
	return v
}

// effectiveMaxBlock 은 디바이스 오버라이드 ?? 에이전트 기본값을 해석한다(REQ-03 / AC-12).
func effectiveMaxBlock(dc DeviceConfig, cfg ModbusConfig) uint16 {
	if dc.MaxBlockRegisters != nil {
		return clampMaxBlock(*dc.MaxBlockRegisters)
	}
	return clampMaxBlock(cfg.MaxBlockRegisters)
}

// buildReadPlan 은 레지스터 그룹 목록을 물리 읽기 블록 목록으로 변환한다(순수 함수).
//
// 병합 규칙(모두 만족해야 병합):
//   - 같은 function code
//   - 같은 폴링 주기(코호트) — 주기가 다르면 애초에 다른 스케줄러가 읽는다
//   - 주소가 연속하거나 겹침 (간극 ≥ 1워드면 병합하지 않음)
//   - 병합 후 span 이 maxBlock 이내
//
// 간극을 허용하지 않는 이유: 정의되지 않은 주소를 읽으면 슬레이브가
// ILLEGAL DATA ADDRESS(02) 예외로 응답해 블록 전체가 실패한다. 미사용(enabled=false)
// 그룹도 계획에서 제외되므로 자연히 블록 경계를 나눈다(REQ-01 + REQ-02).
//
// 단일 그룹의 quantity 가 maxBlock 을 넘으면 분할하지 않고 단독 블록으로 통과시킨다
// (기존 동작 보존, AC-14).
func buildReadPlan(groups []RegisterGroupConfig, maxBlock uint16) []ReadBlock {
	maxBlock = clampMaxBlock(maxBlock)

	// 1) 사용 중이고 실제로 읽을 것이 있는 그룹만 남긴다.
	active := make([]RegisterGroupConfig, 0, len(groups))
	for _, g := range groups {
		if g.IsEnabled() && g.Quantity > 0 {
			active = append(active, g)
		}
	}
	if len(active) == 0 {
		return nil
	}

	// 2) 코호트 → 주소 순으로 안정 정렬한다. 결정적 출력이 보장되어야
	//    블록 스케줄러 키와 테스트 기대값이 안정적이다.
	sort.SliceStable(active, func(i, j int) bool {
		if active[i].PollInterval != active[j].PollInterval {
			return active[i].PollInterval < active[j].PollInterval
		}
		if active[i].FunctionCode != active[j].FunctionCode {
			return active[i].FunctionCode < active[j].FunctionCode
		}
		if active[i].StartAddress != active[j].StartAddress {
			return active[i].StartAddress < active[j].StartAddress
		}
		return active[i].Quantity < active[j].Quantity
	})

	// 3) 선형 스캔 병합.
	plan := make([]ReadBlock, 0, len(active))
	for _, g := range active {
		gStart := uint32(g.StartAddress)
		gEnd := gStart + uint32(g.Quantity) // exclusive

		if len(plan) > 0 {
			cur := &plan[len(plan)-1] // append 전에만 사용 — 재할당 무효화 없음
			curStart := uint32(cur.StartAddress)
			curEnd := curStart + uint32(cur.Quantity)
			newEnd := curEnd
			if gEnd > newEnd {
				newEnd = gEnd
			}
			if cur.FunctionCode == g.FunctionCode &&
				cur.PollInterval == g.PollInterval &&
				gStart <= curEnd && // 연속(==) 또는 겹침(<)
				newEnd-curStart <= uint32(maxBlock) {
				cur.Quantity = uint16(newEnd - curStart)
				cur.Members = append(cur.Members, g)
				continue
			}
		}

		plan = append(plan, ReadBlock{
			FunctionCode: g.FunctionCode,
			StartAddress: g.StartAddress,
			Quantity:     g.Quantity,
			PollInterval: g.PollInterval,
			Members:      []RegisterGroupConfig{g},
		})
	}
	return plan
}

// sliceMemberData 는 블록 읽기 결과에서 멤버 그룹에 해당하는 구간을 잘라낸다(REQ-02 / AC-08).
//
// fc3/fc4(레지스터)는 레지스터당 2바이트이므로 바이트 오프셋이 `(멤버주소-블록주소)*2` 이다.
// fc1/fc2(코일·이산입력)는 비트 패킹이라 바이트 경계에 맞지 않을 수 있으므로,
// 블록 비트열에서 멤버 구간 비트를 뽑아 새 바이트열로 재포장한다.
// 비트 순서는 decodeCoils 와 동일하게 각 바이트의 LSB 가 앞선 주소이다.
//
// 반환 ok=false 는 데이터가 기대 길이보다 짧다는 뜻이며, 호출자는 해당 멤버를 건너뛴다.
func sliceMemberData(block ReadBlock, data []byte, m RegisterGroupConfig) ([]byte, bool) {
	if m.StartAddress < block.StartAddress || m.Quantity == 0 {
		return nil, false
	}
	offset := int(m.StartAddress) - int(block.StartAddress)
	count := int(m.Quantity)

	switch block.FunctionCode {
	case FC01ReadCoils, FC02ReadDiscreteInputs:
		if (offset+count+7)/8 > len(data) {
			return nil, false
		}
		out := make([]byte, (count+7)/8)
		for i := 0; i < count; i++ {
			src := offset + i
			if data[src/8]&(1<<uint(src%8)) != 0 {
				out[i/8] |= 1 << uint(i%8)
			}
		}
		return out, true
	default:
		start := offset * 2
		end := start + count*2
		if end > len(data) {
			return nil, false
		}
		return data[start:end], true
	}
}
