using System;
using System.Collections.Generic;
using System.Threading.Tasks;
using System.Windows.Input;
using XCLing.Wpf.Core;
using XCLing.Wpf.Models;

namespace XCLing.Wpf.ViewModels
{
    /// <summary>设置：主题切换 + Windows 兼容放行开关 + 开机自启动 + 定时关机。</summary>
    public sealed class SettingsViewModel : ViewModelBase, IPageViewModel
    {
        private readonly AppServices _svc;
        private ShutdownConfig _shutdownConfig;
        private int _selectedShutdownHour;
        private bool _loadingShutdownConfig;
        /// <summary>用户每次改动 +1；异步加载完成时若期间有改动，则以用户操作为准丢弃加载结果。</summary>
        private int _shutdownEditStamp;
        /// <summary>本次倒计时窗口内已取消关机后隐藏取消提示。</summary>
        private DateTime _lastShutdownCancel = DateTime.MinValue;

        public SettingsViewModel(AppServices services)
        {
            _svc = services;
            CancelShutdownCommand = new AsyncRelayCommand(CancelShutdownAsync, () => ShutdownEnabled, HandleCommandError);
            ShutdownHours = new List<int>();
            for (int i = 0; i < 24; i++)
            {
                ShutdownHours.Add(i);
            }
            // 倒计时提示条只在关机执行窗口（设定小时的第 0 分钟）显示，用定时器驱动可见性刷新。
            var countdownTimer = new System.Windows.Threading.DispatcherTimer
            {
                Interval = TimeSpan.FromSeconds(5),
            };
            countdownTimer.Tick += (s, e) => Raise(nameof(ShowCancelShutdown));
            countdownTimer.Start();
        }

        public string Key => "settings";

        public List<int> ShutdownHours { get; }

        public ICommand CancelShutdownCommand { get; }

        /// <summary>当前是否为深色主题。</summary>
        public bool IsDarkTheme
        {
            get => !IsLightTheme;
            set { if (value) SetTheme(ThemeManager.Dark); }
        }

        /// <summary>当前是否为明亮主题。</summary>
        public bool IsLightTheme
        {
            get => string.Equals(_svc.Settings.Theme, ThemeManager.Light, System.StringComparison.OrdinalIgnoreCase);
            set { if (value) SetTheme(ThemeManager.Light); }
        }

        /// <summary>是否启用开机自启动。</summary>
        public bool AutoStartEnabled
        {
            get => AutoStart.GetPreference();
            set
            {
                if (AutoStart.GetPreference() == value) return;
                var err = AutoStart.SetEnabled(value);
                if (err != null)
                {
                    _svc.Toast("开机自启动设置失败：" + err, true);
                    Raise();
                    return;
                }
                _svc.Toast(value ? "已启用开机自启动，登录后将最小化到托盘" : "已关闭开机自启动", false);
                Raise();
            }
        }

        public bool ShutdownEnabled
        {
            get => _shutdownConfig?.Enabled ?? false;
            set
            {
                if (_loadingShutdownConfig || ShutdownEnabled == value) return;
                // 乐观更新：立即反映到界面，异步持久化，失败时回读还原。
                _shutdownEditStamp++;
                _shutdownConfig = new ShutdownConfig { Enabled = value, Hour = _selectedShutdownHour };
                Raise();
                Raise(nameof(ShowCancelShutdown));
                _ = SaveShutdownConfigAsync(value, _selectedShutdownHour);
            }
        }

        public int SelectedShutdownHour
        {
            get => _selectedShutdownHour;
            set
            {
                if (_loadingShutdownConfig) return;
                if (Set(ref _selectedShutdownHour, value))
                {
                    _shutdownEditStamp++;
                    Raise(nameof(ShowCancelShutdown));
                    if (ShutdownEnabled)
                    {
                        _shutdownConfig = new ShutdownConfig { Enabled = true, Hour = value };
                        _ = SaveShutdownConfigAsync(true, value);
                    }
                }
            }
        }

        /// <summary>关机倒计时进行中（设定小时的第 0 分钟，shutdown /t 60 执行窗口）且本窗口内未取消过。</summary>
        public bool ShowCancelShutdown
        {
            get
            {
                if (!ShutdownEnabled) return false;
                var now = DateTime.Now;
                if (now.Hour != _selectedShutdownHour || now.Minute != 0) return false;
                var cancelled = _lastShutdownCancel;
                return !(cancelled.Date == now.Date && cancelled.Hour == now.Hour && cancelled.Minute == 0);
            }
        }

        private void SetTheme(string theme)
        {
            if (string.Equals(_svc.Settings.Theme, theme, System.StringComparison.OrdinalIgnoreCase))
            {
                return;
            }
            _svc.Settings.Theme = theme;
            _svc.Settings.Save();
            ThemeManager.Apply(theme);
            Raise(nameof(IsDarkTheme));
            Raise(nameof(IsLightTheme));
        }

        public bool AllowPackagedApps
        {
            get => _svc.Settings.AllowPackagedApps;
            set { if (_svc.Settings.AllowPackagedApps != value) { _svc.Settings.AllowPackagedApps = value; _svc.Settings.Save(); Raise(); } }
        }

        public bool AllowDefenderUpdates
        {
            get => _svc.Settings.AllowDefenderUpdates;
            set { if (_svc.Settings.AllowDefenderUpdates != value) { _svc.Settings.AllowDefenderUpdates = value; _svc.Settings.Save(); Raise(); } }
        }

        public async Task OnActivatedAsync()
        {
            // 进入设置页时刷新自启动状态（可能被其他途径修改过）
            Raise(nameof(AutoStartEnabled));
            await LoadShutdownConfigAsync();
        }

        private async Task LoadShutdownConfigAsync()
        {
            var stamp = _shutdownEditStamp;
            try
            {
                var cfg = await _svc.Api.GetShutdownConfig();
                if (stamp != _shutdownEditStamp)
                {
                    return; // 加载期间用户已改动，以用户操作为准
                }
                // 守卫只覆盖同步赋值段（无异步间隙），不会吞掉用户点击。
                _loadingShutdownConfig = true;
                _shutdownConfig = cfg;
                _selectedShutdownHour = cfg.Hour;
                Raise(nameof(ShutdownEnabled));
                Raise(nameof(SelectedShutdownHour));
                Raise(nameof(ShowCancelShutdown));
            }
            catch (Exception ex)
            {
                _svc.Toast("加载定时关机配置失败：" + ex.Message, true);
            }
            finally
            {
                _loadingShutdownConfig = false;
            }
        }

        private async Task SaveShutdownConfigAsync(bool enabled, int hour)
        {
            try
            {
                await _svc.Api.SetShutdownConfig(enabled, hour);
                _svc.Toast(enabled ? $"已启用定时关机，每天 {hour:00}:00 自动关机" : "已关闭定时关机", false);
            }
            catch (Exception ex)
            {
                _svc.Toast("保存定时关机配置失败：" + ex.Message, true);
                await LoadShutdownConfigAsync(); // 回读后端实际状态，还原乐观更新
            }
        }

        private async Task CancelShutdownAsync()
        {
            try
            {
                await _svc.Api.CancelShutdown();
                _lastShutdownCancel = DateTime.Now;
                Raise(nameof(ShowCancelShutdown));
                _svc.Toast("已取消本次关机，明天将按计划继续", false);
            }
            catch (Exception ex)
            {
                _svc.Toast("取消关机失败：" + ex.Message, true);
            }
        }

        private void HandleCommandError(Exception ex)
        {
            _svc.Toast(ErrorMessages.Humanize(ex, _svc.AppName), true);
        }
    }
}
