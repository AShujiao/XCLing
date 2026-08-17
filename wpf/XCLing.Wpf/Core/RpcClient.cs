using System;
using System.Collections.Concurrent;
using System.Diagnostics;
using System.IO;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using Newtonsoft.Json.Linq;

namespace XCLing.Wpf.Core
{
    /// <summary>sidecar 握手信息。</summary>
    public sealed class HelloInfo
    {
        public string App { get; set; }
        public string Version { get; set; }
        public int Protocol { get; set; }
    }

    /// <summary>
    /// xcling-core sidecar 的宿主与 NDJSON JSON-RPC 客户端。
    ///
    /// 传输只走子进程 stdin/stdout：不开端口、不建命名管道，
    /// 提权进程的控制面只对本进程可达。进程退出（stdout EOF）时
    /// 使所有挂起调用失败并触发 Faulted。
    /// </summary>
    public sealed class RpcClient : IDisposable
    {
        private const int MaxStderrChars = 8 * 1024;
        private const int DefaultRpcTimeoutMs = 30000; // 30 秒默认超时

        private readonly Process _process;
        private readonly StreamWriter _stdin;
        private readonly object _writeLock = new object();
        private readonly ConcurrentDictionary<long, TaskCompletionSource<JToken>> _pending =
            new ConcurrentDictionary<long, TaskCompletionSource<JToken>>();
        private readonly TaskCompletionSource<HelloInfo> _hello =
            new TaskCompletionSource<HelloInfo>(TaskCreationOptions.RunContinuationsAsynchronously);
        private readonly StringBuilder _stderr = new StringBuilder();
        private readonly CancellationTokenSource _shutdownCts = new CancellationTokenSource();
        private long _nextId;
        private volatile bool _disposed;

        public string CorePath { get; }
        public HelloInfo Hello { get; private set; }

        /// <summary>sidecar 意外退出时触发（参数为诊断信息）。可能来自非 UI 线程。</summary>
        public event Action<string> Faulted;

        private RpcClient(Process process, StreamWriter stdin, string corePath)
        {
            _process = process;
            _stdin = stdin;
            CorePath = corePath;
        }

        /// <summary>启动 sidecar 并等待握手完成。失败时抛出异常，调用方负责终止应用。</summary>
        public static async Task<RpcClient> StartAsync(string corePath, TimeSpan helloTimeout)
        {
            if (!File.Exists(corePath))
            {
                throw new FileNotFoundException("核心服务不存在：" + corePath, corePath);
            }

            var psi = new ProcessStartInfo
            {
                FileName = corePath,
                Arguments = "serve --stdio",
                UseShellExecute = false,
                RedirectStandardInput = true,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
                CreateNoWindow = true,
                WorkingDirectory = Path.GetDirectoryName(corePath) ?? "",
                // net48 没有 StandardInputEncoding，stdin 编码在下方用 BaseStream 显式指定 UTF-8。
                StandardOutputEncoding = new UTF8Encoding(false),
                StandardErrorEncoding = new UTF8Encoding(false),
            };

            var process = new Process { StartInfo = psi, EnableRaisingEvents = true };
            if (!process.Start())
            {
                throw new InvalidOperationException("核心服务启动失败：" + corePath);
            }

            var stdin = new StreamWriter(process.StandardInput.BaseStream, new UTF8Encoding(false)) { AutoFlush = true };
            var client = new RpcClient(process, stdin, corePath);
            process.Exited += (s, e) => client.OnProcessExited();

            var readerThread = new Thread(client.ReadLoop) { IsBackground = true, Name = "xcling-core-reader" };
            readerThread.Start();
            var stderrThread = new Thread(client.ReadStderrLoop) { IsBackground = true, Name = "xcling-core-stderr" };
            stderrThread.Start();

            var completed = await Task.WhenAny(client._hello.Task, Task.Delay(helloTimeout)).ConfigureAwait(false);
            if (completed != client._hello.Task)
            {
                client.Dispose();
                throw new TimeoutException("核心服务握手超时（" + helloTimeout.TotalSeconds + " 秒）。");
            }
            client.Hello = await client._hello.Task.ConfigureAwait(false);
            return client;
        }

        public async Task<T> CallAsync<T>(string method, params object[] args)
        {
            var token = await CallRawAsync(method, args, DefaultRpcTimeoutMs).ConfigureAwait(false);
            if (token == null || token.Type == JTokenType.Null)
            {
                return default(T);
            }
            return token.ToObject<T>(Json.Serializer);
        }

        public Task CallAsync(string method, params object[] args)
        {
            return CallRawAsync(method, args, DefaultRpcTimeoutMs);
        }

        private Task<JToken> CallRawAsync(string method, object[] args, int timeoutMs)
        {
            if (_disposed)
            {
                throw new RpcException("RPC_CORE_EXITED: 核心服务已退出");
            }

            var id = Interlocked.Increment(ref _nextId);
            var tcs = new TaskCompletionSource<JToken>(TaskCreationOptions.RunContinuationsAsynchronously);
            _pending[id] = tcs;

            var request = new JObject
            {
                ["id"] = id,
                ["method"] = method,
                ["params"] = args == null
                    ? new JArray()
                    : JArray.FromObject(args, Json.Serializer),
            };

            // 设置超时取消
            CancellationTokenSource timeoutCts = null;
            if (timeoutMs > 0)
            {
                timeoutCts = new CancellationTokenSource(timeoutMs);
                timeoutCts.Token.Register(() =>
                {
                    if (_pending.TryRemove(id, out var timedOutTcs))
                    {
                        timedOutTcs.TrySetException(new TimeoutException($"RPC call '{method}' timed out after {timeoutMs}ms"));
                    }
                });
            }

            try
            {
                var line = request.ToString(Newtonsoft.Json.Formatting.None);
                lock (_writeLock)
                {
                    _stdin.WriteLine(line);
                }
            }
            catch (Exception ex)
            {
                _pending.TryRemove(id, out _);
                timeoutCts?.Dispose();
                throw new RpcException("RPC_WRITE_FAILED: 无法向核心服务发送请求：" + ex.Message);
            }

            // 清理超时取消令牌
            if (timeoutCts != null)
            {
                tcs.Task.ContinueWith(_ => timeoutCts.Dispose(), TaskScheduler.Default);
            }

            return tcs.Task;
        }

        private void ReadLoop()
        {
            try
            {
                string line;
                while (!_shutdownCts.Token.IsCancellationRequested &&
                       (line = _process.StandardOutput.ReadLine()) != null)
                {
                    if (string.IsNullOrWhiteSpace(line))
                    {
                        continue;
                    }
                    JObject message;
                    try
                    {
                        message = JObject.Parse(line);
                    }
                    catch
                    {
                        continue; // 无法解析的行只可能是 sidecar 缺陷，忽略并继续读。
                    }

                    var hello = message["hello"];
                    if (hello != null && hello.Type == JTokenType.Object)
                    {
                        _hello.TrySetResult(hello.ToObject<HelloInfo>(Json.Serializer));
                        continue;
                    }

                    var idToken = message["id"];
                    if (idToken == null)
                    {
                        continue;
                    }
                    var id = idToken.Value<long>();
                    if (!_pending.TryRemove(id, out var tcs))
                    {
                        continue;
                    }

                    var error = message["error"];
                    if (error != null && error.Type == JTokenType.Object)
                    {
                        tcs.TrySetException(new RpcException(error.Value<string>("message")));
                    }
                    else
                    {
                        tcs.TrySetResult(message["result"]);
                    }
                }
            }
            catch (OperationCanceledException)
            {
                // 正常关闭
            }
            catch
            {
                // 读循环里的异常与 EOF 同样处理：统一走退出清理。
            }
            OnProcessExited();
        }

        private void ReadStderrLoop()
        {
            try
            {
                string line;
                while ((line = _process.StandardError.ReadLine()) != null)
                {
                    lock (_stderr)
                    {
                        if (_stderr.Length < MaxStderrChars)
                        {
                            _stderr.AppendLine(line);
                        }
                    }
                    Debug.WriteLine("[xcling-core] " + line);
                }
            }
            catch
            {
                // stderr 断流不影响主流程。
            }
        }

        private void OnProcessExited()
        {
            string diagnostic;
            lock (_stderr)
            {
                diagnostic = _stderr.ToString().Trim();
            }
            var message = "RPC_CORE_EXITED: 核心服务已退出" +
                (diagnostic.Length > 0 ? "。诊断：" + diagnostic : "");

            _hello.TrySetException(new RpcException(message));
            foreach (var id in _pending.Keys)
            {
                if (_pending.TryRemove(id, out var tcs))
                {
                    tcs.TrySetException(new RpcException(message));
                }
            }
            if (!_disposed)
            {
                Faulted?.Invoke(message);
            }
        }

        public void Dispose()
        {
            if (_disposed)
            {
                return;
            }
            _disposed = true;

            // 通知后台线程停止
            _shutdownCts.Cancel();

            try
            {
                // 关闭 stdin（EOF）通知 sidecar 正常退出；3 秒不退再强杀。
                _stdin.Dispose();
                if (!_process.WaitForExit(3000))
                {
                    _process.Kill();
                }
            }
            catch
            {
                // 退出路径尽力而为。
            }
            finally
            {
                _process.Dispose();
                _shutdownCts.Dispose();
            }
        }
    }
}
