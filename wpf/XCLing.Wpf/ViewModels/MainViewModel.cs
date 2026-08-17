using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.Threading.Tasks;
using System.Windows.Input;
using System.Windows.Threading;
using XCLing.Wpf.Core;

namespace XCLing.Wpf.ViewModels
{
    public sealed class NavItem : ViewModelBase
    {
        private bool _isActive;

        public NavItem(string key, string title, ICommand command)
        {
            Key = key;
            Title = title;
            Command = command;
        }

        public string Key { get; }
        public string Title { get; }
        public ICommand Command { get; }

        public bool IsActive
        {
            get { return _isActive; }
            set { Set(ref _isActive, value); }
        }
    }

    public sealed class ToastItem
    {
        public ToastItem(string message, bool isError)
        {
            Message = message;
            IsError = isError;
        }

        public string Message { get; }
        public bool IsError { get; }
    }

    /// <summary>应用外壳：品牌信息、导航、Toast，并持有全部页面视图模型。</summary>
    public sealed class MainViewModel : ViewModelBase
    {
        private readonly Dispatcher _dispatcher;
        private readonly Dictionary<string, IPageViewModel> _pages = new Dictionary<string, IPageViewModel>();
        private readonly Settings _settings;
        private object _currentPage;
        private string _currentKey;

        public MainViewModel(GoApi api, string appName, string coreVersion, Func<ConfirmRequest, bool> confirm, Action showDonate)
        {
            _dispatcher = Dispatcher.CurrentDispatcher;
            AppName = appName;
            CoreVersion = coreVersion;
            _settings = Settings.Load();

            var services = new AppServices(api, appName, coreVersion, _settings, confirm, Toast, Navigate, showDonate);

            Register(new ConsoleViewModel(services), "主控制台");
            Register(new RulesViewModel(services), "白名单");
            Register(new BlocklistViewModel(services), "黑名单");
            Register(new ActivityViewModel(services), "记录");
            Register(new SettingsViewModel(services), "设置");
            Register(new AboutViewModel(services), "关于");
        }

        public string AppName { get; }
        public string CoreVersion { get; }
        public string FooterText => "核心服务 v" + CoreVersion + " 已连接";

        public ObservableCollection<NavItem> NavItems { get; } = new ObservableCollection<NavItem>();
        public ObservableCollection<ToastItem> Toasts { get; } = new ObservableCollection<ToastItem>();

        public object CurrentPage
        {
            get { return _currentPage; }
            private set { Set(ref _currentPage, value); }
        }

        /// <summary>主控制台视图模型，供托盘操作后刷新。</summary>
        public ConsoleViewModel Console => (ConsoleViewModel)_pages["console"];

        private void Register(IPageViewModel page, string title)
        {
            _pages[page.Key] = page;
            var command = new RelayCommand(() => Navigate(page.Key));
            NavItems.Add(new NavItem(page.Key, title, command));
        }

        /// <summary>切换到指定页面并触发其数据加载。可从任意线程调用。</summary>
        public async void Navigate(string key)
        {
            if (!_dispatcher.CheckAccess())
            {
                _ = _dispatcher.BeginInvoke(new Action(() => Navigate(key)));
                return;
            }
            if (!_pages.TryGetValue(key, out var page))
            {
                return;
            }
            _currentKey = key;
            CurrentPage = page;
            foreach (var item in NavItems)
            {
                item.IsActive = item.Key == key;
            }
            try
            {
                await page.OnActivatedAsync();
            }
            catch
            {
                // 页面加载异常由页面自身的错误呈现处理，不冒泡到导航。
            }
        }

        public Task ActivateInitialAsync()
        {
            // 允许通过环境变量深链到指定页面（用于诊断/部署脚本）；缺省进入主控制台。
            var start = Environment.GetEnvironmentVariable("POLICYGUARD_START_PAGE");
            Navigate(!string.IsNullOrWhiteSpace(start) && _pages.ContainsKey(start) ? start : "console");
            return Task.CompletedTask;
        }

        /// <summary>展示一条 4 秒自动消失的提示。线程安全。</summary>
        public void Toast(string message, bool isError)
        {
            if (string.IsNullOrWhiteSpace(message))
            {
                return;
            }
            if (!_dispatcher.CheckAccess())
            {
                _dispatcher.BeginInvoke(new Action(() => Toast(message, isError)));
                return;
            }
            var item = new ToastItem(message, isError);
            Toasts.Add(item);
            var timer = new DispatcherTimer(DispatcherPriority.Normal, _dispatcher)
            {
                Interval = TimeSpan.FromSeconds(4),
            };
            timer.Tick += (s, e) =>
            {
                timer.Stop();
                timer.Tick -= null;
                Toasts.Remove(item);
            };
            timer.Start();
        }
    }
}
