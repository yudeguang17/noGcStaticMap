// Copyright 2022 rateLimit Author(https://github.com/yudeguang17/noGcStaticMap). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/yudeguang17/noGcStaticMap.
package noGcStaticMap

import (
	"strconv"
	"sync"
	"testing"
)

// TestAny 默认类型的基础读写，与官方map逐键对比
func TestAny(t *testing.T) {
	var mapOffical = make(map[string]string)
	var mapAny = NewDefault(tempName(t))
	//set
	mapAny.SetString("", "empty")
	mapAny.SetString("empty", "")
	for i := 0; i < 10000; i++ {
		mapOffical[strconv.Itoa(i)] = strconv.Itoa(i)
		mapAny.SetString(strconv.Itoa(i), strconv.Itoa(i))
	}
	if mapAny.Len() != 10002 {
		t.Fatalf("unexpected len; got %v want %v", mapAny.Len(), 10002)
	}
	mapAny.SetFinished()
	//get
	for i := 0; i < 10000; i++ {
		valOffical, _ := mapOffical[strconv.Itoa(i)]
		valAny, _ := mapAny.GetString(strconv.Itoa(i))
		valAnyUnsafe, _ := mapAny.GetUnsafe([]byte(strconv.Itoa(i)))
		valAnyStringUnsafe, _ := mapAny.GetStringUnsafe(strconv.Itoa(i))
		if valOffical != valAny {
			t.Fatalf("unexpected value obtained; got %q want %q", valAny, valOffical)
		}
		if valOffical != string(valAnyUnsafe) {
			t.Fatalf("unexpected value obtained; got %q want %q", string(valAnyUnsafe), valOffical)
		}
		if valOffical != valAnyStringUnsafe {
			t.Fatalf("unexpected value obtained; got %q want %q", valAnyStringUnsafe, valOffical)
		}
	}
	//value empty
	valEmpty, exist := mapAny.GetString("empty")
	if !exist {
		t.Fatalf("unexpected value obtained; got %v want %v", exist, true)
	}
	if valEmpty != "" {
		t.Fatalf("unexpected value obtained; got %q want %q", valEmpty, "")
	}
	//key empty
	valOFKeyEmpty, exist := mapAny.GetString("")
	if !exist {
		t.Fatalf("unexpected value obtained; got %v want %v", exist, true)
	}
	if valOFKeyEmpty != "empty" {
		t.Fatalf("unexpected value obtained; got %q want %q", valOFKeyEmpty, "empty")
	}
	//不存在的键
	if _, exist := mapAny.GetString("notExistKey"); exist {
		t.Fatal("unexpected exist; got true want false")
	}
	if _, exist := mapAny.GetUnsafe([]byte("notExistKey")); exist {
		t.Fatal("unexpected exist; got true want false")
	}
}

// TestAnyEmptyKeyAndValue 空键、空值、nil值
func TestAnyEmptyKeyAndValue(t *testing.T) {
	m := NewDefault(tempName(t))
	m.Set(nil, nil)
	m.Set([]byte("k"), nil)
	m.Set([]byte("k2"), []byte(""))
	m.SetFinished()
	if v, exist := m.Get(nil); !exist || len(v) != 0 {
		t.Fatalf("unexpected empty kv; got %q exist=%v", v, exist)
	}
	if v, exist := m.GetUnsafe([]byte("k")); !exist || len(v) != 0 {
		t.Fatalf("unexpected empty value; got %q exist=%v", v, exist)
	}
	if s, exist := m.GetStringUnsafe("k2"); !exist || s != "" {
		t.Fatalf("unexpected empty value; got %q exist=%v", s, exist)
	}
}

// TestAnyDuplicateKey 同一个键写入两次必须立即报错，且不影响计数与已写入的值
func TestAnyDuplicateKey(t *testing.T) {
	m := NewDefault(tempName(t))
	m.SetString("k", "v1")
	assertPanic(t, "for twice", func() {
		m.SetString("k", "v2")
	})
	if m.Len() != 1 {
		t.Fatalf("unexpected len after duplicate set; got %v want %v", m.Len(), 1)
	}
	m.SetFinished()
	if v, exist := m.GetString("k"); !exist || v != "v1" {
		t.Fatalf("unexpected value; got %q exist=%v", v, exist)
	}
	//完成存储后写入，报的是状态错误，而不是重复键错误
	assertPanic(t, "can't Set after SetFinished", func() {
		m.SetString("k", "v3")
	})
}

// TestAnyProbeCollision 人为制造哈希冲突，验证单张map的探测序列能正确处理冲突与重复写入
func TestAnyProbeCollision(t *testing.T) {
	m := NewDefault(tempName(t))
	m.SetString("first", "v1")
	//把 "second" 的0号槽位强行指向 "first" 的记录，制造冲突
	m.index[hashKey([]byte("second"))] = 0
	m.SetString("second", "v2")
	if m.Len() != 2 {
		t.Fatalf("unexpected len; got %v want %v", m.Len(), 2)
	}
	//冲突链上的重复写入必须被检测到
	assertPanic(t, "for twice", func() {
		m.SetString("second", "v2again")
	})
	m.SetFinished()
	if v, exist := m.GetString("first"); !exist || v != "v1" {
		t.Fatalf("unexpected value; got %q exist=%v", v, exist)
	}
	if v, exist := m.GetString("second"); !exist || v != "v2" {
		t.Fatalf("unexpected value; got %q exist=%v", v, exist)
	}
	//冲突槽位上查询不存在的键必须正确返回false
	if _, exist := m.GetString("third"); exist {
		t.Fatal("unexpected exist; got true want false")
	}
}

// TestAnyValueTooLong 值的长度上限校验
func TestAnyValueTooLong(t *testing.T) {
	m := NewDefault(tempName(t))
	maxVal := make([]byte, maxKVLen)
	m.Set([]byte("k"), maxVal)
	if m.Len() != 1 {
		t.Fatalf("unexpected len; got %v want %v", m.Len(), 1)
	}
	assertPanic(t, "too long", func() {
		m.Set([]byte("k2"), make([]byte, maxKVLen+1))
	})
	if m.Len() != 1 {
		t.Fatalf("unexpected len after panic; got %v want %v", m.Len(), 1)
	}
	m.SetFinished()
	if v, exist := m.Get([]byte("k")); !exist || len(v) != maxKVLen {
		t.Fatalf("unexpected value; got len=%v exist=%v", len(v), exist)
	}
}

// TestAnySetAfterFinished SetFinished之后不允许再写入
func TestAnySetAfterFinished(t *testing.T) {
	m := NewDefault(tempName(t))
	m.SetString("k", "v1")
	m.SetFinished()
	assertPanic(t, "can't Set after SetFinished", func() {
		m.SetString("k2", "v2")
	})
	if m.Len() != 1 {
		t.Fatalf("unexpected len; got %v want %v", m.Len(), 1)
	}
}

// TestAnyGetBeforeFinished 未完成存储时查询必须报错
func TestAnyGetBeforeFinished(t *testing.T) {
	m := NewDefault(tempName(t))
	m.SetString("k", "v1")
	assertPanic(t, "can't Get before SetFinished", func() {
		m.GetString("k")
	})
	m.SetFinished()
}

// TestAnyDataBeginPos 通过位置偏移取值的两个接口
func TestAnyDataBeginPos(t *testing.T) {
	m := NewDefault(tempName(t))
	m.SetString("k1", "v1")
	m.SetString("k2", "v2")
	m.SetFinished()
	pos, exist := m.GetDataBeginPosOfKVPair([]byte("k2"))
	if !exist {
		t.Fatal("unexpected exist; got false want true")
	}
	if v := m.GetValFromDataBeginPosOfKVPairUnSafe(int(pos)); string(v) != "v2" {
		t.Fatalf("unexpected value; got %q want %q", string(v), "v2")
	}
	if _, exist := m.GetDataBeginPosOfKVPair([]byte("k3")); exist {
		t.Fatal("unexpected exist; got true want false")
	}
}

// TestAnyConcurrentRead SetFinished之后允许多个协程并发只读
func TestAnyConcurrentRead(t *testing.T) {
	m := NewDefault(tempName(t))
	for i := 0; i < 2000; i++ {
		m.SetString(strconv.Itoa(i), strconv.Itoa(i))
	}
	m.SetFinished()
	var wg sync.WaitGroup
	bad := make(chan string, 16)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 2000; i++ {
				v, exist := m.GetString(strconv.Itoa(i))
				if !exist || v != strconv.Itoa(i) {
					select {
					case bad <- "key " + strconv.Itoa(i):
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
		t.Fatalf("concurrent read got wrong value for %v", k)
	}
}
