// Copyright 2022 rateLimit Author(https://github.com/yudeguang17/noGcStaticMap). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/yudeguang17/noGcStaticMap.
package noGcStaticMap

import (
	"strconv"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

// TestBidirect 双向索引的基础读功能，与官方双map逐条对比
func TestBidirect(t *testing.T) {
	const n = 10000
	idToStr := make(map[uint32]string, n)
	strToID := make(map[string]uint32, n)
	m := NewBidirect(tempName(t))
	for i := 0; i < n; i++ {
		id := uint32(i)
		str := "value-" + strconv.Itoa(i)
		idToStr[id] = str
		strToID[str] = id
		m.Set(id, str)
	}
	if m.Len() != n {
		t.Fatalf("unexpected len; got %v want %v", m.Len(), n)
	}
	m.SetFinished()
	for i := 0; i < n; i++ {
		id := uint32(i)
		wantStr := idToStr[id]
		if got, exist := m.GetValue(id); !exist || got != wantStr {
			t.Fatalf("GetValue(%v) = %q,%v want %q,true", id, got, exist, wantStr)
		}
		if got, exist := m.GetValueUnsafe(id); !exist || got != wantStr {
			t.Fatalf("GetValueUnsafe(%v) = %q,%v want %q,true", id, got, exist, wantStr)
		}
		if got, exist := m.GetID(wantStr); !exist || got != id {
			t.Fatalf("GetID(%q) = %v,%v want %v,true", wantStr, got, exist, id)
		}
	}
	//不存在的id与字符串
	if _, exist := m.GetValue(uint32(n + 1)); exist {
		t.Fatal("unexpected exist; got true want false")
	}
	if _, exist := m.GetID("not-exist-value"); exist {
		t.Fatal("unexpected exist; got true want false")
	}
}

// TestBidirectDuplicateID 重复id必须立即报错，且不影响计数与已写入的数据
func TestBidirectDuplicateID(t *testing.T) {
	m := NewBidirect(tempName(t))
	m.Set(1, "a")
	assertPanic(t, "for twice", func() {
		m.Set(1, "b")
	})
	if m.Len() != 1 {
		t.Fatalf("unexpected len after duplicate set; got %v want 1", m.Len())
	}
	//报错之后仍然可以继续写入其它id
	m.Set(2, "b")
	m.SetFinished()
	if v, exist := m.GetValue(1); !exist || v != "a" {
		t.Fatalf("unexpected value; got %q exist=%v", v, exist)
	}
	if id, exist := m.GetID("b"); !exist || id != 2 {
		t.Fatalf("unexpected id; got %v exist=%v", id, exist)
	}
}

// TestBidirectDuplicateStr 重复字符串必须立即报错
func TestBidirectDuplicateStr(t *testing.T) {
	m := NewBidirect(tempName(t))
	m.Set(1, "same")
	assertPanic(t, "for twice", func() {
		m.Set(2, "same")
	})
	if m.Len() != 1 {
		t.Fatalf("unexpected len after duplicate set; got %v want 1", m.Len())
	}
	//被拒绝的id不能留下脏索引
	m.Set(2, "other")
	m.SetFinished()
	if _, exist := m.GetValue(2); !exist {
		t.Fatal("unexpected exist; got false want true")
	}
	if id, exist := m.GetID("same"); !exist || id != 1 {
		t.Fatalf("unexpected id; got %v exist=%v", id, exist)
	}
}

// TestBidirectEmptyStr 空字符串也要能双向查到
func TestBidirectEmptyStr(t *testing.T) {
	m := NewBidirect(tempName(t))
	m.Set(0, "")
	m.Set(1, "x")
	m.SetFinished()
	if v, exist := m.GetValue(0); !exist || v != "" {
		t.Fatalf("unexpected value; got %q exist=%v", v, exist)
	}
	if v, exist := m.GetValueUnsafe(0); !exist || v != "" {
		t.Fatalf("unexpected unsafe value; got %q exist=%v", v, exist)
	}
	if id, exist := m.GetID(""); !exist || id != 0 {
		t.Fatalf("unexpected id; got %v exist=%v", id, exist)
	}
}

// TestBidirectBoundaryID id的边界值
func TestBidirectBoundaryID(t *testing.T) {
	m := NewBidirect(tempName(t))
	m.Set(0, "zero")
	m.Set(^uint32(0), "max")
	m.SetFinished()
	if v, exist := m.GetValue(0); !exist || v != "zero" {
		t.Fatalf("unexpected value; got %q exist=%v", v, exist)
	}
	if v, exist := m.GetValue(^uint32(0)); !exist || v != "max" {
		t.Fatalf("unexpected value; got %q exist=%v", v, exist)
	}
	if id, exist := m.GetID("max"); !exist || id != ^uint32(0) {
		t.Fatalf("unexpected id; got %v exist=%v", id, exist)
	}
}

// TestBidirectStrLenLimit 字符串长度上限校验
func TestBidirectStrLenLimit(t *testing.T) {
	m := NewBidirect(tempName(t))
	long := make([]byte, maxKVLen)
	for i := range long {
		long[i] = 'x'
	}
	m.Set(1, string(long))
	if m.Len() != 1 {
		t.Fatalf("unexpected len; got %v want 1", m.Len())
	}
	assertPanic(t, "too long", func() {
		m.Set(2, string(long)+"x")
	})
	if m.Len() != 1 {
		t.Fatalf("unexpected len after panic; got %v want 1", m.Len())
	}
	m.SetFinished()
	if v, exist := m.GetValue(1); !exist || len(v) != maxKVLen {
		t.Fatalf("unexpected value; got len=%v exist=%v", len(v), exist)
	}
}

// TestBidirectStateMachine SetFinished前后的状态约束
func TestBidirectStateMachine(t *testing.T) {
	m := NewBidirect(tempName(t))
	m.Set(1, "a")
	assertPanic(t, "can't Get before SetFinished", func() { m.GetID("a") })
	assertPanic(t, "can't Get before SetFinished", func() { m.GetValue(1) })
	m.SetFinished()
	m.SetFinished() //幂等
	assertPanic(t, "can't Set after SetFinished", func() { m.Set(2, "b") })
	if m.Len() != 1 {
		t.Fatalf("unexpected len; got %v want 1", m.Len())
	}
	if v, exist := m.GetValue(1); !exist || v != "a" {
		t.Fatalf("unexpected value; got %q exist=%v", v, exist)
	}
}

// TestBidirectProbeCollision 人为制造字符串哈希冲突，验证单表探测与重复检测
func TestBidirectProbeCollision(t *testing.T) {
	m := NewBidirect(tempName(t))
	m.Set(1, "first")
	//把"second"的0号槽位强行指向第一条记录，制造冲突
	m.indexByStr[hashKey([]byte("second"))] = 0
	m.Set(2, "second")
	if m.Len() != 2 {
		t.Fatalf("unexpected len; got %v want 2", m.Len())
	}
	//冲突链上的重复字符串必须被检测到
	assertPanic(t, "for twice", func() {
		m.Set(3, "second")
	})
	m.SetFinished()
	if id, exist := m.GetID("first"); !exist || id != 1 {
		t.Fatalf("unexpected id; got %v exist=%v", id, exist)
	}
	if id, exist := m.GetID("second"); !exist || id != 2 {
		t.Fatalf("unexpected id; got %v exist=%v", id, exist)
	}
	if _, exist := m.GetID("third"); exist {
		t.Fatal("unexpected exist; got true want false")
	}
}

// TestBidirectSetBytes 以[]byte插入
func TestBidirectSetBytes(t *testing.T) {
	m := NewBidirect(tempName(t))
	m.SetBytes(7, []byte("seven"))
	m.SetFinished()
	if v, exist := m.GetValue(7); !exist || v != "seven" {
		t.Fatalf("unexpected value; got %q exist=%v", v, exist)
	}
	if id, exist := m.GetID("seven"); !exist || id != 7 {
		t.Fatalf("unexpected id; got %v exist=%v", id, exist)
	}
}

// TestBidirectConcurrentRead SetFinished之后允许并发双向只读
func TestBidirectConcurrentRead(t *testing.T) {
	const n = 2000
	m := NewBidirect(tempName(t))
	for i := 0; i < n; i++ {
		m.Set(uint32(i), "v"+strconv.Itoa(i))
	}
	m.SetFinished()
	var wg sync.WaitGroup
	bad := make(chan string, 16)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < n; i++ {
				id := uint32(i)
				want := "v" + strconv.Itoa(i)
				if v, exist := m.GetValueUnsafe(id); !exist || v != want {
					select {
					case bad <- "GetValueUnsafe " + strconv.Itoa(i):
					default:
					}
					return
				}
				if got, exist := m.GetID(want); !exist || got != id {
					select {
					case bad <- "GetID " + want:
					default:
					}
					return
				}
			}
		}()
	}
	wg.Wait()
	close(bad)
	for k := range bad {
		t.Fatalf("concurrent read got wrong result for %v", k)
	}
}

// TestBidirectSameTempName 同名临时文件必须明确报错，且不能删掉对方的文件
func TestBidirectSameTempName(t *testing.T) {
	name := tempName(t)
	m := NewBidirect(name)
	defer m.SetFinished()
	assertPanic(t, "can't create temp file", func() { NewBidirect(name) })
	if !fileExist(name + tempFileSuffix) {
		t.Fatal("the temp file of the first map was deleted")
	}
}

// TestBidirectSetBytesEquivalence Set 与 SetBytes 必须得到完全一致的结果
func TestBidirectSetBytesEquivalence(t *testing.T) {
	byString := NewBidirect(tempName(t))
	byBytes := NewBidirect(tempName(t))
	for i := 0; i < 500; i++ {
		str := "value-" + strconv.Itoa(i)
		byString.Set(uint32(i), str)
		byBytes.SetBytes(uint32(i), []byte(str))
	}
	//用SetBytes再写同一个键也必须panic（此时两个map都还没SetFinished）
	assertPanic(t, "for twice", func() {
		byBytes.SetBytes(0, []byte("value-0"))
	})
	byString.SetFinished()
	byBytes.SetFinished()
	if byString.Len() != byBytes.Len() {
		t.Fatalf("unexpected len; got %v want %v", byBytes.Len(), byString.Len())
	}
	for i := 0; i < 500; i++ {
		want, _ := byString.GetValue(uint32(i))
		got, exist := byBytes.GetValue(uint32(i))
		if !exist || got != want {
			t.Fatalf("id %v: got %q exist=%v want %q", i, got, exist, want)
		}
	}
}

// TestBidirectDuplicateStrPanicMessage 重复字符串的panic消息必须被截断并标注总长度
func TestBidirectDuplicateStrPanicMessage(t *testing.T) {
	m := NewBidirect(tempName(t))
	long := make([]byte, 200)
	for i := range long {
		long[i] = 'x'
	}
	m.SetBytes(1, long)
	msg := ""
	func() {
		defer func() {
			if r := recover(); r != nil {
				msg = r.(string)
			}
		}()
		m.SetBytes(2, long)
	}()
	if msg == "" {
		t.Fatal("want panic, got none")
	}
	if len(msg) > 160 {
		t.Fatalf("panic message too long: %v bytes", len(msg))
	}
	if !strings.Contains(msg, "total 200 bytes") {
		t.Fatalf("panic message should carry the total length; got %q", msg)
	}
	if strings.Count(msg, "x") > 64 {
		t.Fatalf("panic message should be truncated; got %q", msg)
	}
	m.SetFinished()
}

// TestBidirectDuplicateStrPanicMessageRuneSafe 截断不能把多字节字符切成半个
func TestBidirectDuplicateStrPanicMessageRuneSafe(t *testing.T) {
	m := NewBidirect(tempName(t))
	long := strings.Repeat("中", 100) //300字节，截断点会落在rune中间
	m.SetBytes(1, []byte(long))
	msg := ""
	func() {
		defer func() {
			if r := recover(); r != nil {
				msg = r.(string)
			}
		}()
		m.SetBytes(2, []byte(long))
	}()
	if msg == "" {
		t.Fatal("want panic, got none")
	}
	if !utf8.ValidString(msg) {
		t.Fatalf("panic message is not valid utf8: %q", msg)
	}
	if !strings.Contains(msg, "total 300 bytes") {
		t.Fatalf("panic message should carry the total length; got %q", msg)
	}
	m.SetFinished()
}
