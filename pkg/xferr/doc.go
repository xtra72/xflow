// Package xferr 는 xflow 에러 및 데드레터 메시지 시스템을 제공한다.
//
// xferr 패키지는 에러 분류, 에러 메시지 래핑, 데드레터 메시지 관리,
// 상태 이벤트 처리, 폐기 정책, 에러 라우팅 기능을 포함한다.
//
// 주요 구성 요소:
//   - ErrorMessage: 원본 메시지와 에러 정보를 래핑하는 인터페이스
//   - DeadLetterMessage: 처리 불가능한 메시지를 보관하는 인터페이스
//   - StatusEvent: 컴포넌트 상태 변경 이벤트 인터페이스
//   - DiscardPolicy: 수신자 없는 에러/데드레터 처리 정책
//   - ErrorRouter: 에러 및 데드레터 메시지 라우팅 인터페이스
//   - AlertThreshold: 경고 임계값 설정 타입
package xferr
