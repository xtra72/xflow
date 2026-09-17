// canvasConfig 파서 단위 테스트 (SPEC-CANVAS-001 T1).
//
// 관용 파서의 계약을 고정한다: 어떤 입력에도 예외를 던지지 않고, 정체성이 없는
// 요소(id 결측·미지 kind)는 버리고, 손상된 수치는 기본값으로 보정한다. DOM 을 쓰지
// 않으므로 jsdom 없이도 돈다.

import { describe, it, expect } from 'vitest';

import {
  parseCanvasConfig,
  buildDefaultCanvasConfig,
  DEFAULT_DECIMALS,
  DEFAULT_STROKE_WIDTH,
  DEFAULT_FONT_SIZE,
  DEFAULT_OPACITY,
  DEFAULT_TWEEN_DURATION_MS,
  DEFAULT_TWEEN_EASING,
  DEFAULT_BOX_GEOMETRY,
  DEFAULT_LINE_GEOMETRY,
  DEFAULT_POINT_GEOMETRY,
  DEFAULT_CANVAS_SIZE,
  MAX_CANVAS_DIMENSION,
  MIN_CANVAS_DIMENSION,
  parseCanvasSize,
  type BoxGeometry,
  type LineGeometry,
  type PointGeometry,
  type CanvasElement,
  isNumericElement,
} from './canvasConfig';
import type { CanvasNode } from './group/groupTypes';
import { isConnector } from './connector/connectorTypes';

/** 최소 유효 요소를 만든다(테스트 잡음 축소). */
function rawRect(over: Record<string, unknown> = {}): Record<string, unknown> {
  return { id: 'a', kind: 'rect', geometry: { x: 0, y: 0, w: 100, h: 100 }, ...over };
}

/** 파싱 결과의 첫 요소를 꺼낸다(없으면 실패). */
function firstElement(raw: unknown): CanvasElement {
  const cfg = parseCanvasConfig(raw);
  const el = asElement(cfg.elements[0]);
  if (!el) throw new Error('요소가 파싱되지 않았다');
  return el;
}

/**
 * 최상위 노드를 요소로 좁힌다(SPEC-CANVAS-004 M1).
 *
 * 004 가 `CanvasPanelConfig.elements` 의 원소 타입을 `CanvasNode` 로 넓혔다. 이 파일은
 * **요소 파서**를 재는 곳이라 그룹을 넣지 않으므로, 좁히기는 타입을 맞추는 일 하나뿐이며
 * 어느 단언도 바뀌지 않는다.
 */
function asElement(node: CanvasNode | undefined): CanvasElement | undefined {
  // SPEC-CANVAS-011 M4 — 최상위 노드에 연결선이 더해졌다. 요소가 아닌 갈래가 둘이 되었을
  // 뿐 이 파일의 단언은 한 글자도 바뀌지 않는다(004 가 그룹에 대해 한 그대로다).
  if (node === undefined || node.kind === 'group' || isConnector(node)) return undefined;
  return node;
}

describe('buildDefaultCanvasConfig', () => {
  it('요소 0개 · 기본 store 소스 · 기본 트윈으로 시작한다', () => {
    const cfg = buildDefaultCanvasConfig();
    expect(cfg.elements).toEqual([]);
    expect(cfg.channel_name).toBe('');
    expect(cfg.data_source).toBe('store');
    expect(cfg.store_source).toEqual({
      agent_name: '',
      namespace: 'default',
      series: [],
      ...cfg.store_source,
    });
    expect(cfg.tween).toEqual({
      duration_ms: DEFAULT_TWEEN_DURATION_MS,
      easing: DEFAULT_TWEEN_EASING,
    });
  });

  it('캔버스 크기 기본값은 500 x 400 이다', () => {
    expect(buildDefaultCanvasConfig().canvas).toEqual({ width: 500, height: 400 });
  });

  it('호출마다 새 크기 객체를 준다(패널 간 공유 방지)', () => {
    const a = buildDefaultCanvasConfig();
    const b = buildDefaultCanvasConfig();
    expect(a.canvas).not.toBe(b.canvas);
    expect(a.canvas).not.toBe(DEFAULT_CANVAS_SIZE);
  });

  it('호출마다 새 배열을 준다(패널 간 요소 누수 방지)', () => {
    expect(buildDefaultCanvasConfig().elements).not.toBe(buildDefaultCanvasConfig().elements);
  });

  it('기본 config 를 다시 파싱해도 같은 값이다(왕복 안정성)', () => {
    expect(parseCanvasConfig(buildDefaultCanvasConfig())).toEqual(buildDefaultCanvasConfig());
  });
});

describe('parseCanvasConfig — 손상/결측 입력', () => {
  it.each([undefined, null, 42, 'not-an-object', [], [{ id: 'a', kind: 'rect' }], true])(
    '비객체 입력(%p)도 예외 없이 기본값으로 파싱한다',
    (raw) => {
      expect(() => parseCanvasConfig(raw as unknown)).not.toThrow();
      const cfg = parseCanvasConfig(raw as unknown);
      expect(cfg.elements).toEqual([]);
      expect(cfg.channel_name).toBe('');
      expect(cfg.background).toBeUndefined();
      expect(cfg.tween).toBeUndefined();
      expect(cfg.data_source).toBeUndefined();
      expect(cfg.store_source).toBeUndefined();
      expect(cfg.tsdb_source).toBeUndefined();
      // 크기만은 예외다 — 투영이 언제나 요구하므로 "미지정" 이라는 상태가 없다.
      expect(cfg.canvas).toEqual({ ...DEFAULT_CANVAS_SIZE });
    },
  );

  it('elements 가 배열이 아니면 빈 목록이다', () => {
    expect(parseCanvasConfig({ elements: 'nope' }).elements).toEqual([]);
    expect(parseCanvasConfig({ elements: { 0: rawRect() } }).elements).toEqual([]);
  });

  it('알 수 없는 최상위 필드는 보존하지 않는다', () => {
    const cfg = parseCanvasConfig({ elements: [], mystery: 1, legacy_mode: 'v0' });
    expect(cfg).not.toHaveProperty('mystery');
    expect(cfg).not.toHaveProperty('legacy_mode');
  });

  it('channel_name 이 문자열이 아니면 빈 문자열로 보정한다', () => {
    expect(parseCanvasConfig({ channel_name: 7 }).channel_name).toBe('');
    expect(parseCanvasConfig({ channel_name: 'ch' }).channel_name).toBe('ch');
    expect(parseCanvasConfig({ channel_name: '' }).channel_name).toBe('');
  });

  it.each(['store', 'tsdb', 'sysmetrics'] as const)('data_source %s 는 통과시킨다', (kind) => {
    expect(parseCanvasConfig({ data_source: kind }).data_source).toBe(kind);
  });

  it('미인정 data_source 는 미지정으로 떨어뜨린다', () => {
    expect(parseCanvasConfig({ data_source: 'channel' }).data_source).toBeUndefined();
  });

  it('소스 블록은 객체일 때만 보존한다', () => {
    const store = { agent_name: 'a', namespace: 'default', series: [] };
    const tsdb = { agent_name: 'a', series: [] };
    const kept = parseCanvasConfig({ store_source: store, tsdb_source: tsdb });
    expect(kept.store_source).toBe(store);
    expect(kept.tsdb_source).toBe(tsdb);
    const dropped = parseCanvasConfig({ store_source: 'x', tsdb_source: 0 });
    expect(dropped.store_source).toBeUndefined();
    expect(dropped.tsdb_source).toBeUndefined();
  });

  it('background 는 비어 있지 않은 문자열만 인정한다', () => {
    expect(parseCanvasConfig({ background: '#101010' }).background).toBe('#101010');
    expect(parseCanvasConfig({ background: '   ' }).background).toBeUndefined();
    expect(parseCanvasConfig({ background: 3 }).background).toBeUndefined();
  });
});

describe('parseCanvasConfig — 요소 탈락 경로', () => {
  it.each([
    ['비객체', 'not-an-object'],
    ['null', null],
    ['숫자', 5],
    ['배열', ['id', 'rect']],
  ])('%s 요소는 버린다', (_label, item) => {
    expect(parseCanvasConfig({ elements: [item] }).elements).toEqual([]);
  });

  it.each([
    ['id 결측', { kind: 'rect' }],
    ['id 비문자열', { id: 12, kind: 'rect' }],
    ['id 공백', { id: '   ', kind: 'rect' }],
  ])('%s 요소는 버린다', (_label, item) => {
    expect(parseCanvasConfig({ elements: [item] }).elements).toEqual([]);
  });

  it.each([
    ['kind 결측', { id: 'a' }],
    ['미지 kind', { id: 'a', kind: 'polygon' }],
    ['kind 비문자열', { id: 'a', kind: 3 }],
  ])('%s 요소는 버린다', (_label, item) => {
    expect(parseCanvasConfig({ elements: [item] }).elements).toEqual([]);
  });

  it('id 중복은 먼저 온 것이 이긴다', () => {
    const cfg = parseCanvasConfig({
      elements: [
        rawRect({ id: 'dup', unit: 'first' }),
        rawRect({ id: 'dup', unit: 'second' }),
        rawRect({ id: 'other' }),
      ],
    });
    expect(cfg.elements.map((e) => e.id)).toEqual(['dup', 'other']);
    expect(asElement(cfg.elements[0])?.unit).toBe('first');
  });

  it('유효 요소는 배열 순서를 보존한다(그리기 순서 = z-order)', () => {
    const cfg = parseCanvasConfig({
      elements: [rawRect({ id: 'back' }), rawRect({ id: 'front' })],
    });
    expect(cfg.elements.map((e) => e.id)).toEqual(['back', 'front']);
  });
});

describe('parseCanvasConfig — 기하 보정(손상 시 기본 기하 대체)', () => {
  it('rect/ellipse 기하의 손상 필드만 기본값으로 대체한다', () => {
    const el = firstElement({
      elements: [rawRect({ geometry: { x: 30, y: NaN, w: '50', h: Infinity } })],
    });
    expect(el.geometry as BoxGeometry).toEqual({
      x: 30,
      y: DEFAULT_BOX_GEOMETRY.y,
      w: DEFAULT_BOX_GEOMETRY.w,
      h: DEFAULT_BOX_GEOMETRY.h,
    });
  });

  it('기하 자체가 결측/비객체면 요소를 버리지 않고 기본 기하를 쓴다', () => {
    const missing = firstElement({ elements: [{ id: 'a', kind: 'ellipse' }] });
    expect(missing.geometry as BoxGeometry).toEqual({ ...DEFAULT_BOX_GEOMETRY });
    const broken = firstElement({ elements: [{ id: 'a', kind: 'rect', geometry: 'x' }] });
    expect(broken.geometry as BoxGeometry).toEqual({ ...DEFAULT_BOX_GEOMETRY });
  });

  it('line 기하는 끝점 단위로 보정한다', () => {
    const el = firstElement({
      elements: [{ id: 'l', kind: 'line', geometry: { x1: 20, y1: 20, x2: null, y2: 90 } }],
    });
    expect(el.geometry as LineGeometry).toEqual({
      x1: 20,
      y1: 20,
      x2: DEFAULT_LINE_GEOMETRY.x2,
      y2: 90,
    });
  });

  it('text 기하는 기준점 단위로 보정한다', () => {
    const el = firstElement({
      elements: [{ id: 't', kind: 'text', geometry: { x: 40 }, text: '{value}' }],
    });
    expect(el.geometry as PointGeometry).toEqual({ x: 40, y: DEFAULT_POINT_GEOMETRY.y });
  });

  it('캔버스 밖 좌표는 잘라내지 않는다(밖으로 걸침도 저술이다)', () => {
    const el = firstElement({
      elements: [rawRect({ geometry: { x: -500, y: 0, w: 2000, h: 1000 } })],
    });
    expect(el.geometry as BoxGeometry).toEqual({ x: -500, y: 0, w: 2000, h: 1000 });
  });
});

// --- 정수 좌표계 (SPEC-CANVAS-002 0.8.0) ---------------------------------
//
// 두 갈래를 함께 고정한다. **반올림**은 좌표계가 정수라는 사실 그 자체이고, **퇴화
// 폴백**은 그 반올림이 만들어 내는 새 함정(옛 0..1 좌표가 크기 0 으로 접힌다)에 대한
// 대책이다. 둘을 한 절에 두는 것은 둘째가 첫째 없이는 존재할 이유가 없기 때문이다.

describe('parseCanvasConfig — 정수 좌표', () => {
  it('소수 좌표를 반올림한다', () => {
    const el = firstElement({
      elements: [rawRect({ geometry: { x: 10.4, y: 10.6, w: 99.5, h: 100.49 } })],
    });
    expect(el.geometry as BoxGeometry).toEqual({ x: 10, y: 11, w: 100, h: 100 });
  });

  it('음수 좌표도 반올림만 하고 부호를 지키지 않는다(캔버스 밖은 합법이다)', () => {
    const el = firstElement({
      elements: [{ id: 't', kind: 'text', geometry: { x: -10.5, y: -0.4 } }],
    });
    // -10.5 는 JS 규칙대로 0 쪽으로(-10) 반올림된다 — 여기서 고정하는 것은 그 규칙이
    // 아니라 "부호가 살아남는다" 는 성질이다.
    expect((el.geometry as PointGeometry).x).toBe(-10);
    expect((el.geometry as PointGeometry).y).toBe(-0);
  });

  it('선의 끝점도 정수로 반올림한다', () => {
    const el = firstElement({
      elements: [{ id: 'l', kind: 'line', geometry: { x1: 1.2, y1: 2.7, x2: 300.5, y2: 4.4 } }],
    });
    expect(el.geometry as LineGeometry).toEqual({ x1: 1, y1: 3, x2: 301, y2: 4 });
  });
});

describe('parseCanvasConfig — 퇴화 기하는 씨앗 기하로 되살린다', () => {
  it('반올림해서 크기 0 이 되는 상자는 기본 기하로 바뀐다 (옛 0..1 config)', () => {
    // 001 · 002 가 저장한 좌표다. `Math.round(0.2)` 는 0 이므로 그대로 두면 **보이지 않는**
    // 요소가 되고, 보이지 않는 요소는 사용자가 찾을 수 없어 고칠 수도 없다.
    const el = firstElement({
      elements: [rawRect({ geometry: { x: 0.1, y: 0.1, w: 0.2, h: 0.2 } })],
    });
    expect(el.geometry as BoxGeometry).toEqual({ ...DEFAULT_BOX_GEOMETRY });
  });

  it('한 축만 0 이어도 퇴화다 — 폭 0 인 사각형은 선이 아니라 없음이다', () => {
    const flat = firstElement({ elements: [rawRect({ geometry: { x: 10, y: 10, w: 0, h: 50 } })] });
    expect(flat.geometry as BoxGeometry).toEqual({ ...DEFAULT_BOX_GEOMETRY });
    const thin = firstElement({ elements: [rawRect({ geometry: { x: 10, y: 10, w: 50, h: 0 } })] });
    expect(thin.geometry as BoxGeometry).toEqual({ ...DEFAULT_BOX_GEOMETRY });
  });

  it('음수 크기 상자도 씨앗으로 되살린다(그려질 것이 없다)', () => {
    const el = firstElement({ elements: [rawRect({ geometry: { x: 10, y: 10, w: -50, h: 50 } })] });
    expect(el.geometry as BoxGeometry).toEqual({ ...DEFAULT_BOX_GEOMETRY });
  });

  it('크기 1 짜리 상자는 퇴화가 아니다 — 한 단위는 저술할 수 있는 가장 작은 크기다', () => {
    const el = firstElement({ elements: [rawRect({ geometry: { x: 10, y: 10, w: 1, h: 1 } })] });
    expect(el.geometry as BoxGeometry).toEqual({ x: 10, y: 10, w: 1, h: 1 });
  });

  it('길이 0 인 선은 기본 선으로 되살린다', () => {
    const el = firstElement({
      elements: [{ id: 'l', kind: 'line', geometry: { x1: 40, y1: 40, x2: 40, y2: 40 } }],
    });
    expect(el.geometry as LineGeometry).toEqual({ ...DEFAULT_LINE_GEOMETRY });
  });

  it('한 단위짜리 선은 퇴화가 아니다', () => {
    const el = firstElement({
      elements: [{ id: 'l', kind: 'line', geometry: { x1: 40, y1: 40, x2: 41, y2: 40 } }],
    });
    expect(el.geometry as LineGeometry).toEqual({ x1: 40, y1: 40, x2: 41, y2: 40 });
  });

  it('기준점에는 퇴화가 없다 — 옛 좌표는 왼쪽 위 모서리 근처로 접힐 뿐 사라지지 않는다', () => {
    const el = firstElement({
      elements: [{ id: 't', kind: 'text', geometry: { x: 0.5, y: 0.5 }, text: 'hi' }],
    });
    // 자리는 틀렸지만 화면에 남아 있어 사용자가 보고 고칠 수 있다.
    expect(el.geometry as PointGeometry).toEqual({ x: 1, y: 1 });
  });

  it('퇴화 폴백은 기하만 바꾸고 스타일·문구·바인딩은 그대로 둔다', () => {
    const el = firstElement({
      elements: [
        rawRect({
          geometry: { x: 0.1, y: 0.1, w: 0.2, h: 0.2 },
          style: { fill: '#abcdef' },
          text: '{value}',
          binding: { series: 's1' },
        }),
      ],
    });
    expect(el.geometry as BoxGeometry).toEqual({ ...DEFAULT_BOX_GEOMETRY });
    expect(el.style.fill).toBe('#abcdef');
    expect(el.text).toBe('{value}');
    expect(el.binding).toEqual({ series: 's1', agg: 'last' });
  });
});

describe('parseCanvasSize', () => {
  it('두 정수를 그대로 통과시킨다', () => {
    expect(parseCanvasSize({ width: 800, height: 600 })).toEqual({ width: 800, height: 600 });
  });

  it('결측·비객체는 기본 크기다', () => {
    expect(parseCanvasSize(undefined)).toEqual({ ...DEFAULT_CANVAS_SIZE });
    expect(parseCanvasSize('nope')).toEqual({ ...DEFAULT_CANVAS_SIZE });
    expect(parseCanvasSize(null)).toEqual({ ...DEFAULT_CANVAS_SIZE });
  });

  it('축마다 따로 떨어진다 — 한 축이 깨졌다고 멀쩡한 축까지 버리지 않는다', () => {
    expect(parseCanvasSize({ width: 640, height: 'x' })).toEqual({
      width: 640,
      height: DEFAULT_CANVAS_SIZE.height,
    });
  });

  it('소수는 반올림한다', () => {
    expect(parseCanvasSize({ width: 640.4, height: 480.6 })).toEqual({ width: 640, height: 481 });
  });

  it('0 이하는 기본값으로 떨어뜨린다(0 축은 모든 요소를 화면에서 지운다)', () => {
    expect(parseCanvasSize({ width: 0, height: -5 })).toEqual({ ...DEFAULT_CANVAS_SIZE });
    expect(parseCanvasSize({ width: 0.4, height: 1 })).toEqual({
      width: DEFAULT_CANVAS_SIZE.width,
      height: MIN_CANVAS_DIMENSION,
    });
  });

  it('비유한 값도 기본값으로 떨어뜨린다', () => {
    expect(parseCanvasSize({ width: NaN, height: Infinity })).toEqual({ ...DEFAULT_CANVAS_SIZE });
  });

  it('상한을 넘는 값은 상한으로 죈다', () => {
    expect(parseCanvasSize({ width: 1e9, height: 1e9 })).toEqual({
      width: MAX_CANVAS_DIMENSION,
      height: MAX_CANVAS_DIMENSION,
    });
  });
});

describe('parseCanvasConfig — 캔버스 크기', () => {
  it('config 의 크기를 읽는다', () => {
    expect(parseCanvasConfig({ canvas: { width: 320, height: 240 } }).canvas).toEqual({
      width: 320,
      height: 240,
    });
  });

  it('크기가 없는 config(001 · 002 가 쓴 전부)는 기본 크기를 얻는다', () => {
    expect(parseCanvasConfig({ elements: [] }).canvas).toEqual({ ...DEFAULT_CANVAS_SIZE });
  });

  it('크기가 든 config 는 왕복에 값이 바뀌지 않는다', () => {
    const cfg = parseCanvasConfig({ canvas: { width: 320, height: 240 }, elements: [] });
    expect(parseCanvasConfig(cfg)).toEqual(cfg);
  });
});

describe('parseCanvasConfig — 스타일 보정', () => {
  it('유효한 스타일은 그대로 통과시킨다', () => {
    const el = firstElement({
      elements: [
        rawRect({
          style: {
            fill: '#ff0000',
            stroke: '#000',
            strokeWidth: 2,
            opacity: 0.5,
            fontSize: 18,
            fontWeight: 'bold',
            textColor: '#fff',
            align: 'center',
            visible: false,
          },
        }),
      ],
    });
    expect(el.style).toEqual({
      fill: '#ff0000',
      stroke: '#000',
      strokeWidth: 2,
      opacity: 0.5,
      fontSize: 18,
      fontWeight: 'bold',
      textColor: '#fff',
      align: 'center',
      visible: false,
    });
  });

  it('style 이 없거나 객체가 아니면 빈 스타일이다(렌더측 기본)', () => {
    expect(firstElement({ elements: [rawRect()] }).style).toEqual({});
    expect(firstElement({ elements: [rawRect({ style: 'x' })] }).style).toEqual({});
    expect(firstElement({ elements: [rawRect({ style: null })] }).style).toEqual({});
  });

  it.each([
    [-0.5, 0],
    [0, 0],
    [1, 1],
    [1.5, 1],
    [0.25, 0.25],
  ])('opacity %p 는 %p 로 clamp 한다', (given, want) => {
    expect(firstElement({ elements: [rawRect({ style: { opacity: given } })] }).style.opacity).toBe(
      want,
    );
  });

  it('손상된 opacity 는 기본 불투명도로 보정하고, 부재는 미지정으로 둔다', () => {
    expect(
      firstElement({ elements: [rawRect({ style: { opacity: 'half' } })] }).style.opacity,
    ).toBe(DEFAULT_OPACITY);
    expect(firstElement({ elements: [rawRect({ style: { opacity: NaN } })] }).style.opacity).toBe(
      DEFAULT_OPACITY,
    );
    expect(
      firstElement({ elements: [rawRect({ style: { opacity: null } })] }).style.opacity,
    ).toBeUndefined();
    expect(firstElement({ elements: [rawRect({ style: {} })] }).style.opacity).toBeUndefined();
  });

  it.each([-1, NaN, Infinity, '3', {}])(
    'strokeWidth %p 는 기본 선 두께로 폴백한다',
    (given) => {
      expect(
        firstElement({ elements: [rawRect({ style: { strokeWidth: given } })] }).style.strokeWidth,
      ).toBe(DEFAULT_STROKE_WIDTH);
    },
  );

  it.each([-1, NaN, -Infinity, 'big'])('fontSize %p 는 기본 글자 크기로 폴백한다', (given) => {
    expect(
      firstElement({ elements: [rawRect({ style: { fontSize: given } })] }).style.fontSize,
    ).toBe(DEFAULT_FONT_SIZE);
  });

  it('0 은 유효한 strokeWidth/fontSize 다(음수만 거른다)', () => {
    const el = firstElement({ elements: [rawRect({ style: { strokeWidth: 0, fontSize: 0 } })] });
    expect(el.style.strokeWidth).toBe(0);
    expect(el.style.fontSize).toBe(0);
  });

  it('부재한 strokeWidth/fontSize 는 채우지 않는다', () => {
    const el = firstElement({ elements: [rawRect({ style: { strokeWidth: null } })] });
    expect(el.style.strokeWidth).toBeUndefined();
    expect(el.style.fontSize).toBeUndefined();
  });

  it('빈 문자열 색은 지정 안 함으로 본다', () => {
    const el = firstElement({
      elements: [rawRect({ style: { fill: '', stroke: '  ', textColor: 9 } })],
    });
    expect(el.style.fill).toBeUndefined();
    expect(el.style.stroke).toBeUndefined();
    expect(el.style.textColor).toBeUndefined();
  });

  it('미인정 enum(fontWeight/align)은 미지정으로 떨어뜨린다', () => {
    const el = firstElement({
      elements: [rawRect({ style: { fontWeight: 'black', align: 'justify', visible: 'yes' } })],
    });
    expect(el.style.fontWeight).toBeUndefined();
    expect(el.style.align).toBeUndefined();
    expect(el.style.visible).toBeUndefined();
  });

  it.each(['normal', 'bold'] as const)('fontWeight %s 는 통과시킨다', (w) => {
    expect(firstElement({ elements: [rawRect({ style: { fontWeight: w } })] }).style.fontWeight).toBe(
      w,
    );
  });

  it.each(['left', 'center', 'right'] as const)('align %s 는 통과시킨다', (a) => {
    expect(firstElement({ elements: [rawRect({ style: { align: a } })] }).style.align).toBe(a);
  });
});

describe('parseCanvasConfig — 문구 · 소수 자리 · 단위 · 바인딩', () => {
  it('문구는 빈 문자열도 보존한다(라벨 지우기)', () => {
    expect(firstElement({ elements: [rawRect({ text: '' })] }).text).toBe('');
    expect(firstElement({ elements: [rawRect({ text: '{value}{unit}' })] }).text).toBe(
      '{value}{unit}',
    );
    expect(firstElement({ elements: [rawRect({ text: 5 })] }).text).toBeUndefined();
  });

  it('decimals 는 유한 비음수만 인정하고 정수화한다', () => {
    expect(firstElement({ elements: [rawRect({ decimals: 2.7 })] }).decimals).toBe(2);
    expect(firstElement({ elements: [rawRect({ decimals: 0 })] }).decimals).toBe(0);
    expect(firstElement({ elements: [rawRect({ decimals: -1 })] }).decimals).toBeUndefined();
    expect(firstElement({ elements: [rawRect({ decimals: 'two' })] }).decimals).toBeUndefined();
    expect(firstElement({ elements: [rawRect()] }).decimals).toBeUndefined();
    // 미지정의 뜻은 canvasText 가 쓰는 기본 소수 자리다.
    expect(DEFAULT_DECIMALS).toBe(1);
  });

  it('unit 은 비어 있지 않은 문자열만 인정한다', () => {
    expect(firstElement({ elements: [rawRect({ unit: 'degC' })] }).unit).toBe('degC');
    expect(firstElement({ elements: [rawRect({ unit: '' })] }).unit).toBeUndefined();
    expect(firstElement({ elements: [rawRect({ unit: 1 })] }).unit).toBeUndefined();
  });

  it('바인딩은 시리즈 키가 있을 때만 성립하고 집계는 last 로 고정한다', () => {
    expect(
      firstElement({ elements: [rawRect({ binding: { series: 'temp.1', agg: 'max' } })] }).binding,
    ).toEqual({ series: 'temp.1', agg: 'last' });
    expect(firstElement({ elements: [rawRect({ binding: { series: '' } })] }).binding).toBeUndefined();
    expect(firstElement({ elements: [rawRect({ binding: {} })] }).binding).toBeUndefined();
    expect(firstElement({ elements: [rawRect({ binding: 'temp' })] }).binding).toBeUndefined();
    expect(firstElement({ elements: [rawRect()] }).binding).toBeUndefined();
  });
});

describe('parseCanvasConfig — 규칙 표', () => {
  it('rules 가 배열이 아니면 미지정이다', () => {
    expect(firstElement({ elements: [rawRect({ rules: 'nope' })] }).rules).toBeUndefined();
    expect(firstElement({ elements: [rawRect()] }).rules).toBeUndefined();
  });

  it('유효 행이 하나도 없으면 미지정이다(빈 표 = 표 없음)', () => {
    expect(firstElement({ elements: [rawRect({ rules: [] })] }).rules).toBeUndefined();
    expect(
      firstElement({ elements: [rawRect({ rules: [{ op: 'wat', value: 1 }] })] }).rules,
    ).toBeUndefined();
  });

  it.each(['gt', 'gte', 'lt', 'lte', 'eq', 'ne'] as const)('비교 연산자 %s 를 통과시킨다', (op) => {
    const rules = firstElement({
      elements: [rawRect({ rules: [{ op, value: 10, patch: { fill: '#f00' } }] })],
    }).rules;
    expect(rules).toEqual([{ op, value: 10, patch: { fill: '#f00' } }]);
  });

  it.each([
    ['비객체 행', 'row'],
    ['null 행', null],
    ['배열 행', ['gt', 1]],
    ['미지 연산자', { op: 'contains', value: 1 }],
    ['연산자 결측', { value: 1 }],
    ['임계값 비숫자', { op: 'gt', value: 'ten' }],
    ['임계값 결측', { op: 'lt' }],
    ['임계값 NaN', { op: 'eq', value: NaN }],
    ['비교 연산자에 튜플', { op: 'gt', value: [1, 2] }],
  ])('%s 은 버린다', (_label, row) => {
    expect(firstElement({ elements: [rawRect({ rules: [row] })] }).rules).toBeUndefined();
  });

  it('between 은 2원소 유한 숫자 튜플만 인정한다', () => {
    const ok = firstElement({
      elements: [rawRect({ rules: [{ op: 'between', value: [10, 20] }] })],
    }).rules;
    expect(ok).toEqual([{ op: 'between', value: [10, 20], patch: {} }]);
  });

  it.each([
    ['1원소', [10]],
    ['3원소', [10, 20, 30]],
    ['0원소', []],
    ['비배열', 10],
    ['첫 원소 비숫자', ['10', 20]],
    ['둘째 원소 비숫자', [10, null]],
  ])('between %s 임계값은 버린다', (_label, value) => {
    expect(
      firstElement({ elements: [rawRect({ rules: [{ op: 'between', value }] })] }).rules,
    ).toBeUndefined();
  });

  it('nodata 는 임계값이 쓰레기여도 받아들이고 0 으로 정규화한다', () => {
    const rules = firstElement({
      elements: [
        rawRect({ rules: [{ op: 'nodata', value: 'junk', patch: { fill: '#888', text: '-' } }] }),
      ],
    }).rules;
    expect(rules).toEqual([{ op: 'nodata', value: 0, patch: { fill: '#888', text: '-' } }]);
  });

  it('nodata 는 임계값이 아예 없어도 성립한다', () => {
    expect(firstElement({ elements: [rawRect({ rules: [{ op: 'nodata' }] })] }).rules).toEqual([
      { op: 'nodata', value: 0, patch: {} },
    ]);
  });

  it('행 순서를 보존하고 무효 행만 걸러 낸다', () => {
    const rules = firstElement({
      elements: [
        rawRect({
          rules: [
            { op: 'nodata' },
            { op: 'bogus', value: 1 },
            { op: 'gte', value: 30 },
            { op: 'between', value: [1, 2] },
          ],
        }),
      ],
    }).rules;
    expect(rules?.map((r) => r.op)).toEqual(['nodata', 'gte', 'between']);
  });

  it('패치는 스타일과 같은 규율로 보정하고, 비객체 패치는 빈 패치다', () => {
    const rules = firstElement({
      elements: [
        rawRect({
          rules: [
            { op: 'gt', value: 1, patch: { opacity: 4, strokeWidth: -2, text: 12, fill: '#0f0' } },
            { op: 'lt', value: 0, patch: 'nope' },
          ],
        }),
      ],
    }).rules;
    expect(rules?.[0]?.patch).toEqual({
      opacity: 1,
      strokeWidth: DEFAULT_STROKE_WIDTH,
      fill: '#0f0',
    });
    expect(rules?.[1]?.patch).toEqual({});
  });
});

describe('parseCanvasConfig — 트윈', () => {
  it('트윈이 객체가 아니면 미지정(즉시 전환)이다', () => {
    expect(parseCanvasConfig({ tween: 'fast' }).tween).toBeUndefined();
    expect(parseCanvasConfig({ tween: null }).tween).toBeUndefined();
    expect(parseCanvasConfig({}).tween).toBeUndefined();
    expect(firstElement({ elements: [rawRect({ tween: 0 })] }).tween).toBeUndefined();
  });

  it.each(['linear', 'ease-in', 'ease-out', 'ease-in-out'] as const)(
    '이징 %s 는 통과시킨다',
    (easing) => {
      expect(parseCanvasConfig({ tween: { duration_ms: 100, easing } }).tween).toEqual({
        duration_ms: 100,
        easing,
      });
    },
  );

  it.each(['bounce', '', 3, undefined, null])('미지 이징(%p)은 linear 로 폴백한다', (easing) => {
    expect(parseCanvasConfig({ tween: { duration_ms: 100, easing } }).tween?.easing).toBe('linear');
  });

  it.each([-100, NaN, Infinity, '250', 0, undefined])(
    '손상/음수 지속 시간(%p)은 0(즉시 전환)으로 죈다',
    (duration_ms) => {
      expect(parseCanvasConfig({ tween: { duration_ms } }).tween?.duration_ms).toBe(0);
    },
  );

  it('요소 트윈은 패널 트윈과 독립으로 파싱된다', () => {
    const cfg = parseCanvasConfig({
      tween: { duration_ms: 500, easing: 'ease-in' },
      elements: [rawRect({ tween: { duration_ms: 120, easing: 'ease-out' } })],
    });
    expect(cfg.tween).toEqual({ duration_ms: 500, easing: 'ease-in' });
    expect(cfg.elements[0]?.tween).toEqual({ duration_ms: 120, easing: 'ease-out' });
  });
});

describe('parseCanvasConfig — 도형 종류별 왕복', () => {
  it('4종 요소를 모두 보존하고 알 수 없는 요소 필드는 버린다', () => {
    const cfg = parseCanvasConfig({
      elements: [
        { id: 'r', kind: 'rect', geometry: { x: 0, y: 0, w: 250, h: 200 }, zIndex: 9 },
        { id: 'e', kind: 'ellipse', geometry: { x: 50, y: 40, w: 100, h: 80 } },
        { id: 'l', kind: 'line', geometry: { x1: 0, y1: 0, x2: 500, y2: 400 } },
        { id: 't', kind: 'text', geometry: { x: 250, y: 200 }, text: 'hi' },
      ],
    });
    expect(cfg.elements.map((e) => e.kind)).toEqual(['rect', 'ellipse', 'line', 'text']);
    expect(cfg.elements[0]).not.toHaveProperty('zIndex');
    expect(parseCanvasConfig(cfg)).toEqual(cfg);
  });
});

// --- 숫자 스위치 (SPEC-CANVAS-002 · AC-E17) -------------------------------
//
// 이 필드의 무게는 스키마가 아니라 **부재의 뜻**에 있다. 001·002 를 거쳐 저장된 모든
// config 에는 이 키가 없고, 그 전부가 종전과 똑같이 그려져야 한다. 그래서 "부재 = 숫자"
// 를 파서 축(키가 생기지 않는다)과 판정 축(`isNumericElement`) 양쪽에서 못박는다.

describe('parseCanvasConfig — numeric(숫자로 읽기)', () => {
  it('부재면 키를 만들지 않는다 — 기존 config 가 한 글자도 넓어지지 않는다', () => {
    const el = firstElement({ elements: [rawRect()] });
    expect(el).not.toHaveProperty('numeric');
    expect(el.numeric).toBeUndefined();
  });

  it.each([true, false])('불리언(%p)은 그대로 보존한다', (numeric) => {
    expect(firstElement({ elements: [rawRect({ numeric })] }).numeric).toBe(numeric);
  });

  it.each(['false', 'true', 0, 1, null, {}, []])(
    '불리언이 아닌 값(%p)은 부재로 떨어뜨린다 — 손상 입력이 표기를 뒤집지 않는다',
    (numeric) => {
      const el = firstElement({ elements: [rawRect({ numeric })] });
      expect(el).not.toHaveProperty('numeric');
    },
  );

  it('numeric 은 decimals · unit 과 독립으로 실린다(끈 상태에서도 값이 지워지지 않는다)', () => {
    const el = firstElement({ elements: [rawRect({ numeric: false, decimals: 2, unit: '℃' })] });
    expect(el.numeric).toBe(false);
    expect(el.decimals).toBe(2);
    expect(el.unit).toBe('℃');
  });

  it('왕복에서 그대로다 — 파싱한 것을 다시 파싱해도 같은 것이 나온다', () => {
    const cfg = parseCanvasConfig({
      elements: [
        rawRect({ id: 'on', numeric: true, decimals: 1, unit: 'kW' }),
        rawRect({ id: 'off', numeric: false, text: '{value}', binding: { series: 's', agg: 'last' } }),
        rawRect({ id: 'absent' }),
      ],
    });
    expect(parseCanvasConfig(cfg)).toEqual(cfg);
    expect(cfg.elements.map((e) => asElement(e)?.numeric)).toEqual([true, false, undefined]);
  });
});

describe('isNumericElement — 부재가 곧 숫자다', () => {
  it.each([
    [{}, true],
    [{ numeric: true }, true],
    [{ numeric: undefined }, true],
    [{ numeric: false }, false],
  ] as const)('%p → %p', (el, expected) => {
    expect(isNumericElement(el)).toBe(expected);
  });

  // 참 판정(`!!el.numeric`)으로 적으면 부재가 거짓이 되어 기존 대시보드가 통째로
  // 문자열 표기로 뒤집힌다. 이 시험이 그 오작성을 잡는 자리다.
  it('부재와 false 는 다르다 — 부재만 숫자다', () => {
    expect(isNumericElement({})).toBe(true);
    expect(isNumericElement({ numeric: false })).toBe(false);
  });
});
