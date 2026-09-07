// 패널 요소 편집의 **상태와 조작**을 한데 묶은 훅 — 통계·바·파이가 함께 쓴다.
//
// 세 패널이 필요로 하는 것은 같다: 고른 요소, 격자 붙임 여부, 기준 상자 실측, 정렬,
// 배치 초기화. 다른 것은 **오프셋을 어디에 저장하는가** 하나뿐이라, 그 부분만 호출자가
// 넘긴다(`writeOffsets`).
//
// 세 벌로 두지 않는 이유: 이 기능은 이미 겉으로 드러나지 않는 계산 실수로 세 번 고쳐졌다
// (절반 속도 · 스냅 기준 · 무리 이동). 사본이 셋이면 다음 실수는 세 곳에서 각각 발견된다.
//
// @spec SPEC-CHART-005 §2.1 [U1] / §2.4 [U4]

import { useCallback, useEffect, useRef, useState } from 'react';

import {
  computeAlignPatches,
  type AlignAxis,
  type AlignMode,
  type StatElementBox,
} from './panels/charts/panelEditAlign';
import {
  EMPTY_SELECTION,
  type StatSelection,
} from './panels/charts/panelEditSelection';

/** 한 요소의 현재 오프셋(패널 상자 대비 %). */
export interface ElementOffset {
  x: number;
  y: number;
}

/** 정렬·초기화가 내놓는 한 요소의 새 오프셋. `undefined` 는 "지운다" 는 뜻이다. */
export interface OffsetPatch<K extends string> {
  kind: K;
  x?: number | undefined;
  y?: number | undefined;
}

export interface PanelElementEdit<K extends string> {
  selection: StatSelection<K>;
  setSelection: (next: StatSelection<K>) => void;
  snap: boolean;
  setSnap: (next: boolean) => void;
  /** 기준 상자(`data-panel-bounds`)에 걸어야 하는 ref. 정렬이 여기서 상자를 잰다. */
  boundsRef: React.RefObject<HTMLDivElement | null>;
  align: (axis: AlignAxis, mode: AlignMode) => void;
  reset: () => void;
}

export function usePanelElementEdit<K extends string>({
  kinds,
  enabled,
  offsets,
  writeOffsets,
}: {
  /** 이 패널이 가진 요소 종류. 정렬 대상 후보이자 초기화 대상이다. */
  kinds: readonly K[];
  /** 편집이 켜져 있는가. 꺼지면 선택을 거둔다. */
  enabled: boolean;
  offsets: Readonly<Record<K, ElementOffset>>;
  /**
   * 오프셋 변경을 **한 번에** 저장한다.
   *
   * 나눠 넘기면 앞의 쓰기가 반영되기 전에 다음 계산이 옛 config 를 읽어 서로를
   * 덮어쓴다 — 정렬은 여러 요소를 동시에 움직이므로 이 점이 특히 중요하다.
   */
  writeOffsets: (patches: ReadonlyArray<OffsetPatch<K>>) => void;
}): PanelElementEdit<K> {
  // 둘 다 런타임 상태다 — config 에 저장하면 다음에 열 때도 무엇인가 골라져 있거나
  // 격자가 꺼져 있고, 그것을 되돌리는 방법이 화면에 없다.
  const [selection, setSelection] = useState<StatSelection<K>>(EMPTY_SELECTION);
  const [snap, setSnap] = useState(true);
  const boundsRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!enabled) setSelection(EMPTY_SELECTION);
  }, [enabled]);

  // 최신 값을 ref 로 잡아 둔다 — 콜백을 안정화해야 호출부가 매 렌더 새 함수를 받지 않는다.
  const stateRef = useRef({ kinds, offsets, selection, writeOffsets });
  stateRef.current = { kinds, offsets, selection, writeOffsets };

  const align = useCallback((axis: AlignAxis, mode: AlignMode): void => {
    const host = boundsRef.current;
    if (!host) return;
    const { kinds: ks, offsets: offs, selection: sel, writeOffsets: write } = stateRef.current;
    const bounds = host.getBoundingClientRect();
    // 2개 이상 골랐으면 **고른 것끼리** 맞춘다. 아니면 렌더된 전부를 맞춘다 —
    // 아무것도 고르지 않은 채 누른 것은 "다 맞춰라" 로 읽는 편이 자연스럽다.
    const targets = sel.size >= 2 ? ks.filter((k) => sel.has(k)) : ks;
    const boxes: StatElementBox<K>[] = [];
    for (const kind of targets) {
      const el = host.querySelector(`[data-panel-drag="${kind}"]`);
      if (!el) continue;
      const r = el.getBoundingClientRect();
      boxes.push({
        kind,
        rect: { left: r.left, top: r.top, width: r.width, height: r.height },
        offsetX: offs[kind].x,
        offsetY: offs[kind].y,
      });
    }
    const patches = computeAlignPatches(boxes, bounds, axis, mode);
    if (patches.length === 0) return;
    write(patches.map((p) => ({ kind: p.kind, x: p.offsetX, y: p.offsetY })));
  }, []);

  const reset = useCallback((): void => {
    const { kinds: ks, writeOffsets: write } = stateRef.current;
    write(ks.map((kind) => ({ kind, x: undefined, y: undefined })));
  }, []);

  return { selection, setSelection, snap, setSnap, boundsRef, align, reset };
}
