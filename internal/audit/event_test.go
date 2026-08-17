package audit

import (
	"strings"
	"testing"

	"XCLing/internal/model"
)

const srpEventXML = `<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'>
 <System>
  <Provider Name='Microsoft-Windows-SoftwareRestrictionPolicies'/>
  <EventID>866</EventID>
  <Level>3</Level>
  <TimeCreated SystemTime='2026-07-10T08:15:30.1234567Z'/>
  <Channel>Application</Channel>
  <EventRecordID>12345</EventRecordID>
  <Security UserID='S-1-5-21-1111-2222-3333-1001'/>
 </System>
 <EventData>
  <Data Name='FilePath'>C:\Users\demo\Downloads\portable.exe</Data>
 </EventData>
 <RenderingInfo Culture='zh-CN'>
  <Message>Access to C:\Users\demo\Downloads\portable.exe has been restricted by your Administrator by location with policy rule.</Message>
 </RenderingInfo>
</Event>`

const srpEventXML2 = `<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'>
 <System>
  <Provider Name='Microsoft-Windows-SoftwareRestrictionPolicies'/>
  <EventID>866</EventID>
  <Level>3</Level>
  <TimeCreated SystemTime='2026-07-11T09:00:00.0000000Z'/>
  <Channel>Application</Channel>
  <EventRecordID>12346</EventRecordID>
 </System>
 <EventData>
  <Data Name='FilePath'>C:\Program Files\Acme\acme.exe</Data>
 </EventData>
 <RenderingInfo Culture='zh-CN'>
  <Message>Access to C:\Program Files\Acme\acme.exe was checked.</Message>
 </RenderingInfo>
</Event>`

func srpSource() SourceSpec {
	return SourceSpec{Channel: "Application", ProviderName: "Microsoft-Windows-SoftwareRestrictionPolicies", Label: "SRP"}
}

func TestParseEvents_FieldsAndMulti(t *testing.T) {
	events := ParseEvents(srpEventXML+srpEventXML2, srpSource())
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	e := events[0]
	if e.Provider != "Microsoft-Windows-SoftwareRestrictionPolicies" {
		t.Errorf("provider mismatch: %q", e.Provider)
	}
	if e.EventID != 866 {
		t.Errorf("eventID mismatch: %d", e.EventID)
	}
	if e.Level != "warning" {
		t.Errorf("level decode mismatch: %q", e.Level)
	}
	if e.Channel != "Application" {
		t.Errorf("channel mismatch: %q", e.Channel)
	}
	if e.ExecutablePath != `C:\Users\demo\Downloads\portable.exe` {
		t.Errorf("exe path extract mismatch: %q", e.ExecutablePath)
	}
	if e.Risk != model.RiskHigh {
		t.Errorf("downloads path must be high risk, got %q", e.Risk)
	}
	if !strings.HasPrefix(e.Timestamp, "2026-07-10T08:15:30") {
		t.Errorf("timestamp normalize mismatch: %q", e.Timestamp)
	}
	if e.ID == "" {
		t.Error("event must have stable id")
	}
	if events[1].Risk != model.RiskLow {
		t.Errorf("program files path should be low risk, got %q", events[1].Risk)
	}
}

func TestParseEvents_EmptyAndMalformed(t *testing.T) {
	if got := ParseEvents("", srpSource()); got != nil {
		t.Fatalf("empty input should return nil, got %v", got)
	}
	// 完全非法的 XML：不 panic，尽力返回（可能为空）。
	_ = ParseEvents("<not-xml garbage <<<", srpSource())
}

func TestParseEvents_StripsXMLDeclaration(t *testing.T) {
	withDecl := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>" + srpEventXML
	events := ParseEvents(withDecl, srpSource())
	if len(events) != 1 {
		t.Fatalf("expected 1 event after stripping declaration, got %d", len(events))
	}
}

func TestExtractExecutablePath(t *testing.T) {
	cases := []struct{ in, want string }{
		{`Access to C:\Users\a\Downloads\x.exe has been restricted`, `C:\Users\a\Downloads\x.exe`},
		{`"C:\Program Files\App\tool.dll" blocked`, `C:\Program Files\App\tool.dll`},
		{`no path here`, ``},
		{`script D:\scripts\run.ps1 was denied`, `D:\scripts\run.ps1`},
	}
	for _, c := range cases {
		got := ExtractExecutablePath(c.in)
		if got != c.want {
			t.Errorf("ExtractExecutablePath(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestSanitizePath(t *testing.T) {
	if got := SanitizePath(`  "C:\a\b.exe"  `); got != `C:\a\b.exe` {
		t.Errorf("quote/space strip failed: %q", got)
	}
	if got := SanitizePath("C:\\a\tb.exe"); strings.Contains(got, "\t") {
		t.Errorf("control char must be stripped: %q", got)
	}
}

func TestClassifyRisk(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{`C:\Users\demo\Downloads\x.exe`, model.RiskHigh},
		{`C:\Users\demo\AppData\Local\Temp\x.exe`, model.RiskHigh},
		{`C:\Users\demo\AppData\Roaming\app\x.exe`, model.RiskHigh},
		{`C:\Windows\System32\notepad.exe`, model.RiskLow},
		{`C:\Program Files\Acme\acme.exe`, model.RiskLow},
		{`C:\Users\demo\tool.exe`, model.RiskMedium},
		{`D:\misc\thing.exe`, model.RiskMedium},
		{``, model.RiskUnknown},
	}
	for _, c := range cases {
		got, reasons := ClassifyRisk(c.path)
		if got != c.want {
			t.Errorf("ClassifyRisk(%q)=%q want %q", c.path, got, c.want)
		}
		if len(reasons) == 0 {
			t.Errorf("ClassifyRisk(%q) must give a reason", c.path)
		}
	}
}

func TestValidateFilter(t *testing.T) {
	// 缺省窗口 → 24h；max 0 → 默认；max 越界 → 夹取。
	f := model.AuditFilter{}
	ValidateFilter(&f)
	if f.Window != model.AuditWindow24h {
		t.Errorf("default window should be 24h, got %q", f.Window)
	}
	if f.Max != model.DefaultAuditRecords {
		t.Errorf("default max should be %d, got %d", model.DefaultAuditRecords, f.Max)
	}

	f2 := model.AuditFilter{Window: "bogus", Max: 9999, Channel: "Fake/Channel"}
	ValidateFilter(&f2)
	if f2.Window != model.AuditWindow24h {
		t.Errorf("bogus window should normalize to 24h")
	}
	if f2.Max != model.MaxAuditRecords {
		t.Errorf("max should clamp to %d, got %d", model.MaxAuditRecords, f2.Max)
	}
	if f2.Channel != "" {
		t.Errorf("unknown channel must be cleared, got %q", f2.Channel)
	}

	// 关键词超长截断。
	long := strings.Repeat("x", model.MaxAuditKeywordLen+50)
	f3 := model.AuditFilter{Keyword: "  " + long + "  "}
	ValidateFilter(&f3)
	if len(f3.Keyword) != model.MaxAuditKeywordLen {
		t.Errorf("keyword should truncate to %d, got %d", model.MaxAuditKeywordLen, len(f3.Keyword))
	}

	// 合法通道保留。
	f4 := model.AuditFilter{Channel: "Application"}
	ValidateFilter(&f4)
	if f4.Channel != "Application" {
		t.Errorf("known channel should be kept, got %q", f4.Channel)
	}
}

func TestFilterAndSort(t *testing.T) {
	events := ParseEvents(srpEventXML+srpEventXML2, srpSource())
	// 关键词只命中 Downloads 事件。
	filtered, truncated := FilterAndSort(events, "downloads", 100)
	if len(filtered) != 1 {
		t.Fatalf("keyword filter expected 1, got %d", len(filtered))
	}
	if truncated {
		t.Error("should not be truncated")
	}

	// 排序：时间倒序（07-11 在前）。
	all, _ := FilterAndSort(events, "", 100)
	if !strings.HasPrefix(all[0].Timestamp, "2026-07-11") {
		t.Errorf("expected newest first, got %q", all[0].Timestamp)
	}

	// 截断。
	capped, truncated2 := FilterAndSort(events, "", 1)
	if len(capped) != 1 || !truncated2 {
		t.Errorf("expected truncation to 1, got %d truncated=%v", len(capped), truncated2)
	}
}

func TestKnownChannels(t *testing.T) {
	chs := KnownChannels()
	if len(chs) == 0 {
		t.Fatal("must have known channels")
	}
	if !IsKnownChannel("Application") {
		t.Error("Application must be a known channel")
	}
	if IsKnownChannel("Totally/Made/Up") {
		t.Error("unknown channel must not be recognized")
	}
}

func TestWindowMillis(t *testing.T) {
	if WindowMillis(model.AuditWindow24h) != 24*60*60*1000 {
		t.Error("24h millis wrong")
	}
	if WindowMillis(model.AuditWindow7d) != 7*24*60*60*1000 {
		t.Error("7d millis wrong")
	}
	if WindowMillis("bogus") != 24*60*60*1000 {
		t.Error("bogus window should default to 24h millis")
	}
}
