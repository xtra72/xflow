// 스크래치패드 저장소 — 전용 키 · 예산 · 저장 불가 (SPEC-CANVAS-008 M8 · REQ-05 · AC-08).
//
// **진짜 `localStorage` 로 잰다.** 저장소를 목(mock)으로 갈아 끼우면 직렬화 왕복이 사라져
// 필드가 사라지는 결함이 보이지 않고(D11), 무엇보다 **예외를 던지는 저장소**라는 이 SPEC 의
// 핵심 실패 모드를 흉내낼 자리가 없어진다. jsdom 의 `localStorage` 는 실물이므로 그대로
// 쓰고, 던지는 환경만 `Storage.prototype` 에 스파이를 걸어 만든다.
//
// @spec SPEC-CANVAS-008 REQ-05 · AC-08 · AC-E8

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { CanvasElement } from '../canvasConfig';
import {
  SCRATCHPAD_STORAGE_KEY,
  scratchpadStorage,
  useScratchpadStore,
} from './scratchpadStore';
import {
  SCRATCHPAD_MAX_ENTRIES,
  cloneElements,
  type ScratchpadEntry,
} from './scratchpadTypes';

// --- 고정 입력 -----------------------------------------------------------

/**
 * 요소 **둘**이고 서로 다른 자리에 있다. 하나짜리 묶음은 좌상단 계산의 결함을 감춘다(D2).
 * 좌상단은 (70, 30) — **원점이 아니다**.
 */
const BUNDLE: readonly CanvasElement[] = [
  { id: 'el-1', kind: 'rect', geometry: { x: 100, y: 30, w: 40, h: 20 }, style: { fill: '#123' } },
  {
    id: 'el-2',
    kind: 'path',
    geometry: { x: 70, y: 90, w: 50, h: 50 },
    path: [
      { c: 'M', x: 0, y: 0 },
      { c: 'L', x: 10000, y: 10000 },
      { c: 'Z' },
    ],
    style: { stroke: '#456' },
  },
];

function readStored(): { state?: { entries?: ScratchpadEntry[] } } | null {
  const raw = localStorage.getItem(SCRATCHPAD_STORAGE_KEY);
  return raw === null ? null : (JSON.parse(raw) as { state?: { entries?: ScratchpadEntry[] } });
}

function bulkEntries(count: number): ScratchpadEntry[] {
  return Array.from({ length: count }, (_, i) => ({
    id: `sp-${i + 1}`,
    name: `묶음 ${i + 1}`,
    created: 1_000 + i,
    origin: { x: 70, y: 30 },
    elements: cloneElements(BUNDLE),
  }));
}

beforeEach(() => {
  localStorage.clear();
  useScratchpadStore.setState({ entries: [], notice: null });
  localStorage.clear();
});

afterEach(() => {
  vi.restoreAllMocks();
});

// --- 저장 자리 -----------------------------------------------------------

describe('저장 자리는 전용 키다 (REQ-05 · 불변식 J5)', () => {
  it('항목은 `xflow-canvas-scratchpad` 에 놓이고 `xflow-ui` 는 손대지 않는다', () => {
    const before = localStorage.getItem('xflow-ui');
    useScratchpadStore.getState().saveEntry(BUNDLE);

    const stored = readStored();
    expect(stored?.state?.entries).toHaveLength(1);
    expect(stored?.state?.entries?.[0]?.elements.map((el) => el.id)).toEqual(['el-1', 'el-2']);
    // 남의 서랍을 건드리지 않는다 — 이 store 가 옆에 선 이유의 절반이다.
    expect(localStorage.getItem('xflow-ui')).toBe(before);
  });

  it('저장한 항목은 묶음의 좌상단을 함께 든다 (REQ-03)', () => {
    const { entry } = useScratchpadStore.getState().saveEntry(BUNDLE);
    // 기본값 함정: (0, 0) 짜리 묶음이면 좌상단을 아예 계산하지 않아도 통과한다.
    expect(entry?.origin).toEqual({ x: 70, y: 30 });
  });

  it('안내에 남는 것은 항목뿐이다 — 문구는 기기에 남지 않는다', () => {
    useScratchpadStore.getState().saveEntry(BUNDLE);
    expect(useScratchpadStore.getState().notice?.status).toBe('stored');
    const stored = readStored();
    expect(Object.keys(stored?.state ?? {})).toEqual(['entries']);
  });

  it('사본이라 원본을 고쳐도 서랍이 따라 바뀌지 않는다', () => {
    const source = cloneElements(BUNDLE);
    useScratchpadStore.getState().saveEntry(source);
    const first = source[0];
    if (first?.kind === 'rect') first.geometry.x = 9999;

    expect(useScratchpadStore.getState().entries[0]?.elements[0]?.geometry).toEqual({
      x: 100,
      y: 30,
      w: 40,
      h: 20,
    });
  });

  it('새것이 맨 앞이다', () => {
    useScratchpadStore.getState().saveEntry(BUNDLE);
    const second = useScratchpadStore.getState().saveEntry(BUNDLE);
    expect(useScratchpadStore.getState().entries[0]?.id).toBe(second.entry?.id);
  });
});

// --- 예산 ---------------------------------------------------------------

describe('예산은 거절하고 말한다 (REQ-05 · AC-08)', () => {
  it('항목 수 상한에서 저장을 거절하고 **오래된 것을 지우지 않는다**', () => {
    useScratchpadStore.setState({ entries: bulkEntries(SCRATCHPAD_MAX_ENTRIES) });
    const oldest = useScratchpadStore.getState().entries.at(-1)?.id;

    const result = useScratchpadStore.getState().saveEntry(BUNDLE);

    expect(result.status).toBe('limit-entries');
    expect(result.entry).toBeUndefined();
    expect(useScratchpadStore.getState().entries).toHaveLength(SCRATCHPAD_MAX_ENTRIES);
    // 조용한 축출이 없다 — 있으면 "저장했는데 나중에 보니 없더라" 가 된다.
    expect(useScratchpadStore.getState().entries.at(-1)?.id).toBe(oldest);
    expect(useScratchpadStore.getState().notice?.status).toBe('limit-entries');
  });

  it('바이트 상한에서도 거절한다 — 항목 수는 하나여도 그렇다', () => {
    // 문구 하나로 300KB 를 넘긴다. 한글이라 코드 단위 10만 · UTF-8 30만 바이트이며,
    // 순진한 `.length` 측정(10만 < 262144)이라면 이 시험이 빨개진다.
    const huge: CanvasElement[] = [
      { id: 'el-1', kind: 'text', geometry: { x: 0, y: 0 }, text: '가'.repeat(100_000), style: {} },
    ];
    const result = useScratchpadStore.getState().saveEntry(huge);

    expect(result.status).toBe('limit-bytes');
    expect(useScratchpadStore.getState().entries).toHaveLength(0);
    // 거절은 기기에도 아무것도 남기지 않는다. (안내를 쓰느라 블롭 자체는 생기지만
    // `partialize` 가 항목만 남기므로 그 안은 비어 있다 — 그 사실까지 여기서 잰다.)
    expect(readStored()?.state?.entries).toEqual([]);
  });

  it('빈 선택은 항목을 만들지 않는다 (REQ-03)', () => {
    const result = useScratchpadStore.getState().saveEntry([]);
    expect(result.status).toBe('empty');
    expect(useScratchpadStore.getState().entries).toHaveLength(0);
  });
});

// --- 저장 불가 -----------------------------------------------------------

describe('저장소를 쓸 수 없어도 예외가 밖으로 나가지 않는다 (REQ-05)', () => {
  it('`setItem` 이 던지면 항목은 메모리에 남고 화면이 그 사실을 안다', () => {
    // 시크릿 창 · 용량 초과가 정확히 이 자리다. zustand 의 `createJSONStorage` 는 이
    // 예외를 잡지 않으므로(실측), 어댑터가 잡지 않으면 `set()` 을 타고 액션 밖으로 나간다.
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('QuotaExceededError');
    });

    const result = useScratchpadStore.getState().saveEntry(BUNDLE);

    expect(result.status).toBe('volatile');
    expect(result.entry).toBeDefined();
    // 메모리에는 들어갔다 — 이번 세션에서는 되쓸 수 있다.
    expect(useScratchpadStore.getState().entries).toHaveLength(1);
    expect(useScratchpadStore.getState().notice?.status).toBe('volatile');
  });

  it('`getItem` 이 던져도 재수화가 던지지 않는다', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new DOMException('SecurityError');
    });
    expect(() => scratchpadStorage.getItem(SCRATCHPAD_STORAGE_KEY)).not.toThrow();
    expect(scratchpadStorage.getItem(SCRATCHPAD_STORAGE_KEY)).toBeNull();
  });

  it('`removeItem` 이 던져도 밖으로 나가지 않는다', () => {
    vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => {
      throw new DOMException('SecurityError');
    });
    expect(() => scratchpadStorage.removeItem(SCRATCHPAD_STORAGE_KEY)).not.toThrow();
  });

  it('저장소가 죽어도 이름 바꾸기 · 지우기는 그대로 돈다', () => {
    useScratchpadStore.getState().saveEntry(BUNDLE);
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('QuotaExceededError');
    });
    const id = useScratchpadStore.getState().entries[0]?.id ?? '';

    expect(() => useScratchpadStore.getState().renameEntry(id, '펌프')).not.toThrow();
    expect(useScratchpadStore.getState().entries[0]?.name).toBe('펌프');
    expect(() => useScratchpadStore.getState().removeEntry(id)).not.toThrow();
    expect(useScratchpadStore.getState().entries).toHaveLength(0);
  });
});

// --- 손상 자료 -----------------------------------------------------------

describe('손상된 저장 자료는 읽을 수 있는 것만 살린다 (REQ-05)', () => {
  it('JSON 이 깨져 있으면 빈 서랍이다', () => {
    localStorage.setItem(SCRATCHPAD_STORAGE_KEY, '{not json');
    expect(scratchpadStorage.getItem(SCRATCHPAD_STORAGE_KEY)).toBeNull();
  });

  it('항목 하나가 깨져 있으면 나머지가 살아난다', () => {
    localStorage.setItem(
      SCRATCHPAD_STORAGE_KEY,
      JSON.stringify({
        version: 0,
        state: {
          entries: [
            { id: 'sp-1', name: '성한 것', created: 1, origin: { x: 70, y: 30 }, elements: BUNDLE },
            { id: 'sp-2', elements: 'nope' },
            { id: 'sp-3', name: '', created: 2, origin: { x: 0, y: 0 }, elements: BUNDLE },
          ],
        },
      }),
    );
    const read = scratchpadStorage.getItem(SCRATCHPAD_STORAGE_KEY) as {
      state: { entries: ScratchpadEntry[] };
    } | null;
    expect(read?.state.entries.map((e) => e.id)).toEqual(['sp-1', 'sp-3']);
  });

  it('블롭의 형상이 어긋나도 던지지 않는다', () => {
    for (const raw of ['null', '[]', '"x"', '{"state":3}', '{"state":{"entries":"x"}}']) {
      localStorage.setItem(SCRATCHPAD_STORAGE_KEY, raw);
      expect(() => scratchpadStorage.getItem(SCRATCHPAD_STORAGE_KEY), raw).not.toThrow();
    }
    // 형상이 서면 빈 목록으로 선다.
    localStorage.setItem(SCRATCHPAD_STORAGE_KEY, '{"state":{"entries":"x"}}');
    const read = scratchpadStorage.getItem(SCRATCHPAD_STORAGE_KEY) as {
      state: { entries: ScratchpadEntry[] };
    };
    expect(read.state.entries).toEqual([]);
  });
});

// --- 목록 편집 -----------------------------------------------------------

describe('이름 바꾸기와 지우기', () => {
  it('이름을 고치면 그 항목만 바뀐다', () => {
    useScratchpadStore.setState({ entries: bulkEntries(3) });
    useScratchpadStore.getState().renameEntry('sp-2', '펌프 조립');
    expect(useScratchpadStore.getState().entries.map((e) => e.name)).toEqual([
      '묶음 1',
      '펌프 조립',
      '묶음 3',
    ]);
  });

  it('지우면 그 항목만 사라지고 기기에도 반영된다', () => {
    useScratchpadStore.setState({ entries: bulkEntries(3) });
    useScratchpadStore.getState().removeEntry('sp-2');
    expect(readStored()?.state?.entries?.map((e) => e.id)).toEqual(['sp-1', 'sp-3']);
  });

  it('같은 결과가 잇달아도 안내가 새것임을 화면이 알아챈다', () => {
    // 일련번호가 없으면 "50 을 넘었습니다" 를 닫은 뒤 다시 눌렀을 때 아무 반응이 없다.
    useScratchpadStore.setState({ entries: bulkEntries(SCRATCHPAD_MAX_ENTRIES) });
    useScratchpadStore.getState().saveEntry(BUNDLE);
    const first = useScratchpadStore.getState().notice;
    useScratchpadStore.getState().saveEntry(BUNDLE);
    const second = useScratchpadStore.getState().notice;
    expect(second?.status).toBe(first?.status);
    expect(second?.seq).toBeGreaterThan(first?.seq ?? 0);
  });

  it('안내를 닫으면 사라진다', () => {
    useScratchpadStore.getState().saveEntry(BUNDLE);
    useScratchpadStore.getState().clearNotice();
    expect(useScratchpadStore.getState().notice).toBeNull();
  });
});
