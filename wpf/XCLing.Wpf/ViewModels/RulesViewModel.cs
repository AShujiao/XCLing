using System;
using System.Collections.ObjectModel;
using System.Linq;
using System.Threading.Tasks;
using System.Windows.Input;
using XCLing.Wpf.Core;
using XCLing.Wpf.Models;

namespace XCLing.Wpf.ViewModels
{
    /// <summary>白名单维护：默认可信目录 + 自定义白名单（白名单模式运行中即时应用，否则待启用）+ 从已安装程序导入。</summary>
    public sealed class RulesViewModel : ViewModelBase, IPageViewModel
    {
        private readonly AppServices _svc;
        private ApplyStatus _status;
        private string _newPath = "";
        private string _newKind = "directory";
        private string _search = "";
        private bool _busy;
        private bool _scanning;
        private bool _scanned;
        private bool _showScanner;
        private string _error = "";

        private DiscoveredApp[] _allApps = new DiscoveredApp[0];

        public RulesViewModel(AppServices services)
        {
            _svc = services;
            AddPathCommand = new AsyncRelayCommand(AddTypedPathAsync, () => !Busy && !string.IsNullOrWhiteSpace(NewPath), OnError);
            BrowseCommand = new RelayCommand(Browse);
            ToggleScannerCommand = new RelayCommand(() => ShowScanner = !ShowScanner);
            ScanCommand = new AsyncRelayCommand(ScanAsync, () => !Scanning, OnError);
            RemoveActiveRuleCommand = new AsyncRelayCommand<ManagedRule>(RemoveActiveRuleAsync, OnError);
            RemovePendingCommand = new RelayCommand<CustomPathEntry>(RemovePending);
            // 未启用时加入待启用列表，已启用时即时应用——两种情况 ApplyDiscoveredAsync 都处理，
            // 因此 canExecute 不要求 Active，只要求候选可选。
            ApplyDiscoveredCommand = new AsyncRelayCommand<DiscoveredApp>(ApplyDiscoveredAsync, OnError, a => a != null && a.Selectable);
            GoConsoleCommand = new RelayCommand(() => _svc.Navigate("console"));
        }

        public string Key => "rules";

        public ICommand AddPathCommand { get; }
        public ICommand BrowseCommand { get; }
        public ICommand ToggleScannerCommand { get; }
        public ICommand ScanCommand { get; }
        public ICommand RemoveActiveRuleCommand { get; }
        public ICommand RemovePendingCommand { get; }
        public ICommand ApplyDiscoveredCommand { get; }
        public ICommand GoConsoleCommand { get; }

        public ObservableCollection<ManagedRule> ActiveExtraRules { get; } = new ObservableCollection<ManagedRule>();
        public ObservableCollection<CustomPathEntry> PendingPaths => _svc.PendingPaths;
        public ObservableCollection<DiscoveredApp> FilteredApps { get; } = new ObservableCollection<DiscoveredApp>();

        public ApplyStatus Status
        {
            get => _status;
            private set
            {
                Set(ref _status, value);
                Raise(nameof(Active));
                Raise(nameof(IsBlacklistMode));
                Raise(nameof(ShowModeGuidance));
                Raise(nameof(GuidanceText));
                Raise(nameof(HeaderHint));
                Raise(nameof(StateLabel));
                Raise(nameof(AddButtonText));
            }
        }

        /// <summary>黑名单模式运行中：后端不维护白名单规则，本页只做待启用配置。</summary>
        public bool IsBlacklistMode => Status != null && Status.PolicyMode == "blacklist"
            && (Status.ProtectionState == "locked" || Status.ProtectionState == "unlocked");
        /// <summary>白名单模式运行中（锁定或临时解锁），规则修改即时应用。</summary>
        public bool Active => Status != null && !IsBlacklistMode
            && (Status.ProtectionState == "locked" || Status.ProtectionState == "unlocked");
        public string StateLabel => Active ? "即时应用" : "待启用";
        public string HeaderHint => Active ? "白名单模式运行中，规则修改会立即生效。" : "标准安装目录默认可信，新增规则将在启用白名单模式时生效。";
        public string AddButtonText => Active ? "添加并应用" : "添加";
        /// <summary>白名单模式未运行时引导用户先去主控制台开启。</summary>
        public bool ShowModeGuidance => Status != null && !Active;
        public string GuidanceText => IsBlacklistMode
            ? "当前为黑名单模式，白名单规则不会生效。如需使用白名单，请先在「主控制台」恢复原状，再启用白名单模式。"
            : "白名单模式尚未开启，此处添加的规则会保存为待启用规则。请前往「主控制台」启用白名单模式后生效。";

        public string NewPath { get => _newPath; set => Set(ref _newPath, value); }
        public string NewKind { get => _newKind; set => Set(ref _newKind, value); }
        public string Search { get => _search; set { if (Set(ref _search, value)) { ApplyFilter(); } } }
        public bool Busy { get => _busy; private set => Set(ref _busy, value); }
        public bool Scanning { get => _scanning; private set => Set(ref _scanning, value); }
        public bool Scanned { get => _scanned; private set => Set(ref _scanned, value); }
        public bool ShowScanner { get => _showScanner; set => Set(ref _showScanner, value); }
        public string Error { get => _error; private set => Set(ref _error, value); }
        public int SelectionCount => PendingPaths.Count;

        public Task OnActivatedAsync() => RefreshStatusAsync();

        private async Task RefreshStatusAsync()
        {
            Error = "";
            try
            {
                Status = await _svc.Api.GetApplyStatus();
                SyncActiveRules();
            }
            catch (Exception ex)
            {
                Error = ErrorMessages.Humanize(ex, _svc.AppName);
            }
        }

        private void SyncActiveRules()
        {
            ActiveExtraRules.Clear();
            if (Status != null && Status.ManagedRules != null)
            {
                foreach (var rule in Status.ManagedRules.Where(r => r.Removable))
                {
                    ActiveExtraRules.Add(rule);
                }
            }
        }

        private async Task AddTypedPathAsync()
        {
            var value = NewPath;
            NewPath = "";
            await AddPathValueAsync(value, NewKind, "");
        }

        private void Browse()
        {
            string picked;
            if (NewKind == "file")
            {
                picked = PathPicker.SelectExecutable();
            }
            else
            {
                picked = PathPicker.SelectDirectory();
            }
            if (!string.IsNullOrEmpty(picked))
            {
                NewPath = picked;
            }
        }

        private async Task AddPathValueAsync(string value, string kind, string label)
        {
            var clean = (value ?? "").Trim();
            if (clean.Length == 0)
            {
                return;
            }
            if (Active)
            {
                Busy = true;
                try
                {
                    var result = await _svc.Api.AddTrustedPath(clean, kind, label);
                    _svc.Toast(result != null ? result.Message : "已添加", false);
                    await RefreshStatusAsync();
                }
                catch (Exception ex)
                {
                    Error = ErrorMessages.Humanize(ex, _svc.AppName);
                    _svc.Toast(Error, true);
                }
                finally
                {
                    Busy = false;
                }
            }
            else
            {
                if (PendingPaths.Any(p => string.Equals(p.Path, clean, StringComparison.OrdinalIgnoreCase)))
                {
                    _svc.Toast("该路径已经存在", true);
                    return;
                }
                PendingPaths.Add(new CustomPathEntry
                {
                    Id = "c" + Guid.NewGuid().ToString("N").Substring(0, 8),
                    Path = clean,
                    Kind = kind,
                    Label = label,
                });
                Raise(nameof(SelectionCount));
                _svc.Toast("已加入待启用规则", false);
            }
        }

        private async Task RemoveActiveRuleAsync(ManagedRule rule)
        {
            if (rule == null) return;
            Busy = true;
            try
            {
                var result = await _svc.Api.RemoveTrustedRule(rule.Id);
                _svc.Toast(result != null ? result.Message : "已删除", false);
                await RefreshStatusAsync();
            }
            catch (Exception ex)
            {
                Error = ErrorMessages.Humanize(ex, _svc.AppName);
                _svc.Toast(Error, true);
            }
            finally
            {
                Busy = false;
            }
        }

        private void RemovePending(CustomPathEntry entry)
        {
            if (entry == null) return;
            PendingPaths.Remove(entry);
            Raise(nameof(SelectionCount));
        }

        private async Task ScanAsync()
        {
            Scanning = true;
            Error = "";
            try
            {
                var apps = await _svc.Api.ListDiscoveredApps();
                _allApps = apps != null ? apps.ToArray() : new DiscoveredApp[0];
                Scanned = true;
                ApplyFilter();
            }
            catch (Exception ex)
            {
                Error = ErrorMessages.Humanize(ex, _svc.AppName);
            }
            finally
            {
                Scanning = false;
            }
        }

        private void ApplyFilter()
        {
            var q = (Search ?? "").Trim().ToLowerInvariant();
            FilteredApps.Clear();
            foreach (var app in _allApps)
            {
                if (q.Length == 0 ||
                    (app.DisplayName ?? "").ToLowerInvariant().Contains(q) ||
                    (app.Publisher ?? "").ToLowerInvariant().Contains(q) ||
                    (app.CandidatePath ?? "").ToLowerInvariant().Contains(q))
                {
                    FilteredApps.Add(app);
                }
            }
        }

        private async Task ApplyDiscoveredAsync(DiscoveredApp app)
        {
            if (app == null || !app.Selectable || string.IsNullOrEmpty(app.CandidatePath))
            {
                return;
            }
            if (!Active)
            {
                await AddPathValueAsync(app.CandidatePath, app.CandidateIsDir ? "directory" : "file", app.DisplayName);
                return;
            }
            Busy = true;
            try
            {
                var result = await _svc.Api.AddTrustedPath(app.CandidatePath, app.CandidateIsDir ? "directory" : "file", app.DisplayName);
                _svc.Toast(result != null ? result.Message : "已放行", false);
                await RefreshStatusAsync();
            }
            catch (Exception ex)
            {
                Error = ErrorMessages.Humanize(ex, _svc.AppName);
                _svc.Toast(Error, true);
            }
            finally
            {
                Busy = false;
            }
        }

        private void OnError(Exception ex)
        {
            Error = ErrorMessages.Humanize(ex, _svc.AppName);
            _svc.Toast(Error, true);
        }
    }
}
