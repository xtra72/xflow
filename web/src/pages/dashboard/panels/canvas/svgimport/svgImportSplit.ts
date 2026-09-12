// 부분 경로 · 감김 · 무리 분할 (SPEC-CANVAS-007 M7).
//
// **이 모듈이 있는 이유는 상한 하나다.** `MAX_PATH_COMMANDS = 256` 은 008 이 REQ-07 로 적은
// 죔쇠이고, 007 이 그것을 넘기는 목록을 **만들어 내는 순간** 이 저장소에서 가장 나쁜 부류의
// 결함이 생긴다: `parsePathCommands` 가 상한 초과분을 **조용히 자르므로**(실측
// `canvasConfig.ts` `if (out.length >= MAX_PATH_COMMANDS) break;`) 저장한 직후에는 옳게
// 보이고 **다이얼로그를 닫았다 다시 열면 뒷부분이 잘려 있다.** 파서를 고칠 일이 아니다 —
// 그 자름은 001 이래의 견고성 규율이다. **만들지 않으면 된다.**
//
// **기각한 안 셋.**
//   - **상수를 올린다.** 프레임당 그리기 비용과 포인터 사건당 평탄화 비용이 함께 오르고
//     (빗나간 클릭 한 번이 모든 경로 요소를 평탄화한다 — `hitTest` 가 처음 맞는 것에서
//     멈추므로), 그 아래를 받쳐 줄 두 번째 죔쇠(공간 색인·캐시)가 이 저장소에 없다.
//     게다가 008 REQ-07 이 그 상한을 요구 조항으로 적었으므로 올리려면 남의 SPEC 을 이
//     SPEC 의 편의로 무르는 일이 된다.
//   - **단순화한다(Douglas–Peucker · 곡선 재적합).** 사용자가 저술한 적 없는 그림을 만든다.
//     허용 오차를 무엇에 걸 것인가에도 답이 없다 — 요소 상자 크기는 **가져오기 시점에
//     우리가 고른 값**이라 그 위에 오차를 세우면 오차가 우리 결정에 종속된다. 게다가 곡선을
//     폴리라인으로 바꾸면 `C` 가 `L` 로 풀려 **명령 수가 오히려 는다.**
//   - **앞에서 256개만 자른다.** 설명 없는 틀린 그림이며, 결정 5 가 금지하는 그것이다.
//
// **채택 — 감김 무리로 나누고, 그래도 넘으면 그 도형을 거절한다.** 나누는 단위가 부분
// 경로가 **아니라 무리**인 것이 이 모듈의 핵심이다: 도넛의 바깥과 구멍은 nonzero 채움이
// 둘을 **함께** 요구하므로 같은 무리이고, 부분 경로 단위로 나누면 **구멍이 사라진다.**
// 그 결함은 히트 규칙으로는 보이지 않는다(윤곽까지의 거리는 구멍이 있든 없든 같다) —
// 채움으로만 보인다(위험: 008 이 감김 가드를 히트 규칙으로 재려다 걸린 그 자리).
//
// **`fill-rule="evenodd"` 도 여기서 감김으로 옮긴다.** 캔버스의 `fill()` 은 nonzero 이고
// `ElementStyle` 에는 채움 규칙 축이 없다. 포함 깊이가 짝수/홀수인지로 방향을 정하면
// evenodd 의 의미가 nonzero 위에서 **정확히** 재현된다 — 부분 경로들이 서로 교차하지 않는
// 한. **그 한계는 이름으로 적히고 언제나 근사로 보고된다**(위험 R9) — 파서가 "이번엔
// 정확했다" 를 판정하려 드는 것보다 사용자가 미리보기에서 확인하는 편이 싸고 정직하다.
//
// **포함 판정을 바운딩 박스로 한다.** 참 포함(윤곽 대 윤곽)을 풀려면 곡선 교차를 풀어야
// 하고 그것은 이 SPEC 의 크기가 아니다. 박스 포함은 참 포함의 **상위 근사**라 무리를
// 실제보다 **크게** 묶는다 — 그 방향의 틀림은 "나눌 수 있었는데 나누지 않아 거절했다" 이고,
// 반대 방향의 틀림은 "도넛의 구멍이 사라졌다" 이다. 앞의 것은 화면이 말하고 뒤의 것은
// 침묵한다.
//
// @spec SPEC-CANVAS-007 REQ-03 · REQ-05 · AC-06 · AC-E7

import type { PathCommand } from '../shapes/pathTypes';

/** 좌표 상자. 이 모듈이 소유한다 — 계획 층이 여기서 가져다 쓴다(순환 수입 방지). */
export interface Bounds {
  minX: number;
  minY: number;
  maxX: number;
  maxY: number;
}

/**
 * 명령 목록의 바운딩 박스 — **제어점까지 함께 센다.**
 *
 * 3차 베지어의 실제 극값은 제어점 볼록 껍질 **안**에 있으므로 이 상자는 참값의 상위집합이다.
 * 극값을 풀어 정확한 상자를 구하는 안을 기각한다: 쓰이는 자리가 `viewBox` 폴백과 포함
 * 판정 둘뿐이고, 둘 다 상위집합이어야 안전한 방향이다.
 */
export function commandBounds(commands: readonly PathCommand[]): Bounds | undefined {
  let bounds: Bounds | undefined;
  const include = (x: number, y: number): void => {
    if (!Number.isFinite(x) || !Number.isFinite(y)) return;
    if (bounds === undefined) {
      bounds = { minX: x, minY: y, maxX: x, maxY: y };
      return;
    }
    bounds.minX = Math.min(bounds.minX, x);
    bounds.minY = Math.min(bounds.minY, y);
    bounds.maxX = Math.max(bounds.maxX, x);
    bounds.maxY = Math.max(bounds.maxY, y);
  };
  for (const cmd of commands) {
    switch (cmd.c) {
      case 'Z':
        break;
      case 'M':
      case 'L':
        include(cmd.x, cmd.y);
        break;
      case 'C':
        include(cmd.x1, cmd.y1);
        include(cmd.x2, cmd.y2);
        include(cmd.x, cmd.y);
        break;
    }
  }
  return bounds;
}

/** 두 상자를 합친다. */
export function unionBounds(a: Bounds | undefined, b: Bounds | undefined): Bounds | undefined {
  if (a === undefined) return b;
  if (b === undefined) return a;
  return {
    minX: Math.min(a.minX, b.minX),
    minY: Math.min(a.minY, b.minY),
    maxX: Math.max(a.maxX, b.maxX),
    maxY: Math.max(a.maxY, b.maxY),
  };
}

// --- 부분 경로 ----------------------------------------------------------

/**
 * 명령 목록을 부분 경로들로 자른다.
 *
 * **`Z` 뒤에 `M` 없이 이어지는 명령이 있으면 그것도 새 부분 경로다** — 그때의 시작점은
 * 직전 점이 아니라 **닫은 부분 경로의 시작점**이며(canvas 와 SVG 가 같다), 여기서 `M` 을
 * 세워 그 사실을 형상으로 적는다. 세우지 않으면 그 조각이 앞의 부분 경로에 붙어 무리
 * 판정이 통째로 어긋난다.
 */
export function splitSubPaths(commands: readonly PathCommand[]): PathCommand[][] {
  const out: PathCommand[][] = [];
  let current: PathCommand[] | undefined;
  let start: { x: number; y: number } | undefined;
  let reopenAt: { x: number; y: number } | undefined;

  for (const cmd of commands) {
    if (cmd.c === 'M') {
      current = [cmd];
      out.push(current);
      start = { x: cmd.x, y: cmd.y };
      reopenAt = undefined;
      continue;
    }
    if (current === undefined) continue; // `M` 없이 시작한 목록 — 세울 자리가 없다.
    if (reopenAt !== undefined) {
      current = [{ c: 'M', x: reopenAt.x, y: reopenAt.y }];
      out.push(current);
      start = reopenAt;
      reopenAt = undefined;
    }
    current.push(cmd);
    if (cmd.c === 'Z' && start !== undefined) reopenAt = start;
  }
  return out;
}

/** `Z` 로 닫혔는가. **닫힌 것만 채움 규칙에 참여한다.** */
export function isClosedSubPath(sub: readonly PathCommand[]): boolean {
  return sub.some((cmd) => cmd.c === 'Z');
}

/** 부분 경로의 끝점 목록(`M`/`L`/`C` 의 도착점). 제어점은 방향에 기여하지 않는다. */
function endPoints(sub: readonly PathCommand[]): { x: number; y: number }[] {
  const points: { x: number; y: number }[] = [];
  for (const cmd of sub) {
    if (cmd.c === 'Z') continue;
    points.push({ x: cmd.x, y: cmd.y });
  }
  return points;
}

/**
 * 부호 있는 넓이(신발끈). **부호가 곧 감김 방향이다.**
 *
 * 제어점을 빼고 끝점만 쓰는 것은 방향 판정에 충분하기 때문이다 — 곡선이 볼록 껍질 안에
 * 있으므로 다각형 근사의 부호와 참 넓이의 부호가 같다(자기 교차가 없는 한).
 */
export function signedArea(sub: readonly PathCommand[]): number {
  const points = endPoints(sub);
  let sum = 0;
  for (let i = 0; i < points.length; i += 1) {
    const a = points[i]!;
    const b = points[(i + 1) % points.length]!;
    sum += a.x * b.y - b.x * a.y;
  }
  return sum / 2;
}

/**
 * 부분 경로의 방향을 뒤집는다. **곡선의 두 제어점도 서로 맞바꾼다.**
 *
 * 맞바꾸지 않으면 뒤집힌 곡선이 원본과 다른 모양이 된다 — 끝점만 맞고 배가 반대로 부푼다.
 * 그 어긋남은 "구멍이 뚫렸는가" 에는 보이지 않으므로 채움 시험으로 잡히지 않는다.
 */
export function reverseSubPath(sub: readonly PathCommand[]): PathCommand[] {
  const first = sub[0];
  if (first === undefined || first.c !== 'M') return [...sub];
  const closed = isClosedSubPath(sub);
  const segments = sub.filter((cmd) => cmd.c === 'L' || cmd.c === 'C');
  const starts: { x: number; y: number }[] = [{ x: first.x, y: first.y }];
  for (const seg of segments) starts.push({ x: seg.x, y: seg.y });
  const last = starts[starts.length - 1]!;
  const out: PathCommand[] = [{ c: 'M', x: last.x, y: last.y }];
  for (let i = segments.length - 1; i >= 0; i -= 1) {
    const seg = segments[i]!;
    const target = starts[i]!;
    if (seg.c === 'L') {
      out.push({ c: 'L', x: target.x, y: target.y });
    } else if (seg.c === 'C') {
      out.push({ c: 'C', x1: seg.x2, y1: seg.y2, x2: seg.x1, y2: seg.y1, x: target.x, y: target.y });
    }
  }
  if (closed) out.push({ c: 'Z' });
  return out;
}

// --- 포함 관계 ----------------------------------------------------------

function boxArea(b: Bounds): number {
  return (b.maxX - b.minX) * (b.maxY - b.minY);
}

/** `outer` 의 상자가 `inner` 의 상자를 감싸는가. 같은 크기는 포함이 아니다. */
function boxContains(outer: Bounds, inner: Bounds): boolean {
  return (
    outer.minX <= inner.minX &&
    outer.maxX >= inner.maxX &&
    outer.minY <= inner.minY &&
    outer.maxY >= inner.maxY &&
    boxArea(outer) > boxArea(inner)
  );
}

/** 각 부분 경로가 몇 겹 안에 들었는가. `fill-rule` 변환과 무리 묶기가 함께 쓴다. */
export function containmentDepths(subPaths: readonly (readonly PathCommand[])[]): number[] {
  const boxes = subPaths.map((sub) => commandBounds(sub));
  return subPaths.map((_, i) => {
    const inner = boxes[i];
    if (inner === undefined) return 0;
    let depth = 0;
    for (let j = 0; j < subPaths.length; j += 1) {
      if (j === i) continue;
      const outer = boxes[j];
      if (outer !== undefined && boxContains(outer, inner)) depth += 1;
    }
    return depth;
  });
}

/**
 * 포함 관계로 이어진 부분 경로들을 한 무리로 묶는다.
 *
 * 무리는 **문서 순서**로 나온다(각 무리의 가장 이른 부분 경로 기준) — 배열 순서가 001 의
 * 유일한 z-order 이므로 순서가 흔들리면 겹친 그림의 앞뒤가 바뀐다.
 */
export function windingGroups(subPaths: readonly (readonly PathCommand[])[]): number[][] {
  const parent = subPaths.map((_, i) => i);
  const find = (i: number): number => {
    let root = i;
    while (parent[root] !== root) root = parent[root]!;
    return root;
  };
  const union = (a: number, b: number): void => {
    const ra = find(a);
    const rb = find(b);
    if (ra !== rb) parent[Math.max(ra, rb)] = Math.min(ra, rb);
  };

  const boxes = subPaths.map((sub) => commandBounds(sub));
  for (let i = 0; i < subPaths.length; i += 1) {
    for (let j = i + 1; j < subPaths.length; j += 1) {
      const a = boxes[i];
      const b = boxes[j];
      if (a === undefined || b === undefined) continue;
      if (boxContains(a, b) || boxContains(b, a)) union(i, j);
    }
  }

  const groups = new Map<number, number[]>();
  for (let i = 0; i < subPaths.length; i += 1) {
    const root = find(i);
    const found = groups.get(root);
    if (found === undefined) groups.set(root, [i]);
    else found.push(i);
  }
  return [...groups.entries()].sort((a, b) => a[0] - b[0]).map(([, members]) => members);
}

// --- evenodd → nonzero --------------------------------------------------

/**
 * `fill-rule="evenodd"` 를 포함 관계 기반 감김 뒤집기로 옮긴다.
 *
 * 깊이가 짝수인 부분 경로는 바깥 윤곽과 **같은** 방향, 홀수인 것은 **반대** 방향이 된다.
 * 기준 방향은 가장 바깥(깊이 0) 부분 경로의 방향이다 — 절대 방향을 고정하지 않는 것은
 * 문서가 시계·반시계 어느 쪽으로도 그릴 수 있고, 기준을 문서에서 뽑아야 **바깥 윤곽의
 * 방향 자체는 보존**되기 때문이다.
 */
export function applyEvenOddWinding(commands: readonly PathCommand[]): PathCommand[] {
  const subs = splitSubPaths(commands);
  if (subs.length < 2) return [...commands];
  const depths = containmentDepths(subs);
  const areas = subs.map((sub) => signedArea(sub));

  let reference = 0;
  for (let i = 0; i < subs.length; i += 1) {
    if (depths[i] === 0 && areas[i] !== 0) {
      reference = Math.sign(areas[i]!);
      break;
    }
  }
  if (reference === 0) return [...commands];

  return subs.flatMap((sub, i) => {
    const area = areas[i]!;
    if (area === 0 || !isClosedSubPath(sub)) return sub;
    const want = depths[i]! % 2 === 0 ? reference : -reference;
    return Math.sign(area) === want ? sub : reverseSubPath(sub);
  });
}

// --- 상한 분할 ----------------------------------------------------------

/** 분할의 결과. `undefined` 는 **그 도형을 거절한다**는 뜻이다(자르지 않는다). */
export type WindingSplit = PathCommand[][] | undefined;

/**
 * 상한을 넘는 도형을 감김 무리 단위로 나눈다.
 *
 * 1. 상한 안이면 **나누지 않는다** — 나눌 이유가 없는데 나누면 요소 수만 늘어난다.
 * 2. 넘으면 무리마다 요소 하나.
 * 3. 한 무리가 그래도 넘으면 **그 도형 전체를 거절한다**(`undefined`). 무리 하나만 버리면
 *    도넛의 바깥만 남은 그림이 나오고, 그것은 잘라 만든 그림과 같은 종류의 거짓말이다.
 */
export function splitByCommandLimit(commands: readonly PathCommand[], limit: number): WindingSplit {
  if (commands.length <= limit) return [[...commands]];
  const subs = splitSubPaths(commands);
  const pieces = windingGroups(subs).map((members) => members.flatMap((i) => subs[i] ?? []));
  return pieces.some((piece) => piece.length > limit) ? undefined : pieces;
}
