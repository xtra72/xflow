// 스테이지 좌표계 순수 함수 테스트.
//
// 핵심 계약 두 가지:
//   1) 스테이지는 기준 도면의 종횡비를 보존하는 최대 박스이며 중앙 정렬된다.
//   2) 레거시(컨테이너 기준) 좌표를 스테이지 기준으로 환산하면 **보이던 픽셀 위치가 보존**된다.

import { describe, it, expect } from 'vitest';

import {
  applyStageTransform,
  computeStageBox,
  containerToStage,
  isIdentityTransform,
  migratePositionsToStage,
} from './stage';

describe('computeStageBox', () => {
  it('종횡비가 없으면(도면 없음) 컨테이너 전체가 스테이지다 — 기존 동작 보존', () => {
    expect(computeStageBox(300, 200, undefined)).toEqual({
      left: 0,
      top: 0,
      width: 300,
      height: 200,
    });
  });

  it('컨테이너가 더 넓으면 높이 가득 + 좌우 여백(중앙 정렬)', () => {
    // 400x200 컨테이너에 1:1 도면 → 200x200, 좌우 100 씩 여백.
    expect(computeStageBox(400, 200, 1)).toEqual({ left: 100, top: 0, width: 200, height: 200 });
  });

  it('컨테이너가 더 좁으면 폭 가득 + 상하 여백(중앙 정렬)', () => {
    // 200x400 컨테이너에 1:1 도면 → 200x200, 상하 100 씩 여백.
    expect(computeStageBox(200, 400, 1)).toEqual({ left: 0, top: 100, width: 200, height: 200 });
  });

  it('컨테이너 비율이 도면과 같으면 여백 없이 딱 맞는다', () => {
    expect(computeStageBox(300, 200, 1.5)).toEqual({ left: 0, top: 0, width: 300, height: 200 });
  });

  it('손상 입력(0/음수/NaN)에 예외 없이 방어한다', () => {
    expect(computeStageBox(0, 100, 1)).toEqual({ left: 0, top: 0, width: 0, height: 0 });
    expect(computeStageBox(100, 0, 1)).toEqual({ left: 0, top: 0, width: 0, height: 0 });
    // 종횡비가 비유한/0 이하 → 컨테이너 전체로 폴백.
    expect(computeStageBox(300, 200, NaN)).toEqual({ left: 0, top: 0, width: 300, height: 200 });
    expect(computeStageBox(300, 200, 0)).toEqual({ left: 0, top: 0, width: 300, height: 200 });
    expect(computeStageBox(300, 200, -2)).toEqual({ left: 0, top: 0, width: 300, height: 200 });
  });
});

describe('containerToStage — 레거시 좌표 이관', () => {
  const container = { width: 400, height: 200 };
  const stage = computeStageBox(400, 200, 1); // 200x200 @ left=100

  it('보이던 픽셀 위치를 보존한다(스테이지 도입으로 마커가 움직이지 않는다)', () => {
    // 컨테이너 중앙(0.5,0.5) = px(200,100) → 스테이지 기준 (200-100)/200=0.5, 100/200=0.5.
    expect(containerToStage({ x: 0.5, y: 0.5 }, container, stage)).toEqual({ x: 0.5, y: 0.5 });
    // 컨테이너 좌측 1/4 = px(100,0) → 스테이지 좌상단 (0,0).
    expect(containerToStage({ x: 0.25, y: 0 }, container, stage)).toEqual({ x: 0, y: 0 });
  });

  it('스테이지 밖(레터박스 여백)에 있던 좌표는 경계로 clamp 된다', () => {
    // 컨테이너 좌단(0) = px 0 → 스테이지 기준 -0.5 → 0 으로 clamp.
    expect(containerToStage({ x: 0, y: 0.5 }, container, stage)).toEqual({ x: 0, y: 0.5 });
    // 컨테이너 우단(1) = px 400 → 스테이지 기준 1.5 → 1 로 clamp.
    expect(containerToStage({ x: 1, y: 0.5 }, container, stage)).toEqual({ x: 1, y: 0.5 });
  });

  it('스테이지 = 컨테이너(도면 없음)이면 항등이다', () => {
    const full = computeStageBox(400, 200, undefined);
    expect(containerToStage({ x: 0.3, y: 0.7 }, container, full)).toEqual({ x: 0.3, y: 0.7 });
  });

  it('스테이지가 아직 측정되지 않았으면(0 크기) 원본을 그대로 둔다', () => {
    const zero = { left: 0, top: 0, width: 0, height: 0 };
    expect(containerToStage({ x: 0.3, y: 0.7 }, container, zero)).toEqual({ x: 0.3, y: 0.7 });
  });
});

describe('migratePositionsToStage', () => {
  it('맵 전체를 같은 규칙으로 환산한다', () => {
    const container = { width: 400, height: 200 };
    const stage = computeStageBox(400, 200, 1);
    expect(
      migratePositionsToStage({ a: { x: 0.5, y: 0.5 }, b: { x: 0.25, y: 1 } }, container, stage),
    ).toEqual({ a: { x: 0.5, y: 0.5 }, b: { x: 0, y: 1 } });
  });

  it('빈 맵은 빈 맵이다', () => {
    expect(migratePositionsToStage({}, { width: 10, height: 10 }, computeStageBox(10, 10, 1))).toEqual(
      {},
    );
  });
});

// SPEC: 스테이지 맞춤 모드(contain/cover/stretch). contain 이 기본이고 나머지는 레터박스 여백을
// 없애는 대신 각각 잘림/왜곡을 감수한다.
describe('computeStageBox — fit 모드', () => {
  it('fit 미지정은 contain 과 동일하다(기존 호출부 회귀 0)', () => {
    expect(computeStageBox(400, 200, 1)).toEqual(computeStageBox(400, 200, 1, 'contain'));
  });

  it('cover: 컨테이너가 더 넓으면 폭 가득 + 상하로 넘쳐 잘린다(top 음수)', () => {
    // 400x200 컨테이너에 1:1 도면 → 400x400, 상하 100 씩 잘림.
    expect(computeStageBox(400, 200, 1, 'cover')).toEqual({
      left: 0,
      top: -100,
      width: 400,
      height: 400,
    });
  });

  it('cover: 컨테이너가 더 좁으면 높이 가득 + 좌우로 넘쳐 잘린다(left 음수)', () => {
    expect(computeStageBox(200, 400, 1, 'cover')).toEqual({
      left: -100,
      top: 0,
      width: 400,
      height: 400,
    });
  });

  it('cover 는 컨테이너를 완전히 덮는다 — 어느 축에도 여백이 없다', () => {
    const box = computeStageBox(300, 500, 16 / 9, 'cover');
    expect(box.width).toBeGreaterThanOrEqual(300);
    expect(box.height).toBeGreaterThanOrEqual(500);
    expect(box.left).toBeLessThanOrEqual(0);
    expect(box.top).toBeLessThanOrEqual(0);
  });

  it('stretch: 종횡비를 무시하고 컨테이너를 그대로 쓴다(여백·잘림 모두 없음)', () => {
    expect(computeStageBox(400, 200, 1, 'stretch')).toEqual({
      left: 0,
      top: 0,
      width: 400,
      height: 200,
    });
  });

  it('종횡비와 컨테이너가 일치하면 세 모드가 모두 같다', () => {
    const contain = computeStageBox(400, 200, 2, 'contain');
    expect(computeStageBox(400, 200, 2, 'cover')).toEqual(contain);
    expect(computeStageBox(400, 200, 2, 'stretch')).toEqual(contain);
  });

  it('컨테이너가 0 이면 모드와 무관하게 빈 박스다', () => {
    expect(computeStageBox(0, 200, 1, 'cover')).toEqual({ left: 0, top: 0, width: 0, height: 0 });
  });
});

describe('applyStageTransform — 도면 직접 배치', () => {
  const container = { width: 400, height: 300 };
  const box = { left: 0, top: 37.5, width: 400, height: 225 };

  it('항등 변형은 입력을 그대로 돌려준다', () => {
    expect(applyStageTransform(box, undefined, container)).toBe(box);
    expect(applyStageTransform(box, { offset_x: 0, offset_y: 0, scale: 1 }, container)).toBe(box);
  });

  it('배율은 박스 중심을 고정하고 키운다', () => {
    const out = applyStageTransform(box, { scale: 2 }, container);
    expect(out.width).toBe(800);
    expect(out.height).toBe(450);
    // 중심 보존: 좌상단이 늘어난 만큼의 절반씩 뒤로 밀린다.
    expect(out.left).toBe(box.left - 200);
    expect(out.top).toBe(box.top - 112.5);
  });

  it('이동은 컨테이너 크기 대비 비율로 적용된다', () => {
    const out = applyStageTransform(box, { offset_x: 0.25, offset_y: -0.5 }, container);
    expect(out.left).toBe(box.left + 100);
    expect(out.top).toBe(box.top - 150);
    expect(out.width).toBe(box.width);
  });

  it('이동과 배율을 함께 적용한다', () => {
    const out = applyStageTransform(box, { offset_x: 0.1, scale: 2 }, container);
    expect(out.width).toBe(800);
    expect(out.left).toBe(box.left - 200 + 40);
  });

  it('배율이 0 이하/비유한이면 1 로 폴백한다(스테이지 붕괴 방지)', () => {
    expect(applyStageTransform(box, { scale: 0, offset_x: 0.1 }, container).width).toBe(400);
    expect(applyStageTransform(box, { scale: -2, offset_x: 0.1 }, container).width).toBe(400);
    expect(applyStageTransform(box, { scale: NaN, offset_x: 0.1 }, container).width).toBe(400);
  });

  it('빈 박스에는 적용하지 않는다', () => {
    const empty = { left: 0, top: 0, width: 0, height: 0 };
    expect(applyStageTransform(empty, { scale: 2 }, container)).toBe(empty);
  });
});

describe('isIdentityTransform', () => {
  it('미지정/기본값은 항등이다', () => {
    expect(isIdentityTransform(undefined)).toBe(true);
    expect(isIdentityTransform({})).toBe(true);
    expect(isIdentityTransform({ offset_x: 0, offset_y: 0, scale: 1 })).toBe(true);
  });

  it('하나라도 다르면 항등이 아니다', () => {
    expect(isIdentityTransform({ scale: 1.5 })).toBe(false);
    expect(isIdentityTransform({ offset_x: 0.1 })).toBe(false);
  });
});
