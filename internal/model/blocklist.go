package model

// 本文件定义「厂商拦截名单」（blocklist）链路的跨 IPC 数据结构。
//
// 与白名单的关系（务必理解，否则会误判功能边界）：
//   - 白名单是 SRP 的 *默认级别*（DefaultLevel=Disallowed）+ 一批 262144(Unrestricted) 放行规则；
//   - 拦截名单是一批 *显式* 0(Disallowed) 规则，写在 CodeIdentifiers\0\Paths 下。
//
// SRP 的判定顺序是「最具体的路径规则优先」，与规则级别无关。因此：
//   - `C:\Program Files\360\*`(disallow) 比 `C:\Program Files\*`(allow) 更具体 → 360 被拦截；
//   - 显式 disallow 规则不依赖 DefaultLevel，所以「临时解锁」白名单时拦截名单**依然生效**。
//
// 这正是拦截名单存在的意义：标准安装目录默认可信，但其中的特定厂商软件仍要被挡住。

// 拦截规则的匹配形态（BlockRule.Kind）。
const (
	// BlockKindFileName 裸文件名规则，例如 `360se.exe`。SRP 会在**任意目录**匹配该文件名，
	// 因此改名/移动/重装都挡得住，是拦截垃圾软件最有效的形态。
	BlockKindFileName = "filename"
	// BlockKindDirectory 目录规则，写入时补 `\*`，拦截该目录下全部可执行文件。
	BlockKindDirectory = "directory"
	// BlockKindFile 精确可执行文件路径规则。
	BlockKindFile = "file"
)

// BlockRule 一条厂商拦截规则。Pattern 为写入 SRP ItemData 的最终模式。
type BlockRule struct {
	ID       string `json:"id"`       // SRP 子键名（GUID 形态，由 pattern 派生，稳定可重算）
	Pattern  string `json:"pattern"`  // 裸文件名 / 目录\* / 精确文件路径
	Kind     string `json:"kind"`     // filename | directory | file
	Label    string `json:"label"`    // 展示名（中文）
	VendorID string `json:"vendorId"` // 来自哪个预设厂商包；空表示用户手工添加
	Preset   bool   `json:"preset"`   // 是否由预设包生成（UI 上按厂商整组增删）
}

// VendorPreset 一个「厂商全家桶」预设包。
//
// 名单来源与可靠性：FileNames 取自这些软件公开的常驻进程名，Directories 取自其默认安装位置。
// 这是启发式清单——厂商可能新增进程名或换目录，因此 UI 同时提供「扫描本机已安装软件」
// 按厂商实际安装路径生成规则，那条路径才是本机的事实依据。
type VendorPreset struct {
	ID          string   `json:"id"`          // 稳定 id（kebab-case）
	Name        string   `json:"name"`        // 展示名，如 "360 全家桶"
	Publisher   string   `json:"publisher"`   // 注册表 Publisher 关键字（用于扫描匹配）
	Description string   `json:"description"` // 包含哪些软件
	FileNames   []string `json:"fileNames"`   // 裸文件名规则
	Directories []string `json:"directories"` // 默认安装目录（不含尾部 \*）
	Applied     bool     `json:"applied"`     // 当前是否已应用（由服务端填充）
}

// BlocklistStatus 拦截名单页面的完整状态。
type BlocklistStatus struct {
	Available       bool   `json:"available"`       // 平台是否支持
	IsAdmin         bool   `json:"isAdmin"`         // 是否管理员
	ProtectionState string `json:"protectionState"` // 复用保护状态：unmanaged|locked|unlocked|attention
	PolicyMode      string `json:"policyMode"`      // whitelist|blacklist（无活动策略时为空）
	Enforcing       bool   `json:"enforcing"`       // 拦截规则当前是否真的在生效（SRP 已激活）
	// CanEnableBlockOnly 无活动策略且具备条件时，可从本页直接启用仅拦截模式。
	CanEnableBlockOnly bool           `json:"canEnableBlockOnly"`
	Rules              []BlockRule    `json:"rules"`     // 当前拦截规则
	Vendors            []VendorPreset `json:"vendors"`   // 预设厂商包（含 Applied 标记）
	RuleCount          int            `json:"ruleCount"` // 拦截规则总数
	Reason             string         `json:"reason"`    // 状态说明（中文）
	CheckedAt          string         `json:"checkedAt"` // RFC3339
}

// BlocklistResult 一次拦截名单变更的结果。
type BlocklistResult struct {
	Changed      bool   `json:"changed"`   // 是否实际写入了变更
	Applied      bool   `json:"applied"`   // 是否已即时生效（保护启用时为 true）
	RuleCount    int    `json:"ruleCount"` // 变更后的拦截规则总数
	AddedCount   int    `json:"addedCount"`
	RemovedCount int    `json:"removedCount"`
	ChangedAt    string `json:"changedAt"` // RFC3339
	Message      string `json:"message"`   // 展示文案（中文）
}

// BlockedVendorScan 「扫描本机」结果里的一条候选：已安装的软件 + 建议的拦截目录。
type BlockedVendorScan struct {
	ID             string `json:"id"`             // 稳定 id
	DisplayName    string `json:"displayName"`    // 软件名
	Publisher      string `json:"publisher"`      // 发行商
	InstallPath    string `json:"installPath"`    // 建议拦截的安装目录（不含 \*）
	MatchedVendor  string `json:"matchedVendor"`  // 命中的预设厂商包 id（空表示未命中，仅供参考）
	Suggested      bool   `json:"suggested"`      // 是否命中已知垃圾软件厂商（UI 默认勾选）
	AlreadyBlocked bool   `json:"alreadyBlocked"` // 该目录是否已在拦截名单内
}

// VendorPresets 返回内置的垃圾软件厂商包清单。
//
// 收录标准：以「静默捆绑安装 / 强制常驻 / 篡改浏览器主页与默认程序 / 弹窗推广」为主要
// 争议点的国产工具软件。同一厂商的正当产品（如 WPS Office、百度网盘、QQ/微信）**不收录**，
// 需要拦截的用户可自行手工添加规则。
func VendorPresets() []VendorPreset {
	return []VendorPreset{
		{
			ID:          "qihoo-360",
			Name:        "360 全家桶",
			Publisher:   "360",
			Description: "360安全卫士、360杀毒、360极速/安全浏览器、360压缩、360驱动大师、360软件管家",
			FileNames: []string{
				// 安全卫士 / 杀毒常驻进程
				"360Safe.exe", "360SafeBox.exe", "360Tray.exe", "SafeBoxTray.exe",
				"ZhuDongFangYu.exe", "360sd.exe", "360rp.exe", "360rps.exe",
				"QHSafeTray.exe", "QHSafeMain.exe", "QHActiveDefense.exe",
				"LiveUpdate360.exe", "360Speedld.exe", "360Base.exe", "360leakfixer.exe",
				// 浏览器
				"360se.exe", "360chrome.exe", "360ChromeX.exe", "360SeUpdate.exe",
				// 压缩 / 驱动 / 软件管家
				"360zip.exe", "360ZipUpdate.exe", "DrvMgr.exe", "360DrvMgr.exe",
				"SoftMgrLite.exe", "SoftManager.exe", "360MoveSetup.exe",
			},
			Directories: []string{
				`C:\Program Files\360`,
				`C:\Program Files (x86)\360`,
				`C:\ProgramData\360safe`,
			},
		},
		{
			ID:          "2345",
			Name:        "2345 全家桶",
			Publisher:   "2345",
			Description: "2345加速浏览器、2345看图王、2345好压、2345安全卫士、2345手机助手",
			FileNames: []string{
				"2345Explorer.exe", "2345ExplorerUpdate.exe", "2345MiniPage.exe",
				"2345Pic.exe", "2345PicViewer.exe", "2345Compress.exe",
				"2345SafeCenter.exe", "2345Svc.exe", "2345Soft.exe",
				"2345MobileAssistant.exe", "2345Update.exe", "HaoZip.exe", "HaoZipUpdate.exe",
			},
			Directories: []string{
				`C:\Program Files\2345Soft`,
				`C:\Program Files (x86)\2345Soft`,
				`C:\Program Files\2345Explorer`,
				`C:\Program Files (x86)\2345Explorer`,
			},
		},
		{
			ID:          "tencent-pcmgr",
			Name:        "腾讯电脑管家",
			Publisher:   "Tencent",
			Description: "腾讯电脑管家及其常驻防护、软件推广组件（不影响 QQ / 微信）",
			FileNames: []string{
				"QQPCMgr.exe", "QQPCTray.exe", "QQPCRTP.exe", "QQPCPatch.exe",
				"QQPCSoftGuardian.exe", "QQPCDownload.exe", "QQPCRealTimeSpeedup.exe",
				"QQPCLeakScan.exe", "QQPCNetFlow.exe", "QMDL.exe",
			},
			Directories: []string{
				`C:\Program Files\Tencent\QQPCMgr`,
				`C:\Program Files (x86)\Tencent\QQPCMgr`,
			},
		},
		{
			ID:          "kingsoft-security",
			Name:        "金山毒霸 / 猎豹",
			Publisher:   "Kingsoft",
			Description: "金山毒霸、金山卫士、猎豹安全浏览器、驱动精灵（不影响 WPS Office）",
			FileNames: []string{
				"kxescore.exe", "kxetray.exe", "kxecenter.exe", "kislive.exe", "KWatch.exe",
				"KSafeTray.exe", "KSafeSvc.exe", "KSafe.exe", "KGameBox.exe",
				"liebao.exe", "liebaotray.exe", "LBUpdate.exe",
				"DrvSetup.exe", "DriverGenius.exe", "DGMain.exe",
			},
			Directories: []string{
				`C:\Program Files\Kingsoft\Kingsoft Antivirus`,
				`C:\Program Files (x86)\Kingsoft\Kingsoft Antivirus`,
				`C:\Program Files\ksafe`,
				`C:\Program Files (x86)\ksafe`,
				`C:\Program Files\liebao`,
				`C:\Program Files (x86)\liebao`,
			},
		},
		{
			ID:          "baidu-junk",
			Name:        "百度全家桶",
			Publisher:   "Baidu",
			Description: "百度杀毒、百度卫士、百度桌面助手、hao123 主页锁定（不影响百度网盘）",
			FileNames: []string{
				"BaiduSd.exe", "BaiduSdSvc.exe", "BaiduSdTray.exe",
				"BaiduAn.exe", "BaiduAnSvc.exe", "BaiduAnTray.exe",
				"BaiduPlayer.exe", "BaiduHi.exe", "BaiduDesktop.exe",
				"hao123.exe", "Hao123Setup.exe", "BaiduPinyinUp.exe",
			},
			Directories: []string{
				`C:\Program Files\Baidu\BaiduSd`,
				`C:\Program Files (x86)\Baidu\BaiduSd`,
				`C:\Program Files\Baidu\BaiduAn`,
				`C:\Program Files (x86)\Baidu\BaiduAn`,
				`C:\Program Files\hao123`,
				`C:\Program Files (x86)\hao123`,
			},
		},
		{
			ID:          "thunder",
			Name:        "迅雷系列",
			Publisher:   "Thunder",
			Description: "迅雷下载、迅雷影音、迅雷游戏加速器及其常驻服务",
			FileNames: []string{
				"Thunder.exe", "ThunderPlatform.exe", "ThunderFW.exe",
				"XLServicePlatform.exe", "XLGameAssistant.exe", "XLLiveUD.exe",
				"DownloadKernel.exe", "ThunderSpeedUp.exe", "XMP.exe", "XLBugReport.exe",
			},
			Directories: []string{
				`C:\Program Files\Thunder Network`,
				`C:\Program Files (x86)\Thunder Network`,
			},
		},
		{
			ID:          "driver-junk",
			Name:        "驱动人生 / 鲁大师",
			Publisher:   "",
			Description: "驱动人生、鲁大师及其游戏盒子、软件推广组件",
			FileNames: []string{
				"DrvLifeCenter.exe", "DriveTheLife.exe", "DTLLiveUpdate.exe", "DTuUpdate.exe",
				"ludashi.exe", "LudashiPromote.exe", "LDSGameMaster.exe", "ldsmain.exe",
				"HardwareDetect.exe", "LDSGameBox.exe",
				// 鲁大师温度监控 / 托盘常驻组件
				"ComputerZ_CN.exe", "ComputerZTray.exe",
			},
			Directories: []string{
				`C:\Program Files\DTLSoft`,
				`C:\Program Files (x86)\DTLSoft`,
				`C:\Program Files\ludashi`,
				`C:\Program Files (x86)\ludashi`,
			},
		},
	}
}

// 拦截名单相关的错误码。
const (
	BlocklistErrInvalidPattern = "BLOCKLIST_INVALID_PATTERN"
	BlocklistErrUnknownVendor  = "BLOCKLIST_UNKNOWN_VENDOR"
	BlocklistErrProtected      = "BLOCKLIST_PROTECTED_PATTERN"
	BlocklistErrDuplicate      = "BLOCKLIST_DUPLICATE"
	BlocklistErrNotFound       = "BLOCKLIST_RULE_NOT_FOUND"
)
