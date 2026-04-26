package tools

import (
	"context"
	"fmt"
)

type outputCallbackKey struct{}
type questionCallbackKey struct{}
type formCallbackKey struct{}

// QuestionCallback은 에이전트가 사용자에게 질문을 던질 때 호출된다.
type QuestionCallback func(context.Context, QuestionRequest) (QuestionResponse, error)

// FormCallback은 에이전트가 사용자에게 폼 입력을 요청할 때 호출된다.
type FormCallback func(context.Context, FormRequest) (FormResponse, error)

// WithOutputCallback attaches a per-execution output callback to context.
func WithOutputCallback(ctx context.Context, cb func(string)) context.Context {
	if cb == nil {
		return ctx
	}
	return context.WithValue(ctx, outputCallbackKey{}, cb)
}

// WithUICallbacks attaches UI interaction callbacks (question/form) to context.
func WithUICallbacks(ctx context.Context, qcb QuestionCallback, fcb FormCallback) context.Context {
	if qcb != nil {
		ctx = context.WithValue(ctx, questionCallbackKey{}, qcb)
	}
	if fcb != nil {
		ctx = context.WithValue(ctx, formCallbackKey{}, fcb)
	}
	return ctx
}

// EmitOutput sends a line to the callback in context when available.
func EmitOutput(ctx context.Context, line string) {
	if line == "" {
		return
	}
	cb, ok := ctx.Value(outputCallbackKey{}).(func(string))
	if !ok || cb == nil {
		return
	}
	cb(line)
}

// RequestQuestion은 Context에서 질문 콜백을 찾아 실행한다.
func RequestQuestion(ctx context.Context, req QuestionRequest) (QuestionResponse, error) {
	cb, ok := ctx.Value(questionCallbackKey{}).(QuestionCallback)
	if !ok || cb == nil {
		return QuestionResponse{}, fmt.Errorf("no question callback in context")
	}
	return cb(ctx, req)
}

// RequestForm은 Context에서 폼 콜백을 찾아 실행한다.
func RequestForm(ctx context.Context, req FormRequest) (FormResponse, error) {
	cb, ok := ctx.Value(formCallbackKey{}).(FormCallback)
	if !ok || cb == nil {
		return FormResponse{}, fmt.Errorf("no form callback in context")
	}
	return cb(ctx, req)
}
