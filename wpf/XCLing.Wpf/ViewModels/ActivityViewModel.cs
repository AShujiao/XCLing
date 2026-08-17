using System;
using System.Collections.ObjectModel;
using System.IO;
using System.Threading.Tasks;
using System.Windows.Input;
using XCLing.Wpf.Core;
using XCLing.Wpf.Models;

namespace XCLing.Wpf.ViewModels
{
    /// <summary>运行记录：策略操作记录 + Windows 软件限制事件。</summary>
    public sealed class ActivityViewModel : ViewModelBase, IPageViewModel
    {
        private readonly AppServices _svc;
        private bool _loading;
        private string _error = "";
        private AuditCapability _capability;
        private int _selectedTabIndex;
        private AuditEvent _selectedEvent;

        public ActivityViewModel(AppServices services)
        {
            _svc = services;
            RefreshCommand = new AsyncRelayCommand(LoadAsync, () => !Loading, OnError);
            EnableAuditCommand = new AsyncRelayCommand(EnableAuditAsync, () => !Loading && ShowEnableAudit, OnError);
            UnblockCommand = new AsyncRelayCommand<AuditEvent>(UnblockAsync, OnError, _ => !Loading);
        }

        public string Key => "activity";

        public ICommand RefreshCommand { get; }
        public ICommand EnableAuditCommand { get; }
        public ICommand UnblockCommand { get; }

        public ObservableCollection<ProtectionEvent> Operations { get; } = new ObservableCollection<ProtectionEvent>();
        public ObservableCollection<AuditEvent> Events { get; } = new ObservableCollection<AuditEvent>();

        public AuditCapability Capability
        {
            get => _capability;
            private set => Set(ref _capability, value);
        }

        public int SelectedTabIndex
        {
            get => _selectedTabIndex;
            set
            {
                if (Set(ref _selectedTabIndex, value))
                {
                    Raise(nameof(ShowInterceptionTab));
                    Raise(nameof(ShowOperationTab));
                }
            }
        }

        public AuditEvent SelectedEvent
        {
            get => _selectedEvent;
            set => Set(ref _selectedEvent, value);
        }

        public bool ShowInterceptionTab => SelectedTabIndex == 0;
        public bool ShowOperationTab => SelectedTabIndex == 1;
        public bool Loading { get => _loading; private set => Set(ref _loading, value); }
        public string Error { get => _error; private set => Set(ref _error, value); }
        public bool IsEmpty => Operations.Count == 0 && Events.Count == 0;
        public bool HasOperations => Operations.Count > 0;
        public bool HasEvents => Events.Count > 0;
        public bool ShowEnableAudit => Capability != null && Capability.AuditAvailable && !Capability.AuditEnabled;

        public string StatusText
        {
            get
            {
                var parts = new System.Collections.Generic.List<string>();
                if (Events.Count > 0) parts.Add($"最近24小时拦截 {Events.Count} 条");
                if (Operations.Count > 0) parts.Add($"操作记录 {Operations.Count} 条");
                if (parts.Count == 0) return "点击刷新可手动同步最新记录";
                return string.Join("，", parts);
            }
        }

        public Task OnActivatedAsync() => LoadAsync();

        private async Task EnableAuditAsync()
        {
            Loading = true;
            Error = "";
            try
            {
                await _svc.Api.EnableAuditPolicy();
                _svc.Toast("已启用应用程序事件日志，后续SRP拦截事件将自动记录", false);
                await LoadAsync();
            }
            catch (Exception ex)
            {
                Error = ErrorMessages.Humanize(ex, _svc.AppName);
                _svc.Toast(Error, true);
            }
            finally
            {
                Loading = false;
            }
        }

        private async Task UnblockAsync(AuditEvent ev)
        {
            if (ev == null || string.IsNullOrWhiteSpace(ev.ExecutablePath))
            {
                _svc.Toast("无法获取被拦截程序路径", true);
                return;
            }

            var path = ev.ExecutablePath.Trim();
            var kind = File.Exists(path) ? "file" : "directory";
            var fileName = Path.GetFileName(path);
            if (string.IsNullOrWhiteSpace(fileName)) fileName = path;

            Loading = true;
            Error = "";
            try
            {
                var result = await _svc.Api.AddTrustedPath(path, kind, $"来自拦截记录：{fileName}");
                _svc.Toast(result != null ? result.Message : $"已添加信任：{fileName}", false);
                await LoadAsync();
            }
            catch (Exception ex)
            {
                Error = ErrorMessages.Humanize(ex, _svc.AppName);
                _svc.Toast(Error, true);
            }
            finally
            {
                Loading = false;
            }
        }

        private async Task LoadAsync()
        {
            Loading = true;
            Error = "";
            try
            {
                // 先获取审核能力状态
                Capability = await _svc.Api.GetAuditCapability();

                var ops = await _svc.Api.ListProtectionEvents();
                Operations.Clear();
                if (ops != null)
                {
                    foreach (var op in ops) Operations.Add(op);
                }

                // 只在审核能力可用时查询事件，默认查询最近20条
                if (Capability.Available)
                {
                    var filter = new { window = "24h", keyword = "", max = 20, channel = "" };
                    var result = await _svc.Api.ListBlockedEvents(Json.Serialize(filter));
                    Events.Clear();
                    if (result != null && result.Events != null)
                    {
                        foreach (var ev in result.Events) Events.Add(ev);
                    }
                }
            }
            catch (Exception ex)
            {
                Error = ErrorMessages.Humanize(ex, _svc.AppName);
            }
            finally
            {
                Loading = false;
                Raise(nameof(IsEmpty));
                Raise(nameof(HasOperations));
                Raise(nameof(HasEvents));
                Raise(nameof(StatusText));
                Raise(nameof(ShowEnableAudit));
            }
        }

        private void OnError(Exception ex)
        {
            Error = ErrorMessages.Humanize(ex, _svc.AppName);
            _svc.Toast(Error, true);
        }
    }
}
