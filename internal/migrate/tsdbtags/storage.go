// storage.go — device_ids.json 의 composite → UUID 매핑 IO 헬퍼.
//
// C1 (deviceids 패키지) 의 동일 매핑 파일 형식을 공유한다. 본 패키지는
// deviceids 의 unexported loadIDMapping 에 의존하지 않고 자체 read-only
// loader 를 제공하여 패키지 간 결합을 최소화한다 (도구 간 독립성).
package tsdbtags

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// IDMapping 은 composite ("agent:unit_id") → UUID 매핑이다.
//
// device_ids.json 의 key 형식은 xflowd 데몬 (agent.ResolveDeviceID) 이 발급하는
// composite key 와 동일하다.
type IDMapping map[string]string

// LoadIDMapping 은 device_ids.json 을 IDMapping 으로 로드한다.
//
// 파일이 없거나 비어 있으면 빈 mapping 을 반환한다 (도구는 graceful: orphan
// 경고만 출력하고 진행 — C2 는 read-only 이므로 안전).
func LoadIDMapping(path string) (IDMapping, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return IDMapping{}, nil
		}
		return nil, fmt.Errorf("device_ids.json 읽기 실패 (%s): %w", path, err)
	}
	if len(data) == 0 {
		return IDMapping{}, nil
	}
	m := IDMapping{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("device_ids.json JSON 파싱 실패 (%s): %w", path, err)
	}
	return m, nil
}

// Reverse 는 UUID → []composite 의 역방향 매핑을 반환한다.
//
// 다대일 매핑 감지 (ambiguous 분류) 에 사용된다.
func (m IDMapping) Reverse() map[string][]string {
	reverse := make(map[string][]string, len(m))
	for composite, uid := range m {
		reverse[uid] = append(reverse[uid], composite)
	}
	return reverse
}
