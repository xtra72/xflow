// 그룹이 설정 목록에서 보이고 다스려진다 (SPEC-CANVAS-004 M6 — REQ-08 목록 조항).
//
// ## 이 파일이 재현하는 결함
//
// 사용자 보고 둘("텍스트 빠짐" · "그룹 선택 안됨")은 **원인이 하나다**:
// `CanvasElementsEditor` 가 `isGroup(el)` 인 노드에 대해 행을 그리지 않고 건너뛰었다.
// 가져온 것을 묶는 순간 (a) 부품이 된 문구의 행이 사라지고(그 요소는 이제 최상위가
// 아니다) (b) 그룹 자신에게는 애초에 행이 없어, **목록이 통째로 빈다.** 결함은 하나인데
// 증상이 둘이었다.
//
// ## 고정 입력이 지키는 함정 (acceptance.md §시험 규율)
//
//   - **E-A** 부품이 셋이고 상자가 서로 다르며, 하나는 가장자리에 닿지 않는다.
//     그리고 **부품 하나짜리 그룹**을 따로 둔다 — 손으로 저술한 config 에는 올 수 있다
//     (`groupNodes` 가 거절하는 것은 **만드는 것**이지 **읽는 것**이 아니다).
//   - **E-C · E-D** 그룹 상자는 `317 × 181` 이고 원점이 `(73, 41)` 이다.
//   - **E-L** 모든 그룹 고정 입력에 **문구 부품**이 하나 있다 — 사용자가 잃었다고 보고한
//     그 요소이며, 도형만 든 고정 입력은 그 소실을 재현하지 못한다.
//   - **목록의 짜임** 함정 — 그룹 하나만 든 목록은 **최상위 행과 그룹 행이 섞이는 자리**를
//     감춘다. 그래서 `[사각형, 그룹, 타원]` 을 함께 둔다: 앞에도 뒤에도 형제가 있어야
//     "첫 자리에 끼우기" 와 "끝에 붙이기" 가 같은 배열을 내지 않는다.
//   - **접힘** 함정 — 접힌 그룹은 그 아래 전부를 감춘다. 부품이 없음을 재는 단언은
//     **먼저 그 행이 펼쳐져 있음(또는 접혀 있음)을 단언**해야 옳은 이유로 초록이 된다.
//
// ## 이 파일이 단언하는 **없음** 둘과 그 근거
//
//   - 부품 행에 **수치 칸이 없다.** 부품 기하는 그룹 로컬 정수 격자(0..`GROUP_LOCAL_EXTENT`)
//     이고 004 는 역방향 중첩 투영(`unproject*In`)을 **금지**했으므로(REQ-05 · A18), 칸을
//     세우면 그 숫자는 화면 어디에도 설명이 없는 단위가 된다. 대신 안내 한 줄이 그 사실과
//     고치는 길(그룹 해제)을 말한다.
//   - 목록에 **그룹/그룹 해제 단추가 없다.** 그 둘은 이미 두 표면(도크 · 떠 있는 줄)에
//     `CanvasGroupTools` 로 서 있고(M6), 한 값에 살아 있는 컨트롤이 둘이면 안 된다(I24).
//     특히 풀기는 규칙 손실 확인 절차를 함께 들고 있어, 확인 없는 두 번째 입구가 생기면
//     "설정에서는 물어보는데 목록에서는 그냥 풀린다" 가 표현 가능해진다.
//
// @spec SPEC-CANVAS-004 REQ-04 · REQ-08

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import type { CanvasElement } from './canvasConfig';
import CanvasElementsEditor from './CanvasElementsEditor';
import {
  CanvasEditSelectionContext,
  useCanvasEditSelectionState,
  type CanvasSelection,
} from './canvasEditContext';
import { isGroup, type CanvasNode } from './group/groupTypes';

afterEach(cleanup);

// --- 고정 입력 -------------------------------------------------------------

/**
 * 부품 셋 — 상자가 서로 다르고, `stem` 은 가장자리에 닿지 않으며, 문구가 하나 있다.
 *
 * 좌표는 **그룹 로컬 정수 격자**다(캔버스 단위가 아니다). 이 파일이 수치 칸의 부재를
 * 단언하는 이유가 정확히 이 단위 차이다.
 */
const PARTS: readonly CanvasElement[] = [
  { id: 'body', kind: 'rect', geometry: { x: 0, y: 0, w: 3000, h: 2000 }, style: {} },
  { id: 'stem', kind: 'ellipse', geometry: { x: 4100, y: 3300, w: 1700, h: 900 }, style: {} },
  { id: 'label', kind: 'text', geometry: { x: 5000, y: 9200 }, style: {}, text: '온도' },
];

function groupNode(over: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 'grp-1',
    kind: 'group',
    // E-C · E-D — 원점이 0 이 아니고, 두 축이 다르며, 변이 10000 을 나누어떨어뜨리지 않는다.
    geometry: { x: 73, y: 41, w: 317, h: 181 },
    parts: PARTS.map((p) => ({ ...p })),
    ...over,
  };
}

function rectNode(id = 'r1'): Record<string, unknown> {
  return { id, kind: 'rect', geometry: { x: 50, y: 80, w: 150, h: 160 }, style: {} };
}

function ellipseNode(id = 'e1'): Record<string, unknown> {
  return { id, kind: 'ellipse', geometry: { x: 300, y: 200, w: 40, h: 40 }, style: {} };
}

function cfg(elements: readonly unknown[]): Record<string, unknown> {
  return { channel_name: '', data_source: 'store', canvas: { width: 500, height: 400 }, elements };
}

// --- 하네스 ---------------------------------------------------------------

function setup(config: Record<string, unknown>) {
  const onConfigChange = vi.fn();
  render(<CanvasElementsEditor config={config} onConfigChange={onConfigChange} />);
  return onConfigChange;
}

/** 마지막 패치의 노드 배열. */
function lastNodes(spy: ReturnType<typeof vi.fn>): CanvasNode[] {
  expect(spy).toHaveBeenCalled();
  const patch = spy.mock.calls[spy.mock.calls.length - 1]![0] as Record<string, unknown>;
  return patch.elements as CanvasNode[];
}

/** 그룹 행을 펼친다. 전제(접혀 있었다)를 함께 단언한다. */
function expandGroup(idx: number): HTMLElement {
  const toggle = screen.getByTestId(`canvas-group-row-toggle-${idx}`);
  expect(toggle, '전제 — 그룹 행은 접힌 채로 태어난다').toHaveAttribute('aria-expanded', 'false');
  fireEvent.click(toggle);
  expect(toggle).toHaveAttribute('aria-expanded', 'true');
  return toggle;
}

// --- 재현 -----------------------------------------------------------------

describe('그룹은 목록에서 제 행을 갖는다 (재현 — REQ-08)', () => {
  it('그룹만 든 목록이 비지 않는다 — 행이 없으면 묶는 순간 목록이 통째로 빈다', () => {
    setup(cfg([groupNode()]));
    // 전제 — "요소가 없습니다" 안내가 뜨는 상태가 아니다(배열에 노드가 하나 있다).
    expect(screen.queryByTestId('canvas-element-empty')).toBeNull();
    expect(screen.queryByTestId('canvas-group-row-0')).not.toBeNull();
  });

  it('그룹 행이 제 id 를 말한다 — 목록에서 어느 그룹인지 알 수 있어야 고를 수 있다', () => {
    setup(cfg([rectNode(), groupNode()]));
    expect(screen.getByTestId('canvas-group-row-1')).toHaveAttribute('data-element-id', 'grp-1');
  });

  it('펼치면 부품 행이 드러나고 **문구 부품**이 그 안에 있다 (사용자 보고 "텍스트 빠짐")', () => {
    setup(cfg([rectNode(), groupNode()]));
    // 전제 — 접혀 있으므로 부품이 없는 것이 옳다. 이 단언이 없으면 아래 "없음" 이
    // "행 자체가 없어서" 인지 "접혀서" 인지 구분되지 않는다.
    expect(screen.queryByTestId('canvas-group-row-part-1-2')).toBeNull();
    expandGroup(1);

    for (const [pIdx, part] of PARTS.entries()) {
      const row = screen.getByTestId(`canvas-group-row-part-1-${pIdx}`);
      expect(row, part.id).toHaveAttribute('data-part-id', part.id);
    }
    // 문구 부품의 행이 제 종류를 말한다 — 펼침 카드가 없으므로 종류를 말할 자리는
    // 이 행뿐이다.
    expect(screen.getByTestId('canvas-group-row-part-1-2')).toHaveTextContent(
      'dashboard.canvas.elements.kindText',
    );
  });
});

// --- 목록의 짜임 -----------------------------------------------------------

describe('그룹 행과 최상위 행이 한 목록에서 섞인다', () => {
  it('배열 자리가 보존된다 — 최상위 행의 순번은 노드 배열의 자리 그대로다', () => {
    setup(cfg([rectNode(), groupNode(), ellipseNode()]));
    expect(screen.getByTestId('canvas-element-0')).toHaveAttribute('data-element-id', 'r1');
    // 1번 자리는 그룹이다 — 요소 행이 그 자리를 차지하면 순번이 한 칸 밀린다.
    expect(screen.queryByTestId('canvas-element-1')).toBeNull();
    expect(screen.getByTestId('canvas-group-row-1')).toHaveAttribute('data-element-id', 'grp-1');
    expect(screen.getByTestId('canvas-element-2')).toHaveAttribute('data-element-id', 'e1');
  });

  it('순번 배지가 1 · 2 · 3 으로 이어진다 — 목록이 한 목록임을 화면이 말한다', () => {
    setup(cfg([rectNode(), groupNode(), ellipseNode()]));
    expect(screen.getByTestId('canvas-element-order-0')).toHaveTextContent('1');
    expect(screen.getByTestId('canvas-group-row-order-1')).toHaveTextContent('2');
    expect(screen.getByTestId('canvas-element-order-2')).toHaveTextContent('3');
  });

  it('부품 하나짜리 그룹도 읽힌다 — 거절되는 것은 **만드는 것**이지 읽는 것이 아니다', () => {
    setup(cfg([groupNode({ parts: [{ ...PARTS[2]! }] })]));
    expandGroup(0);
    expect(screen.getByTestId('canvas-group-row-part-0-0')).toHaveAttribute('data-part-id', 'label');
    expect(screen.queryByTestId('canvas-group-row-part-0-1')).toBeNull();
  });

  it('부품 0 개 그룹은 빈 채로 서고 예외를 내지 않는다 (REQ-05)', () => {
    setup(cfg([groupNode({ parts: [] })]));
    expandGroup(0);
    expect(screen.queryByTestId('canvas-group-row-part-0-0')).toBeNull();
    expect(screen.getByTestId('canvas-group-row-parts-empty-0')).toBeInTheDocument();
  });
});

// --- 순서와 삭제 -----------------------------------------------------------

describe('그룹 행은 최상위 배열을 다스린다', () => {
  it('삭제는 배열에서 **한 자리**를 뺀다 — 부품은 그 안에 있으므로 함께 사라진다', () => {
    const spy = setup(cfg([rectNode(), groupNode(), ellipseNode()]));
    fireEvent.click(screen.getByTestId('canvas-group-row-delete-1'));

    const next = lastNodes(spy);
    expect(next.map((n) => n.id)).toEqual(['r1', 'e1']);
    // 부품이 형제로 승격되지 않았다 — 지우기는 풀기가 아니다.
    expect(next.some((n) => n.id === 'label')).toBe(false);
    expect(next.filter(isGroup)).toHaveLength(0);
  });

  it('위로 이동이 그룹을 한 칸 앞으로 옮긴다 — 요소 행과 같은 규칙을 지난다', () => {
    const spy = setup(cfg([rectNode(), groupNode(), ellipseNode()]));
    fireEvent.click(screen.getByTestId('canvas-group-row-move-up-1'));
    expect(lastNodes(spy).map((n) => n.id)).toEqual(['grp-1', 'r1', 'e1']);
  });

  it('아래로 이동이 그룹을 한 칸 뒤로 옮긴다', () => {
    const spy = setup(cfg([rectNode(), groupNode(), ellipseNode()]));
    fireEvent.click(screen.getByTestId('canvas-group-row-move-down-1'));
    expect(lastNodes(spy).map((n) => n.id)).toEqual(['r1', 'e1', 'grp-1']);
  });

  it('양 끝에서는 바깥쪽 이동 단추가 비활성이다 (요소 행의 규칙 그대로)', () => {
    setup(cfg([groupNode(), rectNode()]));
    expect(screen.getByTestId('canvas-group-row-move-up-0')).toBeDisabled();
    expect(screen.getByTestId('canvas-group-row-move-down-0')).not.toBeDisabled();
  });
});

// --- 선택 -----------------------------------------------------------------

function setupWithSelection(config: Record<string, unknown>): { selection: CanvasSelection } {
  const live: { selection: CanvasSelection } = { selection: new Set() };
  function Harness() {
    const state = useCanvasEditSelectionState();
    live.selection = state.selection;
    return (
      <CanvasEditSelectionContext value={state}>
        <CanvasElementsEditor config={config} onConfigChange={() => {}} />
      </CanvasEditSelectionContext>
    );
  }
  render(<Harness />);
  return live;
}

describe('선택 키는 여전히 `nodeId` 하나다 (REQ-08)', () => {
  it('그룹 행을 펼치면 **그룹**이 골라진다', () => {
    const live = setupWithSelection(cfg([rectNode(), groupNode()]));
    fireEvent.click(screen.getByTestId('canvas-group-row-toggle-1'));
    expect([...live.selection]).toEqual(['grp-1']);
  });

  // **뒤집힌 단언 ① — SPEC-CANVAS-009 REQ-01 · AC-23.**
  //
  // 004 는 여기서 **그룹**이 골라진다고 단언했다. 그 결정의 근거는 "두 번째 편집 UI 를
  // 만들지 않는다"(REQ-08)였고, 009 가 그것을 뒤집은 근거는 **복합 키가 이미 있고 그것이
  // 최상위에서 평평한 키를 유지한다**는 것이다 — `frameKey(nodeId)` 는 `nodeId` 를 그대로
  // 돌려주므로(불변식 G11) 최상위 선택 경로는 한 글자도 바뀌지 않는다. 아래 "최상위 선택
  // 키는 여전히 평평하다" 가 그 절반을 같은 파일에서 계속 지킨다.
  //
  // 삭제하지 않고 뒤집는다 — 뒤집힌 근거가 시험 옆에 있어야 다음 사람이 "왜 004 와
  // 다른가" 를 다시 묻지 않는다(SPEC-CANVAS-009 §뒤집히는 시험).
  it('부품 행을 누르면 **그 부품**이 골라진다 (SPEC-CANVAS-009 에서 뒤집힘)', () => {
    const live = setupWithSelection(cfg([rectNode(), groupNode()]));
    fireEvent.click(screen.getByTestId('canvas-group-row-toggle-1'));
    // 부품 행을 누르기 전에 선택을 비워 두어야 "원래 골라져 있었다" 와 구분된다.
    fireEvent.click(screen.getByTestId('canvas-element-toggle-0'));
    expect([...live.selection]).toEqual(['r1']);

    fireEvent.click(screen.getByTestId('canvas-group-row-part-toggle-1-2'));
    // 복합 키다 — 그룹 id 는 선택에 들어가지 않는다(REQ-01-a).
    expect([...live.selection]).toEqual(['grp-1/label']);
    expect(live.selection.has('grp-1')).toBe(false);
  });

  it('최상위 선택 키는 여전히 평평하다 — 뒤집힌 것은 부품뿐이다 (불변식 G11)', () => {
    const live = setupWithSelection(cfg([rectNode(), groupNode()]));
    fireEvent.click(screen.getByTestId('canvas-element-toggle-0'));
    expect([...live.selection]).toEqual(['r1']);
    fireEvent.click(screen.getByTestId('canvas-group-row-toggle-1'));
    expect([...live.selection]).toEqual(['grp-1']);
  });
});

// --- 없음 둘과 그 근거 ------------------------------------------------------

describe('부품 행이 **내놓지 않는 것**과 그 근거', () => {
  // **뒤집힌 단언 ② — SPEC-CANVAS-009 REQ-04 · AC-19 · AC-21.**
  //
  // 004 는 부품 행에 수치 칸을 두지 않았고 그 기각 근거는 둘이었다(A18): (가) 그룹 로컬
  // 격자를 그대로 보이면 화면에 네 번째 단위가 생긴다, (나) 캔버스 단위로 환산해 보이면
  // **쓰기에 역투영이 필요하다**.
  //
  // **(가) 는 009 도 뒤집지 않는다** — 아래 "로컬 격자 숫자가 화면에 없다" 가 그것을 계속
  // 지킨다. **(나) 만 뒤집혔다**: 004 가 그렇게 적었을 때 그 역투영은 아직 없었고,
  // REQ-07(풀기)을 구현하면서 `groupOps` 가 그것을 만들었다. 009 는 그 함수의 **두 번째
  // 호출자**가 될 뿐 두 번째 역투영을 짓지 않는다(불변식 G2).
  it('부품 행을 펼치면 **캔버스 단위** 수치 칸이 선다 (SPEC-CANVAS-009 에서 뒤집힘)', () => {
    setup(cfg([rectNode(), groupNode()]));
    expandGroup(1);
    // 전제 — 접힌 동안에는 칸이 없다. 이 단언이 없으면 아래 "있다" 가 펼침과 무관해진다.
    expect(screen.queryByTestId('canvas-part-body-1-0')).toBeNull();
    fireEvent.click(screen.getByTestId('canvas-group-row-part-toggle-1-0'));

    const body = screen.getByTestId('canvas-part-body-1-0');
    expect(body.querySelectorAll('input').length).toBeGreaterThan(0);
    expect(body.querySelectorAll('select').length).toBeGreaterThan(0);
    // `body` 부품은 로컬 (0,0)-(3000,2000) 이고 그룹 상자는 (73,41,317,181) 이므로
    // 캔버스 단위로는 x=73 · w=95 다. **로컬 숫자(0 · 3000)가 아니다.**
    expect(screen.getByTestId('canvas-part-geo-x-1-0')).toHaveValue(73);
    expect(screen.getByTestId('canvas-part-geo-w-1-0')).toHaveValue(95);
  });

  it('그룹 로컬 격자의 숫자가 화면에 나오지 않는다 (AC-22 — 004 의 (가) 기각 근거 승계)', () => {
    setup(cfg([rectNode(), groupNode()]));
    expandGroup(1);
    for (const pIdx of [0, 1, 2]) {
      fireEvent.click(screen.getByTestId(`canvas-group-row-part-toggle-1-${pIdx}`));
    }
    // 고정 입력의 로컬 좌표 전량(0 은 캔버스 단위로도 나올 수 있어 뺀다).
    const localValues = [3000, 2000, 4100, 3300, 1700, 900, 5000, 9200];
    const shown = [...document.querySelectorAll('input')].map((el) =>
      Number((el as HTMLInputElement).value),
    );
    for (const v of localValues) {
      expect(shown, String(v)).not.toContain(v);
    }
  });

  it('최상위 요소 행의 수치 칸은 그대로 있다 — 위 "없음" 이 목록 전체의 마비가 아니다', () => {
    setup(cfg([rectNode(), groupNode()]));
    fireEvent.click(screen.getByTestId('canvas-element-toggle-0'));
    fireEvent.click(screen.getByTestId('canvas-element-tab-arrange-0'));
    expect(screen.getByTestId('canvas-element-geo-x-0')).toBeInTheDocument();
  });

  it('목록에 그룹 · 그룹 해제 단추가 없다 — 그 둘은 이미 두 표면에 서 있다(I24)', () => {
    setup(cfg([rectNode(), groupNode()]));
    expandGroup(1);
    // 전제 — 그룹 행은 실제로 그려졌다. 그러지 않으면 아래 둘의 부재는 무의미하다.
    expect(screen.getByTestId('canvas-group-row-1')).toBeInTheDocument();
    // `CanvasGroupTools` 의 두 단추. 목록이 세 번째 입구가 되면 규칙 손실 확인을
    // 지나지 않는 풀기가 생긴다.
    expect(screen.queryByTestId('canvas-group-create')).toBeNull();
    expect(screen.queryByTestId('canvas-group-ungroup')).toBeNull();
  });
});
