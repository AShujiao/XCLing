package release

import "testing"

func TestSHA256Hex_KnownVector(t *testing.T) {
	// "abc" 的 SHA256 是众所周知的测试向量。
	got := SHA256Hex([]byte("abc"))
	want := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got != want {
		t.Errorf("SHA256Hex(abc) = %s，期望 %s", got, want)
	}
}

func TestSHA256Hex_EmptyInput(t *testing.T) {
	got := SHA256Hex(nil)
	want := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got != want {
		t.Errorf("空输入哈希 = %s，期望 %s", got, want)
	}
}

func TestChecksumLine_Format(t *testing.T) {
	got := ChecksumLine("XCLing.exe", "deadbeef")
	want := "deadbeef  XCLing.exe"
	if got != want {
		t.Errorf("ChecksumLine = %q，期望 %q", got, want)
	}
}

func TestBuildSHA256SUMS_DeterministicSorted(t *testing.T) {
	files := map[string][]byte{
		"zeta.exe":  []byte("z"),
		"alpha.exe": []byte("a"),
	}
	out := BuildSHA256SUMS(files)
	// alpha 必须排在 zeta 之前，输出确定。
	alphaLine := ChecksumLine("alpha.exe", SHA256Hex([]byte("a"))) + "\n"
	zetaLine := ChecksumLine("zeta.exe", SHA256Hex([]byte("z"))) + "\n"
	want := alphaLine + zetaLine
	if out != want {
		t.Errorf("BuildSHA256SUMS 输出不确定/未排序：\n得到 %q\n期望 %q", out, want)
	}
	// 再跑一次，确认稳定。
	if BuildSHA256SUMS(files) != out {
		t.Error("BuildSHA256SUMS 结果应稳定可复现")
	}
}
