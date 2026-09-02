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
import { fireEvent, render, screen } from '@testing-library/react';

import { FormField } from './FormField';
import type { ConfigField } from '@/types/node';

// i18n: 실제 ko 번역을 반환하는 mock — 컴포넌트가 useTranslation 을 쓰지만
// 이 테스트는 I18nProvider 로 감싸지 않으므로, ko.json 을 점 표기 키로 해석해
// 기존 한국어 단언을 그대로 통과시킨다.
vi.mock('@/lib/i18n', async () => {
  const ko = (await import('@/lib/i18n/ko.json')).default as Record<string, unknown>;
  const resolve = (key: string): string => {
    const v = key.split('.').reduce<unknown>(
      (o, p) => (o && typeof o === 'object' ? (o as Record<string, unknown>)[p] : undefined),
      ko,
    );
    return typeof v === 'string' ? v : key;
  };
  return {
    useTranslation: () => ({ t: resolve, locale: 'ko' as const, setLocale: () => {} }),
  };
});

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
// storage-write 의 measurement_key 처럼 placeholder 로 $. JSONPath 예시를
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

// object_fields: 중첩 객체를 네이티브 위젯으로 편집하고, 하위 값 변경 시 전체
// 중첩 객체를 불변으로 갱신해 상위 onChange 로 올린다(dotted 키 없음, config[name]={...}).
// enrich 노드(agent/device 블록)가 이 타입을 사용한다.
describe('FormField object_fields (nested native widgets)', () => {
  const agentField: ConfigField = {
    name: 'agent',
    type: 'object_fields',
    label: '에이전트 (agent)',
    fields: [
      { name: 'enabled', type: 'boolean', label: '활성화', default: true },
      { name: 'id_source', type: 'string', label: 'ID 소스', placeholder: '$.metadata.agent.id' },
      { name: 'to_metadata', type: 'boolean', label: '메타데이터 보강', default: false },
      { name: 'to_payload', type: 'string', label: 'Payload 키' },
    ],
  };

  it('하위 필드를 네이티브 위젯(체크박스/텍스트)으로 렌더한다 — JSON textarea 아님', () => {
    const { container } = render(
      <FormField field={agentField} value={{}} onChange={vi.fn()} />,
    );
    // 하위 boolean 2개(enabled, to_metadata) → 체크박스 2개
    expect(container.querySelectorAll('input[type="checkbox"]').length).toBe(2);
    // 하위 string 2개(id_source, to_payload) → 텍스트 입력 2개
    expect(container.querySelectorAll('input[type="text"]').length).toBe(2);
    // object_fields 는 JSON textarea 를 만들지 않는다
    expect(container.querySelector('textarea')).toBeNull();
    // 하위 라벨이 노출된다
    expect(screen.getByText('ID 소스')).toBeInTheDocument();
    expect(screen.getByText('Payload 키')).toBeInTheDocument();
  });

  it('하위 string 편집 시 전체 중첩 객체를 갱신해 onChange 로 올린다 (dotted 키 아님)', () => {
    const onChange = vi.fn();
    const { container } = render(
      <FormField
        field={agentField}
        value={{ enabled: true }}
        onChange={onChange}
      />,
    );
    // id_source 텍스트 입력(placeholder 로 식별)에 값 입력
    const idSource = container.querySelector(
      'input[placeholder="$.metadata.agent.id"]',
    ) as HTMLInputElement;
    expect(idSource).not.toBeNull();
    fireEvent.change(idSource, { target: { value: '$.payload.agent_id' } });

    // onChange 는 전체 중첩 객체를 받는다(기존 키 보존 + 신규 키, dotted 키 없음)
    expect(onChange).toHaveBeenCalledWith({
      enabled: true,
      id_source: '$.payload.agent_id',
    });
  });

  it('하위 boolean 토글 시 전체 중첩 객체를 갱신한다', () => {
    const onChange = vi.fn();
    render(
      <FormField
        field={agentField}
        value={{ id_source: '$.metadata.agent.id' }}
        onChange={onChange}
      />,
    );
    // to_metadata 체크박스 토글 (라벨로 접근)
    const toMetadata = screen.getByLabelText('메타데이터 보강') as HTMLInputElement;
    fireEvent.click(toMetadata);
    expect(onChange).toHaveBeenCalledWith({
      id_source: '$.metadata.agent.id',
      to_metadata: true,
    });
  });

  it('backward compat: 기존 노드의 중첩 객체 값을 하위 위젯에 정확히 로드한다', () => {
    const { container } = render(
      <FormField
        field={agentField}
        value={{ enabled: true, id_source: '$.payload.x', to_metadata: true, to_payload: 'agent_info' }}
        onChange={vi.fn()}
      />,
    );
    const texts = container.querySelectorAll('input[type="text"]');
    const values = Array.from(texts).map((el) => (el as HTMLInputElement).value);
    expect(values).toContain('$.payload.x');
    expect(values).toContain('agent_info');
    const checks = container.querySelectorAll('input[type="checkbox"]');
    // enabled=true, to_metadata=true → 둘 다 checked
    expect(Array.from(checks).every((c) => (c as HTMLInputElement).checked)).toBe(true);
  });

  it('value 가 없으면(undefined) 빈 객체로 취급하고 하위 default 를 표시한다', () => {
    const { container } = render(
      <FormField field={agentField} value={undefined} onChange={vi.fn()} />,
    );
    const checks = container.querySelectorAll('input[type="checkbox"]');
    // enabled(default true) → checked, to_metadata(default false) → unchecked
    const checkedStates = Array.from(checks).map((c) => (c as HTMLInputElement).checked);
    expect(checkedStates).toContain(true);
    expect(checkedStates).toContain(false);
  });
});
