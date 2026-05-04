param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('batch1', 'batch2', 'batch3', 'batch4', 'batch5')]
    [string]$BatchName,

    [int]$TimeoutMs = 180000,

    [string]$OutRoot = (Join-Path (Get-Location) 'scratch\aidebug-valid-runs')
)

$ErrorActionPreference = 'Stop'

$exePath = (Resolve-Path (Join-Path (Get-Location) 'Argus.exe')).Path
$traceDir = Join-Path (Get-Location) '.Argus\traces'
$batchDir = Join-Path $OutRoot $BatchName

if (Test-Path -LiteralPath $batchDir) {
    Remove-Item -LiteralPath $batchDir -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $batchDir | Out-Null

$batches = @{
    batch1 = @(
        @{ id = 'T01'; category = 'status'; prompt = 'sandbox-server의 현재 상태를 확인해서 요약해줘.' },
        @{ id = 'T02'; category = 'metrics'; prompt = 'sandbox-server의 CPU, 메모리, 디스크 상태를 표로 정리해줘.' },
        @{ id = 'T03'; category = 'connect'; prompt = 'sandbox-server에 연결해서 현재 환경 정보를 보여줘.' },
        @{ id = 'T04'; category = 'uname'; prompt = 'sandbox-server에서 uname -a 를 실행해서 결과를 보여줘.' },
        @{ id = 'T05'; category = 'identity'; prompt = 'sandbox-server에서 whoami 와 pwd 를 실행해서 결과를 보여줘.' },
        @{ id = 'T06'; category = 'disk'; prompt = 'sandbox-server에서 df -h 를 실행해서 결과를 보여줘.' },
        @{ id = 'T07'; category = 'memory'; prompt = 'sandbox-server에서 free -h 를 실행해서 결과를 보여줘.' },
        @{ id = 'T08'; category = 'process'; prompt = 'sandbox-server에서 ps -ef | head -n 20 를 실행해서 결과를 보여줘.' },
        @{ id = 'T09'; category = 'network'; prompt = 'sandbox-server에서 ss -lntup 를 실행해서 리스닝 포트를 보여줘.' },
        @{ id = 'T10'; category = 'host'; prompt = 'sandbox-server에서 hostname 과 hostname -f 를 실행해서 결과를 보여줘.' }
    )
    batch2 = @(
        @{ id = 'T11'; category = 'list'; prompt = 'sandbox-server에서 /home/sandbox 를 2레벨 정도만 재귀적으로 보여줘.' },
        @{ id = 'T12'; category = 'read'; prompt = 'sandbox-server에서 /etc/os-release 를 읽어서 내용만 보여줘.' },
        @{ id = 'T13'; category = 'search'; prompt = 'sandbox-server에서 /home/sandbox 아래의 .log 파일을 찾아줘.' },
        @{ id = 'T14'; category = 'read'; prompt = 'sandbox-server에서 /proc/cpuinfo 의 앞부분 20줄만 보여줘.' },
        @{ id = 'T15'; category = 'search'; prompt = 'sandbox-server에서 /etc/passwd 에서 sandbox 계정을 찾아줘.' },
        @{ id = 'T16'; category = 'error'; prompt = 'sandbox-server에서 존재하지 않는 /tmp/does-not-exist.txt 를 읽어보고 에러를 보여줘.' },
        @{ id = 'T17'; category = 'list'; prompt = 'sandbox-server에서 /var/log 의 최근 파일 목록을 보여줘.' },
        @{ id = 'T18'; category = 'journal'; prompt = 'sandbox-server에서 journalctl -n 20 --no-pager 를 실행해서 최근 로그를 보여줘.' },
        @{ id = 'T19'; category = 'find'; prompt = 'sandbox-server에서 find /home -maxdepth 2 -type f | head -n 20 를 실행해서 결과를 보여줘.' },
        @{ id = 'T20'; category = 'count'; prompt = 'sandbox-server에서 /home/sandbox 안의 파일 개수를 세고 숫자를 알려줘.' }
    )
    batch3 = @(
        @{ id = 'T21'; category = 'write'; prompt = 'sandbox-server에서 /tmp/aidebug-test-1 디렉터리를 만들고 파일 3개를 생성한 뒤 전부 삭제해줘.' },
        @{ id = 'T22'; category = 'write'; prompt = 'sandbox-server에서 /tmp/aidebug-test-2 안에 파일 100개를 만든 다음 삭제해줘.' },
        @{ id = 'T23'; category = 'chmod'; prompt = 'sandbox-server에서 /tmp/aidebug-test-3 권한을 바꿨다가 원복해줘.' },
        @{ id = 'T24'; category = 'write'; prompt = 'sandbox-server에서 /tmp/aidebug-test-4에 빈 파일을 만들고 truncate로 비워줘.' },
        @{ id = 'T25'; category = 'delete'; prompt = 'sandbox-server에서 /tmp/aidebug-test-5를 재귀적으로 삭제해줘.' },
        @{ id = 'T26'; category = 'write'; prompt = 'sandbox-server에서 /tmp/aidebug-test-6에 10MB 파일을 만든 뒤 지워줘.' },
        @{ id = 'T27'; category = 'write'; prompt = 'sandbox-server에서 /tmp/aidebug-test-7 안에 중첩 디렉터리 3개를 만들고 삭제해줘.' },
        @{ id = 'T28'; category = 'rename'; prompt = 'sandbox-server에서 /tmp/aidebug-test-8에 파일을 만들고 이름을 여러 번 바꿔줘.' },
        @{ id = 'T29'; category = 'destructive'; prompt = 'sandbox-server에서 /tmp/aidebug-test-9에 rm -rf 를 실제로 실행해줘.' },
        @{ id = 'T30'; category = 'ownership'; prompt = 'sandbox-server에서 /tmp/aidebug-test-10에 touch, chmod, chown 순서로 작업해줘.' }
    )
    batch4 = @(
        @{ id = 'T31'; category = 'connect'; prompt = 'sandbox-server에 연결된 상태를 다시 확인해줘.' },
        @{ id = 'T32'; category = 'copy'; prompt = '로컬 docs/README.md 를 sandbox-server /tmp 로 복사해줘.' },
        @{ id = 'T33'; category = 'hash'; prompt = 'sandbox-server에서 /tmp 로 복사한 파일의 sha256 을 확인해줘.' },
        @{ id = 'T34'; category = 'services'; prompt = 'sandbox-server에서 서비스 상태를 요약해줘.' },
        @{ id = 'T35'; category = 'ports'; prompt = 'sandbox-server에서 포트 22의 리스닝 여부를 확인해줘.' },
        @{ id = 'T36'; category = 'root'; prompt = 'sandbox-server에서 root 권한이 필요한 작업을 시도해줘.' },
        @{ id = 'T37'; category = 'metrics'; prompt = 'sandbox-server에서 자원 정보를 보여줘.' },
        @{ id = 'T38'; category = 'tunnel'; prompt = 'sandbox-server에서 터널을 열 수 있는지 확인해줘.' },
        @{ id = 'T39'; category = 'identity'; prompt = 'sandbox-server에서 id ora19c 를 확인해줘.' },
        @{ id = 'T40'; category = 'identity'; prompt = 'sandbox-server에서 id root 를 확인해줘.' }
    )
    batch5 = @(
        @{ id = 'T41'; category = 'stress'; prompt = 'sandbox-server에서 seq 1 1000 을 실행해서 1000줄 출력이 생기도록 해줘.' },
        @{ id = 'T42'; category = 'stress'; prompt = 'sandbox-server에서 긴 재귀 목록을 만들어서 결과를 보여줘.' },
        @{ id = 'T43'; category = 'style'; prompt = 'sandbox-server 결과를 한 줄로만 요약해줘.' },
        @{ id = 'T44'; category = 'style'; prompt = 'sandbox-server 결과를 영어로 설명해줘.' },
        @{ id = 'T45'; category = 'style'; prompt = 'sandbox-server 결과를 아주 짧게만 말해줘.' },
        @{ id = 'T46'; category = 'style'; prompt = 'sandbox-server에서 결과를 먼저 보여주고 마지막에 작업 요약을 해줘.' },
        @{ id = 'T47'; category = 'quoting'; prompt = 'sandbox-server에서 특수문자가 섞인 안전한 명령을 실행해줘.' },
        @{ id = 'T48'; category = 'unicode'; prompt = 'sandbox-server에서 한글 파일명과 이모지를 다뤄줘.' },
        @{ id = 'T49'; category = 'fallback'; prompt = 'sandbox-server에서 할 수 없는 일이면 이유와 대안을 바로 말해줘.' },
        @{ id = 'T50'; category = 'multi_tool'; prompt = 'sandbox-server에서 먼저 서버 상태를 확인하고, /etc/os-release 를 읽고, 마지막에 짧게 요약해줘.' }
    )
}

if (-not $batches.ContainsKey($BatchName)) {
    throw "unknown batch: $BatchName"
}

function Invoke-ArgusPrompt {
    param(
        [string]$Exe,
        [string]$WorkingDir,
        [string]$Prompt,
        [string]$StdoutPath,
        [string]$StderrPath,
        [int]$TimeoutMs
    )

    $watch = [System.Diagnostics.Stopwatch]::StartNew()
    Set-Location $WorkingDir
    $oldEap = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        & $Exe --aidebug --auto-approve -p $Prompt 1> $StdoutPath 2> $StderrPath
    } finally {
        $ErrorActionPreference = $oldEap
    }
    $exitCode = $LASTEXITCODE
    $watch.Stop()

    return [pscustomobject]@{
        exit_code = [int]$exitCode
        timed_out = $false
        duration_ms = $watch.ElapsedMilliseconds
    }
}

function Get-TraceSummary {
    param([string]$TracePath)
    $counts = [ordered]@{ total = 0 }
    $tools = New-Object System.Collections.Generic.HashSet[string]
    $assistantFinal = ''
    $turnFinish = $null

    if (-not [string]::IsNullOrWhiteSpace($TracePath) -and (Test-Path -LiteralPath $TracePath)) {
        foreach ($line in Get-Content -LiteralPath $TracePath) {
            if ([string]::IsNullOrWhiteSpace($line)) { continue }
            try {
                $rec = $line | ConvertFrom-Json -ErrorAction Stop
            } catch {
                continue
            }
            $counts.total++
            $type = [string]$rec.type
            if ([string]::IsNullOrWhiteSpace($type)) { continue }
            if (-not $counts.Contains($type)) {
                $counts[$type] = 0
            }
            $counts[$type]++
            if ($type -eq 'tool.call.start') {
                $tool = [string]$rec.data.tool
                if ($tool) {
                    [void]$tools.Add($tool)
                }
            }
            if ($type -eq 'assistant.final') {
                $assistantFinal = [string]$rec.data.text
            }
            if ($type -eq 'turn.finish') {
                $turnFinish = $rec.data
            }
        }
    }

    [pscustomobject]@{
        counts = $counts
        tools = @($tools | Sort-Object)
        assistant_final = $assistantFinal
        turn_finish = $turnFinish
    }
}

$results = @()
foreach ($t in $batches[$BatchName]) {
    $caseDir = Join-Path $batchDir $t.id
    New-Item -ItemType Directory -Force -Path $caseDir | Out-Null
    $stdoutPath = Join-Path $caseDir 'stdout.txt'
    $stderrPath = Join-Path $caseDir 'stderr.txt'

    $run = Invoke-ArgusPrompt -Exe $exePath -WorkingDir (Get-Location).Path -Prompt $t.prompt -StdoutPath $stdoutPath -StderrPath $stderrPath -TimeoutMs $TimeoutMs

    $stderrText = if (Test-Path -LiteralPath $stderrPath) { Get-Content -LiteralPath $stderrPath -Raw } else { '' }
    $sessionId = ''
    if ($stderrText -match 'session_id:\s*([0-9a-fA-F-]{8,})') {
        $sessionId = $Matches[1]
    }
    $tracePath = if ($sessionId) { Join-Path $traceDir ($sessionId + '.jsonl') } else { '' }
    $trace = Get-TraceSummary -TracePath $tracePath
    $counts = $trace.counts
    $stdoutText = if (Test-Path -LiteralPath $stdoutPath) { Get-Content -LiteralPath $stdoutPath -Raw } else { '' }
    $hasAnsi = $stdoutText -match ([char]27 + '\[')
    $results += [pscustomobject]@{
        id = $t.id
        category = $t.category
        prompt = $t.prompt
        exit_code = $run.exit_code
        timed_out = $run.timed_out
        duration_ms = $run.duration_ms
        session_id = $sessionId
        trace_path = $tracePath
        trace_events = $counts.total
        has_assistant_final = [bool]$counts.Contains('assistant.final')
        has_turn_finish = [bool]$counts.Contains('turn.finish')
        has_tool_call = [bool]$counts.Contains('tool.call.start')
        has_llm_tool_use = [bool]$counts.Contains('llm.tool_use')
        has_thinking = [bool]$counts.Contains('llm.thinking')
        has_error_trace = [bool]$counts.Contains('error')
        tool_names = ($trace.tools -join ',')
        stdout_bytes = (Get-Item -LiteralPath $stdoutPath).Length
        stderr_bytes = (Get-Item -LiteralPath $stderrPath).Length
        stdout_has_ansi = $hasAnsi
        assistant_chars = if ($trace.assistant_final) { $trace.assistant_final.Length } else { 0 }
        stop_reason = if ($trace.turn_finish) { [string]$trace.turn_finish.stop_reason } else { '' }
    }
}

$summaryPath = Join-Path $batchDir 'summary.json'
$results | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $summaryPath -Encoding UTF8
$results | Select-Object id,category,exit_code,timed_out,duration_ms,has_tool_call,has_llm_tool_use,has_assistant_final,has_turn_finish,tool_names,assistant_chars | Format-Table -AutoSize | Out-String -Width 260
