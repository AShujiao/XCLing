# 星陈守护（XCLing）

<p align="center">
  <img src="logo/logo-title.png" alt="星陈守护 Logo" width="280"/>
</p>

基于 Windows 软件限制策略（SRP）的桌面管理工具，用白名单控制允许运行的软件，或用黑名单限制指定程序。提供策略备份、临时解锁、重新锁定和恢复原状操作，适合需要限制软件运行的个人电脑。

原生 WPF 界面，支持明亮与深色主题，无需 WebView2。当前源码版本为 **0.3.17**，更新内容见 [更新日志](changelog.md)。

[下载安装包](https://github.com/AShujiao/XCLing/releases) · [反馈问题](https://github.com/AShujiao/XCLing/issues) · [源代码](https://github.com/AShujiao/XCLing)

## 系统要求

| 项目 | 要求 |
| --- | --- |
| 系统架构 | 64 位 Windows |
| Windows 10 / 11 | 需要 .NET Framework 4.8；未安装的系统需先补装 |
| Windows 7 | Windows 7 SP1，安装 KB4490628、KB4474419 和 .NET Framework 4.8，使用含 Win7 兼容核心的安装包或部署目录 |
| 运行权限 | 以管理员身份运行，修改 SRP 需要管理员权限 |
| 域环境 | 不允许在域成员电脑上接管策略 |

Win7 使用单独编译的 Go 1.20.14 核心，不能用 Win10/11 核心替代。当前版本保留 Win7 构建支持；本次 UI 更新尚未经过 Win7 实机或虚拟机验证。

## 快速开始

1. 从 [Releases](https://github.com/AShujiao/XCLing/releases) 下载适合系统的安装包，完成安装后以管理员身份运行。
2. 在「概览」选择保护模式。使用白名单模式时，先在「白名单」添加需要放行的软件路径，再启用保护。
3. 使用黑名单模式时，启用保护后进入「黑名单」，添加规则、应用厂商预设或扫描本机软件。
4. 在「概览」查看保护状态；需要安装或调试软件时，可按需临时解锁，完成后手动重新锁定。
5. 在「运行记录」查看保护操作和可用的 Windows 拦截事件。
6. **卸载或停止使用前，先执行「恢复原状」并确认成功。** 关闭窗口、退出程序或卸载程序不会自动撤销系统策略。

关闭主窗口会隐藏到系统托盘。可通过托盘重新打开窗口、临时解锁、重新锁定或退出程序。

## 保护模式

| 对比项 | 白名单模式 | 黑名单模式 |
| --- | --- | --- |
| 默认行为 | 限制未放行的软件 | 默认放行，限制命中黑名单的软件 |
| 规则配置 | 默认可信目录、自定义放行路径，也可叠加黑名单 | 文件名、目录或精确文件拦截规则 |
| 适用场景 | 只允许指定范围的软件运行 | 限制若干指定软件 |
| 临时解锁 | 放开默认限制，显式黑名单规则仍可能拦截 | 移除当前生效的拦截规则，规则保留在恢复记录中 |
| 重新锁定 | 恢复默认限制 | 恢复保存的拦截规则 |

两种模式互斥，切换前需要先恢复原状。临时解锁不会因退出或重启自动结束，需手动重新锁定。

SRP 只拦新启动的程序：已经在运行的程序不受影响。策略的生效形态（从放行变为拦截、或反之）在进程启动时被读取，因此启用保护或新增第一条拦截规则后，程序会自动重启资源管理器，让双击启动的程序立即遵循新策略；若概览页提示资源管理器尚未加载最新策略，可点击提示条上的「重启资源管理器」。

SRP 按规则匹配程序，不负责识别病毒，也不会卸载软件。规则效果取决于配置的路径、文件名及系统策略。

## 功能

### 概览

集中展示保护状态、当前模式、规则数量、权限与备份信息，提供启用保护、临时解锁和重新锁定操作。最近四条保护操作直接显示在概览中，可跳转查看完整记录；恢复原状位于独立的次要操作区域。

### 白名单与黑名单

白名单支持选择目录或程序文件，也可扫描已安装软件辅助添加。系统基础规则与程序自身规则受到保护；启用前暂存的待放行路径只在当前会话内保留。

黑名单支持以下匹配方式：

| 类型 | 示例 | 匹配范围 |
| --- | --- | --- |
| 文件名 | `example.exe` | 匹配不同目录下的同名程序；改名后不再命中这条规则 |
| 目录 | `C:\Apps\Example\*` | 限制指定目录树中的程序 |
| 精确文件 | `C:\Apps\Example\app.exe` | 限制指定路径的程序 |

内置七组厂商预设：360、2345、腾讯电脑管家、金山毒霸 / 猎豹、百度、迅雷、驱动人生 / 鲁大师。可整组应用或移除，也可扫描本机安装路径后选择添加。

添加目录拦截时，会从目录内的可执行文件派生文件名规则，匹配范围可能扩展到目录外的同名程序。添加拦截规则后，程序会尝试结束已运行的匹配进程；系统目录和程序自身等受保护目标会被跳过。

### 运行记录

- **保护操作**：保存最近 200 条启用、解锁、锁定、恢复、规则变更等操作记录。
- **拦截事件**：查询 Windows 事件日志中最近 24 小时的拦截事件（最多 20 条）；是否有记录取决于系统支持及日志配置。「启用日志」会修改相关审计或日志配置。

### 设置与关于

- 明亮 / 深色主题，切换后保存偏好；首次使用默认明亮，升级保留已保存的主题。
- 开机自启动，默认启用，登录后进入托盘；优先使用具有最高权限的计划任务。
- 每日定时关机，触发前提供 60 秒倒计时与取消操作，需要程序及核心保持运行。
- Packaged Apps（UWP / Store 应用）、Defender 更新相关兼容选项。
- 「关于与捐助」提供版本信息、手动检查更新和捐助入口。检查更新访问 GitHub Releases，下载通过浏览器完成，不自动安装。

## 策略与本地数据

接管前备份原有策略，策略写入后回读校验，失败时尝试回滚。检测到外部策略变化时会提示异常，应根据界面提示核对并恢复。不要在保护启用期间手动删除恢复记录。

| 路径 | 用途 |
| --- | --- |
| `%ProgramData%\XCLing\recovery\active.json` | 策略恢复记录，使用限制到管理员与 SYSTEM 的访问权限 |
| `%ProgramData%\XCLing\activity\events.json` | 最近 200 条保护操作 |
| `%AppData%\XCLing\wpf-settings.json` | 主题与界面侧设置 |
| `%AppData%\XCLing\shutdown-config.json` | 定时关机配置 |
| `%AppData%\XCLing\wpf-crash.log` | 界面异常日志 |

自启动设置还会使用计划任务及当前用户注册表，偏好保存在 `HKCU\Software\XCLing\AutoStartEnabled`。

应用不包含遥测。手动检查更新时会请求 GitHub Releases API；点击外部链接会打开浏览器。

## 开发与构建

### 技术栈

界面使用 **WPF / .NET Framework 4.8 / MVVM**，策略逻辑由 **Go 核心进程**处理，通过标准输入输出上的 NDJSON JSON-RPC 通信。

```text
wpf/XCLing.Wpf/    原生界面、视图模型、托盘与核心通信
cmd/core/          Go 核心入口
internal/          策略、规则、审计、平台访问与数据存储
cmd/checksum/      发布产物校验和工具
scripts/          构建、安装包与 UI 验证脚本
setup.iss         Inno Setup 安装脚本
VERSION           版本号来源
changelog.md      版本更新日志
```

在 Windows 开发环境构建，需要：

| 工具 | 用途 |
| --- | --- |
| Go 1.25 或更高版本 | 编译 Win10/11 核心，以 `go.mod` 为准 |
| .NET SDK 8 或更高版本 | 构建 net48 WPF 项目，引用程序集通过 NuGet 还原 |
| Go 1.20.14 | 编译 Win7 核心，使用 `go.win7.mod` 和 `win7` 构建标签 |
| Inno Setup | 可选，用于编译 `setup.iss` 生成安装包 |

### 构建程序

在仓库根目录运行：

```powershell
# 同时生成 Win10/11 和 Win7 部署目录
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build-wpf.ps1

# 仅生成 Win10/11 部署目录
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build-wpf.ps1 -SkipWin7

# 指定 Win7 工具链根目录（目录下应包含 bin\go.exe）
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build-wpf.ps1 -GoRoot "C:\toolchains\go1.20.14"
```

默认 Win7 工具链路径为 `C:\tmp\policyguard-go1.20.14\go`。未安装该工具链时，使用 `-SkipWin7`。

| 产物 | 内容 |
| --- | --- |
| `build/bin/wpf-win10/` | Win10/11 界面、核心及依赖文件 |
| `build/bin/wpf-win7/` | 同一 WPF 界面、Win7 兼容核心及依赖文件 |
| `build/installer/` | 安装包输出目录 |

部署目录中，`星陈守护.exe` 与对应的 `xcling-core.exe` 位于同一目录。运行时请保留整个目录的依赖文件，不要只复制主程序。

### 构建安装包

```powershell
# 构建程序并生成含 Win7 核心的安装包
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build-installer.ps1

# 生成仅面向 Win10/11 的安装包
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build-installer.ps1 -SkipWin7
```

脚本支持 `-InnoSetupPath` 指定编译器，支持 `-SkipBuild` 复用已有部署产物；复用前需确认产物版本与目标系统一致。

完整安装包将 Win7 核心放在安装目录的 `win7/xcling-core.exe`，由界面按系统版本选择。安装包文件名为 `星陈守护-Setup-{版本}.exe`，仅 Win10/11 变体追加 `-win10+`。

### 验证

```powershell
# Go 单元测试、静态检查与编译
go test -count=1 ./...
go vet ./...
go build ./...

# 构建部署目录后，运行 WPF 界面冒烟检查
powershell -NoProfile -STA -ExecutionPolicy Bypass -File scripts/verify-wpf-ui.ps1
```

UI 验证脚本使用测试数据检查页面、主题、窗口尺寸、绑定和部分交互，不启动业务核心、不修改系统策略；截图默认输出到 `build/bin/ui-smoke/`。界面渲染验证不能替代目标系统上的实际运行测试。

### 版本维护

版本号统一维护在 [VERSION](VERSION)，构建脚本将其用于应用与安装包。每次更新在 [changelog.md](changelog.md) 顶部补充对应版本的新增、优化、修复及兼容性说明。

发布时，GitHub Release 标签应与 `VERSION` 对齐，例如 `v0.3.17`，并上传对应安装包。应用内检查更新读取 GitHub 最新 Release，修改源码版本号本身不会发布更新。

## 反馈与支持

提交 [Issue](https://github.com/AShujiao/XCLing/issues) 时，请提供软件版本、Windows 版本、保护模式、复现步骤和错误信息。分享日志或截图前，请检查其中的个人路径等信息。

可在应用「关于与捐助」中查看收款二维码，也可使用下方二维码支持维护：

<p align="center">
  <img src="wpf/XCLing.Wpf/Assets/donate.jpg" width="360" alt="微信与支付宝捐助二维码"/>
</p>

感谢 [Linux.do](https://linux.do/) 社区的支持与反馈。

## 许可证

项目采用 [MIT License](LICENSE)。界面使用的 Lucide 图标及相关许可见 [图标许可文件](wpf/XCLing.Wpf/Assets/LUCIDE-LICENSE.txt)。
