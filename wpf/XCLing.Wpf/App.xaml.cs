using System;
using System.IO;
using System.Threading;
using System.Threading.Tasks;
using System.Windows;
using XCLing.Wpf.Core;
using XCLing.Wpf.ViewModels;
using XCLing.Wpf.Views;
using WinForms = System.Windows.Forms;

namespace XCLing.Wpf
{
    /// <summary>
    /// 应用入口：单实例锁、xcling-core sidecar 宿主、托盘驻留与生命周期管理。
    /// 行为约定：
    ///   - 重复启动只唤起已驻留的主窗口（带 --minimized 的自启动竞态保持托盘驻留）；
    ///   - 关闭主窗口隐藏到托盘，托盘菜单提供 打开/临时解锁/重新锁定/退出。
    /// </summary>
    public partial class App : Application
    {
        private const string MutexName = "com.xcling.single-instance.wpf";
        private const string ShowEventName = "com.xcling.show-main-window.wpf";
        private const string CoreExeName = "xcling-core.exe";

        private Mutex _instanceMutex;
        private EventWaitHandle _showEvent;
        private RpcClient _rpc;
        private GoApi _api;
        private MainViewModel _main;
        private MainWindow _window;
        private WinForms.NotifyIcon _tray;
        private bool _exiting;

        protected override async void OnStartup(StartupEventArgs e)
        {
            base.OnStartup(e);
            InstallCrashLogging();
            var minimized = HasMinimizedFlag(e.Args);

            // 应用持久化的主题（默认深色）。App.xaml 已加载 Dark.xaml + Shared.xaml，
            // 如果持久化的是 light 则需要切换；如果是 dark 则仅初始化状态。
            try
            {
                var savedTheme = Settings.Load().Theme;
                if (string.Equals(savedTheme, ThemeManager.Light, StringComparison.OrdinalIgnoreCase))
                {
                    ThemeManager.Apply(ThemeManager.Light);
                }
                else
                {
                    ThemeManager.Init(ThemeManager.Dark);
                }
            }
            catch { /* 主题加载失败回退默认深色 */ }

            bool createdNew;
            _instanceMutex = new Mutex(true, MutexName, out createdNew);
            if (!createdNew)
            {
                // 已有实例：非自启动场景请求它显示主窗口，然后退出自己。
                if (!minimized)
                {
                    try
                    {
                        using (var handle = EventWaitHandle.OpenExisting(ShowEventName))
                        {
                            handle.Set();
                        }
                    }
                    catch
                    {
                        // 首实例尚未建好事件句柄时放弃唤起，仅保证单实例。
                    }
                }
                Shutdown();
                return;
            }
            _showEvent = new EventWaitHandle(false, EventResetMode.AutoReset, ShowEventName);
            StartShowListener();

            try
            {
                _rpc = await RpcClient.StartAsync(ResolveCorePath(), TimeSpan.FromSeconds(10));
            }
            catch (Exception ex)
            {
                MessageBox.Show(
                    "核心服务启动失败，无法继续运行。\n\n" + ex.Message,
                    "XCLing", MessageBoxButton.OK, MessageBoxImage.Error);
                Shutdown();
                return;
            }
            _rpc.Faulted += OnCoreFaulted;

            _api = new GoApi(_rpc);
            var appName = _rpc.Hello != null && !string.IsNullOrEmpty(_rpc.Hello.App)
                ? _rpc.Hello.App
                : "星陈守护";
            var coreVersion = _rpc.Hello != null ? _rpc.Hello.Version : "";

            _main = new MainViewModel(_api, appName, coreVersion, ConfirmDialog, ShowDonateDialog);
            _window = new MainWindow { DataContext = _main };
            _window.Closing += (s, args) =>
            {
                if (!_exiting)
                {
                    args.Cancel = true;
                    _window.Hide(); // 驻留托盘
                }
            };

            SetupTray(appName);

            // 应用开机自启动偏好（默认启用）。放到后台线程执行：ApplyPreference 内部会
            // 启动 schtasks 子进程（创建/查询计划任务），同步跑在 UI 线程会阻塞消息循环，
            // 导致自启动后托盘右键菜单迟迟不弹出。await 会让出 UI 线程，托盘立即可响应。
            var autoStartErr = await Task.Run(() => Core.AutoStart.ApplyPreference());
            if (autoStartErr != null)
            {
                _tray.ShowBalloonTip(3000, appName, "开机自启动设置失败：" + autoStartErr, WinForms.ToolTipIcon.Warning);
            }

            if (!minimized)
            {
                _window.Show();
            }
            await _main.ActivateInitialAsync();
        }

        protected override void OnExit(ExitEventArgs e)
        {
            _exiting = true;
            if (_tray != null)
            {
                _tray.Visible = false;
                _tray.Dispose();
            }
            _rpc?.Dispose();
            _showEvent?.Dispose();
            _instanceMutex?.Dispose();
            base.OnExit(e);
        }

        private void SetupTray(string appName)
        {
            _tray = new WinForms.NotifyIcon
            {
                Text = appName.Length > 60 ? appName.Substring(0, 60) : appName,
                Visible = true,
            };
            try
            {
                var exe = System.Diagnostics.Process.GetCurrentProcess().MainModule.FileName;
                _tray.Icon = System.Drawing.Icon.ExtractAssociatedIcon(exe);
            }
            catch
            {
                _tray.Icon = System.Drawing.SystemIcons.Shield;
            }

            var menu = new WinForms.ContextMenuStrip();
            menu.Items.Add("打开控制台", null, (s, e) => ShowMainWindow());
            menu.Items.Add(new WinForms.ToolStripSeparator());
            menu.Items.Add("临时解锁", null, async (s, e) => await TrayTransitionAsync(false));
            menu.Items.Add("重新锁定", null, async (s, e) => await TrayTransitionAsync(true));
            menu.Items.Add(new WinForms.ToolStripSeparator());
            menu.Items.Add("退出", null, (s, e) => ExitApplication());
            _tray.ContextMenuStrip = menu;
            _tray.DoubleClick += (s, e) => ShowMainWindow();
        }

        private async Task TrayTransitionAsync(bool toLock)
        {
            try
            {
                var result = toLock ? await _api.LockProtection() : await _api.UnlockProtection();
                _tray.ShowBalloonTip(3000, _main.AppName,
                    result != null && !string.IsNullOrEmpty(result.Message) ? result.Message : "操作完成",
                    WinForms.ToolTipIcon.Info);
            }
            catch (Exception ex)
            {
                _tray.ShowBalloonTip(3000, _main.AppName,
                    ErrorMessages.Humanize(ex, _main.AppName), WinForms.ToolTipIcon.Error);
            }
            await _main.Console.RefreshAsync();
        }

        private void ShowMainWindow()
        {
            if (_window == null)
            {
                return;
            }
            _window.Show();
            if (_window.WindowState == WindowState.Minimized)
            {
                _window.WindowState = WindowState.Normal;
            }
            _window.Activate();
        }

        private void ExitApplication()
        {
            _exiting = true;
            Shutdown();
        }

        private bool ConfirmDialog(ConfirmRequest request)
        {
            return ConfirmWindow.Ask(_window, request);
        }

        private void ShowDonateDialog()
        {
            DonateWindow.Show(_window);
        }

        private void OnCoreFaulted(string message)
        {
            Dispatcher.BeginInvoke(new Action(() =>
            {
                if (_exiting)
                {
                    return;
                }
                MessageBox.Show(
                    "核心服务意外退出，应用即将关闭。\n\n" + message,
                    _main != null ? _main.AppName : "XCLing",
                    MessageBoxButton.OK, MessageBoxImage.Error);
                ExitApplication();
            }));
        }

        private void StartShowListener()
        {
            var thread = new Thread(() =>
            {
                while (!_exiting)
                {
                    if (_showEvent.WaitOne(500)) // 使用超时而非无限等待
                    {
                        Dispatcher.BeginInvoke(new Action(ShowMainWindow));
                    }
                }
            })
            {
                IsBackground = true,
                Name = "xcling-show-listener",
            };
            thread.Start();
        }

        private static string ResolveCorePath()
        {
            var overridePath = Environment.GetEnvironmentVariable("XCLING_CORE");
            if (!string.IsNullOrEmpty(overridePath))
            {
                return overridePath;
            }

            var baseDir = AppDomain.CurrentDomain.BaseDirectory;
            var defaultPath = Path.Combine(baseDir, CoreExeName);

            // Windows 7/8/8.1 使用 win7 目录下的兼容版本
            // Win7: 6.1, Win8: 6.2, Win8.1: 6.3
            var osVersion = Environment.OSVersion.Version;
            if (osVersion.Major < 6 || (osVersion.Major == 6 && osVersion.Minor <= 3))
            {
                var win7Path = Path.Combine(baseDir, "win7", CoreExeName);
                if (File.Exists(win7Path))
                {
                    return win7Path;
                }
            }

            return defaultPath;
        }

        private static bool HasMinimizedFlag(string[] args)
        {
            foreach (var arg in args)
            {
                if (arg == "--minimized" || arg == "--minimised")
                {
                    return true;
                }
            }
            return false;
        }

        // 将未处理异常写入崩溃日志，便于诊断 UI 层问题；不吞掉致命错误。
        private void InstallCrashLogging()
        {
            DispatcherUnhandledException += (s, e) =>
            {
                LogCrash("Dispatcher", e.Exception);
            };
            AppDomain.CurrentDomain.UnhandledException += (s, e) =>
            {
                LogCrash("AppDomain", e.ExceptionObject as Exception);
            };
        }

        private static void LogCrash(string source, Exception ex)
        {
            try
            {
                var dir = Path.Combine(
                    Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "XCLing");
                Directory.CreateDirectory(dir);
                var text = "[" + source + "] " + (ex != null ? ex.ToString() : "unknown") + "\r\n\r\n";
                File.AppendAllText(Path.Combine(dir, "wpf-crash.log"), text);
            }
            catch
            {
                // 崩溃日志写入失败时无能为力。
            }
        }
    }
}
