// React Context 기반 i18n 국제화 시스템.
// 외부 라이브러리 없이 Context + JSON 번역 파일로 다국어를 지원한다.

import {
  createContext,
  createElement,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from 'react';

import koTranslations from './ko.json';
import enTranslations from './en.json';

// --- 타입 정의 ---

/** 지원 로케일 */
export type Locale = 'ko' | 'en';

/** 번역 함수 타입 */
export type TranslationFn = (key: string) => string;

interface I18nContextValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: TranslationFn;
}

// --- 번역 데이터 ---

const STORAGE_KEY = 'xflow-locale';
const DEFAULT_LOCALE: Locale = 'ko';

const translations: Record<Locale, Record<string, unknown>> = {
  ko: koTranslations as Record<string, unknown>,
  en: enTranslations as Record<string, unknown>,
};

// --- 유틸리티 ---

/** localStorage에서 저장된 로케일을 읽는다. 유효하지 않으면 기본값을 반환한다. */
function getStoredLocale(): Locale {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored === 'ko' || stored === 'en') return stored;
  } catch {
    // localStorage 접근 불가 시 무시
  }
  return DEFAULT_LOCALE;
}

/** 점(dot) 표기법 키로 중첩 객체에서 값을 조회한다. 예: 'nav.dashboard' */
function resolveKey(obj: Record<string, unknown>, key: string): string {
  const parts = key.split('.');
  let current: unknown = obj;

  for (const part of parts) {
    if (current === null || current === undefined || typeof current !== 'object') {
      return key; // 경로를 찾지 못하면 키 자체를 반환
    }
    current = (current as Record<string, unknown>)[part];
  }

  return typeof current === 'string' ? current : key;
}

// --- Context ---

const I18nContext = createContext<I18nContextValue | null>(null);

// --- Provider ---

interface I18nProviderProps {
  children: ReactNode;
}

/**
 * 앱 최상단에 배치하여 i18n 기능을 제공한다.
 * locale, setLocale, t 함수를 하위 컴포넌트에서 useTranslation()으로 사용할 수 있다.
 */
export function I18nProvider({ children }: I18nProviderProps) {
  const [locale, setLocaleState] = useState<Locale>(getStoredLocale);

  const setLocale = useCallback((newLocale: Locale) => {
    setLocaleState(newLocale);
    try {
      localStorage.setItem(STORAGE_KEY, newLocale);
    } catch {
      // localStorage 접근 불가 시 무시
    }
  }, []);

  const t: TranslationFn = useCallback(
    (key: string) => resolveKey(translations[locale], key),
    [locale],
  );

  const value = useMemo<I18nContextValue>(
    () => ({ locale, setLocale, t }),
    [locale, setLocale, t],
  );

  return createElement(I18nContext.Provider, { value }, children);
}

// --- Hook ---

/**
 * i18n 번역 기능을 사용하는 훅.
 * @returns t: 번역 함수, locale: 현재 로케일, setLocale: 로케일 변경 함수
 */
export function useTranslation() {
  const context = useContext(I18nContext);
  if (!context) {
    throw new Error('useTranslation은 I18nProvider 내부에서 사용해야 합니다.');
  }
  return context;
}
