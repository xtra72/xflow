// Package century 는 Century HVAC 마스터-슬레이브 바이너리 프로토콜의
// RS-485 회선 패시브 캡처(passive sniff) 에이전트와 관련 유틸리티를 제공한다.
//
// 본 패키지는 SPEC-CENTURY-001 의 구현이며, M1 (Foundation) 범위는
// CRC-16/ARC 알고리즘, 헤더 + payload + CRC 단일 프레임 파서, 그리고
// 바이트 스트림으로부터 프레임을 추출하는 스캐너이다. 후속 마일스톤
// (M2 디코더, M3 에이전트 런타임 + ring buffer + DeviceProvider, M4 노드)
// 은 별도 파일에서 점진적으로 추가된다.
//
// 본 에이전트는 회선에 RX-only 로 부착되며 어떤 경우에도 바이트를 송신하지 않는다.
package century
