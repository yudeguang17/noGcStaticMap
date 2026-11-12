// Copyright 2022 rateLimit Author(https://github.com/yudeguang17/noGcStaticMap). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/yudeguang17/noGcStaticMap.
package noGcStaticMap

import (
	"strings"
	"testing"
)

// TestBytesToStruct 常规解析
func TestBytesToStruct(t *testing.T) {
	cases := []struct {
		data string
		want NoGcStructExample
	}{
		{"1`a`b`c", NoGcStructExample{Col1: 1, Col2: "a", Col3: "b", Col4: "c"}},
		{"-2`中文`x``", NoGcStructExample{Col1: -2, Col2: "中文", Col3: "x", Col4: ""}},
		{"1````", NoGcStructExample{Col1: 1, Col2: "", Col3: "", Col4: ""}},
		{"1`a`b", NoGcStructExample{Col1: 1, Col2: "a", Col3: "b", Col4: ""}},
		{"1`a", NoGcStructExample{Col1: 1, Col2: "a", Col3: "", Col4: ""}},
		{"1", NoGcStructExample{Col1: 1, Col2: "", Col3: "", Col4: ""}},
	}
	for _, c := range cases {
		if got := BytesToStruct([]byte(c.data)); got != c.want {
			t.Fatalf("BytesToStruct(%q) = %+v, want %+v", c.data, got, c.want)
		}
	}
}

// TestBytesToStructTrailingSep 末尾分隔符不允许被算进最后一个字段
func TestBytesToStructTrailingSep(t *testing.T) {
	cases := []struct {
		data string
		want NoGcStructExample
	}{
		{"1`a`b`c`", NoGcStructExample{Col1: 1, Col2: "a", Col3: "b", Col4: "c"}},
		{"1`a`b`", NoGcStructExample{Col1: 1, Col2: "a", Col3: "b", Col4: ""}},
		{"1`a``", NoGcStructExample{Col1: 1, Col2: "a", Col3: "", Col4: ""}},
		{"1`", NoGcStructExample{Col1: 1, Col2: "", Col3: "", Col4: ""}},
	}
	for _, c := range cases {
		got := BytesToStruct([]byte(c.data))
		if got != c.want {
			t.Fatalf("BytesToStruct(%q) = %+v, want %+v", c.data, got, c.want)
		}
	}
}

// TestBytesToStructMultiByteSep 多字节分隔符也要按完整分隔符比较，而不是只看首字节
func TestBytesToStructMultiByteSep(t *testing.T) {
	old := SplitSep
	SplitSep = []byte("||")
	t.Cleanup(func() { SplitSep = old })
	got := BytesToStruct([]byte("1||a||b||c||"))
	want := NoGcStructExample{Col1: 1, Col2: "a", Col3: "b", Col4: "c"}
	if got != want {
		t.Fatalf("unexpected value; got %+v want %+v", got, want)
	}
	//此时单字节的"`"不应再被当作分隔符
	got = BytesToStruct([]byte("1||a`b"))
	want = NoGcStructExample{Col1: 1, Col2: "a`b"}
	if got != want {
		t.Fatalf("unexpected value; got %+v want %+v", got, want)
	}
}

// TestBytesToStructEmptySep 空分隔符必须报错，而不是数组越界
func TestBytesToStructEmptySep(t *testing.T) {
	old := SplitSep
	SplitSep = []byte{}
	t.Cleanup(func() { SplitSep = old })
	assertPanic(t, "SplitSep can't be empty", func() {
		BytesToStruct([]byte("1`a"))
	})
}

// TestBytesToStructBadData 非法输入必须报错
func TestBytesToStructBadData(t *testing.T) {
	assertPanic(t, "data err", func() { BytesToStruct(nil) })
	assertPanic(t, "", func() { BytesToStruct([]byte("`a`b`c")) })    // Col1不是数字
	assertPanic(t, "", func() { BytesToStruct([]byte("1`a`b`c`d")) }) // 字段过多
}

// TestBytesToStructBoundaryLen 记录长度正好落在缓存池边界时不能被误判为超长
func TestBytesToStructBoundaryLen(t *testing.T) {
	const total = 10240000 - 4
	data := make([]byte, 0, total)
	data = append(data, []byte("1`a`b`c")...)
	data = append(data, strings.Repeat("x", total-len(data))...)
	if len(data) != total {
		t.Fatalf("unexpected data len; got %v want %v", len(data), total)
	}
	got := BytesToStruct(data)
	if got.Col1 != 1 || got.Col2 != "a" || got.Col3 != "b" || len(got.Col4) != total-6 {
		t.Fatalf("unexpected value; got Col1=%v Col2=%q Col3=%q len(Col4)=%v", got.Col1, got.Col2, got.Col3, len(got.Col4))
	}
	//超过上限才报错
	tooLong := make([]byte, 10240001)
	tooLong[0] = '1'
	assertPanic(t, "too long", func() { BytesToStruct(tooLong) })
}

// TestSliceToStrRoundTrip 结构体切片序列化后再反序列化必须完全一致
func TestSliceToStrRoundTrip(t *testing.T) {
	ps := []NoGcStructExample{
		{Col1: 1, Col2: "a", Col3: "b", Col4: "c"},
		{Col1: -2, Col2: "", Col3: "中文", Col4: ""},
		{Col1: 0, Col2: "x", Col3: "", Col4: "end"},
		{Col1: 999, Col2: "", Col3: "", Col4: ""},
	}
	s := SliceToStr(ps)
	back := BytesToSlice([]byte(s))
	if len(back) != len(ps) {
		t.Fatalf("unexpected len; got %v want %v", len(back), len(ps))
	}
	for i := range ps {
		if back[i] != ps[i] {
			t.Fatalf("unexpected value at %v; got %+v want %+v", i, back[i], ps[i])
		}
	}
	//空切片
	if s := SliceToStr(nil); s != "" {
		t.Fatalf("unexpected value; got %q want empty", s)
	}
	if ps := BytesToSlice(nil); len(ps) != 0 {
		t.Fatalf("unexpected len; got %v want 0", len(ps))
	}
}

// TestBytesToSliceCorrupt 数据损坏时必须明确报错，而不是悄悄丢数据或结束进程
func TestBytesToSliceCorrupt(t *testing.T) {
	//长度前缀不完整
	assertPanic(t, "truncated length prefix", func() { BytesToSlice([]byte{1, 2}) })
	//声明的长度超过实际数据
	assertPanic(t, "invalid record length", func() { BytesToSlice([]byte{0xff, 0xff, 0x00, 0x00, 'a'}) })
}

// TestStructToStrCheckSep 字段中出现分隔符时必须报错
func TestStructToStrCheckSep(t *testing.T) {
	assertPanic(t, "can't contain the SplitSep", func() {
		StructToStr(NoGcStructExample{Col1: 1, Col2: "a`b"})
	})
}

// TestJoinInterface 拼接函数
func TestJoinInterface(t *testing.T) {
	if got := JoinInterface("`", 1, "a", "b", "c"); got != "1`a`b`c" {
		t.Fatalf("unexpected value; got %q", got)
	}
	if got := JoinInterface("`"); got != "" {
		t.Fatalf("unexpected value; got %q", got)
	}
	if got := JoinInterface("`", 1); got != "1" {
		t.Fatalf("unexpected value; got %q", got)
	}
}

// TestUnsafeConvert 零拷贝转换
func TestUnsafeConvert(t *testing.T) {
	if got := UnsafeBytesToString([]byte("abc")); got != "abc" {
		t.Fatalf("unexpected value; got %q", got)
	}
	if got := UnsafeBytesToString(nil); got != "" {
		t.Fatalf("unexpected value; got %q", got)
	}
	if got := string(UnsafeStringToBytes("abc")); got != "abc" {
		t.Fatalf("unexpected value; got %q", got)
	}
	if got := string(UnsafeStringToBytes("")); got != "" {
		t.Fatalf("unexpected value; got %q", got)
	}
}

// TestUint32ToByte 长度前缀必须是uint32小端序
func TestUint32ToByte(t *testing.T) {
	got := uint32ToByte(0x01020304)
	want := []byte{0x04, 0x03, 0x02, 0x01}
	if len(got) != 4 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] || got[3] != want[3] {
		t.Fatalf("unexpected value; got %v want %v", got, want)
	}
}
