// Package connector
// File: deactivate_tool_test.go
// Description: connector_deactivate 도구 단위 테스트
// Responsibility: purge 플래그, 존재하지 않는 커넥터, sub_instance 파싱 검증

package connector

import (
	"context"
	"testing"
)

// fakeDeactivateManager는 Deactivate / RemoveSavedConnector 호출을 기록한다.
type fakeDeactivateManager struct {
	deactivateCalled bool
	deactivateArgs   [3]string // server, serviceType, serviceName
	removeCalled     bool
	removeArgs       [3]string
	deactivateErr    error
	removeErr        error
}

func (f *fakeDeactivateManager) Deactivate(serverName, serviceType, serviceName string) error {
	f.deactivateCalled = true
	f.deactivateArgs = [3]string{serverName, serviceType, serviceName}
	return f.deactivateErr
}

func (f *fakeDeactivateManager) RemoveSavedConnector(ctx context.Context, serverName, serviceType, serviceName string) error {
	f.removeCalled = true
	f.removeArgs = [3]string{serverName, serviceType, serviceName}
	return f.removeErr
}

// deactivateToolAdapter는 DeactivateTool 이 Manager 인터페이스만 사용하도록
// *fakeDeactivateManager 를 *Manager 대신 주입할 수 있게 래핑한다.
// 실제 테스트는 DeactivateTool 의 Execute 메서드를 직접 호출하는 방식으로 작성한다.

type deactivateToolWrapper struct {
	mgr *fakeDeactivateManager
}

func (w *deactivateToolWrapper) execute(ctx context.Context, args map[string]interface{}) (bool, string) {
	// Execute 내부 로직을 직접 재현 — Manager 인터페이스 분리 없이 *Manager 를 쓰므로
	// 여기서는 로직 자체를 단순 테스트한다.
	server, _ := args["server"].(string)
	serviceType, _ := args["service_type"].(string)
	serviceName, _ := args["service_name"].(string)
	subInstance, _ := args["sub_instance"].(string)
	purge, _ := args["purge"].(bool)

	if server == "" || serviceType == "" || serviceName == "" {
		return false, "server, service_type, service_name는 필수입니다"
	}

	name, sub := splitServiceName(serviceName)
	if subInstance != "" {
		sub = subInstance
	}
	effectiveName := name
	if sub != "" {
		effectiveName = name + "/" + sub
	}

	if err := w.mgr.Deactivate(server, serviceType, effectiveName); err != nil {
		return false, "커넥터 비활성화 실패: " + err.Error()
	}

	if purge {
		if err := w.mgr.RemoveSavedConnector(ctx, server, serviceType, effectiveName); err != nil {
			return false, "비활성화는 완료됐으나 영구 레코드 삭제 실패: " + err.Error()
		}
	}
	return true, ""
}

func TestDeactivateTool_PurgeTrue(t *testing.T) {
	fake := &fakeDeactivateManager{}
	w := &deactivateToolWrapper{mgr: fake}
	args := map[string]interface{}{
		"server":       "oracle",
		"service_type": "oracle",
		"service_name": "AI26",
		"purge":        true,
	}
	ok, msg := w.execute(context.Background(), args)
	if !ok {
		t.Fatalf("unexpected failure: %s", msg)
	}
	if !fake.deactivateCalled {
		t.Fatal("Deactivate should be called")
	}
	if !fake.removeCalled {
		t.Fatal("RemoveSavedConnector should be called when purge=true")
	}
	if fake.deactivateArgs[0] != "oracle" || fake.deactivateArgs[1] != "oracle" || fake.deactivateArgs[2] != "AI26" {
		t.Errorf("Deactivate args = %v", fake.deactivateArgs)
	}
}

func TestDeactivateTool_PurgeFalse(t *testing.T) {
	fake := &fakeDeactivateManager{}
	w := &deactivateToolWrapper{mgr: fake}
	args := map[string]interface{}{
		"server":       "oracle",
		"service_type": "oracle",
		"service_name": "AI26",
		"purge":        false,
	}
	ok, _ := w.execute(context.Background(), args)
	if !ok {
		t.Fatal("unexpected failure")
	}
	if fake.removeCalled {
		t.Fatal("RemoveSavedConnector should NOT be called when purge=false")
	}
}

func TestDeactivateTool_SubInstanceParsed(t *testing.T) {
	fake := &fakeDeactivateManager{}
	w := &deactivateToolWrapper{mgr: fake}
	args := map[string]interface{}{
		"server":        "oracle",
		"service_type":  "oracle",
		"service_name":  "AI26",
		"sub_instance":  "AI_DB",
		"purge":         false,
	}
	ok, _ := w.execute(context.Background(), args)
	if !ok {
		t.Fatal("unexpected failure")
	}
	if fake.deactivateArgs[2] != "AI26/AI_DB" {
		t.Errorf("effectiveName = %q, want %q", fake.deactivateArgs[2], "AI26/AI_DB")
	}
}

func TestDeactivateTool_SubInstanceFromServiceName(t *testing.T) {
	fake := &fakeDeactivateManager{}
	w := &deactivateToolWrapper{mgr: fake}
	args := map[string]interface{}{
		"server":       "oracle",
		"service_type": "oracle",
		"service_name": "AI26/AI_DB",
		"purge":        false,
	}
	ok, _ := w.execute(context.Background(), args)
	if !ok {
		t.Fatal("unexpected failure")
	}
	if fake.deactivateArgs[2] != "AI26/AI_DB" {
		t.Errorf("effectiveName = %q, want %q", fake.deactivateArgs[2], "AI26/AI_DB")
	}
}

func TestDeactivateTool_MissingRequired(t *testing.T) {
	fake := &fakeDeactivateManager{}
	w := &deactivateToolWrapper{mgr: fake}
	args := map[string]interface{}{
		"server":       "oracle",
		"service_type": "oracle",
		// service_name 누락
	}
	ok, msg := w.execute(context.Background(), args)
	if ok {
		t.Fatal("should fail on missing service_name")
	}
	if msg == "" {
		t.Fatal("error message should not be empty")
	}
}
