// 유휴 로그아웃 훅 (@SPEC:SPEC-AUTH-IDLE-001).
//
// 사용자 입력이 설정된 시간 동안 없으면 로그아웃한다. 판정은 순수 층
// (lib/idle/idlePolicy)이 하고, 이 훅은 그 판정에 필요한 재료 — 마지막 활동 시각,
// 지금 시각, 설정 — 을 모아 잇는 배선이다.

import { useCallback, useEffect, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';

import { fetchIdleLogoutSetting } from '@/services/api/idleLogoutService';
import {
  DEFAULT_IDLE_SETTING,
  evaluateIdle,
  type IdleLogoutSetting,
  type IdlePhase,
} from '@/lib/idle/idlePolicy';
import { useAuth } from '@/hooks/useAuth';
import { useAuthStore } from '@/stores/authStore';

/** 설정 쿼리 키 — 설정 카드와 공유한다. */
export const IDLE_LOGOUT_QUERY_KEY = ['settings', 'idle-logout'] as const;

/** 탭 사이로 마지막 활동 시각을 나르는 localStorage 키. */
export const IDLE_ACTIVITY_STORAGE_KEY = 'xflow_idle_last_activity';

/** 판정 주기(ms). 남은 시간 표시가 1초 단위이므로 그에 맞춘다. */
const TICK_MS = 1_000;

/** localStorage 쓰기 간격(ms). 매 입력마다 쓰면 다른 탭의 이벤트가 폭주한다. */
const STORAGE_WRITE_INTERVAL_MS = 5_000;

/**
 * 활동으로 치는 이벤트.
 *
 * `pointermove` 를 넣은 것은 "읽고 있는 사람" 도 사용자이기 때문이다 — 대시보드를
 * 보며 마우스를 움직이는 동안 잠기면 목적이 아니라 방해가 된다. 반대로 탭 전환·
 * 창 포커스는 넣지 않는다. 두 시간 만에 돌아온 탭이 돌아왔다는 이유로 연장되면
 * "자리를 비우면 잠근다" 가 무너진다.
 */
const ACTIVITY_EVENTS = [
  'pointerdown',
  'pointermove',
  'keydown',
  'wheel',
  'touchstart',
  'scroll',
] as const;

/** 저장된 마지막 활동 시각을 읽는다(없거나 망가지면 0). */
function readStoredActivity(): number {
  try {
    const raw = localStorage.getItem(IDLE_ACTIVITY_STORAGE_KEY);
    if (!raw) return 0;
    const parsed = Number(raw);
    return Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
  } catch {
    return 0;
  }
}

/** 마지막 활동 시각을 기록한다(실패는 무시 — 이 탭의 메모리 값으로도 동작한다). */
function writeStoredActivity(at: number): void {
  try {
    localStorage.setItem(IDLE_ACTIVITY_STORAGE_KEY, String(at));
  } catch {
    // 사생활 보호 모드 등 — 탭 간 공유만 포기한다.
  }
}

export interface IdleLogoutState {
  /** 현재 국면. 기능이 꺼져 있거나 미인증이면 항상 'active'. */
  phase: IdlePhase;
  /** 로그아웃까지 남은 시간(ms). 경고 화면의 카운트다운에 쓴다. */
  remainingMs: number;
  /** 적용 중인 설정(화면 안내용). */
  setting: IdleLogoutSetting;
  /** 사용자가 '계속 사용' 을 눌렀을 때 — 지금을 활동으로 기록한다. */
  extend: () => void;
}

/**
 * 유휴 상태를 추적하고, 한도를 넘기면 로그아웃한다.
 *
 * 앱에서 한 번만 마운트한다(IdleLogoutGuard). 여러 번 마운트하면 타이머와 리스너가
 * 그만큼 늘어난다 — 동작은 같지만 낭비다.
 */
export function useIdleLogout(): IdleLogoutState {
  const { logout } = useAuth();
  const authEnabled = useAuthStore((s) => s.authEnabled);
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);

  const { data: setting = DEFAULT_IDLE_SETTING } = useQuery({
    queryKey: IDLE_LOGOUT_QUERY_KEY,
    queryFn: fetchIdleLogoutSetting,
    staleTime: 60_000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  // 기능이 실제로 도는 조건. 인증이 꺼진 서버나 로그인 화면에서는 돌지 않는다.
  const armed = authEnabled === true && isAuthenticated && setting.enabled;

  const lastActivityRef = useRef<number>(Date.now());
  const lastStorageWriteRef = useRef<number>(0);
  // 로그아웃이 두 번 불리지 않게 한다 — 만료 tick 과 다른 탭의 tick 이 겹칠 수 있다.
  const loggingOutRef = useRef(false);

  const [phase, setPhase] = useState<IdlePhase>('active');
  const [remainingMs, setRemainingMs] = useState<number>(setting.timeoutMinutes * 60_000);

  /** 지금을 활동으로 기록한다(메모리 + 탭 공유). */
  const recordActivity = useCallback((now: number = Date.now()) => {
    lastActivityRef.current = now;
    if (now - lastStorageWriteRef.current >= STORAGE_WRITE_INTERVAL_MS) {
      lastStorageWriteRef.current = now;
      writeStoredActivity(now);
    }
  }, []);

  const extend = useCallback(() => {
    const now = Date.now();
    lastStorageWriteRef.current = 0; // '계속 사용' 은 즉시 다른 탭에도 알린다.
    recordActivity(now);
    setPhase('active');
  }, [recordActivity]);

  useEffect(() => {
    if (!armed) {
      setPhase('active');
      return;
    }

    // 무장 시점을 활동으로 본다 — 로그인 직후부터 한도가 시작된다.
    recordActivity();
    loggingOutRef.current = false;

    const onActivity = () => recordActivity();
    for (const type of ACTIVITY_EVENTS) {
      window.addEventListener(type, onActivity, { passive: true, capture: true });
    }

    const tick = () => {
      const now = Date.now();
      // 다른 탭의 활동도 내 활동이다 — 한쪽에서 일하는 동안 다른 탭이 잠기면 안 된다.
      const lastActivityAt = Math.max(lastActivityRef.current, readStoredActivity());

      const next = evaluateIdle({
        now,
        lastActivityAt,
        timeoutMinutes: setting.timeoutMinutes,
      });
      setPhase(next.phase);
      setRemainingMs(next.remainingMs);

      if (next.phase === 'expired' && !loggingOutRef.current) {
        loggingOutRef.current = true;
        // 서버 블랙리스트까지 닿는 경로다. 실패해도 로컬 정리는 보장된다.
        void logout();
      }
    };

    tick();
    const timer = setInterval(tick, TICK_MS);

    return () => {
      clearInterval(timer);
      for (const type of ACTIVITY_EVENTS) {
        window.removeEventListener(type, onActivity, { capture: true });
      }
    };
  }, [armed, setting.timeoutMinutes, recordActivity, logout]);

  return { phase: armed ? phase : 'active', remainingMs, setting, extend };
}
