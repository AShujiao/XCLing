package model

// AppVersion 的唯一版本源是仓库根目录的 VERSION 文件：
// 发布构建由 scripts/build-wpf.ps1 通过 -ldflags "-X XCLing/internal/model.AppVersion=x.y.z" 注入；
// 未注入的本地开发构建显示 dev。修改版本号只改 VERSION 文件，不要改这里。
var AppVersion = "dev"
var AppName = "星陈守护"
