// 팔레트 묶음의 접힘 상태 (SPEC-CANVAS-008 M7 · REQ-06).
//
// 도크 폭은 `w-44` 고정이고 카탈로그는 30칸이다. 넷을 한 목록으로 펴면 자주 쓰는 원시형이
// 스크롤 아래로 밀리므로(위험 R10), 카탈로그 묶음 셋은 **기본으로 접혀** 있고 원시형 묶음만
// 펼쳐진다. 006 이 배율 칸을 도크의 맨 앞에 둔 것과 같은 판단이다.
//
// **접힌 묶음은 자식을 아예 그리지 않는다**(`hidden` 이 아니라 미마운트). 그래서 도크가
// 열릴 때 만들어지는 미리보기 `<canvas>` 가 0개이고, 사용자가 묶음을 편 그 순간에만 그
// 묶음의 칸들이 생긴다.
//
// **`uiStore` 에 얹지 않는다.** 그 store 는 부팅 때 테마를 읽으려고 `xflow-ui` 블롭을 통째로
// 역직렬화하고, 버전 6까지 이어진 마이그레이션 사슬을 갖고 있다. 접힘 상태 하나를 그 사슬
// 안으로 들이는 값이 맞지 않는다 — 여기 있는 것은 불리언 넷이고, 읽지 못해도 기능이
// 성립한다(REQ-06 은 이 항목을 "가능하면" 으로 적었다).
//
// **읽기도 쓰기도 예외를 밖으로 내지 않는다.** 시크릿 창 · 용량 초과 · 접근 불가에서 도형
// 팔레트가 통째로 죽으면, 잃은 것은 접힘 상태가 아니라 팔레트다(`uiStore` 의 규율과 같다).
//
// @spec SPEC-CANVAS-008 REQ-06

import { useState } from 'react';

import type { ShapeGroupId } from './shapeCatalog';

/** 팔레트 묶음 넷 — 원시형 하나와 카탈로그 셋. */
export type PaletteGroupId = 'primitive' | ShapeGroupId;

/**
 * 화면 차례. 원시형이 맨 앞인 것에 뜻이 있다 — 오늘 쓰이는 넷이 어제와 같은 자리에 있어야
 * 이 변경이 회귀가 아니다(AC-E10).
 */
export const PALETTE_GROUP_IDS: readonly PaletteGroupId[] = [
  'primitive',
  'general',
  'basic',
  'arrow',
];

/** 묶음 제목의 i18n 키. 리터럴 맵이라 어떤 키가 쓰이는지 검색으로 확인된다. */
export const PALETTE_GROUP_TITLE_KEYS: Readonly<Record<PaletteGroupId, string>> = {
  primitive: 'dashboard.canvas.edit.paletteGroupPrimitive',
  general: 'dashboard.canvas.edit.paletteGroupGeneral',
  basic: 'dashboard.canvas.edit.paletteGroupBasic',
  arrow: 'dashboard.canvas.edit.paletteGroupArrow',
};

/** 처음 열었을 때의 모습 — 원시형만 펼침. */
export const DEFAULT_COLLAPSED: Readonly<Record<PaletteGroupId, boolean>> = {
  primitive: false,
  general: true,
  basic: true,
  arrow: true,
};

/** 기기 지역 저장 키. `xflow-ui` 와 나누지 않고 제 키를 쓴다(위 머리말). */
export const PALETTE_STORAGE_KEY = 'xflow-canvas-palette';

export type PaletteCollapseState = Record<PaletteGroupId, boolean>;

/**
 * 저장된 접힘 상태. **관용적으로 읽는다** — 저장소의 내용은 사용자가 손으로 고칠 수 있는
 * 값이므로, 모르는 키는 버리고 불리언이 아닌 값은 기본값으로 떨어뜨린다(001 파서와 같은 규율).
 */
export function readCollapsed(): PaletteCollapseState {
  const next: PaletteCollapseState = { ...DEFAULT_COLLAPSED };
  let raw: string | null = null;
  try {
    raw = localStorage.getItem(PALETTE_STORAGE_KEY);
  } catch {
    return next;
  }
  if (raw === null) return next;
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return next;
  }
  if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) return next;
  const source = parsed as Record<string, unknown>;
  for (const id of PALETTE_GROUP_IDS) {
    const value = source[id];
    if (typeof value === 'boolean') next[id] = value;
  }
  return next;
}

/** 접힘 상태를 적는다. 쓰지 못해도 조용히 넘어간다 — 화면은 그대로 동작한다. */
export function writeCollapsed(state: PaletteCollapseState): void {
  try {
    localStorage.setItem(PALETTE_STORAGE_KEY, JSON.stringify(state));
  } catch {
    // 시크릿 창 · 용량 초과 · 접근 불가. 접힘 상태는 이번 세션에만 산다.
  }
}

/**
 * 접힘 상태와 그 토글. 초기값은 **한 번만** 읽는다(`useState` 의 지연 초기화) — 매 렌더마다
 * 저장소를 읽으면 도크가 다시 그려질 때마다 동기 I/O 가 붙는다.
 */
export function usePaletteCollapse(): {
  collapsed: PaletteCollapseState;
  toggle: (id: PaletteGroupId) => void;
} {
  const [collapsed, setCollapsed] = useState<PaletteCollapseState>(readCollapsed);
  const toggle = (id: PaletteGroupId): void => {
    setCollapsed((prev) => {
      const next = { ...prev, [id]: !prev[id] };
      writeCollapsed(next);
      return next;
    });
  };
  return { collapsed, toggle };
}
