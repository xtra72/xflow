// 영역 선택(마키)의 순수 기하 + 집합 산술 시험 (SPEC-CANVAS-009).
//
// **jsdom 없이 돈다** — `previewPan` 의 순수 기하 셋이 세운 그 관용구다. 사각형 산술과
// 합집합은 DOM 을 보지 않으므로, 레이아웃을 하지 않는 jsdom 에 기대어 재는 대신 여기서
// 값으로 못박는다. 오버레이 쪽 시험(`CanvasEditOverlay.marquee.test.tsx`)은 그 값이
// **실제 몸짓으로 닿는지**를 따로 잰다(시험 규율 D6 — 경로와 구조를 한 시험이 겸하지 않는다).
//
// ## 고정 입력을 고른 방식 — 네 함정을 피해 간다
//
// 이 주제에서 초록을 거짓으로 만드는 고정 입력이 넷 있고, 아래 `FIXTURE` 는 그 넷을 모두
// 피한다.
//   1. **전부 덮는 사각형** — 포함 판정이 죽어 있어도(언제나 참) 통과한다. 그래서 어떤
//      사각형도 요소를 전부 감싸지 않는다.
//   2. **아무것도 덮지 않는 사각형** — "비우는가 / 남기는가" 가 갈리지 않는다.
//   3. **오른쪽·아래로만 끈 드래그** — `marqueeRect` 의 정규화가 없어도 통과한다.
//      그래서 네 방향을 모두 잰다.
//   4. **크기가 같고 나란히 붙은 요소들** — 부분 덮임이 드러나지 않는다. 그래서 크기를
//      서로 다르게 두고, **변에 걸친 것**(`edge`)을 하나 심는다.
//
// 여기에 이 저장소가 이미 데인 둘을 더한다.
//   - **원소가 하나뿐인 고정 입력은 합집합 산술을 숨긴다.** 그래서 바탕 선택에 사각형
//     **안**의 것과 **밖**의 것을 함께 둔다.
//   - **바깥 것을 배열 끝에 두면 자르기 결함이 드러나지 않는다.** 그래서 `outFar` 를
//     배열 **한가운데**에 둔다.

import { describe, expect, it } from 'vitest';

import type { CanvasElement } from './canvasConfig';
import type { PxBox } from './canvasGeometry';
import type { CanvasNode, GroupElement } from './group/groupTypes';
import {
  enclosesBox,
  marqueeCandidates,
  marqueeRect,
  marqueeSelection,
  type MarqueeCandidate,
} from './canvasMarquee';

// --- 고정 입력 -----------------------------------------------------------

function box(x: number, y: number, w: number, h: number): PxBox {
  return { x, y, w, h };
}

/** 판정 사각형 — 0..100 의 정사각. 어떤 요소도 전부 담지 않는다(함정 1). */
const RECT = box(0, 0, 100, 100);

/**
 * 후보 다섯. 크기가 **서로 다르고**(함정 4) 관계가 셋으로 갈린다:
 * 온전히 안 · 변에 걸침 · 완전히 밖. `outFar` 가 한가운데인 것에 뜻이 있다.
 */
const FIXTURE: readonly MarqueeCandidate[] = [
  { nodeId: 'inSmall', box: box(10, 10, 20, 20) },
  { nodeId: 'edge', box: box(90, 40, 30, 20) }, // 오른쪽 변을 20px 넘는다 — 부분 덮임.
  { nodeId: 'outFar', box: box(200, 200, 10, 10) }, // 배열 한가운데의 바깥 것.
  { nodeId: 'inWide', box: box(40, 50, 50, 40) },
  { nodeId: 'flush', box: box(0, 0, 100, 100) }, // 네 변이 정확히 맞닿는다.
];

/** 사각형이 실제로 고르는 셋 — 위 고정 입력에서 손으로 센 값이다. */
const ENCLOSED = ['inSmall', 'inWide', 'flush'];

function rectEl(id: string, visible?: boolean): CanvasElement {
  return {
    id,
    kind: 'rect',
    geometry: { x: 0, y: 0, w: 10, h: 10 },
    style: visible === undefined ? {} : { visible },
  };
}

function groupEl(id: string, visible?: boolean): GroupElement {
  return {
    id,
    kind: 'group',
    geometry: { x: 0, y: 0, w: 10, h: 10 },
    parts: [],
    ...(visible === undefined ? {} : { style: { visible } }),
  };
}

/** 후보를 만들 때 상자를 어디서 받는지만 보는 눈 — 투영은 이 모듈의 것이 아니다. */
const STUB_BOX = box(1, 2, 3, 4);
const stubBoxOf = (): PxBox => STUB_BOX;

// --- 사각형 정규화 --------------------------------------------------------

describe('marqueeRect — 어느 방향으로 끌어도 같은 사각형이다', () => {
  const A = { x: 20, y: 30 };
  const B = { x: 70, y: 90 };
  const EXPECTED = box(20, 30, 50, 60);

  // 네 방향을 **전부** 잰다(함정 3). 오른쪽·아래로만 끄는 시험은 정규화를 통째로
  // 지워도 초록이다 — 그 한 방향에서는 `min` 과 `abs` 가 항등이기 때문이다.
  it.each([
    ['오른쪽 아래로', A, B],
    ['왼쪽 위로', B, A],
    ['오른쪽 위로', { x: A.x, y: B.y }, { x: B.x, y: A.y }],
    ['왼쪽 아래로', { x: B.x, y: A.y }, { x: A.x, y: B.y }],
  ])('%s 끌어도 같은 값이다', (_name, from, to) => {
    expect(marqueeRect(from, to)).toEqual(EXPECTED);
  });

  it('움직임이 없으면 넓이 0 인 사각형이다 — 음수가 아니다', () => {
    expect(marqueeRect(A, A)).toEqual(box(20, 30, 0, 0));
  });
});

// --- 포함 판정 -------------------------------------------------------------

describe('enclosesBox — 온전히 든 것만 참이다', () => {
  it('안쪽 상자는 참이고, 변에 걸친 넷은 모두 거짓이다', () => {
    expect(enclosesBox(RECT, box(10, 10, 20, 20))).toBe(true);
    // 네 변을 **각각** 넘겨 본다. 한 변만 재면 그 축만 비교하는 구현이 통과한다.
    expect(enclosesBox(RECT, box(-1, 10, 20, 20)), '왼쪽으로 넘침').toBe(false);
    expect(enclosesBox(RECT, box(10, -1, 20, 20)), '위로 넘침').toBe(false);
    expect(enclosesBox(RECT, box(90, 10, 20, 20)), '오른쪽으로 넘침').toBe(false);
    expect(enclosesBox(RECT, box(10, 90, 20, 20)), '아래로 넘침').toBe(false);
  });

  it('변이 정확히 맞닿으면 든 것이다 — 사각형 자신도 자신에게 든다', () => {
    expect(enclosesBox(RECT, RECT)).toBe(true);
  });

  it('완전히 밖은 거짓이고, 사각형을 **품는** 큰 상자도 거짓이다', () => {
    expect(enclosesBox(RECT, box(200, 200, 10, 10))).toBe(false);
    // 겹침 판정으로 잘못 구현하면 이것이 참이 된다 — 포함과 겹침이 갈리는 자리다.
    expect(enclosesBox(RECT, box(-50, -50, 300, 300))).toBe(false);
  });

  it('넓이 0 사각형은 같은 자리의 퇴화 상자만 담는다', () => {
    const point = box(20, 30, 0, 0);
    expect(enclosesBox(point, point)).toBe(true);
    expect(enclosesBox(point, box(20, 30, 1, 0))).toBe(false);
  });

  it('비유한 좌표는 **아무것도 고르지 않는** 쪽으로 떨어진다', () => {
    // `NaN` 과의 비교는 모두 거짓이므로 판정이 거짓이 된다. 방어의 방향이 안전한 쪽임을
    // 값으로 못박는다 — 반대로 떨어지면 손상된 포인터 하나가 캔버스를 통째로 고른다.
    expect(enclosesBox(box(Number.NaN, 0, 100, 100), box(10, 10, 5, 5))).toBe(false);
    expect(enclosesBox(RECT, box(Number.NaN, 10, 5, 5))).toBe(false);
    expect(enclosesBox(box(0, 0, Number.POSITIVE_INFINITY, 100), box(10, 10, 5, 5))).toBe(true);
  });
});

// --- 후보 고르기 -----------------------------------------------------------

describe('marqueeCandidates — `hitTest` 가 건너뛰는 것을 그대로 건너뛴다', () => {
  it('보이지 않는 최상위 요소는 빠지고, 나머지는 상자를 받아 남는다', () => {
    const nodes: CanvasNode[] = [rectEl('shown'), rectEl('hidden', false), rectEl('explicit', true)];

    const out = marqueeCandidates(nodes, stubBoxOf);

    expect(out.map((c) => c.nodeId)).toEqual(['shown', 'explicit']);
    // 상자는 **넘겨받은 것 그대로**다 — 이 모듈이 제 투영을 지으면 두 번째 측정원이 된다.
    expect(out[0]?.box).toBe(STUB_BOX);
  });

  it('그룹은 제 `style.visible` 을 보지 않는다 — `hitsGroup` 과 같은 결정이다', () => {
    // 그리는 쪽(`drawElements`)도 그룹 자신의 visible 을 보지 않는다. 여기서만 보면
    // "그려지는데 골라지지 않는" 어긋남이 이 경로에만 생긴다.
    const nodes: CanvasNode[] = [groupEl('g-hidden', false), groupEl('g-plain')];

    expect(marqueeCandidates(nodes, stubBoxOf).map((c) => c.nodeId)).toEqual([
      'g-hidden',
      'g-plain',
    ]);
  });

  it('그룹은 부품이 아니라 **제 상자 하나**로 참여한다 — 선택 키는 `nodeId` 다', () => {
    const group = groupEl('g');
    group.parts = [rectEl('p1'), rectEl('p2')];

    const out = marqueeCandidates([group], stubBoxOf);

    // 부품 둘이 있어도 후보는 하나이고 그 키는 그룹 id 다(REQ-06).
    expect(out).toHaveLength(1);
    expect(out[0]?.nodeId).toBe('g');
  });

  it('빈 배열은 빈 후보다 — 조용히 무언가를 만들어 내지 않는다', () => {
    expect(marqueeCandidates([], stubBoxOf)).toEqual([]);
  });
});

// --- 합집합 ---------------------------------------------------------------

describe('marqueeSelection — 바탕과의 합집합이다', () => {
  it('빈 바탕에서는 **온전히 든 것만** 남는다 (부분 덮임은 빠진다)', () => {
    const next = marqueeSelection(FIXTURE, RECT, new Set());

    expect([...next].sort()).toEqual([...ENCLOSED].sort());
    // 변에 걸친 것과 완전히 밖의 것이 각각 빠졌다는 것을 이름으로 적는다.
    expect(next.has('edge'), '변에 걸친 것은 빠진다').toBe(false);
    expect(next.has('outFar'), '밖의 것은 빠진다').toBe(false);
  });

  it('바탕의 **밖에 있는** 항목은 사각형이 건드리지 않는다 (합집합의 좌변)', () => {
    const next = marqueeSelection(FIXTURE, RECT, new Set(['outFar']));

    expect(next.has('outFar')).toBe(true);
    expect([...next].sort()).toEqual([...ENCLOSED, 'outFar'].sort());
  });

  it('사각형이 덮은 것이 **이미 골라져 있어도 풀리지 않는다** — 토글이 아니다', () => {
    // `nextSelection(.., additive=true)` 를 요소마다 부르면 여기서 `inSmall` 이 **빠진다**.
    // 감싼 것이 풀리는 마키는 사용자가 이해할 수 없는 그림이므로, 그 구현을 이 한 줄이 막는다.
    const next = marqueeSelection(FIXTURE, RECT, new Set(['inSmall']));

    expect(next.has('inSmall')).toBe(true);
    expect([...next].sort()).toEqual([...ENCLOSED].sort());
  });

  it('더한 것이 없으면 **바탕을 그대로** 돌려준다 (참조가 같다)', () => {
    const base = new Set(['outFar']);
    // 아무것도 담지 않는 사각형.
    const next = marqueeSelection(FIXTURE, box(500, 500, 10, 10), base);

    // 값이 같은 새 Set 을 돌려주면 React 가 매 이동마다 헛되이 다시 그린다.
    expect(next).toBe(base);
  });

  it('이미 전부 골라 둔 뒤 같은 사각형을 다시 그으면 참조가 그대로다', () => {
    const base: ReadonlySet<string> = new Set(ENCLOSED);

    expect(marqueeSelection(FIXTURE, RECT, base)).toBe(base);
  });

  it('사각형을 줄이면 빠져나간 것이 실제로 풀린다 (바탕에서 매번 다시 센다)', () => {
    // 넓은 사각형 → 좁은 사각형. 직전 결과에 얹는 구현이면 `inWide` 가 남는다.
    const wide = marqueeSelection(FIXTURE, RECT, new Set());
    expect(wide.has('inWide')).toBe(true);

    const narrow = marqueeSelection(FIXTURE, box(0, 0, 35, 35), new Set());

    expect([...narrow]).toEqual(['inSmall']);
  });

  it('바탕이 빈 Set 이고 담은 것도 없으면 그 빈 Set 이 그대로 나온다', () => {
    const base = new Set<string>();
    expect(marqueeSelection([], RECT, base)).toBe(base);
  });
});
