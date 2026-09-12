// 캔버스 트윈 엔진 시험 (SPEC-CANVAS-001 T9).
//
// 시각이 전부 `nowMs` 인자로 들어오므로 가짜 타이머도 jsdom 도 필요 없다 — 시계를
// 손으로 밀며 결정론적으로 확인한다.
//
// @spec SPEC-CANVAS-001

import { describe, expect, it } from 'vitest';

import type { ElementStyle, TweenEasing, TweenSpec } from './canvasConfig';
import {
  EASING_FUNCTIONS,
  beginTween,
  easeIn,
  easeInOut,
  easeLinear,
  easeOut,
  easingFor,
  lerpColor,
  lerpNumber,
  lerpNumericRecord,
  retargetTween,
  sampleTween,
} from './canvasTween';

/** T5 의 `ResolvedStyle` 형상 — 얹혀 오는 `text` 가 즉시 전환되는지 보기 위한 지역 타입. */
type ResolvedLike = ElementStyle & { text?: string };

const EASE_OUT_300: TweenSpec = { duration_ms: 300, easing: 'ease-out' };

/** 깊은 사본. 순수성(입력 불변) 확인의 기준값을 뜬다. */
function snapshot<T>(v: T): T {
  return JSON.parse(JSON.stringify(v)) as T;
}

describe('이징', () => {
  const cases: Array<[string, (t: number) => number, number]> = [
    ['linear', easeLinear, 0.5],
    ['ease-in', easeIn, 0.25],
    ['ease-out', easeOut, 0.75],
    ['ease-in-out', easeInOut, 0.5],
  ];

  for (const [name, fn, mid] of cases) {
    it(`${name} 은 f(0)=0 · f(1)=1 이고 중점이 ${mid} 다`, () => {
      expect(fn(0)).toBe(0);
      expect(fn(1)).toBe(1);
      expect(fn(0.5)).toBeCloseTo(mid, 10);
    });

    it(`${name} 은 0..1 밖 입력을 죈다`, () => {
      expect(fn(-1)).toBe(0);
      expect(fn(2)).toBe(1);
      // 손상된 진행도(NaN)는 0 으로 떨어져 NaN 이 값으로 번지지 않는다.
      expect(fn(Number.NaN)).toBe(0);
    });
  }

  it('ease-in 과 ease-out 은 중점에서 비대칭이다', () => {
    expect(easeIn(0.5)).toBeLessThan(0.5);
    expect(easeOut(0.5)).toBeGreaterThan(0.5);
    expect(easeIn(0.5) + easeOut(0.5)).toBeCloseTo(1, 10);
  });

  it('ease-in-out 은 전반부와 후반부가 서로 다른 가지를 탄다', () => {
    expect(easeInOut(0.25)).toBeCloseTo(0.125, 10);
    expect(easeInOut(0.75)).toBeCloseTo(0.875, 10);
  });

  it('easingFor 는 토큰을 함수로 풀고 미지 토큰·미지정은 등속으로 떨어진다', () => {
    expect(easingFor('ease-in')).toBe(EASING_FUNCTIONS['ease-in']);
    expect(easingFor('linear')).toBe(easeLinear);
    expect(easingFor(undefined)).toBe(easeLinear);
    expect(easingFor('bogus' as TweenEasing)).toBe(easeLinear);
  });
});

describe('lerpNumber', () => {
  it('양수·음수·0 길이 구간을 모두 다룬다', () => {
    expect(lerpNumber(0, 10, 0.5)).toBe(5);
    expect(lerpNumber(-10, 10, 0.5)).toBe(0);
    expect(lerpNumber(-10, -20, 0.25)).toBe(-12.5);
    // 0 길이 구간: 어느 지점을 물어도 그 값이다.
    expect(lerpNumber(7, 7, 0)).toBe(7);
    expect(lerpNumber(7, 7, 0.5)).toBe(7);
    expect(lerpNumber(7, 7, 1)).toBe(7);
  });

  it('t 를 0..1 로 죈다', () => {
    expect(lerpNumber(0, 10, -5)).toBe(0);
    expect(lerpNumber(0, 10, 5)).toBe(10);
    expect(lerpNumber(0, 10, Number.NaN)).toBe(0);
  });

  it('비유한 끝점은 없는 셈 치고 반대편을 준다(NaN 을 내지 않는다)', () => {
    expect(lerpNumber(Number.NaN, 10, 0.5)).toBe(10);
    expect(lerpNumber(0, Number.NaN, 0.5)).toBe(0);
    expect(lerpNumber(Number.POSITIVE_INFINITY, 3, 0.5)).toBe(3);
  });
});

describe('lerpColor', () => {
  it('#rgb 를 해석한다', () => {
    expect(lerpColor('#f00', '#00f', 0)).toBe('rgb(255, 0, 0)');
    expect(lerpColor('#f00', '#00f', 1)).toBe('rgb(0, 0, 255)');
    expect(lerpColor('#000', '#fff', 0.5)).toBe('rgb(128, 128, 128)');
  });

  it('#rrggbb 를 해석한다', () => {
    expect(lerpColor('#ffff00', '#ff0000', 0.5)).toBe('rgb(255, 128, 0)');
    // 대문자 표기도 같은 형식이다.
    expect(lerpColor('#FFFF00', '#FF0000', 0)).toBe('rgb(255, 255, 0)');
  });

  it('rgb() 를 해석하고 채널을 0..255 로 죈다', () => {
    expect(lerpColor('rgb(0, 0, 0)', 'rgb(100, 200, 50)', 0.5)).toBe('rgb(50, 100, 25)');
    expect(lerpColor('rgb(300, -10, 0)', 'rgb(300, -10, 0)', 0.5)).toBe('rgb(255, 0, 0)');
  });

  it('rgba() 는 알파도 보간하며 알파가 1 미만이면 rgba 로 낸다', () => {
    expect(lerpColor('rgba(0, 0, 0, 0)', 'rgba(0, 0, 0, 1)', 0.5)).toBe('rgba(0, 0, 0, 0.5)');
    // 알파 표기도 0..1 로 죈다.
    expect(lerpColor('rgba(0, 0, 0, 5)', 'rgba(0, 0, 0, -5)', 0)).toBe('rgb(0, 0, 0)');
    expect(lerpColor('#000', 'rgba(0, 0, 0, 0.2)', 1)).toBe('rgba(0, 0, 0, 0.2)');
  });

  it('시작 끝점을 해석하지 못하면 목표 색으로 스냅한다', () => {
    // CSS 이름 있는 색 · CSS 변수 · 그라디언트 · 쓰레기 — 전부 추측하지 않는다.
    expect(lerpColor('red', '#00ff00', 0.5)).toBe('#00ff00');
    expect(lerpColor('var(--accent)', '#00ff00', 0.5)).toBe('#00ff00');
    expect(lerpColor('linear-gradient(red, blue)', '#00ff00', 0.5)).toBe('#00ff00');
    expect(lerpColor('', '#00ff00', 0.5)).toBe('#00ff00');
  });

  it('목표 끝점을 해석하지 못해도 목표 색으로 스냅한다', () => {
    expect(lerpColor('#ff0000', 'red', 0.5)).toBe('red');
    expect(lerpColor('#ff0000', '#ff00', 0.5)).toBe('#ff00');
  });

  it('형식이 어긋난 rgb 표기는 해석 실패로 떨어진다', () => {
    const target = '#010203';
    // 닫는 괄호 없음
    expect(lerpColor('rgb(1, 2, 3', target, 0.5)).toBe(target);
    // 칸 수 부족 · 초과
    expect(lerpColor('rgb(1, 2)', target, 0.5)).toBe(target);
    expect(lerpColor('rgba(1, 2, 3, 4, 5)', target, 0.5)).toBe(target);
    // 채널이 숫자가 아니다(백분율 표기 포함)
    expect(lerpColor('rgb(a, 2, 3)', target, 0.5)).toBe(target);
    expect(lerpColor('rgb(1, b, 3)', target, 0.5)).toBe(target);
    expect(lerpColor('rgb(100%, 0%, 0%)', target, 0.5)).toBe(target);
    // 알파가 숫자가 아니다
    expect(lerpColor('rgba(1, 2, 3, x)', target, 0.5)).toBe(target);
  });
});

describe('lerpNumericRecord (003 기하 보간 재사용 축)', () => {
  it('레코드의 모든 수치를 함께 보간한다', () => {
    const from = { x: 0, y: 0, w: 1, h: 1 };
    const to = { x: 1, y: 2, w: 3, h: 5 };
    expect(lerpNumericRecord(from, to, 0.5)).toEqual({ x: 0.5, y: 1, w: 2, h: 3 });
    expect(lerpNumericRecord(from, to, 0)).toEqual(from);
    expect(lerpNumericRecord(from, to, 1)).toEqual(to);
  });

  it('결과 형상은 목표의 형상이다 — from 에만 있는 칸은 버리고 to 에만 있는 칸은 즉시 전환한다', () => {
    const out = lerpNumericRecord<Record<string, number>>({ x: 0, z: 9 }, { x: 10, y: 5 }, 0.5);
    expect(out).toEqual({ x: 5, y: 5 });
    expect('z' in out).toBe(false);
  });

  it('입력을 고치지 않는다', () => {
    const from = { x: 0, y: 0 };
    const to = { x: 4, y: 8 };
    const beforeFrom = snapshot(from);
    const beforeTo = snapshot(to);
    lerpNumericRecord(from, to, 0.5);
    expect(from).toEqual(beforeFrom);
    expect(to).toEqual(beforeTo);
  });
});

describe('AC-03 — 300ms ease-out 상태 전이 트위닝', () => {
  const yellow: ElementStyle = { fill: '#ffff00' };
  const red: ElementStyle = { fill: '#ff0000' };

  it('색은 즉시 바뀌지 않고 300ms 동안 노랑→빨강으로 이징 보간된다', () => {
    const state = beginTween(yellow, red, EASE_OUT_300, 1000);

    // 시작: 아직 노랑이며 끝나지 않았다.
    const start = sampleTween(state, 1000);
    expect(start.style.fill).toBe('rgb(255, 255, 0)');
    expect(start.done).toBe(false);

    // 중점: 진행도 0.5 에 ease-out 을 먹여 0.75 — 녹색 채널이 255*0.25 로 내려온다.
    const mid = sampleTween(state, 1150);
    expect(mid.style.fill).toBe('rgb(255, 64, 0)');
    expect(mid.done).toBe(false);

    // 끝: 목표값 그대로이며 done 이 선다.
    const end = sampleTween(state, 1300);
    expect(end.style.fill).toBe('#ff0000');
    expect(end.done).toBe(true);

    // 지난 뒤에도 목표에 머문다(넘겨 쏘지 않는다).
    const after = sampleTween(state, 9999);
    expect(after.style.fill).toBe('#ff0000');
    expect(after.done).toBe(true);
  });

  it('done 은 끝에서만 선다', () => {
    const state = beginTween(yellow, red, EASE_OUT_300, 1000);
    for (const t of [1000, 1001, 1150, 1299, 1299.9]) {
      expect(sampleTween(state, t).done).toBe(false);
    }
    expect(sampleTween(state, 1300).done).toBe(true);
  });

  it('색이 보간되는 동안 text · visible · fontWeight · align 은 즉시 전환된다', () => {
    const from: ResolvedLike = {
      fill: '#ffff00',
      text: '정상',
      visible: true,
      fontWeight: 'normal',
      align: 'left',
    };
    const to: ResolvedLike = {
      fill: '#ff0000',
      text: '경보',
      visible: false,
      fontWeight: 'bold',
      align: 'right',
    };
    const state = beginTween(from, to, EASE_OUT_300, 0);

    const first = sampleTween(state, 0);
    // 색은 아직 출발점이다.
    expect(first.style.fill).toBe('rgb(255, 255, 0)');
    // 보간이 성립하지 않는 속성은 첫 프레임부터 목표값이다.
    expect(first.style.text).toBe('경보');
    expect(first.style.visible).toBe(false);
    expect(first.style.fontWeight).toBe('bold');
    expect(first.style.align).toBe('right');

    const mid = sampleTween(state, 150);
    expect(mid.style.fill).toBe('rgb(255, 64, 0)');
    expect(mid.style.text).toBe('경보');
    expect(mid.style.visible).toBe(false);
  });

  it('수치 속성도 함께 보간된다', () => {
    const state = beginTween(
      { opacity: 0, strokeWidth: 2, fontSize: 10 },
      { opacity: 1, strokeWidth: 6, fontSize: 20 },
      { duration_ms: 100, easing: 'linear' },
      0,
    );
    const mid = sampleTween(state, 50);
    expect(mid.style.opacity).toBeCloseTo(0.5, 10);
    expect(mid.style.strokeWidth).toBeCloseTo(4, 10);
    expect(mid.style.fontSize).toBeCloseTo(15, 10);
  });
});

describe('즉시 전환 (루프를 깨우지 않는 경로)', () => {
  it('duration_ms 가 0 이면 첫 샘플에서 done 이다', () => {
    const state = beginTween({ fill: '#ffff00' }, { fill: '#ff0000' }, { duration_ms: 0, easing: 'ease-out' }, 1000);
    const first = sampleTween(state, 1000);
    expect(first.done).toBe(true);
    expect(first.style.fill).toBe('#ff0000');
  });

  it('spec 이 undefined 면 첫 샘플에서 done 이다', () => {
    const state = beginTween({ fill: '#ffff00' }, { fill: '#ff0000' }, undefined, 1000);
    expect(state.durationMs).toBe(0);
    expect(state.easing).toBe('linear');
    const first = sampleTween(state, 1000);
    expect(first.done).toBe(true);
    expect(first.style.fill).toBe('#ff0000');
  });

  it('손상된 duration_ms 는 즉시 전환으로 떨어진다', () => {
    for (const bad of [Number.NaN, Number.POSITIVE_INFINITY, -100]) {
      const state = beginTween({}, { fill: '#ff0000' }, { duration_ms: bad, easing: 'linear' }, 0);
      expect(state.durationMs).toBe(0);
      expect(sampleTween(state, 0).done).toBe(true);
    }
  });
});

describe('AC-03 — 보간 도중 리타깃', () => {
  it('리타깃 직후 값이 직전 값과 이어진다(튀지 않는다)', () => {
    const first = beginTween({ fill: '#ffff00' }, { fill: '#ff0000' }, EASE_OUT_300, 1000);

    const before = sampleTween(first, 1150);
    expect(before.style.fill).toBe('rgb(255, 64, 0)');

    // 같은 시각에 새 목표로 갈아 끼운다.
    const second = retargetTween(first, { fill: '#0000ff' }, EASE_OUT_300, 1150);
    const after = sampleTween(second, 1150);

    // 값 연속성: 같은 시각의 두 샘플이 같은 색이다.
    expect(after.style.fill).toBe(before.style.fill);
    expect(after.done).toBe(false);
  });

  it('리타깃한 트윈은 새 목표로 수렴한다', () => {
    const first = beginTween({ fill: '#ffff00' }, { fill: '#ff0000' }, EASE_OUT_300, 1000);
    const second = retargetTween(first, { fill: '#0000ff' }, EASE_OUT_300, 1150);

    // 원래 목표(빨강)가 아니라 새 목표(파랑)로 간다.
    const end = sampleTween(second, 1450);
    expect(end.style.fill).toBe('#0000ff');
    expect(end.done).toBe(true);

    // 도중 값은 리타깃 시작색(rgb(255, 64, 0))과도 새 목표와도 다르며, 파랑 채널이 자라난다.
    const mid = sampleTween(second, 1300);
    expect(mid.done).toBe(false);
    expect(mid.style.fill).not.toBe('rgb(255, 64, 0)');
    expect(mid.style.fill).toMatch(/^rgb\((\d+), (\d+), (\d+)\)$/);
    const blue = Number(/^rgb\(\d+, \d+, (\d+)\)$/.exec(mid.style.fill ?? '')?.[1]);
    expect(blue).toBeGreaterThan(0);
    expect(blue).toBeLessThan(255);
  });

  it('리타깃 시각의 spec 이 즉시 전환이면 새 목표가 곧바로 선다', () => {
    const first = beginTween({ fill: '#ffff00' }, { fill: '#ff0000' }, EASE_OUT_300, 1000);
    const second = retargetTween(first, { fill: '#0000ff' }, undefined, 1150);
    const sample = sampleTween(second, 1150);
    expect(sample.done).toBe(true);
    expect(sample.style.fill).toBe('#0000ff');
  });

  it('이미 끝난 트윈을 리타깃하면 목표값에서 출발한다', () => {
    const first = beginTween({ fill: '#ffff00' }, { fill: '#ff0000' }, EASE_OUT_300, 1000);
    const second = retargetTween(first, { fill: '#0000ff' }, EASE_OUT_300, 5000);
    expect(second.from.fill).toBe('#ff0000');
    expect(sampleTween(second, 5000).style.fill).toBe('rgb(255, 0, 0)');
  });
});

describe('한쪽에만 있는 키 (결과 형상 = 목표 형상)', () => {
  it('to 에만 있는 색은 첫 프레임부터 목표값이다', () => {
    const state = beginTween({}, { fill: '#00ff00' }, EASE_OUT_300, 0);
    expect(sampleTween(state, 150).style.fill).toBe('#00ff00');
  });

  it('from 에만 있는 색은 결과에서 사라진다(undefined 가 새지 않는다)', () => {
    const state = beginTween({ fill: '#00ff00' }, {}, EASE_OUT_300, 0);
    const mid = sampleTween(state, 150);
    expect(mid.style.fill).toBeUndefined();
    expect('fill' in mid.style).toBe(false);
  });

  it('to 에만 있는 수치는 첫 프레임부터 목표값이다', () => {
    const state = beginTween({}, { opacity: 0.25, strokeWidth: 3 }, EASE_OUT_300, 0);
    const mid = sampleTween(state, 150);
    expect(mid.style.opacity).toBe(0.25);
    expect(mid.style.strokeWidth).toBe(3);
  });

  it('from 에만 있는 수치는 결과에서 사라진다(NaN 이 새지 않는다)', () => {
    const state = beginTween({ opacity: 0.25, fontSize: 12 }, {}, EASE_OUT_300, 0);
    const mid = sampleTween(state, 150);
    expect(mid.style.opacity).toBeUndefined();
    expect(mid.style.fontSize).toBeUndefined();
    expect(Object.values(mid.style).some((v) => typeof v === 'number' && Number.isNaN(v))).toBe(false);
  });
});

describe('시계 이상', () => {
  it('시작 시각보다 이른 nowMs 는 진행도 0 이다(음수 진행도를 만들지 않는다)', () => {
    const state = beginTween({ opacity: 0 }, { opacity: 1 }, { duration_ms: 100, easing: 'linear' }, 1000);
    const early = sampleTween(state, 500);
    expect(early.style.opacity).toBe(0);
    expect(early.done).toBe(false);
    expect(sampleTween(state, 1000).style.opacity).toBe(0);
  });

  it('NaN nowMs 는 NaN 을 내지 않고 목표에서 끝난다(루프가 멈춘다)', () => {
    const state = beginTween({ opacity: 0 }, { opacity: 1 }, { duration_ms: 100, easing: 'linear' }, 1000);
    const broken = sampleTween(state, Number.NaN);
    expect(broken.done).toBe(true);
    expect(broken.style.opacity).toBe(1);
  });

  it('beginTween 이 NaN 시계를 받으면 즉시 전환으로 떨어진다', () => {
    const state = beginTween({ opacity: 0 }, { opacity: 1 }, { duration_ms: 300, easing: 'linear' }, Number.NaN);
    expect(state.startMs).toBe(0);
    expect(state.durationMs).toBe(0);
    // 시작 시각이 NaN 인 트윈은 영원히 끝나지 않으므로 지속 시간을 0 으로 떨어뜨린다.
    expect(sampleTween(state, 0).done).toBe(true);
    expect(sampleTween(state, Number.NaN).done).toBe(true);
  });
});

describe('순수성', () => {
  const from: ResolvedLike = { fill: '#ffff00', opacity: 0, text: '가', visible: true };
  const to: ResolvedLike = { fill: '#ff0000', opacity: 1, text: '나', visible: false };

  it('beginTween · sampleTween · retargetTween 은 입력을 고치지 않는다', () => {
    const beforeFrom = snapshot(from);
    const beforeTo = snapshot(to);

    const state = beginTween(from, to, EASE_OUT_300, 0);
    const stateBefore = snapshot(state);

    sampleTween(state, 150);
    sampleTween(state, 300);
    retargetTween(state, { fill: '#0000ff' }, EASE_OUT_300, 150);

    expect(from).toEqual(beforeFrom);
    expect(to).toEqual(beforeTo);
    expect(state).toEqual(stateBefore);
  });

  it('상태는 인자 객체를 참조로 붙들지 않는다', () => {
    const mutableFrom: ElementStyle = { fill: '#ffff00' };
    const mutableTo: ElementStyle = { fill: '#ff0000' };
    const state = beginTween(mutableFrom, mutableTo, EASE_OUT_300, 0);

    mutableFrom.fill = '#123456';
    mutableTo.fill = '#654321';

    expect(state.from.fill).toBe('#ffff00');
    expect(state.to.fill).toBe('#ff0000');
    expect(sampleTween(state, 0).style.fill).toBe('rgb(255, 255, 0)');
  });

  it('샘플 결과는 매번 새 객체다(호출자가 고쳐도 상태가 흔들리지 않는다)', () => {
    const state = beginTween(from, to, EASE_OUT_300, 0);
    const a = sampleTween(state, 150).style;
    const b = sampleTween(state, 150).style;
    expect(a).not.toBe(b);
    expect(a).toEqual(b);

    a.fill = '#000000';
    expect(sampleTween(state, 150).style.fill).toBe(b.fill);

    // 종료 경로도 목표 객체를 그대로 넘기지 않는다.
    const done = sampleTween(state, 300).style;
    expect(done).not.toBe(state.to);
    done.fill = '#000000';
    expect(state.to.fill).toBe('#ff0000');
  });
});
