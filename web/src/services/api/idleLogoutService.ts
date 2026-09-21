// 유휴 로그아웃 설정의 서버 전역 1벌 읽기/쓰기 (@SPEC:SPEC-AUTH-IDLE-001).
//
// 기존 전역 key-value 설정 저장소(GET/PUT /settings/{key})를 그대로 쓴다 — 새 테이블도
// 새 엔드포인트도 만들지 않는다. 읽기는 system.read, 쓰기는 system.update 권한을
// 요구하며, 빌트인 세 역할(admin·editor·viewer)은 모두 읽을 수 있다.

import { getSetting, putSetting } from './settingsService';
import {
  DEFAULT_IDLE_SETTING,
  clampTimeoutMinutes,
  normalizeIdleSetting,
  type IdleLogoutSetting,
} from '@/lib/idle/idlePolicy';

/** 전역 설정 키. 네임스페이스는 프론트엔드가 정한다(저장소 규약). */
export const IDLE_LOGOUT_SETTING_KEY = 'auth.idle-logout';

/**
 * 유휴 설정을 읽는다.
 *
 * 미저장(404) · 권한 없음(403) · 망가진 값 — 어느 쪽이든 기본값으로 떨어진다.
 * 설정을 못 읽었다고 유휴 잠금을 꺼 버리면 보안 정책이 조용히 사라지므로,
 * 폴백은 "끔" 이 아니라 **기본 정책(10분, 켜짐)** 이다.
 */
export async function fetchIdleLogoutSetting(): Promise<IdleLogoutSetting> {
  try {
    const raw = await getSetting<unknown>(IDLE_LOGOUT_SETTING_KEY);
    return normalizeIdleSetting(raw);
  } catch {
    return { ...DEFAULT_IDLE_SETTING };
  }
}

/** 유휴 설정을 저장한다(관리자). 저장 전에 한도를 허용 범위로 자른다. */
export async function saveIdleLogoutSetting(
  setting: IdleLogoutSetting,
): Promise<IdleLogoutSetting> {
  const normalized: IdleLogoutSetting = {
    enabled: setting.enabled,
    timeoutMinutes: clampTimeoutMinutes(setting.timeoutMinutes),
  };
  const saved = await putSetting<IdleLogoutSetting>(IDLE_LOGOUT_SETTING_KEY, normalized);
  return normalizeIdleSetting(saved);
}
