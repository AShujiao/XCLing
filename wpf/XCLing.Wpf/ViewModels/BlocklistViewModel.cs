using System;
using System.Collections.ObjectModel;
using System.Linq;
using System.Threading.Tasks;
using System.Windows.Input;
using XCLing.Wpf.Core;
using XCLing.Wpf.Models;

namespace XCLing.Wpf.ViewModels
{
    /// <summary>黑名单：预设厂商包一键应用 + 扫描本机安装软件 + 手动添加规则。启用/切换模式统一在「概览」。</summary>
    public sealed class BlocklistViewModel : ViewModelBase, IPageViewModel
    {
        private readonly AppServices _svc;
        private BlocklistStatus _status;
        private bool _busy;
        private bool _scanning;
        private string _error = "";
        private string _newPattern = "";
        private string _newKind = "filename";

        public BlocklistViewModel(AppServices services)
        {
            _svc = services ?? throw new ArgumentNullException(nameof(services));

            RefreshCommand = new AsyncRelayCommand(RefreshAsync, () => !Busy, OnError);
            AddRuleCommand = new AsyncRelayCommand(AddRuleAsync, () => !Busy, OnError);
            RemoveRuleCommand = new AsyncRelayCommand<BlockRule>(RemoveRuleAsync, OnError);
            ApplyVendorCommand = new AsyncRelayCommand<VendorPreset>(ApplyVendorAsync, OnError);
            RemoveVendorCommand = new AsyncRelayCommand<VendorPreset>(RemoveVendorAsync, OnError);
            ScanCommand = new AsyncRelayCommand(ScanAsync, () => !Scanning, OnError);
            ApplyScanItemCommand = new AsyncRelayCommand<BlockedVendorScan>(ApplyScanItemAsync, OnError);
            GoConsoleCommand = new RelayCommand(() => _svc.Navigate("console"));
        }

        public string Key => "blocklist";

        public ICommand RefreshCommand { get; }
        public ICommand AddRuleCommand { get; }
        public ICommand RemoveRuleCommand { get; }
        public ICommand ApplyVendorCommand { get; }
        public ICommand RemoveVendorCommand { get; }
        public ICommand ScanCommand { get; }
        public ICommand ApplyScanItemCommand { get; }
        public ICommand GoConsoleCommand { get; }

        public ObservableCollection<VendorPreset> Vendors { get; } = new ObservableCollection<VendorPreset>();
        public ObservableCollection<BlockRule> Rules { get; } = new ObservableCollection<BlockRule>();
        public ObservableCollection<BlockedVendorScan> ScanResults { get; } = new ObservableCollection<BlockedVendorScan>();

        public bool Busy { get => _busy; private set { if (Set(ref _busy, value)) { Raise(nameof(StatusText)); } } }
        public bool Scanning { get => _scanning; private set => Set(ref _scanning, value); }
        public string Error { get => _error; private set => Set(ref _error, value); }
        public string NewPattern { get => _newPattern; set => Set(ref _newPattern, value); }
        public string NewKind { get => _newKind; set => Set(ref _newKind, value); }

        public bool Enforcing => _status?.Enforcing ?? false;
        public bool IsAdmin => _status?.IsAdmin ?? false;
        public int RuleCount => _status?.RuleCount ?? 0;
        /// <summary>保护完全未启用时提示先去「概览」启用黑名单模式。</summary>
        public bool ShowNotEnabledWarning => _status != null && _status.ProtectionState == "unmanaged";
        public string StatusText => _status == null
            ? "正在加载..."
            : _status.Reason ?? (_status.Enforcing ? $"共 {_status.RuleCount} 条黑名单规则，正在生效" : "保护未启用，规则配置后生效");

        public string EnforcingLabel => Enforcing
            ? "拦截生效中"
            : (_status != null && _status.ProtectionState == "unlocked" ? "已临时解锁" : "保护未启用");
        public bool HasRules => Rules.Count > 0;
        public bool HasScanResults => ScanResults.Count > 0;

        public Task OnActivatedAsync()
        {
            return RefreshAsync();
        }

        private async Task RefreshAsync()
        {
            Busy = true;
            Error = "";
            try
            {
                _status = await _svc.Api.GetBlocklistStatus();

                if (_status == null)
                {
                    Error = "获取黑名单状态失败：返回值为空";
                    return;
                }

                SyncVendors(_status.Vendors);
                SyncRules(_status.Rules);
                RaiseAll();
            }
            catch (Exception ex)
            {
                Error = ErrorMessages.Humanize(ex, _svc.AppName);
            }
            finally
            {
                Busy = false;
            }
        }

        private async Task AddRuleAsync()
        {
            var pattern = NewPattern;
            var kind = NewKind;
            // 第一条拦截规则会新建 SRP 的 0\Paths 键，属于策略生效形态变化：
            // 已在运行的资源管理器不会采用它，需要重新加载才能拦住双击启动的程序。
            var firstRule = RuleCount == 0;
            NewPattern = "";
            Busy = true;
            Error = "";
            try
            {
                var result = await _svc.Api.AddBlockRule(pattern, kind, "");
                _svc.Toast(result.Message, false);
                if (firstRule)
                {
                    await _svc.NotifyPolicyShapeChangedAsync();
                }
                await RefreshAsync();
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

        private async Task RemoveRuleAsync(BlockRule rule)
        {
            if (rule == null) return;
            Busy = true;
            try
            {
                var result = await _svc.Api.RemoveBlockRule(rule.Id);
                _svc.Toast(result.Message, false);
                await RefreshAsync();
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

        private async Task ApplyVendorAsync(VendorPreset vendor)
        {
            if (vendor == null) return;
            var firstRule = RuleCount == 0;
            Busy = true;
            try
            {
                var result = await _svc.Api.ApplyVendorPreset(vendor.Id);
                _svc.Toast(result.Message, false);
                if (firstRule)
                {
                    await _svc.NotifyPolicyShapeChangedAsync();
                }
                await RefreshAsync();
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

        private async Task RemoveVendorAsync(VendorPreset vendor)
        {
            if (vendor == null) return;
            if (!_svc.Confirm(new ConfirmRequest
            {
                Title = "移除拦截规则",
                Message = $"将移除 {vendor.Name} 的全部拦截规则。",
                ConfirmText = "确认移除",
                Danger = false,
            })) return;
            Busy = true;
            try
            {
                var result = await _svc.Api.RemoveVendorPreset(vendor.Id);
                _svc.Toast(result.Message, false);
                await RefreshAsync();
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

        private async Task ScanAsync()
        {
            Scanning = true;
            Error = "";
            try
            {
                var results = await _svc.Api.ScanVendorTargets();
                ScanResults.Clear();
                if (results != null)
                {
                    foreach (var item in results.OrderByDescending(r => r.Suggested).ThenBy(r => r.DisplayName))
                        ScanResults.Add(item);
                }
                Raise(nameof(HasScanResults));
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

        private async Task ApplyScanItemAsync(BlockedVendorScan item)
        {
            if (item == null || item.AlreadyBlocked) return;
            var firstRule = RuleCount == 0;
            Busy = true;
            try
            {
                var result = await _svc.Api.ApplyScanResult(new System.Collections.Generic.List<string> { item.InstallPath });
                item.AlreadyBlocked = true;
                // BlockedVendorScan 是纯 DTO，不实现 INotifyPropertyChanged：
                // 用 Replace 触发条目容器重建，让「拦截」按钮立即变为「已拦截」，避免重复点击无反馈。
                var index = ScanResults.IndexOf(item);
                if (index >= 0)
                {
                    ScanResults[index] = item;
                }
                _svc.Toast(result.Message, false);
                if (firstRule)
                {
                    await _svc.NotifyPolicyShapeChangedAsync();
                }
                await RefreshAsync();
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

        private void SyncVendors(System.Collections.Generic.List<VendorPreset> vendors)
        {
            Vendors.Clear();
            if (vendors == null) return;
            foreach (var v in vendors)
                Vendors.Add(v);
        }

        private void SyncRules(System.Collections.Generic.List<BlockRule> rules)
        {
            Rules.Clear();
            if (rules == null) return;
            foreach (var r in rules)
                Rules.Add(r);
            Raise(nameof(HasRules));
        }

        private void RaiseAll()
        {
            Raise(nameof(Enforcing));
            Raise(nameof(IsAdmin));
            Raise(nameof(RuleCount));
            Raise(nameof(ShowNotEnabledWarning));
            Raise(nameof(StatusText));
            Raise(nameof(EnforcingLabel));
            Raise(nameof(HasRules));
        }

        private void OnError(Exception ex)
        {
            Error = ErrorMessages.Humanize(ex, _svc.AppName);
            _svc.Toast(Error, true);
        }
    }
}
