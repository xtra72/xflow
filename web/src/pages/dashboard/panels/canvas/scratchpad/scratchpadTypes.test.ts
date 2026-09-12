// 스크래치패드 항목의 형상과 관용 파서 (SPEC-CANVAS-008 M8 · REQ-05 · REQ-08).
//
// **고정 입력은 기본값을 피한다.** 이 파일의 각 시험은 주석에 "어떤 기본값이 이 결함을
// 감추는가" 를 적는다 — 그것이 시험 규율 D2 가 요구하는 것이다.
//
// 무게중심은 **진짜 직렬화 왕복**이다(시험 규율 D11). 저장소를 흉내낸 목(mock)을 지나면
// 객체가 그대로 돌아오므로 필드가 사라지거나 형이 바뀌는 것이 보이지 않는다. 그래서
// `JSON.stringify` → `JSON.parse` → 파서 → 깊은 비교로 잰다.
//
// @spec SPEC-CANVAS-008 REQ-05 · REQ-08 · AC-08

import { describe, expect, it } from 'vitest';

import type { CanvasElement } from '../canvasConfig';
import { PATH_LOCAL_EXTENT, type PathCommand } from '../shapes/pathTypes';
import {
  SCRATCHPAD_MAX_BYTES,
  SCRATCHPAD_MAX_ENTRIES,
  cloneElements,
  elementsBounds,
  elementsOrigin,
  nextScratchpadId,
  parseScratchpad,
  parseScratchpadEntry,
  scratchpadBytes,
  type ScratchpadEntry,
} from './scratchpadTypes';

// --- 고정 입력 -----------------------------------------------------------

/**
 * 명령 **네 종 전부**(`M`·`L`·`C`·`Z`)를 담은 경로. 한 종류만 담으면 왕복에서 다른 셋의
 * 필드가 떨어져도 드러나지 않는다(D11).
 */
const COMMANDS: readonly PathCommand[] = [
  { c: 'M', x: 0, y: PATH_LOCAL_EXTENT },
  { c: 'L', x: PATH_LOCAL_EXTENT, y: PATH_LOCAL_EXTENT },
  { c: 'C', x1: 9000, y1: 3000, x2: 6000, y2: 500, x: 0, y: 0 },
  { c: 'Z' },
];

/**
 * 다섯 종류 전부. 좌표는 **서로 다른 오프셋**에 둔다 — 같은 자리에 겹쳐 두면 좌상단을
 * 잘못 계산해도(예: 첫 요소만 본다) 시험이 통과한다.
 *
 * 선은 끝점을 **거꾸로** 든다(`x2 < x1`). 왼쪽에서 오른쪽으로 그은 선을 쓰면
 * `Math.min(x1, x2)` 를 `x1` 로 바꿔 놓아도 드러나지 않는다.
 */
const MIXED: readonly CanvasElement[] = [
  { id: 'r', kind: 'rect', geometry: { x: 120, y: 200, w: 60, h: 40 }, style: { fill: '#112233' } },
  {
    id: 'e',
    kind: 'ellipse',
    geometry: { x: 300, y: 260, w: 40, h: 40 },
    style: { stroke: '#445566', strokeWidth: 3 },
  },
  { id: 'l', kind: 'line', geometry: { x1: 400, y1: 340, x2: 90, y2: 210 }, style: {} },
  {
    id: 't',
    kind: 'text',
    geometry: { x: 250, y: 500 },
    text: '{name} {value}',
    style: { textColor: '#778899', fontSize: 18 },
    numeric: false,
    unit: '℃',
  },
  {
    id: 'p',
    kind: 'path',
    geometry: { x: 150, y: 180, w: 80, h: 90 },
    path: COMMANDS.map((c) => ({ ...c })),
    catalog_id: 'star5',
    style: { fill: '#aabbcc' },
  },
];

function entry(over: Partial<ScratchpadEntry> = {}): ScratchpadEntry {
  return {
    id: 'sp-1',
    name: '밸브 묶음',
    created: 1_757_000_000_000,
    origin: { x: 90, y: 180 },
    elements: cloneElements(MIXED),
    ...over,
  };
}

// --- 왕복 (D11) ----------------------------------------------------------

describe('진짜 직렬화 왕복 (시험 규율 D11)', () => {
  it('다섯 종류 · 명령 네 종을 담은 항목이 JSON 왕복 뒤에도 같다', () => {
    const before = entry();
    const after = parseScratchpad(JSON.parse(JSON.stringify([before])));
    expect(after).toHaveLength(1);
    expect(after[0]).toEqual(before);
  });

  it('경로 명령의 제어점 필드가 왕복에서 살아남는다', () => {
    // `C` 명령만 따로 잰다 — 위 시험의 깊은 비교가 통째로 실패하면 어느 필드가 사라졌는지
    // 말해 주지 않는다. 기본값 함정: `M`/`L` 만 든 경로는 제어점 손실을 감춘다.
    const after = parseScratchpad(JSON.parse(JSON.stringify([entry()])));
    const path = after[0]?.elements.find((el) => el.kind === 'path');
    expect(path?.kind).toBe('path');
    expect(path?.kind === 'path' ? path.path : []).toEqual(COMMANDS);
  });

  it('요소 파서는 001 의 그것이다 — 알 수 없는 종류는 왕복에서 떨어진다', () => {
    // 두 번째 요소 파서가 생기면 이 규칙이 갈라진다(spec.md §스크래치패드).
    const raw = {
      ...entry(),
      elements: [{ id: 'x', kind: 'blob', geometry: {}, style: {} }, ...MIXED],
    };
    const [parsed] = parseScratchpad([raw]);
    expect(parsed?.elements.map((el) => el.id)).toEqual(['r', 'e', 'l', 't', 'p']);
  });
});

// --- 좌상단 -------------------------------------------------------------

describe('묶음의 좌상단과 바깥 상자', () => {
  it('여러 요소의 바깥 상자를 잡는다 — 첫 요소의 상자가 아니다', () => {
    // 기본값 함정: 요소 하나짜리 묶음이면 좌상단이 그 요소의 (x, y) 와 같아지므로,
    // `reduce` 를 잘못 적어도(예: 첫 요소만 본다) 이 시험이 통과한다.
    const box = elementsBounds(MIXED);
    // 왼쪽은 선의 되돌아온 끝점(90), 위쪽은 사각형(180 = 경로의 y).
    expect(box.x).toBe(90);
    expect(box.y).toBe(180);
    // 오른쪽은 선의 시작점(400), 아래쪽은 문구의 기준점(500).
    expect(box.w).toBe(400 - 90);
    expect(box.h).toBe(500 - 180);
  });

  it('좌상단은 바깥 상자에서 나온다', () => {
    const box = elementsBounds(MIXED);
    expect(elementsOrigin(MIXED)).toEqual({ x: box.x, y: box.y });
  });

  it('원점이 아닌 자리의 묶음도 제 자리를 말한다', () => {
    // 기본값 함정: (0, 0) 에 있는 묶음은 "옮기지 않는다" 는 결함을 감춘다 — 델타가 0 이라
    // 번역이 없어도 결과가 같기 때문이다.
    const moved = MIXED.map((el) =>
      el.kind === 'line'
        ? { ...el, geometry: { x1: 1000, y1: 900, x2: 1300, y2: 1200 } }
        : el,
    ) as CanvasElement[];
    expect(elementsOrigin(moved)).toEqual({ x: 120, y: 180 });
  });

  it('문구 요소는 넓이를 갖지 않는다 — 측정값을 저술에 끌어들이지 않는다', () => {
    const only: CanvasElement[] = [
      { id: 't', kind: 'text', geometry: { x: 40, y: 70 }, style: {} },
    ];
    expect(elementsBounds(only)).toEqual({ x: 40, y: 70, w: 0, h: 0 });
  });

  it('빈 목록은 원점의 0 상자다 — -Infinity 를 밖으로 흘리지 않는다', () => {
    expect(elementsBounds([])).toEqual({ x: 0, y: 0, w: 0, h: 0 });
  });
});

// --- 예산 ---------------------------------------------------------------

describe('예산은 UTF-8 바이트로 잰다', () => {
  it('한글 이름은 코드 단위가 아니라 바이트로 계산된다', () => {
    // 기본값 함정: ASCII 이름만 쓰면 `JSON.stringify(...).length` 와 바이트 수가 같아
    // 잘못된 측정이 드러나지 않는다. 한글 한 글자는 코드 단위 1 · UTF-8 3 바이트다.
    const korean = [entry({ name: '가'.repeat(10), elements: cloneElements(MIXED) })];
    const ascii = [entry({ name: 'a'.repeat(10), elements: cloneElements(MIXED) })];
    expect(scratchpadBytes(korean) - scratchpadBytes(ascii)).toBe(20);
    // 이름을 ASCII 로 되돌려도 순진한 측정과는 여전히 갈린다 — 고정 입력의 문구 요소가
    // 단위로 `℃`(코드 단위 1 · UTF-8 3 바이트)를 들고 있기 때문이다. 이 2 바이트가
    // `.length` 로 되돌리는 변경을 잡는다.
    expect(scratchpadBytes(ascii) - JSON.stringify(ascii).length).toBe(2);
  });

  it('상한 둘이 문서가 적은 값이다', () => {
    expect(SCRATCHPAD_MAX_ENTRIES).toBe(50);
    expect(SCRATCHPAD_MAX_BYTES).toBe(262144);
  });
});

// --- 사본 ---------------------------------------------------------------

describe('사본은 깊다', () => {
  it('원본의 중첩 필드를 고쳐도 사본이 따라 바뀌지 않는다', () => {
    // 기본값 함정: 얕은 사본(`[...elements]`)은 요소 배열 자체를 바꾸는 시험만으로는
    // 통과한다 — 갈라지는 것은 `style` · `path` 처럼 **요소 안쪽**을 고칠 때다.
    const source = cloneElements(MIXED);
    const copy = cloneElements(source);
    const path = source.find((el) => el.kind === 'path');
    if (path?.kind === 'path') {
      path.path[0] = { c: 'M', x: 7777, y: 7777 };
      path.style.fill = '#000000';
    }
    const copied = copy.find((el) => el.kind === 'path');
    expect(copied?.kind === 'path' ? copied.path[0] : null).toEqual(COMMANDS[0]);
    expect(copied?.style.fill).toBe('#aabbcc');
  });
});

// --- 관용 파서 -----------------------------------------------------------

describe('관용 파서는 예외를 던지지 않는다 (REQ-05)', () => {
  it('배열이 아니면 빈 목록이다', () => {
    for (const raw of [null, undefined, 0, 'x', {}, { entries: [] }]) {
      expect(parseScratchpad(raw), String(raw)).toEqual([]);
    }
  });

  it('깨진 항목 하나가 나머지를 죽이지 않는다', () => {
    // 기본값 함정: 항목 하나짜리 목록으로 재면 "항목 단위로 살린다" 가 "전부 살린다" 와
    // 구별되지 않는다. 그래서 성한 것 둘 사이에 깨진 것을 끼운다.
    const good1 = entry({ id: 'sp-1' });
    const good2 = entry({ id: 'sp-2', name: '' });
    const parsed = parseScratchpad([good1, { id: 'sp-9', elements: 'nope' }, good2]);
    expect(parsed.map((e) => e.id)).toEqual(['sp-1', 'sp-2']);
  });

  it('id 가 없거나 빈 문자열이면 버린다 — 정체성이 없다', () => {
    expect(parseScratchpadEntry({ ...entry(), id: undefined })).toBeNull();
    expect(parseScratchpadEntry({ ...entry(), id: '' })).toBeNull();
  });

  it('그릴 요소가 하나도 없으면 버린다', () => {
    // 이름만 남은 행은 놓아도 아무 일이 없고, 사용자가 화면에서 고칠 수 없다.
    expect(parseScratchpadEntry({ ...entry(), elements: [] })).toBeNull();
    expect(parseScratchpadEntry({ ...entry(), elements: [{ kind: 'rect' }] })).toBeNull();
  });

  it('이름 · 시각이 없으면 메우고, 좌상단이 없으면 요소에서 다시 잰다', () => {
    // 계산으로 메울 수 있는 것을 탈락 사유로 삼지 않는다(001 의 기하 손상 정책).
    const parsed = parseScratchpadEntry({
      id: 'sp-7',
      elements: cloneElements(MIXED),
    });
    expect(parsed?.name).toBe('');
    expect(parsed?.created).toBe(0);
    expect(parsed?.origin).toEqual(elementsOrigin(MIXED));
  });

  it('좌상단은 한 축만 읽혀도 부재로 떨어진다 — 반쪽 원점은 번역을 어긋낸다', () => {
    const parsed = parseScratchpadEntry({ ...entry(), origin: { x: 10 } });
    expect(parsed?.origin).toEqual(elementsOrigin(MIXED));
  });

  it('좌상단의 소수는 정수로 굳는다', () => {
    const parsed = parseScratchpadEntry({ ...entry(), origin: { x: 10.6, y: -3.2 } });
    expect(parsed?.origin).toEqual({ x: 11, y: -3 });
  });

  it('항목 id 중복은 먼저 온 것이 이긴다', () => {
    const parsed = parseScratchpad([entry({ name: '먼저' }), entry({ name: '나중' })]);
    expect(parsed).toHaveLength(1);
    expect(parsed[0]?.name).toBe('먼저');
  });
});

describe('id 발급', () => {
  it('쓰이지 않은 이름을 고른다', () => {
    expect(nextScratchpadId([])).toBe('sp-1');
    expect(nextScratchpadId([entry({ id: 'sp-1' }), entry({ id: 'sp-3' })])).toBe('sp-2');
  });
});
