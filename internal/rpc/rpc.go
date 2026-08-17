// Package rpc 实现 sidecar 的 stdio JSON-RPC 协议，供原生 GUI 壳（WPF）调用
// 服务层方法。
//
// 协议：每行一个 JSON 对象（NDJSON），请求为 {"id","method","params"}，
// 响应为 {"id","result"} 或 {"id","error":{"message"}}。method 形如
// "ApplyService.GetApplyStatus"，params 为按位置排列的 JSON 数组，
// 与 GUI 壳 GoApi.Call(service, method, ...args) 的调用形状一一对应。
//
// 安全约束：
//   - 只暴露 Register 时显式列出的方法（白名单），反射永不遍历未列出的导出方法；
//   - 传输只走 stdin/stdout，不监听任何端口、不创建命名管道，
//     提权进程的控制面只对启动它的父进程可达；
//   - 服务层错误原样透传 err.Error()（"CODE: 说明" 形状），协议层错误统一用 RPC_ 前缀，
//     客户端错误码解析逻辑与 GUI 壳 RpcException 保持一致。
package rpc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
)

// Request 是一次方法调用。
type Request struct {
	ID     int64             `json:"id"`
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

// Error 携带服务层或协议层错误文本。
type Error struct {
	Message string `json:"message"`
}

// Response 是一次调用结果；Error 为 nil 表示成功。
type Response struct {
	ID     int64       `json:"id"`
	Result interface{} `json:"result"`
	Error  *Error      `json:"error,omitempty"`
}

// Hello 是服务端启动后主动写出的握手行内容。
type Hello struct {
	App      string `json:"app"`
	Version  string `json:"version"`
	Protocol int    `json:"protocol"`
}

var errType = reflect.TypeOf((*error)(nil)).Elem()

type boundService struct {
	instance reflect.Value
	methods  map[string]reflect.Value
}

// Registry 持有服务实例和显式方法白名单。
type Registry struct {
	services map[string]*boundService
}

func NewRegistry() *Registry {
	return &Registry{services: make(map[string]*boundService)}
}

// Register 注册一个服务实例并显式列出允许调用的方法。
// 方法缺失或签名不受支持时立即报错，保证接线错误在启动期暴露。
func (r *Registry) Register(name string, instance interface{}, methods ...string) error {
	if name == "" || instance == nil || len(methods) == 0 {
		return fmt.Errorf("rpc: 服务 %q 注册参数不完整", name)
	}
	if _, exists := r.services[name]; exists {
		return fmt.Errorf("rpc: 服务 %q 重复注册", name)
	}
	value := reflect.ValueOf(instance)
	bound := &boundService{instance: value, methods: make(map[string]reflect.Value, len(methods))}
	for _, methodName := range methods {
		method := value.MethodByName(methodName)
		if !method.IsValid() {
			return fmt.Errorf("rpc: %s.%s 不存在", name, methodName)
		}
		if err := validateSignature(method.Type()); err != nil {
			return fmt.Errorf("rpc: %s.%s 签名不受支持: %w", name, methodName, err)
		}
		bound.methods[methodName] = method
	}
	r.services[name] = bound
	return nil
}

// 返回值形状约束：()、(error)、(T)、(T, error)。
func validateSignature(t reflect.Type) error {
	switch t.NumOut() {
	case 0, 1:
		return nil
	case 2:
		if !t.Out(1).Implements(errType) {
			return fmt.Errorf("第二个返回值必须是 error")
		}
		return nil
	default:
		return fmt.Errorf("返回值数量超过 2 个")
	}
}

// Dispatch 按白名单调用 "Service.Method"。返回值 result 为方法的数据返回值（可能为 nil）。
func (r *Registry) Dispatch(method string, params []json.RawMessage) (result interface{}, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = nil
			err = fmt.Errorf("RPC_PANIC: %s 执行异常: %v", method, recovered)
		}
	}()

	dot := strings.IndexByte(method, '.')
	if dot <= 0 || dot >= len(method)-1 {
		return nil, fmt.Errorf("RPC_BAD_METHOD: 方法名必须为 Service.Method 形式: %q", method)
	}
	serviceName, methodName := method[:dot], method[dot+1:]
	bound, ok := r.services[serviceName]
	if !ok {
		return nil, fmt.Errorf("RPC_UNKNOWN_SERVICE: %q", serviceName)
	}
	fn, ok := bound.methods[methodName]
	if !ok {
		return nil, fmt.Errorf("RPC_UNKNOWN_METHOD: %s.%s 未注册", serviceName, methodName)
	}

	t := fn.Type()
	if len(params) != t.NumIn() {
		return nil, fmt.Errorf("RPC_BAD_PARAMS: %s 期望 %d 个参数，收到 %d 个", method, t.NumIn(), len(params))
	}
	args := make([]reflect.Value, t.NumIn())
	for i := 0; i < t.NumIn(); i++ {
		target := reflect.New(t.In(i))
		if unmarshalErr := json.Unmarshal(params[i], target.Interface()); unmarshalErr != nil {
			return nil, fmt.Errorf("RPC_BAD_PARAMS: %s 第 %d 个参数解析失败: %v", method, i+1, unmarshalErr)
		}
		args[i] = target.Elem()
	}

	out := fn.Call(args)
	switch len(out) {
	case 0:
		return nil, nil
	case 1:
		if t.Out(0).Implements(errType) {
			return nil, asError(out[0])
		}
		return out[0].Interface(), nil
	default:
		return out[0].Interface(), asError(out[1])
	}
}

func asError(v reflect.Value) error {
	if v.IsNil() {
		return nil
	}
	return v.Interface().(error)
}

// Server 在一对读写流上运行 NDJSON 协议循环。
type Server struct {
	registry *Registry
	hello    *Hello
}

func NewServer(registry *Registry, hello *Hello) *Server {
	return &Server{registry: registry, hello: hello}
}

// Serve 阻塞处理请求直到 reader 关闭（EOF）。每个请求在独立 goroutine 中执行，
// 并发语义：服务层自身的操作互斥（如 OPERATION_IN_PROGRESS）继续生效。
func (s *Server) Serve(reader io.Reader, writer io.Writer) error {
	var writeMu sync.Mutex
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	write := func(v interface{}) {
		writeMu.Lock()
		defer writeMu.Unlock()
		// 编码失败（writer 关闭）时无法向对端报告，只能静默；下一次读循环会因 EOF 退出。
		_ = encoder.Encode(v)
	}

	if s.hello != nil {
		write(map[string]*Hello{"hello": s.hello})
	}

	scanner := bufio.NewScanner(reader)
	// 草案/档案 JSON 作为字符串参数传入，放宽单行上限到 16MB。
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)

	var pending sync.WaitGroup
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			write(Response{ID: req.ID, Error: &Error{Message: "RPC_BAD_REQUEST: " + err.Error()}})
			continue
		}
		pending.Add(1)
		go func(req Request) {
			defer pending.Done()
			result, err := s.registry.Dispatch(req.Method, req.Params)
			if err != nil {
				write(Response{ID: req.ID, Error: &Error{Message: err.Error()}})
				return
			}
			write(Response{ID: req.ID, Result: result})
		}(req)
	}
	pending.Wait()
	return scanner.Err()
}
