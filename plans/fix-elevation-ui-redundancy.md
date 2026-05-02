# 설정 메뉴 Elevation UI 중복 제거 및 개선 계획

## 1. 개요
사용자가 지적한 "Allow elevation" 토글과 "Allow root" 체크박스 간의 논리적/UI적 중복 문제를 해결하기 위해, Elevation 설정을 더 직관적인 구조로 개편합니다.

## 2. 문제점 분석
- **중복된 단계**: Elevation 기능을 켜기 위해 토글을 [x] ON으로 바꾼 후, 다시 Method를 선택하고, 또 "Allow root"를 체크해야 함.
- **모호한 용어**: "Allow elevation"이 사실상 "Allow root"와 동일하게 인식되어 사용자에게 혼란을 줌.

## 3. 개선 방향 (렌더링 예상도)

기존:
```
─── Work Accounts (su / sudo) ───
  Allow elevation: [x] ON
  Method         : (●) sudo   ( ) su
  Allow root     : [x] Yes (pw: (uses Login Password))
```

개선안:
```
─── Work Accounts (su / sudo) ───
> Elevation      : ( ) None  (●) sudo  ( ) su
  Allow root     : [x] Yes (pw: (uses Login Password))
```

- **통합**: "Allow elevation" 토글과 "Method" 라디오를 "Elevation" 하나로 통합 (None / sudo / su).
- **자동화**: Elevation 모드를 sudo나 su로 선택하면 "Allow root"가 기본적으로 [x] Yes로 설정되도록 하여 단계를 단축.

## 4. 상세 작업 내용

### 4.1. `internal/tui/modal_server_form.go` 수정
- `activeServerFormFields`: `sfAllowElevation`을 제거하고 `sfElevationMethod`가 항상 노출되도록 수정.
- `handleServerFormKey`:
  - `sfElevationMethod`에서 'Space' 또는 'Left/Right' 입력 시 `None -> sudo -> su` 순환하도록 로직 변경.
  - 모드 변경 시 `sf.AllowElevation` 및 `sf.AllowRoot` 상태를 자동으로 업데이트.
- `submitServerForm`: 변경된 상태값들이 백엔드 구조체(`Elevation`)에 올바르게 매핑되도록 검증.

### 4.2. `internal/tui/modal_server_form_render.go` 수정
- `elevMethodDisplay`: 3가지 상태(None, sudo, su)를 표시할 수 있도록 수정.
- `modalServerFormRender`: "Allow elevation" 렌더링을 제거하고 "Elevation" 라디오 버튼을 최상단에 배치.

### 4.3. `internal/tui/model.go` 수정
- 서버 정보 로드 시(`serverEntryToFormState`), 기존의 `Allowed` 및 `Mode` 값을 새로운 TUI 상태로 올바르게 변환하도록 수정.

## 5. 검증 계획
- `argus` 빌드 후 서버 추가/편집 화면 진입.
- Elevation 모드를 None에서 sudo/su로 변경할 때 하위 필드(Allow root 등)가 나타나는지 확인.
- 설정 저장 후 다시 편집 화면에서 값이 유지되는지 확인.
- `--aidebug` 모드에서 UI 출력이 예상도와 일치하는지 확인.
