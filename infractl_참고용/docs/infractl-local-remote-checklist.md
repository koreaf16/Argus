# Infractl 로컬/원격 판별 체크리스트

## 목적
로컬 경로 요청(`C:\...`)을 원격 제약으로 오판하는 실수를 방지한다.  
원칙은 `컨텍스트 검증 -> 대상 경로 검증 -> 사실 기반 응답`이다.

## 1) 컨텍스트 1차 검증 (필수)
요청 처리 전에 현재 실행 위치를 먼저 확인한다.

### PowerShell
```powershell
Get-Location
[Environment]::MachineName
$env:USERNAME
$env:SSH_CONNECTION
```

### Bash
```bash
pwd
hostname
whoami
echo "$SSH_CONNECTION"
```

판정 규칙:
- Windows 경로가 보이고 대상 경로 접근이 되면 로컬 Windows로 취급한다.
- `SSH_CONNECTION` 존재만으로 "로컬 경로 접근 불가"를 단정하지 않는다.
- 접근 가능 여부는 항상 `대상 경로 실제 검사`로 확정한다.

## 2) 대상 경로 검증 (필수)
사용자가 요청한 경로를 직접 검사한다.

```powershell
$target = "C:\Users\jhkwa\Downloads"
Test-Path $target
Get-ChildItem -Name $target
```

판정 규칙:
- `Test-Path=True`: 접근 가능. 요청한 목록/필터 결과를 바로 제공.
- `Test-Path=False`: "접근 불가"가 아니라 "해당 경로 미존재/권한 문제"로 사실만 전달.

## 3) 키워드 결론 규칙 (Oracle 예시)
```powershell
Get-ChildItem -File $target |
  Where-Object Name -match "oracle|26ai|db_home|sqlcl" |
  Select-Object Name, LastWriteTime, Length
```

결론 문구 기준:
- 결과 없음: `키워드 기준 파일은 확인되지 않았습니다.`
- `sqlcl`만 있음: `Oracle 관련 파일은 있으나 26ai DB home 설치본으로 보이는 파일은 미확인입니다.`
- `db_home`/`26ai` 있음: `26ai 설치 후보 파일이 확인되었습니다.`

## 4) 응답 템플릿 (고정)
아래 순서로만 응답한다.

1. 실행 환경 사실 (`cwd`, `host`, `shell`)
2. 검증 명령과 결과 요약 (`Test-Path`, 목록/필터)
3. 결론 (단정 대신 증거 기반 문장)
4. 다음 액션 1-2개 (요청 범위 안에서만)

## 5) 금지 패턴
- 검증 없이 `원격 서버라 C:\ 접근 불가` 단정
- 사용자 요청(목록 확인) 전에 설치/마이그레이션 가이드로 확장
- 집계표만 제공하고 원본 파일명/필터 결과 생략

## 6) 30초 표준 명령 세트 (PowerShell)
```powershell
$target = "C:\Users\jhkwa\Downloads"
Write-Host "cwd=$((Get-Location).Path) host=$env:COMPUTERNAME user=$env:USERNAME"
Write-Host "ssh_connection=$env:SSH_CONNECTION"

if (-not (Test-Path $target)) {
  Write-Host "TARGET_NOT_FOUND: $target"
  exit 1
}

Get-ChildItem -Name $target
Get-ChildItem -File $target |
  Where-Object Name -match "oracle|26ai|db_home|sqlcl" |
  Select-Object Name, LastWriteTime, Length
```
