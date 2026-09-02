// Store 에이전트 키 메타데이터(타입/태그) 편집 요청·응답 타입.
//
// 백엔드 엔드포인트 `PUT /api/v1/store/{agent_name}/keys/{key}/meta` 와 1:1 매핑된다.
// - 정적 키뿐 아니라 동적 키 등 임의 엔트리에 field/tags 를 부여한다.
// - tags 는 부분 병합이 아니라 **전체 교체** 이므로, 편집 UI 는 기존 태그를 불러와
//   추가/삭제 후 전체 맵을 전송해야 한다.
//
// @spec SPEC-STORE-003 v0.4.0

/**
 * `PUT /api/v1/store/{agent_name}/keys/{key}/meta` 요청 바디.
 *
 * - `field`: 생략/빈 문자열 시 백엔드가 `"unknown"` 으로 normalize 한다.
 *   정규식 `^[a-zA-Z0-9_-]+$` 위반 시 400 (ErrInvalidMetricType).
 * - `tags`: **전체 교체**. 부분 수정 시에도 기존+변경 전체를 전송한다.
 *   생략 시 빈 객체로 취급된다. 태그 키 정규식 위반 시 400 (ErrInvalidTagKey).
 *
 * @spec SPEC-STORE-003 v0.4.0
 */
export interface SetStoreKeyMetaRequest {
  field?: string;
  tags?: Record<string, string>;
}

/**
 * `PUT /api/v1/store/{agent_name}/keys/{key}/meta` 응답 (envelope 제거 후).
 *
 * 백엔드가 normalize 한 결과를 반환한다 (field 은 항상 비어있지 않으며,
 * tags 는 항상 객체).
 *
 * @spec SPEC-STORE-003 v0.4.0
 */
export interface SetStoreKeyMetaResponse {
  key: string;
  field: string;
  tags: Record<string, string>;
}
