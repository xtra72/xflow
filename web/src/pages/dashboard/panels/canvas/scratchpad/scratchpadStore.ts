// 스크래치패드 저장소 — 기기 지역 영속과 예산 (SPEC-CANVAS-008 M8 · REQ-05).
//
// **`uiStore` 에 얹지 않고 옆에 세운다.** 이유 둘을 이름으로 적는다.
//
// (1) 스크래치패드는 **환경설정이 아니라 저술**이다. `uiStore` 의 `partialize` 는 스스로
//     제 범위를 못박아 두었다 — "기기별 환경설정만 영속한다". 도형 서랍을 그 안에 넣는
//     순간 그 파일이 그은 경계가 흐려지고, 다음 사람은 무엇이 그 store 에 들어가도 되는지
//     판단할 근거를 잃는다.
// (2) `xflow-ui` 블롭은 **부팅 때 테마를 읽으려고 통째로 역직렬화된다.** 서랍을 그 안에
//     넣으면 첫 그림이 서랍 크기(최대 256KB)만큼 늦어진다. 서랍은 도크가 열릴 때 처음
//     읽히면 되고, 이 store 는 그 자리에서 처음 구독된다.
//
// **패널 config 도 대시보드 snapshot 도 서버도 아니다**(REQ-05 · 불변식 J5). config 에
// 넣으면 다른 캔버스 패널에서 꺼낼 수 없고(재사용 재료라는 목적이 사라진다), 남의 256KB
// 예산을 먹으며, 대시보드를 **보는** 사람의 페이로드에 남의 작업 서랍이 실린다. 서버에
// 넣으면 004·005·006 이 나란히 지켜 온 백엔드 변경 0 이 깨진다 — 되돌릴 수 없는 결정이
// 아니라 나중에 얹을 수 있는 결정이므로 지금 치를 값이 아니다.
//
// **대가를 숨기지 않는다**: 다른 브라우저·다른 기기로 따라가지 않고, 저장소를 비우면
// 사라지며, 시크릿 창에서는 창을 닫을 때 사라진다. 셋 다 화면이 말해야 한다(REQ-06).
//
// **예외를 밖으로 내지 않는다**(REQ-05). zustand 의 `createJSONStorage` 는 `getItem` /
// `setItem` 의 예외를 잡지 않는다(실측: `node_modules/zustand/esm/middleware.mjs`) —
// `setItem` 의 예외는 `set()` 을 타고 액션 밖으로 나간다. 시크릿 창과 용량 초과가 정확히
// 그 자리이므로 저장소 어댑터를 직접 적고 세 메서드를 전부 감싼다. 그림과 편집은 서랍이
// 죽어도 그대로 돌아야 한다.
//
// @spec SPEC-CANVAS-008 REQ-05 · AC-08 · AC-E8

import { create } from 'zustand';
import { persist, type PersistStorage, type StorageValue } from 'zustand/middleware';

import type { CanvasElement } from '../canvasConfig';
import {
  SCRATCHPAD_MAX_BYTES,
  SCRATCHPAD_MAX_ENTRIES,
  cloneElements,
  elementsOrigin,
  nextScratchpadId,
  parseScratchpad,
  scratchpadBytes,
  type ScratchpadEntry,
} from './scratchpadTypes';

// --- 저장 자리 -----------------------------------------------------------

/** 전용 persist 키. `xflow-ui` 와 나누지 않는다(위 머리말). */
export const SCRATCHPAD_STORAGE_KEY = 'xflow-canvas-scratchpad';

/** 영속되는 몫 — **항목뿐**이다. 안내 문구는 이번 세션의 것이라 남기지 않는다. */
interface ScratchpadPersisted {
  entries: ScratchpadEntry[];
}

/**
 * 마지막 쓰기가 실패했는가.
 *
 * 모듈 지역 플래그인 것에 뜻이 있다: persist 의 쓰기는 `set()` **안에서** 동기로 일어나
 * 므로, 액션이 `set()` 을 부른 직후에 이 값을 읽으면 그 쓰기의 결과를 본다. 상태 안에
 * 두면 쓰기 도중에 상태를 또 쓰는 재진입이 된다.
 */
let writeFailed = false;

/**
 * 저장소 어댑터. **세 메서드가 전부 예외를 삼킨다.**
 *
 * `getItem` 이 관용 파서를 지나는 것이 이 어댑터의 요지다 — 손상된 항목 하나가 서랍
 * 전체를 비우지 않는다(REQ-05). JSON 자체가 깨졌으면 살릴 것이 없으므로 빈 서랍이다.
 */
export const scratchpadStorage: PersistStorage<ScratchpadPersisted> = {
  getItem(name: string): StorageValue<ScratchpadPersisted> | null {
    let raw: string | null;
    try {
      raw = globalThis.localStorage.getItem(name);
    } catch {
      // 시크릿 창 · 접근 불가. 서랍은 이번 세션에만 산다.
      return null;
    }
    if (raw === null) return null;
    let parsed: unknown;
    try {
      parsed = JSON.parse(raw);
    } catch {
      return null;
    }
    if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) return null;
    const blob = parsed as Record<string, unknown>;
    const state = blob.state;
    const entries =
      state !== null && typeof state === 'object' && !Array.isArray(state)
        ? (state as Record<string, unknown>).entries
        : undefined;
    return {
      state: { entries: parseScratchpad(entries) },
      version: typeof blob.version === 'number' ? blob.version : 0,
    };
  },
  setItem(name: string, value: StorageValue<ScratchpadPersisted>): void {
    try {
      globalThis.localStorage.setItem(name, JSON.stringify(value));
      writeFailed = false;
    } catch {
      // 용량 초과 · 시크릿 창 · 접근 불가. 던지지 않는다 — 액션이 플래그로 읽는다.
      writeFailed = true;
    }
  },
  removeItem(name: string): void {
    try {
      globalThis.localStorage.removeItem(name);
    } catch {
      // 지우지 못해도 화면은 그대로 동작한다.
    }
  },
};

// --- 결과 ---------------------------------------------------------------

/**
 * 저장 시도의 결과. **거절도 결과다** — 조용히 실패하지 않는다(REQ-05).
 *
 * `stored` 는 "성공했고 기기에도 남았다", `volatile` 은 "메모리에는 들어갔지만 기기에
 * 남기지 못했다" 다. 둘을 가르는 것이 시크릿 창의 사용자에게 정직한 유일한 길이다 —
 * 합치면 서랍이 다음 세션에 사라지는 것을 아무도 미리 말해 주지 않는다.
 */
export type ScratchpadSaveStatus =
  | 'stored'
  | 'volatile'
  | 'empty'
  | 'limit-entries'
  | 'limit-bytes';

export interface ScratchpadSaveResult {
  status: ScratchpadSaveStatus;
  /** 만들어진 항목. 거절이면 없다. */
  entry?: ScratchpadEntry;
}

/** 화면이 말할 거리. 저장 시도마다 갈아 끼운다. */
export interface ScratchpadNotice {
  status: ScratchpadSaveStatus;
  /** 같은 결과가 잇달아도 화면이 다시 알아채도록 하는 일련번호. */
  seq: number;
}

interface ScratchpadState extends ScratchpadPersisted {
  notice: ScratchpadNotice | null;
  /**
   * 고른 요소들의 **사본**을 항목 하나로 넣는다 — **저장 경로는 이 함수 하나뿐**이다
   * (불변식 J11). 끌어 넣기와 단추가 다른 함수를 부르면 "끌어 넣은 것과 단추로 넣은 것이
   * 다르다" 가 생기고, 그 차이는 서랍을 열어 보기 전까지 드러나지 않는다.
   */
  saveEntry: (elements: readonly CanvasElement[]) => ScratchpadSaveResult;
  renameEntry: (id: string, name: string) => void;
  removeEntry: (id: string) => void;
  clearNotice: () => void;
}

/** 안내 일련번호. 상태 밖에 두어 영속 대상이 아님을 형상으로 못박는다. */
let noticeSeq = 0;

function notice(status: ScratchpadSaveStatus): ScratchpadNotice {
  noticeSeq += 1;
  return { status, seq: noticeSeq };
}

export const useScratchpadStore = create<ScratchpadState>()(
  persist(
    (set, get) => ({
      entries: [],
      notice: null,

      saveEntry(elements) {
        // 빈 선택은 항목을 만들지 않는다(REQ-03). 단추는 이 상태에서 이미 꺼져 있으나,
        // 끌어 넣기는 단추를 지나지 않으므로 판정이 여기에도 있어야 한다.
        if (elements.length === 0) {
          set({ notice: notice('empty') });
          return { status: 'empty' };
        }

        const entries = get().entries;
        if (entries.length >= SCRATCHPAD_MAX_ENTRIES) {
          // **오래된 것을 지우지 않는다.** 거절하고 말한다(REQ-05).
          set({ notice: notice('limit-entries') });
          return { status: 'limit-entries' };
        }

        const copy = cloneElements(elements);
        const entry: ScratchpadEntry = {
          id: nextScratchpadId(entries),
          // 이름은 비워 둔다 — 자동 이름은 번역이 필요하고, 저장 시점의 로케일로 굳히면
          // 언어를 바꾼 뒤에도 옛 이름이 서랍에 남는다(`scratchpadTypes` §name).
          name: '',
          created: Date.now(),
          origin: elementsOrigin(copy),
          elements: copy,
        };

        // **새것이 맨 앞이다.** 도크는 폭 `w-44` 의 좁은 목록이라, 방금 넣은 것이 스크롤
        // 아래에 있으면 저장이 되었는지조차 화면이 답하지 못한다.
        const next = [entry, ...entries];
        if (scratchpadBytes(next) > SCRATCHPAD_MAX_BYTES) {
          set({ notice: notice('limit-bytes') });
          return { status: 'limit-bytes' };
        }

        writeFailed = false;
        set({ entries: next });
        // persist 의 쓰기는 위 `set()` 안에서 동기로 끝났다.
        const status: ScratchpadSaveStatus = writeFailed ? 'volatile' : 'stored';
        set({ notice: notice(status) });
        return { status, entry };
      },

      renameEntry(id, name) {
        set({ entries: get().entries.map((e) => (e.id === id ? { ...e, name } : e)) });
      },

      removeEntry(id) {
        set({ entries: get().entries.filter((e) => e.id !== id) });
      },

      clearNotice() {
        set({ notice: null });
      },
    }),
    {
      name: SCRATCHPAD_STORAGE_KEY,
      storage: scratchpadStorage,
      // 안내 문구와 액션은 기기에 남지 않는다 — 남는 것은 저술뿐이다.
      partialize: (state) => ({ entries: state.entries }),
    },
  ),
);
