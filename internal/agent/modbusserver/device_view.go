package modbusserver

// ---------------------------------------------------------------------------
// deviceView — intra-server 공유 세그먼트 주소 변환 (B2)
// ---------------------------------------------------------------------------
//
// deviceView 는 한 서빙 디바이스의 영역별 세그먼트를 로컬(디바이스 자체 맵) 또는
// 공유(unit_id 0 컨테이너 맵, 주소 변환) 로 라우팅하는 registerStore 구현이다.
//
// - 로컬 세그먼트: target = 디바이스 자체 *RegisterMap, mapStart = devStart (변환 없음).
// - 공유 세그먼트: target = 컨테이너 *RegisterMap, mapStart = shared_address.
//   디바이스 주소 X → target 주소 shared_address + (X - devStart).
//
// 요청이 한 세그먼트에 완전히 포함되지 않으면(세그먼트 경계 횡단) illegal address 로
// 거부한다(조용히 분할하지 않는다). 공유 세그먼트가 하나도 없는 디바이스는 deviceView
// 를 쓰지 않고 자체 *RegisterMap 을 그대로 store 로 사용하므로 기존 동작과 동일하다.

// viewSegment 는 한 영역 내 세그먼트의 주소 변환 정보이다.
type viewSegment struct {
	devStart uint16       // 디바이스 주소 시작
	count    uint16       // 세그먼트 길이
	mapStart uint16       // target 맵에서의 시작 주소 (로컬은 devStart, 공유는 shared_address)
	target   *RegisterMap // 서빙 대상 맵 (자체 또는 컨테이너)
}

// deviceView 는 영역별 세그먼트 라우팅을 수행하는 registerStore 이다.
type deviceView struct {
	coils          []viewSegment
	discreteInputs []viewSegment
	holding        []viewSegment
	input          []viewSegment
}

// 컴파일 타임 인터페이스 체크
var _ registerStore = (*deviceView)(nil)

// buildViewSegments 는 세그먼트 설정을 viewSegment 로 변환한다.
// 로컬은 own 맵, 공유는 container 맵을 target 으로 한다.
func buildViewSegments(segs []*RegisterAreaConfig, own, container *RegisterMap) []viewSegment {
	out := make([]viewSegment, 0, len(segs))
	for _, s := range segs {
		if s.IsShared {
			out = append(out, viewSegment{devStart: s.StartAddress, count: s.Count, mapStart: s.SharedAddress, target: container})
		} else {
			out = append(out, viewSegment{devStart: s.StartAddress, count: s.Count, mapStart: s.StartAddress, target: own})
		}
	}
	return out
}

// newDeviceView 는 디바이스 register_map 설정으로부터 deviceView 를 구성한다.
func newDeviceView(cfg RegisterMapConfig, own, container *RegisterMap) *deviceView {
	return &deviceView{
		coils:          buildViewSegments(cfg.Coils, own, container),
		discreteInputs: buildViewSegments(cfg.DiscreteInputs, own, container),
		holding:        buildViewSegments(cfg.HoldingRegisters, own, container),
		input:          buildViewSegments(cfg.InputRegisters, own, container),
	}
}

// resolve 는 [start, start+qty) 를 완전히 포함하는 세그먼트를 찾아 (target, 변환된 시작주소)
// 를 반환한다. 포함하는 세그먼트가 없으면(경계 횡단 포함) ok=false.
func resolveSegment(segs []viewSegment, start, qty uint16) (*RegisterMap, uint16, bool) {
	end := uint32(start) + uint32(qty)
	for _, s := range segs {
		if uint32(start) >= uint32(s.devStart) && end <= uint32(s.devStart)+uint32(s.count) {
			translated := s.mapStart + (start - s.devStart)
			return s.target, translated, true
		}
	}
	return nil, 0, false
}

// ---------------------------------------------------------------------------
// registerStore 구현 (6개 메서드) — 세그먼트 해석 후 target 맵에 위임
// ---------------------------------------------------------------------------

func (v *deviceView) ReadCoils(start, quantity uint16) ([]bool, error) {
	target, tstart, ok := resolveSegment(v.coils, start, quantity)
	if !ok {
		return nil, ErrAddressNotMapped
	}
	return target.ReadCoils(tstart, quantity)
}

func (v *deviceView) ReadDiscreteInputs(start, quantity uint16) ([]bool, error) {
	target, tstart, ok := resolveSegment(v.discreteInputs, start, quantity)
	if !ok {
		return nil, ErrAddressNotMapped
	}
	return target.ReadDiscreteInputs(tstart, quantity)
}

func (v *deviceView) ReadHoldingRegisters(start, quantity uint16) ([]uint16, error) {
	target, tstart, ok := resolveSegment(v.holding, start, quantity)
	if !ok {
		return nil, ErrAddressNotMapped
	}
	return target.ReadHoldingRegisters(tstart, quantity)
}

func (v *deviceView) ReadInputRegisters(start, quantity uint16) ([]uint16, error) {
	target, tstart, ok := resolveSegment(v.input, start, quantity)
	if !ok {
		return nil, ErrAddressNotMapped
	}
	return target.ReadInputRegisters(tstart, quantity)
}

func (v *deviceView) WriteCoils(start uint16, values []bool) (*ChangeSet, error) {
	target, tstart, ok := resolveSegment(v.coils, start, uint16(len(values)))
	if !ok {
		return nil, ErrAddressNotMapped
	}
	return target.WriteCoils(tstart, values)
}

func (v *deviceView) WriteHoldingRegisters(start uint16, values []uint16) (*ChangeSet, error) {
	target, tstart, ok := resolveSegment(v.holding, start, uint16(len(values)))
	if !ok {
		return nil, ErrAddressNotMapped
	}
	return target.WriteHoldingRegisters(tstart, values)
}

// ---------------------------------------------------------------------------
// 실효(effective) 스냅샷 — get_device_status 서빙 관점 노출 (SPEC-MODBUS-008)
// ---------------------------------------------------------------------------
//
// get_device_status 는 디바이스가 "실제로 서빙하는" 레지스터 맵을 보여야 한다. 공유
// 세그먼트를 가진 디바이스의 자체 *RegisterMap 은 로컬 세그먼트만 담으므로, 마스터가
// 읽는 실효 맵(공유 세그먼트=컨테이너 값)을 재현하려면 deviceView 의 세그먼트 라우팅을
// 거쳐야 한다. 아래 메서드는 세그먼트별로 자신의 Read* 메서드(라우팅 재사용)를 호출하여
// RegisterMap.GetSnapshot/RegisterCounts 와 동일한 형태의 스냅샷/카운트를 만든다.
// (세그먼트 라우팅 로직을 복제하지 않고 기존 읽기 프리미티브를 그대로 재사용한다.)

// EffectiveSnapshot 는 서빙 관점의 실효 레지스터 스냅샷을 RegisterMap.GetSnapshot 과
// 동일한 형태(map[string]any, 영역→map[디바이스주소]value)로 반환한다. 로컬 세그먼트는
// 자체 맵 값을, 공유 세그먼트는 컨테이너 값을 디바이스-로컬 주소에 매핑한다. 세그먼트가
// 없는 영역은 키를 생략한다(GetSnapshot 과 동일).
func (v *deviceView) EffectiveSnapshot() map[string]any {
	snap := make(map[string]any)
	if m := snapshotBoolSegments(v.coils, v.ReadCoils); m != nil {
		snap["coils"] = m
	}
	if m := snapshotBoolSegments(v.discreteInputs, v.ReadDiscreteInputs); m != nil {
		snap["discrete_inputs"] = m
	}
	if m := snapshotRegSegments(v.holding, v.ReadHoldingRegisters); m != nil {
		snap["holding_registers"] = m
	}
	if m := snapshotRegSegments(v.input, v.ReadInputRegisters); m != nil {
		snap["input_registers"] = m
	}
	return snap
}

// EffectiveCounts 는 영역별 실효 주소 개수를 RegisterMap.RegisterCounts 와 동일한 형태로
// 반환한다(네 영역 키를 항상 포함). 각 값은 해당 영역 세그먼트 길이의 합이며, 디바이스
// 주소가 세그먼트 간 중복되지 않으므로 실효 맵의 주소 개수와 일치한다.
func (v *deviceView) EffectiveCounts() map[string]int {
	return map[string]int{
		"coils":             segmentAddrCount(v.coils),
		"discrete_inputs":   segmentAddrCount(v.discreteInputs),
		"holding_registers": segmentAddrCount(v.holding),
		"input_registers":   segmentAddrCount(v.input),
	}
}

// snapshotBoolSegments 는 bool 영역 세그먼트들을 라우팅 읽기로 스냅샷하여 디바이스 주소로
// 키잉한 맵을 반환한다. 세그먼트가 없으면 nil(영역 키 생략용).
func snapshotBoolSegments(segs []viewSegment, read func(uint16, uint16) ([]bool, error)) map[uint16]bool {
	if len(segs) == 0 {
		return nil
	}
	out := make(map[uint16]bool)
	for _, s := range segs {
		vals, err := read(s.devStart, s.count)
		if err != nil {
			// 방어적: 정상 구성에서는 세그먼트 자기범위 읽기가 실패하지 않는다.
			continue
		}
		for i, val := range vals {
			out[s.devStart+uint16(i)] = val
		}
	}
	return out
}

// snapshotRegSegments 는 레지스터 영역 세그먼트들을 라우팅 읽기로 스냅샷하여 디바이스 주소로
// 키잉한 맵을 반환한다. 세그먼트가 없으면 nil(영역 키 생략용).
func snapshotRegSegments(segs []viewSegment, read func(uint16, uint16) ([]uint16, error)) map[uint16]uint16 {
	if len(segs) == 0 {
		return nil
	}
	out := make(map[uint16]uint16)
	for _, s := range segs {
		vals, err := read(s.devStart, s.count)
		if err != nil {
			continue
		}
		for i, val := range vals {
			out[s.devStart+uint16(i)] = val
		}
	}
	return out
}

// segmentAddrCount 는 세그먼트 길이의 합(=실효 주소 개수)을 반환한다.
func segmentAddrCount(segs []viewSegment) int {
	n := 0
	for _, s := range segs {
		n += int(s.count)
	}
	return n
}
