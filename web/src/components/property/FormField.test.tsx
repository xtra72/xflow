// FormField boolean 기본값 렌더링 회귀 테스트.
//
// 버그: field.default 가 true 인데 value 가 undefined 일 때
// 체크박스가 unchecked 로 렌더링되어 SPEC-STORE-003 allow_dynamic_keys
// (default true) 같은 필드가 잘못 비활성화로 보인다.
//
// 수정 후: value 가 undefined 이면 field.default 를 fallback 으로 사용.
//
// @spec SPEC-STORE-003

import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import { FormField } from './FormField';
import type { ConfigField } from '@/types/node';

const boolField = (defaultValue?: unknown): ConfigField => ({
  name: 'allow_dynamic_keys',
  type: 'boolean',
  label: 'Allow dynamic keys',
  default: defaultValue,
});

describe('FormField boolean default fallback', () => {
  it('default=true, value=undefined 이면 체크박스가 checked 로 렌더된다', () => {
    render(
      <FormField
        field={boolField(true)}
        value={undefined}
        onChange={vi.fn()}
      />,
    );
    const checkbox = screen.getByRole('checkbox') as HTMLInputElement;
    expect(checkbox.checked).toBe(true);
  });

  it('default=false, value=undefined 이면 체크박스가 unchecked 로 렌더된다', () => {
    render(
      <FormField
        field={boolField(false)}
        value={undefined}
        onChange={vi.fn()}
      />,
    );
    const checkbox = screen.getByRole('checkbox') as HTMLInputElement;
    expect(checkbox.checked).toBe(false);
  });

  it('default=true, value=false 이면 명시값이 default 를 덮어써 unchecked 가 된다', () => {
    render(
      <FormField
        field={boolField(true)}
        value={false}
        onChange={vi.fn()}
      />,
    );
    const checkbox = screen.getByRole('checkbox') as HTMLInputElement;
    expect(checkbox.checked).toBe(false);
  });

  it('default=true, value=true 이면 체크박스가 checked 로 렌더된다', () => {
    render(
      <FormField
        field={boolField(true)}
        value={true}
        onChange={vi.fn()}
      />,
    );
    const checkbox = screen.getByRole('checkbox') as HTMLInputElement;
    expect(checkbox.checked).toBe(true);
  });

  it('default 미지정, value=undefined 이면 체크박스가 unchecked 로 렌더된다', () => {
    render(
      <FormField
        field={boolField(undefined)}
        value={undefined}
        onChange={vi.fn()}
      />,
    );
    const checkbox = screen.getByRole('checkbox') as HTMLInputElement;
    expect(checkbox.checked).toBe(false);
  });

  it('readOnly=true + default=true + value=undefined 이면 disabled 상태에서도 checked 로 표시된다', () => {
    render(
      <FormField
        field={boolField(true)}
        value={undefined}
        onChange={vi.fn()}
        readOnly
      />,
    );
    const checkbox = screen.getByRole('checkbox') as HTMLInputElement;
    expect(checkbox.checked).toBe(true);
    expect(checkbox.disabled).toBe(true);
  });
});

// readOnly 모드에서 input value 가시성 회귀 테스트.
//
// 버그: text/number input 에 disabled 속성이 붙어 브라우저가
// 기본적으로 텍스트를 흐리게 렌더링하고, 추가로 opacity-60 이
// 적용되어 다크모드에서 값이 거의 안 보인다.
//
// 수정: text/number/textarea 등 readOnly attr 를 지원하는 input 은
// disabled 대신 readOnly attr 를 사용하여 값이 정상 색상으로
// 보이도록 한다. checkbox/select 처럼 readOnly attr 가 없는 위젯은
// disabled 를 그대로 유지한다.
describe('FormField readOnly visibility', () => {
  it('readOnly=true 인 string field 는 readOnly attr 만 가지며 disabled 가 아니어야 한다', () => {
    const { container } = render(
      <FormField
        field={{ name: 'history_ttl', type: 'string', label: '히스토리 보관 시간' }}
        value="1h"
        onChange={vi.fn()}
        readOnly
      />,
    );
    const input = container.querySelector('input[type="text"]') as HTMLInputElement;
    expect(input).not.toBeNull();
    expect(input.readOnly).toBe(true);
    expect(input.disabled).toBe(false);
    expect(input.value).toBe('1h');
  });

  it('readOnly=true 인 number field 는 readOnly attr 만 가지며 disabled 가 아니어야 한다', () => {
    const { container } = render(
      <FormField
        field={{ name: 'max_key_length', type: 'number', label: '최대 키 길이' }}
        value={512}
        onChange={vi.fn()}
        readOnly
      />,
    );
    const input = container.querySelector('input[type="number"]') as HTMLInputElement;
    expect(input).not.toBeNull();
    expect(input.readOnly).toBe(true);
    expect(input.disabled).toBe(false);
    expect(Number(input.value)).toBe(512);
  });

  it('readOnly=true 인 object(textarea) field 는 readOnly attr 만 가지며 disabled 가 아니어야 한다', () => {
    const { container } = render(
      <FormField
        field={{ name: 'metadata', type: 'object', label: '메타데이터' }}
        value={{ foo: 'bar' }}
        onChange={vi.fn()}
        readOnly
      />,
    );
    const textarea = container.querySelector('textarea') as HTMLTextAreaElement;
    expect(textarea).not.toBeNull();
    expect(textarea.readOnly).toBe(true);
    expect(textarea.disabled).toBe(false);
  });

  it('readOnly=true 인 boolean(checkbox) field 는 disabled 사용 (readOnly 미지원)', () => {
    const { container } = render(
      <FormField
        field={{ name: 'enabled', type: 'boolean', label: '활성화', default: true }}
        value={true}
        onChange={vi.fn()}
        readOnly
      />,
    );
    const cb = container.querySelector('input[type="checkbox"]') as HTMLInputElement;
    expect(cb).not.toBeNull();
    expect(cb.disabled).toBe(true);
  });

  it('readOnly=true 인 select field 는 disabled 사용 (readOnly 미지원)', () => {
    const { container } = render(
      <FormField
        field={{ name: 'mode', type: 'select', label: '모드', options: ['a', 'b'] }}
        value="a"
        onChange={vi.fn()}
        readOnly
      />,
    );
    const select = container.querySelector('select') as HTMLSelectElement;
    expect(select).not.toBeNull();
    expect(select.disabled).toBe(true);
  });
});

// SPEC-SUBFLOW-002 W01/AC-1: select 의 default fallback.
//   값 미지정이고 비어있지 않은 default 가 있으면 그 default 가 선택값으로 표시된다
//   (flow-node mode 토글이 미지정 시 shared 로 표시됨). default 가 없거나 ''이면
//   기존 "선택..." 동작을 유지한다(회귀 0).
describe('FormField select default fallback', () => {
  it('value 미지정 + 비어있지 않은 default 면 default 가 선택값으로 표시된다', () => {
    const { container } = render(
      <FormField
        field={{
          name: 'mode',
          type: 'select',
          label: '참조 방식',
          options: ['shared', 'instance'],
          default: 'shared',
        }}
        value={undefined}
        onChange={vi.fn()}
      />,
    );
    const select = container.querySelector('select') as HTMLSelectElement;
    expect(select.value).toBe('shared');
  });

  it('명시값이 있으면 default 가 아니라 명시값이 선택된다', () => {
    const { container } = render(
      <FormField
        field={{
          name: 'mode',
          type: 'select',
          label: '참조 방식',
          options: ['shared', 'instance'],
          default: 'shared',
        }}
        value="instance"
        onChange={vi.fn()}
      />,
    );
    const select = container.querySelector('select') as HTMLSelectElement;
    expect(select.value).toBe('instance');
  });

  it('default 가 없으면 value 미지정 시 "선택..."(빈 값)을 유지한다(회귀 0)', () => {
    const { container } = render(
      <FormField
        field={{ name: 'x', type: 'select', label: 'X', options: ['a', 'b'] }}
        value={undefined}
        onChange={vi.fn()}
      />,
    );
    const select = container.querySelector('select') as HTMLSelectElement;
    expect(select.value).toBe('');
  });
});

// field.placeholder opt-in 회귀 테스트.
//
// influxdb-write 의 measurement_key 처럼 placeholder 로 $. JSONPath 예시를
// 보여줘야 하는 string 필드를 위해 field.placeholder 를 추가했다.
// 미지정 시에는 기존 동작(default 값을 placeholder 로 표시)을 유지해야 한다.
describe('FormField string placeholder', () => {
  it('field.placeholder 지정 시 input placeholder 에 적용된다', () => {
    const { container } = render(
      <FormField
        field={{ name: 'measurement_key', type: 'string', label: 'Measurement 키', placeholder: '$.payload.metric_name' }}
        value=""
        onChange={vi.fn()}
      />,
    );
    const input = container.querySelector('input[type="text"]') as HTMLInputElement;
    expect(input.placeholder).toBe('$.payload.metric_name');
  });

  it('field.placeholder 미지정 시 default 값을 placeholder 로 사용한다(기존 동작)', () => {
    const { container } = render(
      <FormField
        field={{ name: 'time_range', type: 'string', label: '시간 범위', default: '1h' }}
        value=""
        onChange={vi.fn()}
      />,
    );
    const input = container.querySelector('input[type="text"]') as HTMLInputElement;
    expect(input.placeholder).toBe('1h');
  });

  it('placeholder 와 default 둘 다 없으면 placeholder 가 비어있다', () => {
    const { container } = render(
      <FormField
        field={{ name: 'measurement', type: 'string', label: 'Measurement' }}
        value=""
        onChange={vi.fn()}
      />,
    );
    const input = container.querySelector('input[type="text"]') as HTMLInputElement;
    expect(input.placeholder).toBe('');
  });
});
