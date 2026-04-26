# File: system_risk.ps1
# Description: Fast-path command-risk filter for Windows PowerShell.
# Responsibility: Read hook input JSON from stdin and emit HookOutput JSON.

function Write-Allow {
    @{ decision = 'allow' } | ConvertTo-Json -Compress
    exit 0
}

function Write-Deny([string]$reason) {
    [ordered]@{
        decision       = 'deny'
        reason         = $reason
        systemMessage  = "Blocked: $reason"
    } | ConvertTo-Json -Compress
    exit 0
}

$inputJson = [Console]::In.ReadToEnd()
if ([string]::IsNullOrWhiteSpace($inputJson)) {
    $inputJson = $Input | Out-String
}
if ([string]::IsNullOrWhiteSpace($inputJson)) {
    Write-Allow
}

try {
    $hookInput = $inputJson | ConvertFrom-Json -ErrorAction Stop
} catch {
    Write-Allow
}

$tool = $hookInput.tool
$cmd = $null

if ($hookInput.PSObject.Properties['input']) {
    $nested = $hookInput.input
    if ($nested -ne $null -and $nested.PSObject.Properties['command']) {
        $cmd = [string]$nested.command
    }
}
if ([string]::IsNullOrWhiteSpace($cmd) -and $hookInput.PSObject.Properties['command']) {
    $cmd = [string]$hookInput.command
}

if ($tool -notmatch '^(bash|shell_exec|shell|exec)$') {
    Write-Allow
}
if ([string]::IsNullOrWhiteSpace($cmd)) {
    Write-Allow
}

if ($cmd -match '(^|[^A-Za-z_])rm\s+(-[A-Za-z0-9]*r[A-Za-z0-9]*\s+)+(-[A-Za-z0-9]+\s+)*(/\*?(\s|$))') {
    Write-Deny 'rm -rf critical path'
}
if ($cmd -match '(^|[^A-Za-z_])rm\s+(-[A-Za-z0-9]*r[A-Za-z0-9]*\s+)+(-[A-Za-z0-9]+\s+)*\/(etc|var|usr|bin|sbin|boot|root|proc|sys|dev)(/.*)?(\s|$)') {
    Write-Deny 'rm -rf critical path'
}
if ($cmd -match '(^|[^A-Za-z_])rm\s+(-[A-Za-z0-9]*r[A-Za-z0-9]*\s+)+(-[A-Za-z0-9]+\s+)*(~|\$HOME)(\s|$)') {
    Write-Deny 'rm -rf home directory'
}
if ($cmd -match 'dd\s+.*of=/dev/(sd[a-z]|nvme|hd[a-z]|xvd)') {
    Write-Deny 'dd to device'
}
if ($cmd -match '(^|[^A-Za-z_])mkfs(\.[A-Za-z0-9]+)?\s+/dev/') {
    Write-Deny 'mkfs device'
}
if ($cmd -match ':\(\)\s*\{\s*:\s*\|\s*:') {
    Write-Deny 'fork bomb'
}
if ($cmd -match 'chmod\s+(-R\s+)?777\s+(/|/\*|/etc|/var|/usr|/bin|/sbin|/root)(/\*)?(\s|$)') {
    Write-Deny 'chmod 777 critical path'
}
if ($cmd -match 'chown\s+-R\s+\S+\s+(/|/etc|/var|/usr|/bin|/sbin|/root)(/\*)?(\s|$)') {
    Write-Deny 'chown -R critical path'
}
if ($cmd -match '(^|[^A-Za-z_])iptables\s+(-F|--flush)(\s|$)') {
    Write-Deny 'iptables flush'
}
if ($cmd -match '(^|[^A-Za-z_])ufw\s+disable(\s|$)') {
    Write-Deny 'ufw disable'
}
if ($cmd -match '>\s*/dev/(sd[a-z]|nvme|hd[a-z]|xvd)') {
    Write-Deny 'redirection to device'
}

Write-Allow
