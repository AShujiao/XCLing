package rpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type echoResult struct {
	Text  string `json:"text"`
	Count int    `json:"count"`
}

type fakeService struct{}

func (f *fakeService) Echo(text string, count int) (echoResult, error) {
	return echoResult{Text: text, Count: count}, nil
}

func (f *fakeService) Fail() (echoResult, error) {
	return echoResult{}, errors.New("ADMIN_REQUIRED: 需要管理员")
}

func (f *fakeService) Drop(id string) error {
	if id == "bad" {
		return errors.New("INVALID_STATE: 不可删除")
	}
	return nil
}

func (f *fakeService) Panics() (string, error) { panic("boom") }

func (f *fakeService) Secret() string { return "不应可达" }

func (f *fakeService) BadShape() (int, string) { return 0, "" }

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	registry := NewRegistry()
	if err := registry.Register("Fake", &fakeService{}, "Echo", "Fail", "Drop", "Panics"); err != nil {
		t.Fatalf("register: %v", err)
	}
	return registry
}

func rawParams(t *testing.T, values ...interface{}) []json.RawMessage {
	t.Helper()
	params := make([]json.RawMessage, 0, len(values))
	for _, v := range values {
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal param: %v", err)
		}
		params = append(params, data)
	}
	return params
}

func TestRegisterRejectsMissingMethod(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("Fake", &fakeService{}, "NoSuchMethod"); err == nil {
		t.Fatal("expected error for missing method")
	}
}

func TestRegisterRejectsBadSignature(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("Fake", &fakeService{}, "BadShape"); err == nil {
		t.Fatal("expected error for unsupported signature")
	}
}

func TestRegisterRejectsDuplicateService(t *testing.T) {
	registry := newTestRegistry(t)
	if err := registry.Register("Fake", &fakeService{}, "Echo"); err == nil {
		t.Fatal("expected error for duplicate service")
	}
}

func TestDispatchSuccess(t *testing.T) {
	registry := newTestRegistry(t)
	result, err := registry.Dispatch("Fake.Echo", rawParams(t, "你好", 3))
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	echoed, ok := result.(echoResult)
	if !ok || echoed.Text != "你好" || echoed.Count != 3 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestDispatchServiceErrorPassthrough(t *testing.T) {
	registry := newTestRegistry(t)
	_, err := registry.Dispatch("Fake.Fail", nil)
	if err == nil || !strings.HasPrefix(err.Error(), "ADMIN_REQUIRED:") {
		t.Fatalf("expected service error passthrough, got %v", err)
	}
}

func TestDispatchErrorOnlyMethod(t *testing.T) {
	registry := newTestRegistry(t)
	result, err := registry.Dispatch("Fake.Drop", rawParams(t, "ok"))
	if err != nil || result != nil {
		t.Fatalf("expected nil result and nil error, got %v %v", result, err)
	}
	if _, err := registry.Dispatch("Fake.Drop", rawParams(t, "bad")); err == nil {
		t.Fatal("expected error for bad id")
	}
}

func TestDispatchRejectsUnlistedMethod(t *testing.T) {
	registry := newTestRegistry(t)
	if _, err := registry.Dispatch("Fake.Secret", nil); err == nil ||
		!strings.Contains(err.Error(), "RPC_UNKNOWN_METHOD") {
		t.Fatalf("allowlist must reject unlisted exported method, got %v", err)
	}
}

func TestDispatchRejectsUnknownService(t *testing.T) {
	registry := newTestRegistry(t)
	if _, err := registry.Dispatch("Nope.Echo", nil); err == nil ||
		!strings.Contains(err.Error(), "RPC_UNKNOWN_SERVICE") {
		t.Fatalf("expected unknown service error, got %v", err)
	}
}

func TestDispatchRejectsBadMethodShape(t *testing.T) {
	registry := newTestRegistry(t)
	for _, method := range []string{"", "Echo", ".Echo", "Fake."} {
		if _, err := registry.Dispatch(method, nil); err == nil {
			t.Fatalf("expected error for method %q", method)
		}
	}
}

func TestDispatchRejectsParamMismatch(t *testing.T) {
	registry := newTestRegistry(t)
	if _, err := registry.Dispatch("Fake.Echo", rawParams(t, "only-one")); err == nil ||
		!strings.Contains(err.Error(), "RPC_BAD_PARAMS") {
		t.Fatalf("expected param count error, got %v", err)
	}
	if _, err := registry.Dispatch("Fake.Echo", rawParams(t, "text", "not-an-int")); err == nil ||
		!strings.Contains(err.Error(), "RPC_BAD_PARAMS") {
		t.Fatalf("expected param type error, got %v", err)
	}
}

func TestDispatchRecoversPanic(t *testing.T) {
	registry := newTestRegistry(t)
	if _, err := registry.Dispatch("Fake.Panics", nil); err == nil ||
		!strings.Contains(err.Error(), "RPC_PANIC") {
		t.Fatalf("expected recovered panic error, got %v", err)
	}
}

func TestServeEndToEnd(t *testing.T) {
	registry := newTestRegistry(t)
	server := NewServer(registry, &Hello{App: "测试", Version: "0.0.1", Protocol: 1})

	input := strings.Join([]string{
		`{"id":1,"method":"Fake.Echo","params":["第一",1]}`,
		`not-json`,
		``,
		`{"id":2,"method":"Fake.Fail","params":[]}`,
		`{"id":3,"method":"Fake.Drop","params":["ok"]}`,
	}, "\n") + "\n"

	var output bytes.Buffer
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 5 { // hello + 4 responses（含坏行错误响应）
		t.Fatalf("expected 5 output lines, got %d: %v", len(lines), lines)
	}

	var hello struct {
		Hello *Hello `json:"hello"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &hello); err != nil || hello.Hello == nil {
		t.Fatalf("first line must be hello, got %q", lines[0])
	}
	if hello.Hello.App != "测试" || hello.Hello.Protocol != 1 {
		t.Fatalf("unexpected hello: %#v", hello.Hello)
	}

	responses := make(map[int64]Response)
	badRequestSeen := false
	for _, line := range lines[1:] {
		var resp Response
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("bad response line %q: %v", line, err)
		}
		if resp.Error != nil && strings.Contains(resp.Error.Message, "RPC_BAD_REQUEST") {
			badRequestSeen = true
			continue
		}
		responses[resp.ID] = resp
	}
	if !badRequestSeen {
		t.Fatal("malformed line must produce RPC_BAD_REQUEST response")
	}
	if resp := responses[1]; resp.Error != nil {
		t.Fatalf("id 1 must succeed: %#v", resp)
	}
	if resp := responses[2]; resp.Error == nil || !strings.HasPrefix(resp.Error.Message, "ADMIN_REQUIRED:") {
		t.Fatalf("id 2 must carry service error: %#v", responses[2])
	}
	if resp := responses[3]; resp.Error != nil || resp.Result != nil {
		t.Fatalf("id 3 must succeed with null result: %#v", resp)
	}
}
