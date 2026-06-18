# xflowd systemd 통합

원격 자가 업데이트(버전 관리 Phase 2)의 **자동 검증 + 자동 롤백**을 완성하려면 xflowd 를
systemd 서비스로 운영하는 것을 권장합니다.

## 자동 롤백 동작

업데이트는 다음 순서로 안전하게 진행됩니다.

1. **pre-flight 스모크 테스트** — 다운로드·서명 검증 후, 교체 *전*에 후보 바이너리를
   `xflowd verify --config <path>` 로 실행해 기동 가능성(아키텍처/링크/설정 호환)을 확인합니다.
   실패하면 교체하지 않고 중단 → **다운타임 0**.
2. **교체 + (opt-in) 재시작** — 원자적 교체(`.previous` 백업) 후, `restart=true` 면 재시작.
3. **부팅 자가 검증** — 새 바이너리가 부팅하면 `<binary>.update-state` 를 보고 로컬 `/health`
   를 폴링합니다. 정상이면 통과(상태 정리), 비정상이면 **자동 롤백**(`.previous` 복원 후 재시작).
4. **부팅 실패 누적 롤백** — 새 바이너리가 health 서버 기동 *전* 크래시하면 자가 검증 고루틴도
   죽습니다. 이때 **systemd `Restart=on-failure`** 가 다시 띄우면, 다음 부팅에서 `boot_attempts`
   가 누적되어 임계(기본 2) 초과 시 자동 롤백됩니다.

> systemd(또는 동등한 supervisor) 없이 단독 실행하면 4번(기동 전 크래시)은 복구되지
> 않습니다. 1~3번은 supervisor 없이도 동작합니다.

## 설치

```bash
sudo cp examples/systemd/xflowd.service /etc/systemd/system/xflowd.service
# User/Group/ExecStart 경로를 환경에 맞게 수정
sudo systemctl daemon-reload
sudo systemctl enable --now xflowd
```

## 참고

- `xflowd verify [--config <path>]` 는 설정만 로드·검증하고 종료합니다(서버/에이전트/포트
  바인딩 없음). pre-flight 스모크 테스트에 사용되며 수동 점검에도 유용합니다.
- 시리얼 장치(RS-485 등) 접근이 필요하면 유닛에 `DeviceAllow=` / 적절한 그룹 권한을 추가하세요.
