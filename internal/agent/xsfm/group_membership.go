package xsfm

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// 그룹 멤버십 도출 · 다대다 · 마이그레이션 · 커스텀 CRUD (agent 레이어)
// (SPEC-XSFM-GROUP-001 M2~M5)
// ---------------------------------------------------------------------------
//
// 이 파일은 그룹 레지스트리(group_registry.go, 순수 데이터 레이어)를 station 레지스트리·
// 로스터와 결합하는 에이전트 레이어이다. 멤버 도출은 그룹 id 의 타입 접두사로 분기하며
// (RD-2), 모든 반환 경로는 현재 로스터에 존재하는 device_id 만 통과시킨다(RD-3 유령 멤버
// 필터). 기본 그룹(station/line)은 저장하지 않고 station 레지스트리에서 파생한다.
//
// 락 규율(REQ-01-07/NF-04): 각 도출 단계는 자기 락(그룹 레지스트리 락 → 해제, station
// 레지스트리 락 → 해제, 로스터 락 → 해제)을 순차로만 잡고 절대 중첩하지 않는다. fan-out
// 은 상위(control.go)가 대상 스냅샷을 받은 뒤 락 해제 상태로 controlDevice 를 반복한다.

// GroupMembers 는 group_id 에 속한 device_id 목록을 도출한다(재구현, RD-2 접두사 분기).
//
// 접두사로 타입을 판별해 분기한다: custom:→저장된 멤버(로스터 대조 필터 후), station:→
// DevicesByStation, line:→DevicesByLine. 접두사가 없는 레거시 group_id 는 primary 그룹
// (Device.GroupID)과 대조하는 무회귀 폴백 경로로 처리한다(NF-02 — 기존 셀렉터 동작 보존).
// 모든 경로의 반환은 로스터에 실재하는 device_id 뿐이다(RD-3). 시그니처는 재구현 전과
// 동일해 호출부(handleSelectorControl)는 변경되지 않는다.
//
// @MX:ANCHOR: 그룹 셀렉터 fan-out 대상 도출의 단일 진입점 — handleSelectorControl(control.go)이
// 소비하고, 접두사 분기가 station/line/custom/legacy 4경로를 결정한다.
// @MX:REASON: fan_in>=2 (handleSelectorControl + GroupsForDevice 계열) + RD-2/RD-3 규약이 이 함수에 baked-in.
func (a *XSFMAgent) GroupMembers(groupID string) []string {
	switch groupTypeFromID(groupID) {
	case groupTypeCustom:
		if a.groups == nil {
			return nil
		}
		members, ok := a.groups.membersOf(groupID)
		if !ok {
			return nil // 미등록 커스텀 그룹 → 멤버 없음(fan-out 존재 검사는 ensureGroupExists 가 담당).
		}
		return a.filterRosterPresent(members) // RD-3: 로스터 대조 필터.
	case groupTypeStation:
		return a.DevicesByStation(customStationLineCode(groupID, groupPrefixStation))
	case groupTypeLine:
		targets, _ := a.DevicesByLine(customStationLineCode(groupID, groupPrefixLine))
		return targets
	default:
		return a.devicesByPrimaryGroup(groupID) // 레거시 접두사 없음 → primary 그룹 무회귀 폴백.
	}
}

// customStationLineCode 는 station:/line: 접두사를 벗겨 원천 코드(Ref)를 반환한다.
func customStationLineCode(id, prefix string) string { return strings.TrimPrefix(id, prefix) }

// devicesByPrimaryGroup 은 레거시 무회귀 경로이다: Device.GroupID(primary 그룹)와 일치하는
// device_id 를 로스터에서 도출한다(정렬). 재구현 전 GroupMembers 의 동작을 그대로 보존한다.
// 로스터에서 도출하므로 이미 실재 device_id 뿐이다(RD-3 자동 충족).
func (a *XSFMAgent) devicesByPrimaryGroup(groupID string) []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var ids []string
	for id, dev := range a.devices {
		if dev.GroupID == groupID {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// filterRosterPresent 는 members 중 현재 로스터에 존재하는 device_id 만 정렬해 반환한다
// (RD-3 유령 멤버 필터). 그룹 레지스트리 락은 호출부(membersOf)에서 이미 해제된 상태이며,
// 여기서는 로스터 락만 취득한다(락 중첩 금지).
func (a *XSFMAgent) filterRosterPresent(members []string) []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]string, 0, len(members))
	for _, id := range members {
		if _, ok := a.devices[id]; ok {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// GroupsForDevice 는 특정 디바이스가 속한 전체 그룹 id 목록을 도출한다(역방향 질의, REQ-02-03).
//
// 멤버십 저장 방향은 그룹→디바이스(커스텀만)이므로 디바이스→그룹은 도출한다: (1) 커스텀 그룹
// 중 members 에 device_id 를 포함하는 것, (2) 디바이스의 Station 에서 파생한 station 그룹,
// (3) 그 station 의 line 에서 파생한 line 그룹. 결정적 순서(정렬)로 반환한다.
//
// 락 규율: 그룹 레지스트리(ListGroups) → 해제, 로스터 → 해제, station 레지스트리(ResolveLine)
// 순으로 순차 취득하며 중첩하지 않는다.
func (a *XSFMAgent) GroupsForDevice(deviceID string) []string {
	var out []string

	// (1) 커스텀 그룹: members 포함 여부(그룹 레지스트리 락, 스냅샷 후 해제).
	if a.groups != nil {
		for _, g := range a.groups.ListGroups() {
			for _, m := range g.Members {
				if m == deviceID {
					out = append(out, g.ID)
					break
				}
			}
		}
	}

	// (2)/(3) 파생 station/line 그룹: 디바이스 Station 스냅샷(로스터 락) 후 line 해석(station 락).
	a.mu.RLock()
	dev, ok := a.devices[deviceID]
	station := ""
	if ok {
		station = dev.Station
	}
	a.mu.RUnlock()

	if ok && station != "" {
		out = append(out, groupPrefixStation+station)
		if a.stations != nil {
			if line, err := a.stations.ResolveLine(station); err == nil && line != "" {
				out = append(out, groupPrefixLine+line)
			}
		}
	}

	return dedupSortedStrings(out)
}

// ---------------------------------------------------------------------------
// M2 — 마이그레이션 + primary 그룹 규칙 (REQ-02-04/02-07, §4.4/§4.5)
// ---------------------------------------------------------------------------

// migrateGroupMembership 은 로드(Init) 시 비어있지 않은 Device.GroupID 를 커스텀 그룹으로
// 편입한다(REQ-02-04, A-4). RD-1 확정: Device.GroupID 는 primary 그룹으로 유지되어 단일
// group_id 태그 방출(status.go/monitor.go/provider.go)이 무회귀로 보존되고(값을 바꾸지
// 않는다), 다중 소속은 그룹 레지스트리가 담당한다. custom:<value> 그룹을 보장 생성하고
// device_id 를 멤버로 편입한다. 시동 시 1회 호출하며 idempotent(재시작 시 재실행 무해).
//
// 락 규율: 로스터를 RLock 하에 스냅샷한 뒤 해제하고, 그다음 그룹 레지스트리에 편입한다
// (로스터 락과 그룹 레지스트리 락을 중첩하지 않는다). primary 값은 변경하지 않는다.
//
// @MX:WARN: 로스터 락(a.mu) → 해제 → 그룹 레지스트리 락 순서를 반드시 지킬 것(중첩 금지).
// @MX:REASON: 프로젝트 RWMutex 재진입 RLock deadlock 트랩(HVAC 에이전트 교훈) 회피 —
// 두 락을 동시에 잡으면 fan-out 동시 실행 시 교착 위험.
func (a *XSFMAgent) migrateGroupMembership() {
	if a.groups == nil {
		return
	}
	a.mu.RLock()
	pairs := make([]legacyMembership, 0, len(a.devices))
	for id, dev := range a.devices {
		if dev.GroupID == "" {
			continue
		}
		pairs = append(pairs, legacyMembership{
			groupID:  customIDFor(dev.GroupID),
			name:     dev.GroupID,
			deviceID: id,
		})
	}
	a.mu.RUnlock()

	a.groups.importLegacyMemberships(pairs)
}

// enrollPrimaryIfEmpty 는 커스텀 그룹에 처음 편입되는 디바이스가 primary 를 아직 갖지 않았으면
// (빈 값) 그 그룹을 primary 로 설정한다(§4.5 커스텀 편입 시 선택적 규칙). 이미 primary 가
// 있으면 유지한다. primary 값은 접두사 없는 커스텀 name(customPart)이다 — 단일 태그 방출이
// 레거시 포맷(접두사 없음)을 유지하도록.
func (a *XSFMAgent) enrollPrimaryIfEmpty(deviceIDs []string, primary string) {
	if primary == "" || len(deviceIDs) == 0 {
		return
	}
	changed := false
	a.mu.Lock()
	for _, id := range deviceIDs {
		if dev, ok := a.devices[id]; ok && dev.GroupID == "" {
			dev.GroupID = primary
			changed = true
		}
	}
	a.mu.Unlock()
	if changed {
		a.persistRoster()
	}
}

// resetPrimaryGroup 은 primary 가 삭제된 커스텀 그룹(customPart)을 가리키던 디바이스의 primary
// 를 빈 값으로 리셋한다(§4.5 primary 그룹 삭제 시 규칙). 단일 태그는 이후 미소속으로 방출된다.
func (a *XSFMAgent) resetPrimaryGroup(primary string) {
	if primary == "" {
		return
	}
	changed := false
	a.mu.Lock()
	for _, dev := range a.devices {
		if dev.GroupID == primary {
			dev.GroupID = ""
			changed = true
		}
	}
	a.mu.Unlock()
	if changed {
		a.persistRoster()
	}
}

// ---------------------------------------------------------------------------
// M5 — 그룹 셀렉터 fan-out 존재 검사 (REQ-05-05)
// ---------------------------------------------------------------------------

// ensureGroupExists 는 group_id 셀렉터 fan-out 전에 그룹 존재를 검증한다(REQ-05-05).
//
// custom: 접두사 그룹만 레지스트리 등록 여부를 검사해 미등록 시 ErrGroupNotFound 를 반환한다.
// station:/line: 접두사와 접두사 없는 레거시 group_id 는 파생/무회귀 경로로 수렴하므로 존재
// 검사를 하지 않고(빈 결과는 fanOutControl 이 ErrEmptyGroup 으로 처리) RD-4 셀렉터 병존
// 동일 결과를 보존한다 — 즉 group_id=station:<code> 는 station 셀렉터와 동일하게 동작한다.
func (a *XSFMAgent) ensureGroupExists(groupID string) error {
	if groupTypeFromID(groupID) == groupTypeCustom {
		if a.groups == nil || !a.groups.exists(groupID) {
			return fmt.Errorf("%w: %q", ErrGroupNotFound, groupID)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// M3 — 커스텀 그룹 CRUD 명령 핸들러 (REQ-03-01..06)
// ---------------------------------------------------------------------------

// handleAddGroup 은 add_group 명령을 처리한다(REQ-03-01). type=custom 그룹을 생성하고
// (name 필수 + 초기 members 선택) group_registered 이벤트 방출 + 영속화한다. 이미 존재하는
// id 는 ErrGroupAlreadyExists. 새 멤버 중 primary 빈 디바이스는 이 그룹을 primary 로 설정한다.
func (a *XSFMAgent) handleAddGroup(req processRequest, raw []byte) ([]byte, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("%w: add_group requires name", ErrInvalidCommand)
	}
	// SPEC-XSFM-LINE-001 RD-2: code 가 제공되면 통일 코드 포맷을 검증하고 id=custom:<code> 로
	// 인코딩한다. code 키가 아예 없는 레거시 요청(name 만 제공)은 무회귀 폴백으로 id=custom:<name>
	// 을 사용한다(GROUP-001 동작 보존). code 키가 존재하지만 빈/비적합이면 ErrInvalidCode 로 거부.
	var gid, code string
	if keyPresent(raw, req.Params, "code") {
		if !validCode(req.Code) {
			return nil, fmt.Errorf("%w: %q", ErrInvalidCode, req.Code)
		}
		code = req.Code
		gid = groupPrefixCustom + code
	} else {
		gid = customIDFor(req.Name)
		code = customIDName(gid)
	}
	if a.groups.exists(gid) {
		return nil, fmt.Errorf("%w: %q", ErrGroupAlreadyExists, gid)
	}
	members, _ := extractMembers(raw, req.Params)

	g := Group{ID: gid, Name: req.Name, Type: groupTypeCustom, Code: code, Members: dedupSortedStrings(members)}
	if err := a.groups.UpsertGroup(g); err != nil {
		return nil, err
	}

	a.enrollPrimaryIfEmpty(members, customIDName(gid))
	a.sendEvent("group_registered", map[string]any{"group_id": gid, "name": req.Name})

	return json.Marshal(map[string]any{
		"status":   "ok",
		"group_id": gid,
		"command":  "add_group",
	})
}

// handleRemoveGroup 은 remove_group 명령을 처리한다(REQ-03-02/05/06). 커스텀 그룹만 제거
// 가능하며(기본 그룹은 ErrGroupNotCustom), 미등록은 ErrGroupNotFound(부분 변경 없음).
// 제거 성공 시 primary 가 이 그룹을 가리키던 디바이스를 리셋하고 group_unregistered 방출.
func (a *XSFMAgent) handleRemoveGroup(req processRequest) ([]byte, error) {
	if req.GroupID == "" {
		return nil, fmt.Errorf("%w: remove_group requires group_id", ErrInvalidCommand)
	}
	if groupTypeFromID(req.GroupID) != groupTypeCustom {
		return nil, fmt.Errorf("%w: %q", ErrGroupNotCustom, req.GroupID)
	}
	if err := a.groups.RemoveGroup(req.GroupID); err != nil {
		return nil, err
	}

	a.resetPrimaryGroup(customIDName(req.GroupID))
	a.sendEvent("group_unregistered", map[string]any{"group_id": req.GroupID})

	return json.Marshal(map[string]any{
		"status":   "ok",
		"group_id": req.GroupID,
		"command":  "remove_group",
	})
}

// handleSetGroup 은 set_group 명령을 처리한다(REQ-03-03/05/06). name 및/또는 members 를 부분
// 갱신하며(제공되지 않은 필드 보존, handleSetDevice 의 present() 패턴 재사용), 커스텀 그룹만
// 편집 가능하다(기본 그룹 ErrGroupNotCustom, 미등록 ErrGroupNotFound). members 갱신 시 primary
// 빈 신규 멤버는 이 그룹을 primary 로 설정한다.
func (a *XSFMAgent) handleSetGroup(req processRequest, raw []byte) ([]byte, error) {
	if req.GroupID == "" {
		return nil, fmt.Errorf("%w: set_group requires group_id", ErrInvalidCommand)
	}
	if groupTypeFromID(req.GroupID) != groupTypeCustom {
		return nil, fmt.Errorf("%w: %q", ErrGroupNotCustom, req.GroupID)
	}
	g, err := a.groups.GetGroup(req.GroupID) // 미등록 → ErrGroupNotFound(부분 변경 전 거부).
	if err != nil {
		return nil, err
	}

	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawFields); err != nil {
		return nil, fmt.Errorf("xsfm set_group: %w", err)
	}
	// present 는 부분 갱신 대상 판별기이다(handleSetDevice 패턴): top-level raw 키 또는 params
	// 키 중 어느 한쪽에 존재하면 갱신 대상으로 본다.
	present := func(key string) bool {
		if _, ok := rawFields[key]; ok {
			return true
		}
		if req.Params != nil {
			if _, ok := req.Params[key]; ok {
				return true
			}
		}
		return false
	}

	if present("name") {
		g.Name = req.Name
	}
	var newMembers []string
	membersChanged := false
	if present("members") {
		newMembers, _ = extractMembers(raw, req.Params)
		g.Members = dedupSortedStrings(newMembers)
		membersChanged = true
	}

	if err := a.groups.UpsertGroup(g); err != nil {
		return nil, err
	}

	if membersChanged {
		a.enrollPrimaryIfEmpty(newMembers, customIDName(req.GroupID))
	}

	return json.Marshal(map[string]any{
		"status":   "ok",
		"group_id": req.GroupID,
		"command":  "set_group",
	})
}

// handleListGroups 는 list_groups 명령을 처리한다(REQ-03-04). 전체 그룹(커스텀 + 파생
// station/line)을 {id,name,type,member_count,members} 로 결정적 순서(type→name→id)로 반환한다.
//
// 커스텀 그룹은 그룹 레지스트리에서, 기본 그룹은 station 레지스트리에서 파생한다: station 그룹은
// 등록된 각 역사에서(name=표시명), line 그룹은 역사들의 호선 집합에서(REQ-04-01..04). member_count/
// members 는 GroupMembers 로 도출하므로 로스터 대조 필터·파생이 일관 적용된다(RD-3).
// allGroups 는 전 타입 그룹(커스텀 + 파생 station/line)의 값 복사본 목록을 정렬 없이 반환한다.
// 이름 리졸버(GroupByName, RD-5 전 타입 매칭)와 list_groups 핸들러가 공용한다(단일 도출 지점).
//
// 락 규율: 각 레지스트리 락은 내부(ListGroups/ListStations)에서 취득·해제되며 서로 중첩하지
// 않는다 — 반환 시 어떤 락도 보유하지 않는다. 파생 그룹 표시명은 handleListGroups 와 동일 규칙
// (station=DisplayName 폴백 코드, line=호선 코드)으로 도출한다.
func (a *XSFMAgent) allGroups() []Group {
	var groups []Group

	// 커스텀 그룹 (레지스트리).
	if a.groups != nil {
		groups = append(groups, a.groups.ListGroups()...)
	}

	// 파생 기본 그룹 (station 레지스트리): station 그룹 + line 그룹.
	if a.stations != nil {
		stations := a.stations.ListStations()
		lineSeen := make(map[string]bool)
		for _, st := range stations {
			name := st.DisplayName
			if name == "" {
				name = st.Station
			}
			groups = append(groups, Group{
				ID:   groupPrefixStation + st.Station,
				Name: name,
				Type: groupTypeStation,
				Ref:  st.Station,
			})
		}
		for _, st := range stations {
			if st.Line == "" || lineSeen[st.Line] {
				continue
			}
			lineSeen[st.Line] = true
			groups = append(groups, Group{
				ID:   groupPrefixLine + st.Line,
				Name: st.Line,
				Type: groupTypeLine,
				Ref:  st.Line,
			})
		}
	}

	return groups
}

func (a *XSFMAgent) handleListGroups() ([]byte, error) {
	groups := a.allGroups()

	// 결정적 순서: type → name → id.
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Type != groups[j].Type {
			return groups[i].Type < groups[j].Type
		}
		if groups[i].Name != groups[j].Name {
			return groups[i].Name < groups[j].Name
		}
		return groups[i].ID < groups[j].ID
	})

	out := make([]map[string]any, 0, len(groups))
	for _, g := range groups {
		members := a.GroupMembers(g.ID)
		if members == nil {
			members = []string{}
		}
		out = append(out, map[string]any{
			"id":           g.ID,
			"name":         g.Name,
			"type":         g.Type,
			"code":         g.Code,
			"ref":          g.Ref,
			"member_count": len(members),
			"members":      members,
		})
	}
	return json.Marshal(map[string]any{"status": "ok", "groups": out})
}

// ---------------------------------------------------------------------------
// params/raw 추출 헬퍼
// ---------------------------------------------------------------------------

// keyPresent 는 raw 최상위 JSON 키 또는 params 키에 key 가 존재하는지 판정한다(부분 갱신/신규
// 폼 판별용, handleSetGroup 의 present 패턴 재사용). raw 파싱 실패는 params 만 검사한다.
func keyPresent(raw []byte, params map[string]any, key string) bool {
	if len(raw) > 0 {
		var top map[string]json.RawMessage
		if json.Unmarshal(raw, &top) == nil {
			if _, ok := top[key]; ok {
				return true
			}
		}
	}
	if params != nil {
		if _, ok := params[key]; ok {
			return true
		}
	}
	return false
}

// extractMembers 는 members 목록을 raw 최상위 또는 params 에서 추출한다(HTTP exec 경로 지원).
// 반환: (문자열 슬라이스, 존재 여부). 존재하지만 빈 배열이면 (빈 슬라이스, true).
func extractMembers(raw []byte, params map[string]any) ([]string, bool) {
	// 1) raw 최상위 "members".
	var top struct {
		Members *[]string `json:"members"`
	}
	if len(raw) > 0 && json.Unmarshal(raw, &top) == nil && top.Members != nil {
		return *top.Members, true
	}
	// 2) params["members"] (JSON 은 []any{string,...}).
	if params != nil {
		if v, ok := params["members"]; ok {
			return toStringSlice(v), true
		}
	}
	return nil, false
}

// toStringSlice 는 any(주로 []any 또는 []string)를 []string 으로 정규화한다. 문자열이 아닌
// 원소는 무시한다.
func toStringSlice(v any) []string {
	switch s := v.(type) {
	case []string:
		return append([]string(nil), s...)
	case []any:
		out := make([]string, 0, len(s))
		for _, e := range s {
			if str, ok := e.(string); ok {
				out = append(out, str)
			}
		}
		return out
	default:
		return nil
	}
}
