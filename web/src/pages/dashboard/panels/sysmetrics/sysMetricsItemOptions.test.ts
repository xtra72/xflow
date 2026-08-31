// 항목 표시 옵션 테스트.
//
// 핵심 계약은 "미지정 = 패널을 따름" 이다. 이것이 깨지면 패널 기본값을 바꿔도
// 항목이 따라오지 않거나, 반대로 항목에 고정한 값이 패널을 바꿀 때 함께 흔들린다.

import { describe, expect, it } from 'vitest';

import {
  MAX_ITEM_HEIGHT,
  MIN_ITEM_HEIGHT,
  hasOverride,
  isTimeSeriesStyle,
  optionFieldsFor,
  readPanelOptions,
  resolveItemOptions,
  resolveValueColor,
  stylesFor,
  stylesForPanel,
  supportsStyle,
  withItemOverride,
} from './sysMetricsItemOptions';

describe('패널 기본값', () => {
  it('설정이 없으면 내장 기본값이다', () => {
    const o = readPanelOptions(undefined);
    expect(o.style).toBe('tile');
    // 높이 미지정 = 칸 채움. 고정 px 가 기본이면 패널을 늘려도 아래가 빈다.
    expect(o.height).toBeUndefined();
    expect(o.legend).toBe('bottom');
    expect(o.windowSec).toBe(300);
  });

  it('모르는 값은 기본값으로 떨어뜨린다', () => {
    const o = readPanelOptions({ style: 'pie', legend: 'middle', height: 'tall' });
    expect(o.style).toBe('tile');
    expect(o.legend).toBe('bottom');
    expect(o.height).toBeUndefined();
  });

  it('높이는 허용 범위로 죈다', () => {
    // 너무 낮으면 축이 겹치고, 너무 높으면 한 화면을 넘긴다.
    expect(readPanelOptions({ height: 1 }).height).toBe(MIN_ITEM_HEIGHT);
    expect(readPanelOptions({ height: 99_999 }).height).toBe(MAX_ITEM_HEIGHT);
  });

  it('패널 유형이 기본 스타일을 정한다', () => {
    // 유형마다 어울리는 모양이 다르다 (네트워크는 라인, 스토리지는 진행막대).
    expect(readPanelOptions(undefined, 'sysmetrics-network').style).toBe('line');
  });
});

describe('항목별 덮어쓰기', () => {
  const config = {
    style: 'line',
    height: 200,
    itemOptions: { cpu: { style: 'gauge' }, memory: { height: 320 } },
  };

  it('지정하지 않은 값은 패널을 따른다', () => {
    const cpu = resolveItemOptions(config, 'cpu', 'ratio');
    expect(cpu.style).toBe('gauge'); // 항목이 고정
    expect(cpu.height).toBe(200); // 패널을 따름
  });

  it('덮어쓰기가 없는 항목은 전부 패널을 따른다', () => {
    const other = resolveItemOptions(config, 'diskIo', 'counter');
    expect(other.style).toBe('line');
    expect(other.height).toBe(200);
  });

  it('한 키만 덮어써도 나머지는 패널을 따른다', () => {
    const mem = resolveItemOptions(config, 'memory', 'ratio');
    expect(mem.height).toBe(320);
    expect(mem.style).toBe('line');
  });

  it('값 성격에 맞지 않는 스타일은 타일로 떨어뜨린다', () => {
    // 상한 없는 증가량에 게이지를 그리면 바늘이 무엇을 가리키는지 말할 수 없다.
    const bad = resolveItemOptions(
      { itemOptions: { network: { style: 'gauge' } } },
      'network',
      'counter',
    );
    expect(bad.style).toBe('tile');
  });

  it('itemOptions 가 이상한 타입이어도 무너지지 않는다', () => {
    expect(resolveItemOptions({ itemOptions: 'x' }, 'cpu', 'ratio').style).toBe('tile');
    expect(resolveItemOptions({ itemOptions: { cpu: 5 } }, 'cpu', 'ratio').style).toBe('tile');
  });
});

describe('withItemOverride', () => {
  it('값을 넣으면 그 항목에 저장된다', () => {
    const next = withItemOverride(undefined, 'cpu', { style: 'gauge' });
    expect(next).toEqual({ cpu: { style: 'gauge' } });
  });

  it('undefined 를 넣으면 그 키를 지워 "패널 따름"으로 되돌린다', () => {
    // 되돌릴 방법이 없으면 한 번 고른 값에 갇힌다.
    const config = { itemOptions: { cpu: { style: 'gauge', height: 200 } } };
    expect(withItemOverride(config, 'cpu', { style: undefined })).toEqual({
      cpu: { height: 200 },
    });
  });

  it('빈 문자열도 해제로 본다 (select 의 빈 옵션)', () => {
    const config = { itemOptions: { cpu: { style: 'gauge' } } };
    expect(withItemOverride(config, 'cpu', { style: '' })).toEqual({});
  });

  it('덮어쓰기가 모두 사라진 항목은 항목째 지운다', () => {
    // 빈 껍데기가 쌓이면 config 가 이유 없이 불어난다.
    const config = { itemOptions: { cpu: { style: 'gauge' }, memory: { height: 200 } } };
    expect(withItemOverride(config, 'cpu', { style: undefined })).toEqual({
      memory: { height: 200 },
    });
  });

  it('다른 항목의 덮어쓰기는 건드리지 않는다', () => {
    const config = { itemOptions: { cpu: { style: 'gauge' } } };
    expect(withItemOverride(config, 'memory', { height: 240 })).toEqual({
      cpu: { style: 'gauge' },
      memory: { height: 240 },
    });
  });
});

describe('스타일 제약', () => {
  it('비율 항목은 여섯 스타일을 모두 쓴다', () => {
    expect(stylesFor('ratio')).toHaveLength(6);
  });

  it('증가량 항목은 비율 스타일을 뺀다', () => {
    const styles = stylesFor('counter');
    expect(styles).not.toContain('gauge');
    expect(styles).not.toContain('progress');
    expect(styles).toContain('line');
  });

  it('supportsStyle 이 두 축을 함께 답한다', () => {
    expect(supportsStyle('ratio', 'gauge')).toBe(true);
    expect(supportsStyle('counter', 'gauge')).toBe(false);
    expect(supportsStyle('counter', 'bar')).toBe(true);
  });

  it('시계열 스타일 판별', () => {
    expect(isTimeSeriesStyle('line')).toBe(true);
    expect(isTimeSeriesStyle('area')).toBe(true);
    expect(isTimeSeriesStyle('bar')).toBe(true);
    expect(isTimeSeriesStyle('tile')).toBe(false);
    expect(isTimeSeriesStyle('gauge')).toBe(false);
  });
});

describe('hasOverride', () => {
  it('덮어쓴 항목만 true 다', () => {
    const config = { itemOptions: { cpu: { style: 'gauge' } } };
    expect(hasOverride(config, 'cpu')).toBe(true);
    expect(hasOverride(config, 'memory')).toBe(false);
    expect(hasOverride(undefined, 'cpu')).toBe(false);
  });
});

describe('패널 유형별 기본 스타일 (단일 출처)', () => {
  // 예전에는 각 패널이 호출 인자로 자기 기본값을 넘겨, 설정 화면의 미리보기가
  // 그 인자를 알 수 없어 늘 타일을 그렸다. 표 하나를 보게 만든 뒤의 회귀 가드다.
  it('유형마다 어울리는 모양을 기본으로 한다', () => {
    expect(readPanelOptions(undefined, 'sysmetrics-system').style).toBe('tile');
    expect(readPanelOptions(undefined, 'sysmetrics-network').style).toBe('line');
    expect(readPanelOptions(undefined, 'sysmetrics-storage').style).toBe('progress');
  });

  it('resolveItemOptions 도 같은 기본값을 본다', () => {
    // 패널과 미리보기가 같은 답을 내야 미리보기가 실제 화면과 맞는다.
    expect(resolveItemOptions(undefined, '/', 'ratio', 'sysmetrics-storage').style).toBe('progress');
    expect(resolveItemOptions(undefined, 'bytes_recv', 'counter', 'sysmetrics-network').style).toBe('line');
  });

  it('패널 설정이 유형 기본값을 이긴다', () => {
    expect(readPanelOptions({ style: 'gauge' }, 'sysmetrics-network').style).toBe('gauge');
  });

  it('모르는 유형은 내장 기본값이다', () => {
    expect(readPanelOptions(undefined, 'nope').style).toBe('tile');
  });
});

describe('스타일별 설정 항목', () => {
  it('타일과 진행 막대는 아무 세부 설정도 쓰지 않는다', () => {
    // 쓰지 않는 칸을 보여 주면 바꿔 놓고 왜 안 변하는지 찾아 헤매게 된다.
    for (const style of ['tile', 'progress'] as const) {
      expect(optionFieldsFor(style)).toEqual({
        height: false, legend: false, window: false,
        smooth: false, stacked: false, gauge: false, tile: true,
      });
    }
  });

  it('게이지는 높이와 게이지 세부(모양·눈금)를 쓴다', () => {
    expect(optionFieldsFor('gauge')).toEqual({
      height: true, legend: false, window: false,
      smooth: false, stacked: false, gauge: true, tile: false,
    });
  });

  it('시계열 스타일은 높이·범례·구간을 쓴다', () => {
    for (const style of ['line', 'area', 'bar'] as const) {
      const f = optionFieldsFor(style);
      expect(f.height).toBe(true);
      expect(f.legend).toBe(true);
      expect(f.window).toBe(true);
      expect(f.gauge).toBe(false);
      expect(f.tile).toBe(false);
    }
  });

  it('곡선·누적 노출은 차트 패널의 규칙을 그대로 따른다', () => {
    // 라인은 쌓아도 겹친 선일 뿐이라 누적이 없고, 막대는 선 모양이 없어 곡선이 없다.
    expect(optionFieldsFor('line')).toMatchObject({ smooth: true, stacked: false });
    expect(optionFieldsFor('area')).toMatchObject({ smooth: true, stacked: true });
    expect(optionFieldsFor('bar')).toMatchObject({ smooth: false, stacked: true });
  });
});

describe('차트·게이지 어휘가 기존 패널과 같다', () => {
  it('게이지 모양은 게이지 패널의 8종 목록에서 온다', () => {
    const o = readPanelOptions({ gaugeType: 'needle' });
    expect(o.gaugeType).toBe('needle');
    // 모르는 값은 기본 도넛으로 떨어진다.
    expect(readPanelOptions({ gaugeType: 'spiral' }).gaugeType).toBe('simple');
  });

  it('눈금 범위·단위를 게이지 패널과 같은 키로 읽는다', () => {
    const o = readPanelOptions({ min: 10, max: 90, unit: 'MB' });
    expect(o.min).toBe(10);
    expect(o.max).toBe(90);
    expect(o.unit).toBe('MB');
  });

  it('곡선·누적을 차트 패널과 같은 키로 읽는다', () => {
    const o = readPanelOptions({ smooth: true, stacked: true });
    expect(o.smooth).toBe(true);
    expect(o.stacked).toBe(true);
  });

  it('항목이 게이지 모양만 덮어써도 나머지는 패널을 따른다', () => {
    const config = { gaugeType: 'needle', min: 0, max: 200, itemOptions: { cpu: { gaugeType: 'half' } } };
    const cpu = resolveItemOptions(config, 'cpu', 'ratio');
    expect(cpu.gaugeType).toBe('half');
    expect(cpu.max).toBe(200);
  });

  it('캔들은 제외한다 (시·고·저·종 축이 없다)', () => {
    expect(stylesForPanel('sysmetrics-storage')).not.toContain('candle');
  });
});

describe('패널이 고를 수 있는 스타일', () => {
  // 고를 수 없는 것을 제시하면 "골랐는데 아무 일도 안 일어남" 이 된다.
  it('네트워크 패널은 비율 스타일을 제시하지 않는다', () => {
    const styles = stylesForPanel('sysmetrics-network');
    expect(styles).not.toContain('gauge');
    expect(styles).not.toContain('progress');
    expect(styles).toEqual(['tile', 'line', 'area', 'bar']);
  });

  it('스토리지 패널은 여섯 가지를 모두 제시한다', () => {
    expect(stylesForPanel('sysmetrics-storage')).toHaveLength(6);
  });

  it('시스템 패널은 여섯 가지를 제시한다 (게이지는 비율 항목에 적용)', () => {
    // CPU·메모리는 비율이라 게이지가 뜻을 갖고, 증가량 항목에서는 타일로 떨어진다.
    expect(stylesForPanel('sysmetrics-system')).toHaveLength(6);
  });

  it('모르는 유형은 전부 제시한다', () => {
    expect(stylesForPanel('nope')).toHaveLength(6);
  });
});

describe('높이 — 미지정은 칸 채움', () => {
  it('설정이 없으면 undefined 다 (채움)', () => {
    expect(readPanelOptions(undefined).height).toBeUndefined();
    expect(resolveItemOptions(undefined, 'cpu', 'ratio').height).toBeUndefined();
  });

  it('값을 넣으면 그 높이로 고정된다', () => {
    expect(readPanelOptions({ height: 240 }).height).toBe(240);
  });

  it('범위를 벗어난 값은 죈다', () => {
    expect(readPanelOptions({ height: 1 }).height).toBe(MIN_ITEM_HEIGHT);
    expect(readPanelOptions({ height: 99_999 }).height).toBe(MAX_ITEM_HEIGHT);
  });

  it('항목이 높이만 고정해도 나머지는 패널을 따른다', () => {
    const o = resolveItemOptions({ itemOptions: { cpu: { height: 300 } } }, 'cpu', 'ratio');
    expect(o.height).toBe(300);
    expect(resolveItemOptions({ itemOptions: { cpu: { height: 300 } } }, 'memory', 'ratio').height)
      .toBeUndefined();
  });
});

describe('타일 표시 옵션', () => {
  it('기본은 왼쪽 정렬·값 크게', () => {
    const o = readPanelOptions(undefined);
    expect(o.align).toBe('left');
    expect(o.valueSize).toBe('xl');
    expect(o.labelSize).toBe('sm');
    expect(o.labelWeight).toBe('normal');
    expect(o.valueColor).toBeUndefined();
  });

  it('정렬·글자 크기를 읽는다', () => {
    const o = readPanelOptions({ align: 'center', valueSize: '2xl', labelWeight: 'bold' });
    expect(o.align).toBe('center');
    expect(o.valueSize).toBe('2xl');
    expect(o.labelWeight).toBe('bold');
  });

  it('모르는 값은 기본값으로 떨어뜨린다', () => {
    expect(readPanelOptions({ align: 'middle' }).align).toBe('left');
    expect(readPanelOptions({ valueSize: 'huge' }).valueSize).toBe('xl');
  });

  it('항목이 정렬만 덮어써도 나머지는 패널을 따른다', () => {
    const config = { align: 'right', valueSize: 'lg', itemOptions: { 'cpu.usage_percent': { align: 'center' } } };
    const o = resolveItemOptions(config, 'cpu.usage_percent', 'ratio');
    expect(o.align).toBe('center');
    expect(o.valueSize).toBe('lg');
  });

  it('타일 스타일에서만 타일 설정을 노출한다', () => {
    expect(optionFieldsFor('tile').tile).toBe(true);
    expect(optionFieldsFor('progress').tile).toBe(true);
    expect(optionFieldsFor('line').tile).toBe(false);
  });
});

describe('값 범위별 색', () => {
  const thresholds = [
    { name: '경고', color: '#f59e0b', from: 75, to: 90 },
    { name: '위험', color: '#ef4444', from: 90, to: 100 },
  ];

  it('구간에 들면 그 색을 쓴다', () => {
    const o = readPanelOptions({ thresholds, valueColor: '#3b82f6' });
    expect(resolveValueColor(95, o)).toBe('#ef4444');
    expect(resolveValueColor(80, o)).toBe('#f59e0b');
  });

  it('구간에 들지 않으면 지정 색을 쓴다', () => {
    const o = readPanelOptions({ thresholds, valueColor: '#3b82f6' });
    expect(resolveValueColor(10, o)).toBe('#3b82f6');
  });

  it('범위가 없으면 지정 색을 그대로 쓴다', () => {
    const o = readPanelOptions({ valueColor: '#3b82f6' });
    expect(resolveValueColor(95, o)).toBe('#3b82f6');
  });

  it('값이 없으면 지정 색을 쓴다', () => {
    const o = readPanelOptions({ thresholds });
    expect(resolveValueColor(undefined, o)).toBeUndefined();
  });

  it('모양이 어긋난 범위 항목은 버린다', () => {
    const o = readPanelOptions({ thresholds: [{ color: 1, from: 0, to: 1 }, { color: '#fff', from: 0, to: 50 }] });
    expect(o.thresholds).toHaveLength(1);
  });
});

describe('counterMode — 누적 카운터의 표현', () => {
  it('기본은 증가량이다', () => {
    // 누적값은 커지기만 하는 수라 추이를 읽을 수 없다.
    expect(readPanelOptions(undefined).counterMode).toBe('rate');
    expect(resolveItemOptions(undefined, 'network.bytes_recv', 'counter').counterMode).toBe('rate');
  });

  it('패널 기본값을 항목이 따른다', () => {
    const config = { counterMode: 'total' };
    expect(resolveItemOptions(config, 'network.bytes_recv', 'counter').counterMode).toBe('total');
  });

  it('항목별 덮어쓰기가 패널을 이긴다', () => {
    const config = {
      counterMode: 'total',
      itemOptions: { 'network.bytes_recv': { counterMode: 'rate' } },
    };
    expect(resolveItemOptions(config, 'network.bytes_recv', 'counter').counterMode).toBe('rate');
    // 덮어쓰지 않은 항목은 패널을 따른다.
    expect(resolveItemOptions(config, 'network.bytes_sent', 'counter').counterMode).toBe('total');
  });

  it('모르는 값은 폴백한다', () => {
    expect(readPanelOptions({ counterMode: 'cumulative' }).counterMode).toBe('rate');
  });
});
