package message

// CopyMetadata 는 src 메타데이터의 모든 항목(string 값 + nested group 값)을 dst 로
// 복사한다. dst 에 이미 존재하는 키는 덮어쓴다.
//
// P2: 많은 소비자가 `for k, v := range msg.Metadata().All()` 로 메타데이터를
// 재구성/복사하는데, All() 은 string 값만 반환하므로 group 이 누락된다. 메타데이터를
// 통째로 전파해야 하는 경로에서는 이 헬퍼를 사용해 group 까지 보존한다.
//
// 값 계약(Raw() 기준): string → Set, map[string]string → SetGroup.
func CopyMetadata(dst, src Metadata) {
	if dst == nil || src == nil {
		return
	}
	for k, v := range src.Raw() {
		switch val := v.(type) {
		case string:
			dst.Set(k, val)
		case map[string]string:
			dst.SetGroup(k, val)
		}
	}
}

// CopyMetadataGroups 는 src 메타데이터의 nested group 값만 dst 로 복사한다.
// string(flat) 값은 건드리지 않는다.
//
// string 값을 이미 다른 방식(WithMetadata 옵션, 필터링 등)으로 처리한 노드가
// 출력 메시지에 group 만 추가로 보존할 때 사용한다. group 은 현재 string-only
// 필터(All() 기반)에 보이지 않으므로, src 의 모든 group 을 보존하는 것이
// group 차원에서 동작 보존(behavior-preserving)이다.
func CopyMetadataGroups(dst, src Metadata) {
	if dst == nil || src == nil {
		return
	}
	for k, v := range src.Raw() {
		if g, ok := v.(map[string]string); ok {
			dst.SetGroup(k, g)
		}
	}
}
