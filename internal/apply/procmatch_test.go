package apply

import "testing"

func TestMatchesAnyBlockPattern(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		patterns []string
		want     bool
	}{
		{"裸文件名命中任意目录", `D:\ludashi\ludashi.exe`, []string{"ludashi.exe"}, true},
		{"裸文件名大小写不敏感", `C:\Foo\LUDASHI.EXE`, []string{"ludashi.exe"}, true},
		{"裸文件名不命中不同名", `D:\ludashi\other.exe`, []string{"ludashi.exe"}, false},
		{"目录模式命中直接子文件", `D:\ludashi\ludashi.exe`, []string{`D:\ludashi\*`}, true},
		{"目录模式命中深层子目录", `D:\ludashi\bin\tray\ComputerZTray.exe`, []string{`D:\ludashi\*`}, true},
		{"目录模式不命中同前缀目录", `D:\ludashi2\app.exe`, []string{`D:\ludashi\*`}, false},
		{"目录模式大小写不敏感", `d:\LuDaShi\app.exe`, []string{`D:\ludashi\*`}, true},
		{"精确文件命中", `D:\tools\bad.exe`, []string{`D:\tools\bad.exe`}, true},
		{"精确文件不命中子目录", `D:\tools\bad.exe\x.exe`, []string{`D:\tools\bad.exe`}, false},
		{"正斜杠归一化", `D:/ludashi/ludashi.exe`, []string{`D:\ludashi\*`}, true},
		{"空模式跳过", `D:\x\a.exe`, []string{"", "  "}, false},
		{"空路径不命中", ``, []string{`D:\ludashi\*`}, false},
		{"多模式任一命中", `D:\ludashi\a.exe`, []string{"none.exe", `D:\ludashi\*`}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MatchesAnyBlockPattern(tc.path, tc.patterns); got != tc.want {
				t.Fatalf("MatchesAnyBlockPattern(%q, %v) = %v, want %v", tc.path, tc.patterns, got, tc.want)
			}
		})
	}
}
