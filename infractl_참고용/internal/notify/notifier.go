// Package notify
// File: notifier.go
// Description: OS 수준 알림 트리거 (macOS, Linux, Windows 지원)
// Responsibility: 플랫폼별 명령어를 사용하여 바탕화면 알림 표시

package notify

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

// Notify 는 OS 알림을 표시한다.
func Notify(ctx context.Context, title, message string) error {
	switch runtime.GOOS {
	case "darwin":
		return notifyDarwin(ctx, title, message)
	case "linux":
		return notifyLinux(ctx, title, message)
	case "windows":
		return notifyWindows(ctx, title, message)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func notifyDarwin(ctx context.Context, title, message string) error {
	script := fmt.Sprintf(`display notification "%s" with title "%s"`, message, title)
	return exec.CommandContext(ctx, "osascript", "-e", script).Run()
}

func notifyLinux(ctx context.Context, title, message string) error {
	// notify-send 가 있는지 확인 후 실행
	return exec.CommandContext(ctx, "notify-send", title, message).Run()
}

func notifyWindows(ctx context.Context, title, message string) error {
	// PowerShell 을 사용하여 토스트 알림 표시
	// BurntToast 모듈이 없어도 작동하는 기본 방식 사용
	psCommand := fmt.Sprintf(
		`[void][System.Reflection.Assembly]::LoadWithPartialName('System.Windows.Forms'); `+
			`$notification = New-Object System.Windows.Forms.NotifyIcon; `+
			`$notification.Icon = [System.Drawing.SystemIcons]::Information; `+
			`$notification.Visible = $true; `+
			`$notification.ShowBalloonTip(5000, '%s', '%s', [System.Windows.Forms.ToolTipIcon]::Info); `+
			`Start-Sleep -Seconds 1; `+
			`$notification.Dispose()`,
		title, message,
	)
	return exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", psCommand).Run()
}
