using System;
using System.Reflection;
using System.Threading.Tasks;
using System.Windows.Input;
using XCLing.Wpf.Core;
using XCLing.Wpf.Models;

namespace XCLing.Wpf.ViewModels
{
    public sealed class AboutViewModel : ViewModelBase, IPageViewModel
    {
        private readonly AppServices _svc;
        private UpdateInfo _lastUpdate;
        private string _updateStatus = "";
        private bool _isCheckingUpdate;

        public AboutViewModel(AppServices svc)
        {
            _svc = svc;
            UiVersion = Assembly.GetExecutingAssembly().GetName().Version.ToString(3);
        }

        public string Key => "about";
        public string Title => "关于";

        public string AppName => _svc.AppName;
        public string Description => "软件限制策略配置工具，支持白名单与黑名单两种保护模式";
        public string UiVersion { get; }
        public string CoreVersion => _svc.CoreVersion;

        public string Author => "满猪星";
        public string Github => "https://github.com/AShujiao/XCLing";
        public string Contact => "209820988@qq.com";
        public string Copyright => "Copyright © 2026 满猪星";
        public string License => "MIT License";
        public string Acknowledgments => "";

        /// <summary>检查更新结果文案（进行中/成功/失败均在这里展示）。</summary>
        public string UpdateStatus
        {
            get => _updateStatus;
            set => Set(ref _updateStatus, value);
        }

        /// <summary>检查进行中（禁用按钮并显示"正在检查…"）。</summary>
        public bool IsCheckingUpdate
        {
            get => _isCheckingUpdate;
            set => Set(ref _isCheckingUpdate, value);
        }

        /// <summary>存在新版本且拿到 release 地址时可点「去下载」。</summary>
        public bool CanOpenRelease => _lastUpdate != null && _lastUpdate.HasUpdate && !string.IsNullOrEmpty(_lastUpdate.ReleaseUrl);

        public ICommand OpenGithubCommand => new RelayCommand(() => TryOpenUrl(Github));
        public ICommand DonateCommand => new RelayCommand(ShowDonate);
        public ICommand CheckUpdateCommand => new AsyncRelayCommand(CheckUpdateAsync, () => true, HandleCommandError);
        public ICommand OpenReleaseCommand => new RelayCommand(OpenRelease, () => CanOpenRelease);

        private async Task CheckUpdateAsync()
        {
            IsCheckingUpdate = true;
            UpdateStatus = "正在检查更新…";
            _lastUpdate = null;
            Raise(nameof(CanOpenRelease));
            CommandManager.InvalidateRequerySuggested();
            try
            {
                var info = await _svc.Api.CheckUpdate();
                _lastUpdate = info;
                Raise(nameof(CanOpenRelease));
                CommandManager.InvalidateRequerySuggested();
                if (info.HasUpdate)
                {
                    var size = info.AssetSize > 0 ? $"（{FormatSize(info.AssetSize)}）" : "";
                    UpdateStatus = $"发现新版本 v{info.LatestVersion}{size}，点击「去下载」前往 GitHub Release 页面。";
                }
                else
                {
                    UpdateStatus = $"当前已是最新版本 v{info.CurrentVersion}。";
                }
            }
            catch (Exception ex)
            {
                _lastUpdate = null;
                Raise(nameof(CanOpenRelease));
                UpdateStatus = ErrorMessages.Humanize(ex, _svc.AppName);
            }
            finally
            {
                IsCheckingUpdate = false;
            }
        }

        private void OpenRelease()
        {
            var url = _lastUpdate?.ReleaseUrl;
            if (!string.IsNullOrEmpty(url))
            {
                TryOpenUrl(url);
            }
        }

        private void ShowDonate()
        {
            _svc.ShowDonate?.Invoke();
        }

        private static void TryOpenUrl(string url)
        {
            try
            {
                System.Diagnostics.Process.Start(new System.Diagnostics.ProcessStartInfo
                {
                    FileName = url,
                    UseShellExecute = true,
                });
            }
            catch
            {
            }
        }

        private static string FormatSize(long bytes)
        {
            if (bytes <= 0)
            {
                return "";
            }
            const double kb = 1024.0;
            const double mb = kb * 1024;
            const double gb = mb * 1024;
            if (bytes >= gb)
            {
                return (bytes / gb).ToString("0.#") + " GB";
            }
            if (bytes >= mb)
            {
                return (bytes / mb).ToString("0.#") + " MB";
            }
            return (bytes / kb).ToString("0.#") + " KB";
        }

        private void HandleCommandError(Exception ex)
        {
            _svc.Toast(ErrorMessages.Humanize(ex, _svc.AppName), true);
        }

        public Task OnActivatedAsync()
        {
            return Task.CompletedTask;
        }
    }
}
