// Package tui
// File: program_box.go
// Description: tea.Program 포인터를 간접 참조하는 컨테이너
// Responsibility: NewApp() 이후에 tea.NewProgram() 결과를 주입할 수 있도록 하여 데드락 없이 Println() 제공

package tui

import tea "github.com/charmbracelet/bubbletea"

// ProgramBox는 tea.Program 포인터를 간접 참조하는 컨테이너다.
// tea.NewProgram() 전에 AppModel을 생성해야 하는 닭·달걀 문제를 해결한다:
//  1. NewProgramBox()로 박스 생성
//  2. 박스를 AppOptions.ProgramBox에 넣어 AppModel 생성
//  3. tea.NewProgram(app) 호출
//  4. box.Set(p)로 프로그램 주입 — p.Run() 전에 호출해도 데드락 없음
type ProgramBox struct {
	p *tea.Program
}

// NewProgramBox는 새 ProgramBox를 생성한다.
func NewProgramBox() *ProgramBox {
	return &ProgramBox{}
}

// Set은 tea.Program 포인터를 주입한다. p.Run() 직전에 호출한다.
func (b *ProgramBox) Set(p *tea.Program) {
	b.p = p
}

// Println은 Program.Println()을 비동기로 호출한다. Update() 내부에서 직접
// Program.Println()을 호출하면 Bubble Tea 이벤트 루프가 자기 채널에
// 동기 송신하면서 데드락될 수 있으므로 goroutine으로 분리한다.
func (b *ProgramBox) Println(s string) {
	if b != nil && b.p != nil {
		go b.p.Println(s)
	}
}
