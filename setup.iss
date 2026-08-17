; 星陈守护 - Inno Setup 安装脚本
; 需要 Inno Setup 6.0+ 支持 Unicode
; 可通过命令行参数 /dIncludeWin7=0 跳过Win7版本构建更小安装包
#define MyAppName "星陈守护"
; 版本号唯一来源是仓库根目录的 VERSION 文件
#define VerFile FileOpen(SourcePath + "\VERSION")
#define MyAppVersion Trim(FileRead(VerFile))
#expr FileClose(VerFile)
#define MyAppPublisher "星陈守护"
#define MyAppExeName "星陈守护.exe"
#define MyAppCoreName "xcling-core.exe"
#define MyAppConfigName "app.config.json"
#ifndef IncludeWin7
  #define IncludeWin7 "1"
#endif

[Setup]
AppId={{70D25F8A-78A4-4E1C-9F0D-123456789ABC}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
DefaultDirName={autopf}\XCLing
DefaultGroupName={#MyAppName}
AllowNoIcons=yes
LicenseFile=LICENSE
OutputDir=build\installer
#if IncludeWin7 == "1"
OutputBaseFilename=星陈守护-Setup-{#MyAppVersion}
#else
OutputBaseFilename=星陈守护-Setup-{#MyAppVersion}-win10+
#endif
SetupIconFile=build\windows\icon.ico
Compression=lzma2/ultra
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=admin
ArchitecturesInstallIn64BitMode=x64compatible
UninstallDisplayIcon={app}\{#MyAppExeName}

[Languages]
Name: "chinesesimplified"; MessagesFile: "compiler:Languages\ChineseSimplified.isl"

[Tasks]
Name: "desktopicon"; Description: "创建桌面快捷方式"; GroupDescription: "附加图标:"; Flags: unchecked

[Files]
; Win10/11 版本为主，Win7用户可手动选择win7目录运行
Source: "build\bin\wpf-win10\{#MyAppExeName}"; DestDir: "{app}"; Flags: ignoreversion
Source: "build\bin\wpf-win10\{#MyAppExeName}.config"; DestDir: "{app}"; Flags: ignoreversion
Source: "build\bin\wpf-win10\Newtonsoft.Json.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "build\bin\wpf-win10\{#MyAppCoreName}"; DestDir: "{app}"; Flags: ignoreversion
Source: "build\bin\wpf-win10\{#MyAppConfigName}"; DestDir: "{app}"; Flags: ignoreversion
#if IncludeWin7 == "1"
; Win7版本兼容包
Source: "build\bin\wpf-win7\{#MyAppCoreName}"; DestDir: "{app}\win7"; Flags: ignoreversion
#endif

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{group}\卸载 {#MyAppName}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "运行 {#MyAppName}"; Flags: nowait postinstall skipifsilent

#if IncludeWin7 == "1"
[UninstallDelete]
Type: filesandordirs; Name: "{app}\win7"
#endif
