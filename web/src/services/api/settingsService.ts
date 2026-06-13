// 전역(앱 전체 공유) key-value 설정 API 클라이언트.
//
// GET /settings/{key}  → { key, value }. 미저장 시 404 (APIError 로 전파).
// PUT /settings/{key}  → body 는 불투명 JSON value. 응답 { key, value }.
//
// 서버는 value 의 스키마를 강제하지 않으며, 키 네임스페이스는 프론트엔드가 정한다
// (예: "device-list-columns"). 전역 1벌이므로 사용자 구분이 없다.

import type { SettingResponse } from '@/types/settings';

import { get, put } from './client';

/**
 * 키에 저장된 설정 value 를 조회한다.
 *
 * 미저장 키는 서버가 404 를 반환하며, APIError(status 404)로 전파된다.
 * 호출자(훅)는 404 를 "기본값"으로 폴백 처리한다.
 *
 * @param key - 설정 키 (예: "device-list-columns")
 * @returns 파싱된 value (저장한 JSON 그대로)
 */
export async function getSetting<T = unknown>(key: string): Promise<T> {
  const res = await get<SettingResponse<T>>(`/settings/${encodeURIComponent(key)}`);
  return res.value;
}

/**
 * 키에 value(JSON)를 저장한다(UPSERT).
 *
 * body 는 불투명 JSON value 로 전송되며, 서버는 유효한 JSON 인지만 검증한다.
 *
 * @param key - 설정 키
 * @param value - 저장할 값(직렬화 가능한 JSON)
 * @returns 저장된 value (서버 응답 기준)
 */
export async function putSetting<T = unknown>(key: string, value: T): Promise<T> {
  const res = await put<SettingResponse<T>>(`/settings/${encodeURIComponent(key)}`, value);
  return res.value;
}
