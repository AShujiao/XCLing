# WPF UI smoke check: renders every page in both themes at several window sizes and
# bitmap scales against test data, then asserts layout, bindings, state gates and
# virtualization. It never starts the Go core or App startup and never touches SRP.
#
# KEEP THIS FILE ASCII-ONLY. Windows PowerShell 5.1 reads BOM-less files as ANSI, so
# literal CJK text here is decoded as DBCS and can swallow the following line break,
# silently joining the next code line into a comment. Use \uXXXX escapes in the C#
# fixture and English comments at PowerShell level.
param(
    [string]$OutputDirectory = (Join-Path $PSScriptRoot '..\build\bin\ui-smoke')
)

$ErrorActionPreference = 'Stop'
if ([Threading.Thread]::CurrentThread.ApartmentState -ne 'STA') {
    throw 'Run with Windows PowerShell -STA (the net48 WPF runtime is required).'
}
$binaryDirectory = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\wpf\XCLing.Wpf\bin\Release\net48'))
$binary = Join-Path $binaryDirectory 'XCLing.Wpf.exe'
if (-not (Test-Path -LiteralPath $binary)) { throw 'Build the Release WPF project first.' }
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
[IO.Directory]::CreateDirectory($OutputDirectory) | Out-Null
Add-Type -AssemblyName PresentationFramework, PresentationCore, WindowsBase, System.Xaml
[Reflection.Assembly]::LoadFrom((Join-Path $binaryDirectory 'Newtonsoft.Json.dll')) | Out-Null
$assembly = [Reflection.Assembly]::LoadFrom($binary)
[Windows.Application]::ResourceAssembly = $assembly

# A plain Application supplies resources only: never instantiate App or run its startup.
$application = New-Object Windows.Application
$application.ShutdownMode = [Windows.ShutdownMode]::OnExplicitShutdown
foreach ($resource in 'Light','Shared') {
    $uri = [Uri]::new(('/XCLing.Wpf;component/Themes/' + $resource + '.xaml'), [UriKind]::Relative)
    $application.Resources.MergedDictionaries.Add([Windows.Application]::LoadComponent($uri))
}
$application.Resources.Add('BoolToVis', (New-Object XCLing.Wpf.Core.BoolToVisibilityConverter))
$application.Resources.Add('StringToVis', (New-Object XCLing.Wpf.Core.StringToVisibilityConverter))
$application.Resources.Add('Time', (New-Object XCLing.Wpf.Core.TimeConverter))
foreach ($name in 'Console','Rules','Blocklist','Activity','Settings','About') {
    $vmType = $assembly.GetType('XCLing.Wpf.ViewModels.' + $name + 'ViewModel')
    $viewType = $assembly.GetType('XCLing.Wpf.Views.' + $name + 'View')
    $template = New-Object Windows.DataTemplate $vmType
    $template.VisualTree = New-Object Windows.FrameworkElementFactory $viewType
    $application.Resources.Add($template.DataTemplateKey, $template)
}

$references = @(
    [Windows.Application].Assembly.Location,
    [Windows.Media.Visual].Assembly.Location,
    [Windows.DependencyObject].Assembly.Location,
    [System.Xaml.XamlReader].Assembly.Location,
    'System.dll', 'System.Core.dll', $binary,
    (Join-Path $binaryDirectory 'Newtonsoft.Json.dll')
)
$fixtureSource = @'
using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Reflection;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Data;
using System.Windows.Media;
using System.Windows.Media.Imaging;
using System.Windows.Threading;
using XCLing.Wpf;
using XCLing.Wpf.Core;
using XCLing.Wpf.Models;
using XCLing.Wpf.ViewModels;

public sealed class UiBindingTrace : TraceListener {
    public readonly List<string> Errors = new List<string>();
    public override void Write(string message) { }
    public override void WriteLine(string message) { Errors.Add(message); }
}
public static class UiSmoke {
    private static readonly BindingFlags Hidden = BindingFlags.Instance | BindingFlags.NonPublic;
    private static void Set(object target, string name, object value) {
        target.GetType().GetProperty(name, BindingFlags.Instance | BindingFlags.Public).GetSetMethod(true).Invoke(target, new object[] { value });
    }
    private static void Assert(bool condition, string message) { if (!condition) throw new Exception(message); }
    private static IEnumerable<T> Descendants<T>(DependencyObject parent) where T : DependencyObject {
        for (int i = 0; i < VisualTreeHelper.GetChildrenCount(parent); i++) {
            var child = VisualTreeHelper.GetChild(parent, i);
            if (child is T) yield return (T)child;
            foreach (var nested in Descendants<T>(child)) yield return nested;
        }
    }
    private static void Layout(Window window) {
        window.Dispatcher.Invoke(new Action(delegate { }), DispatcherPriority.ApplicationIdle);
        window.UpdateLayout();
    }
    private static void Render(Window window, string path, double dpi) {
        Layout(window);
        var root = (FrameworkElement)window.Content;
        int w = (int)Math.Ceiling(root.ActualWidth * dpi / 96);
        int h = (int)Math.Ceiling(root.ActualHeight * dpi / 96);
        Assert(w > 0 && h > 0, "Blank root layout");
        var bitmap = new RenderTargetBitmap(w, h, dpi, dpi, PixelFormats.Pbgra32);
        bitmap.Render(root);
        var pixels = new byte[w * h * 4]; bitmap.CopyPixels(pixels, w * 4, 0);
        Assert(pixels.Distinct().Count() > 20, "Blank or unrendered bitmap");
        var encoder = new PngBitmapEncoder(); encoder.Frames.Add(BitmapFrame.Create(bitmap));
        using (var stream = File.Create(path)) encoder.Save(stream);
        foreach (var image in Descendants<Image>(root)) Assert(image.Source != null, "Image resource missing");
        foreach (var button in Descendants<Button>(root).Where(b => b.IsVisible)) {
            Assert(button.ActualWidth > 0 && button.ActualHeight > 0, "Button has no layout");
            if (button.Content is string) {
                var text = new FormattedText((string)button.Content, System.Globalization.CultureInfo.CurrentCulture,
                    FlowDirection.LeftToRight, new Typeface(button.FontFamily, button.FontStyle, button.FontWeight, button.FontStretch), button.FontSize, Brushes.Black);
                Assert(text.Width <= button.ActualWidth + 1, "Button label clipped: " + button.Content);
                foreach (var label in Descendants<TextBlock>(button)) {
                    Assert(label.Foreground.ToString() == button.Foreground.ToString(), "Button foreground overridden: " + button.Content);
                }
            }
        }
    }
    public static void Run(string output) {
        var trace = new UiBindingTrace();
        PresentationTraceSources.DataBindingSource.Listeners.Add(trace);
        PresentationTraceSources.DataBindingSource.Switch.Level = SourceLevels.Error;
        int confirmations = 0;
        var main = new MainViewModel(null, "\u661f\u9648\u5b88\u62a4", "__CORE_VERSION__", delegate(ConfirmRequest r) { confirmations++; return false; }, delegate { });
        var pages = (Dictionary<string, IPageViewModel>)typeof(MainViewModel).GetField("_pages", Hidden).GetValue(main);
        var console = main.Console;
        var status = new ApplyStatus { IsAdmin = true, Active = true, ProtectionState = "locked", PolicyMode = "whitelist",
            CanRestore = true, CanUnlock = true, CanLock = true, ExistingRuleCount = 12,
            Reason = "\u767d\u540d\u5355\u6a21\u5f0f\u5df2\u542f\u7528", BackupCreatedAt = "2026-09-07T09:00:00+08:00" };
        Set(console, "Status", status);
        for (int i = 0; i < 4; i++) console.RecentOperations.Add(new ProtectionEvent { Action = i == 0 ? "rule_add" : "lock", Success = true, CreatedAt = "2026-09-07T10:42:00+08:00", Message = "Sample operation" });
        var rules = (RulesViewModel)pages["rules"];
        Set(rules, "Status", status);
        rules.ActiveExtraRules.Add(new ManagedRule { Id = "sample", Path = @"D:\Applications\OfficeTools\VeryLongDirectoryNameForLayoutVerification\Subfolder\*", Description = "\u529e\u516c\u5de5\u5177", Removable = true });
        rules.NewPath = @"D:\Applications\OfficeTools";
        var block = (BlocklistViewModel)pages["blocklist"];
        typeof(BlocklistViewModel).GetField("_status", Hidden).SetValue(block, new BlocklistStatus { ProtectionState = "locked", Enforcing = true, IsAdmin = true, RuleCount = 2, Reason = "2 rules" });
        block.Rules.Add(new BlockRule { Id = "sample", Pattern = @"C:\Users\Public\Untrusted\startup.exe", Label = "\u81ea\u5b9a\u4e49\u62e6\u622a", Kind = "file" });
        block.Vendors.Add(new VendorPreset { Id = "sample", Name = "\u793a\u4f8b\u5382\u5546", Description = "\u9884\u8bbe\u7a0b\u5e8f\u89c4\u5219", Applied = false });
        block.Vendors.Add(new VendorPreset { Id = "applied", Name = "\u5df2\u5e94\u7528\u5382\u5546", Description = "\u5df2\u5e94\u7528\u7684\u9884\u8bbe\u5305", Applied = true });
        block.ScanResults.Add(new BlockedVendorScan { Id = "scan1", DisplayName = "\u626b\u63cf\u7ed3\u679c", Publisher = "Sample publisher", InstallPath = @"C:\Program Files\Sample\app.exe", Suggested = true, AlreadyBlocked = false });
        block.ScanResults.Add(new BlockedVendorScan { Id = "scan2", DisplayName = "\u5df2\u62e6\u622a\u9879", Publisher = "Sample publisher", InstallPath = @"C:\Program Files\Sample2\app.exe", Suggested = false, AlreadyBlocked = true });
        var activity = (ActivityViewModel)pages["activity"];
        for (int i = 0; i < 200; i++) {
            activity.Operations.Add(new ProtectionEvent { Action = "lock", Success = true, CreatedAt = "2026-09-07T10:42:00+08:00", Message = "Sample operation " + i });
            activity.Events.Add(new AuditEvent { ExecutablePath = @"C:\Users\Public\VeryLongFolderNameForTooltip\Untrusted\startup.exe", Timestamp = "2026-09-07T10:42:00+08:00", User = "User", Message = "Sample blocked event" });
        }
        var window = new MainWindow { DataContext = main, ShowInTaskbar = false, ShowActivated = false,
            WindowStartupLocation = WindowStartupLocation.Manual, Left = -30000, Top = -30000 };
        try {
            window.Show();
            Assert(main.NavItems.Count == 5 && main.AboutItem.Key == "about", "Navigation missing pages");
            Assert(new Settings().Theme == "light", "Fresh default must be light");
            foreach (string theme in new[] { "light", "dark" }) {
                ((Settings)typeof(MainViewModel).GetField("_settings", Hidden).GetValue(main)).Theme = theme;
                ThemeManager.Init(theme);
                Assert(ThemeManager.Current == theme, "Theme startup not synchronized");
                var expected = theme == "light" ? "#FFFFFFFF" : "#FF252729";
                Assert(((SolidColorBrush)Application.Current.FindResource("Surface0Brush")).Color.ToString() == expected, "Actual theme mismatch");
                foreach (int width in new[] { 1100, 900, 760 }) {
                    window.Width = width; window.Height = width == 1100 ? 700 : width == 900 ? 560 : 480;
                    foreach (string key in new[] { "console", "rules", "blocklist", "activity", "settings", "about" }) {
                        Set(main, "CurrentPage", pages[key]);
                        foreach (var nav in main.NavItems) nav.IsActive = nav.Key == key;
                        main.AboutItem.IsActive = key == "about";
                        Render(window, Path.Combine(output, theme + "-" + key + "-" + width + ".png"), 96);
                        if (key == "rules") {
                            var combo = Descendants<ComboBox>(window).First();
                            combo.IsDropDownOpen = true; Layout(window);
                            Assert(combo.IsDropDownOpen, "ComboBox did not open");
                            combo.SelectedIndex = 1; Layout(window);
                            Assert(rules.NewKind == "file", "ComboBox binding failed");
                            combo.SelectedIndex = 0; combo.IsDropDownOpen = false;
                        }
                        if (key == "activity") {
                            var tabs = Descendants<TabControl>(window).First(); tabs.SelectedIndex = 1; Layout(window);
                            Assert(activity.SelectedTabIndex == 1 && activity.ShowOperationTab, "Tab binding failed");
                            var grid = Descendants<DataGrid>(window).First(g => g.IsVisible);
                            Assert(Descendants<DataGridRow>(grid).Count() < 200, "DataGrid virtualization lost");
                            tabs.SelectedIndex = 0;
                        }
                    }
                }
            }
            // Blocklist page layout: vendor presets must render above the rule list, so applying a
            // preset cannot push already-read content down while the user watches it.
            Set(main, "CurrentPage", pages["blocklist"]);
            foreach (var nav in main.NavItems) nav.IsActive = nav.Key == "blocklist";
            Layout(window);
            var presetTitle = Descendants<TextBlock>(window).First(t => t.Text == "\u5382\u5546\u9884\u8bbe");
            var ruleTitle = Descendants<TextBlock>(window).First(t => t.Text == "\u5f53\u524d\u62e6\u622a\u89c4\u5219");
            double presetTop = presetTitle.TransformToAncestor(window).Transform(new Point(0, 0)).Y;
            double ruleTop = ruleTitle.TransformToAncestor(window).Transform(new Point(0, 0)).Y;
            Assert(presetTop < ruleTop, "Vendor presets must render above the rule list");
            Render(window, Path.Combine(output, "blocklist-order.png"), 96);
            Set(main, "CurrentPage", console); window.Width = 900; window.Height = 560;
            foreach (var nav in main.NavItems) nav.IsActive = nav.Key == "console";
            main.AboutItem.IsActive = false;
            foreach (string state in new[] { "unmanaged", "locked", "unlocked", "attention" }) {
                status.ProtectionState = state; status.CanApply = state == "unmanaged";
                status.Reason = state == "unmanaged" ? "\u5c1a\u672a\u542f\u7528\u4fdd\u62a4" : state == "unlocked" ? "\u89c4\u5219\u4ecd\u4fdd\u7559" : state == "attention" ? "\u7b56\u7565\u5df2\u88ab\u5916\u90e8\u4fee\u6539" : "\u767d\u540d\u5355\u6a21\u5f0f\u5df2\u542f\u7528";
                status.ExistingRuleCount = state == "unmanaged" ? 0 : 12;
                status.BackupCreatedAt = state == "unmanaged" ? "" : "2026-09-07T09:00:00+08:00";
                Set(console, "Status", status);
                Assert(console.ShowModeSelection == (state == "unmanaged"), "Mode selector state mismatch");
                Assert(console.PrimaryEnabled == (state != "unmanaged"), "Primary command gate mismatch");
                Render(window, Path.Combine(output, "state-" + state + ".png"), 120);
            }
            console.PrimaryCommand.Execute(null);
            Assert(confirmations == 1, "Attention recovery must confirm before any RPC");
            status.IsAdmin = false; status.CanUnlock = false; status.CanLock = false; status.CanRestore = false;
            Set(console, "Status", status); Assert(!console.PrimaryEnabled, "Non-admin action enabled");
            Set(console, "Status", null);
            Assert(console.State == "loading" && !console.ShowModeSelection && !console.PrimaryEnabled, "Loading mistaken for unmanaged");
            Render(window, Path.Combine(output, "state-loading.png"), 144);
            Set(console, "ErrorText", "Sample failure");
            Assert(console.State == "unavailable" && !console.PrimaryEnabled, "Error state not safe");
            Render(window, Path.Combine(output, "state-error.png"), 96);
            // Overview "recent operations" empty state: no empty hint while loading or after an error.
            console.RecentOperations.Clear();
            Assert(console.RecentOperations.Count == 0, "Recent operations clear failed");
            Set(console, "Loading", true);
            Assert(!console.ShowEmptyRecent, "Empty recent list shown while loading");
            Set(console, "Loading", false);
            Assert(console.ShowEmptyRecent, "Empty recent list not shown after load");
            // Audit capability unavailable: surface the backend reason instead of "no blocked events".
            activity.Events.Clear();
            Set(activity, "Capability", new AuditCapability { Available = false, Reason = "Sample audit reason" });
            typeof(ViewModelBase).GetMethod("Raise", Hidden).Invoke(activity, new object[] { "HasEvents" });
            Assert(activity.ShowAuditUnavailable && !activity.ShowEmptyEvents && activity.AuditUnavailableText == "Sample audit reason", "Audit-unavailable empty state not applied");
            Set(main, "CurrentPage", activity);
            foreach (var nav in main.NavItems) nav.IsActive = nav.Key == "activity";
            Render(window, Path.Combine(output, "state-audit-unavailable.png"), 96);
            Set(main, "CurrentPage", console);
            var confirm = new XCLing.Wpf.Views.ConfirmWindow(); confirm.Close();
            var donate = new XCLing.Wpf.Views.DonateWindow(); donate.Close();
            File.WriteAllLines(Path.Combine(output, "binding-errors.txt"), trace.Errors);
            Assert(trace.Errors.Count == 0, "WPF binding errors; inspect binding-errors.txt");
            Console.WriteLine("PASS: 6 pages, 2 themes, 3 window sizes; 100/125/150% bitmap scales; state/permission gates, dropdown, tabs, virtualization, dialogs, images and binding diagnostics. No core or App startup executed.");
        } finally { window.Close(); PresentationTraceSources.DataBindingSource.Listeners.Remove(trace); }
    }
}
'@
# Core version comes from the repository VERSION file so the fixture cannot drift.
$coreVersion = (Get-Content -Raw (Join-Path $PSScriptRoot '..\VERSION')).Trim()
$fixtureSource = $fixtureSource.Replace('__CORE_VERSION__', $coreVersion)
$compiler = New-Object Microsoft.CSharp.CSharpCodeProvider
$compilerOptions = New-Object CodeDom.Compiler.CompilerParameters
$compilerOptions.GenerateInMemory = $true
foreach ($reference in $references) { $compilerOptions.ReferencedAssemblies.Add($reference) | Out-Null }
$compilation = $compiler.CompileAssemblyFromSource($compilerOptions, $fixtureSource)
if ($compilation.Errors.HasErrors) { throw ($compilation.Errors | Out-String) }
try { [UiSmoke]::Run($OutputDirectory) }
finally { $application.Shutdown(); $compiler.Dispose() }
