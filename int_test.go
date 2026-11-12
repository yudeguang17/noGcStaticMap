// Copyright 2022 rateLimit Author(https://github.com/yudeguang17/noGcStaticMap). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/yudeguang17/noGcStaticMap.
package noGcStaticMap

import (
	"strconv"
	"testing"
)

// TestInt 键为int的类型，与官方map逐键对比
func TestInt(t *testing.T) {
	var mapOffical = make(map[string]string)
	var mapAny = NewInt(tempName(t))
	//set
	for i := 0; i < 10000; i++ {
		mapOffical[strconv.Itoa(i)] = strconv.Itoa(i)
		mapAny.SetString(i, strconv.Itoa(i))
	}
	mapAny.SetFinished()
	//get
	for i := 0; i < 10000; i++ {
		valOffical, _ := mapOffical[strconv.Itoa(i)]
		valAny, _ := mapAny.GetString(i)
		valAnyUnsafe, _ := mapAny.GetUnsafe(i)
		valAnyStringUnsafe, _ := mapAny.GetStringUnsafe(i)
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
	if mapAny.Len() != 10000 {
		t.Fatalf("unexpected len; got %v want %v", mapAny.Len(), 10000)
	}
	//不存在的键
	if _, exist := mapAny.Get(-1); exist {
		t.Fatal("unexpected exist; got true want false")
	}
}

// TestIntDuplicateKey 重复键必须立即报错
func TestIntDuplicateKey(t *testing.T) {
	m := NewInt(tempName(t))
	m.SetString(1, "v1")
	assertPanic(t, "for twice", func() {
		m.SetString(1, "v2")
	})
	if m.Len() != 1 {
		t.Fatalf("unexpected len after duplicate set; got %v want %v", m.Len(), 1)
	}
	m.SetFinished()
	if v, exist := m.GetString(1); !exist || v != "v1" {
		t.Fatalf("unexpected value; got %q exist=%v", v, exist)
	}
}

// TestIntEmptyValue 空值也要能区分"存在"与"不存在"
func TestIntEmptyValue(t *testing.T) {
	m := NewInt(tempName(t))
	m.Set(1, nil)
	m.Set(2, []byte(""))
	m.SetFinished()
	if v, exist := m.Get(1); !exist || len(v) != 0 {
		t.Fatalf("unexpected empty value; got %q exist=%v", v, exist)
	}
	if v, exist := m.GetUnsafe(1); !exist || len(v) != 0 {
		t.Fatalf("unexpected empty value; got %q exist=%v", v, exist)
	}
	if _, exist := m.Get(3); exist {
		t.Fatal("unexpected exist; got true want false")
	}
}

// TestIntValueTooLong 值的长度上限校验
func TestIntValueTooLong(t *testing.T) {
	m := NewInt(tempName(t))
	m.Set(1, make([]byte, maxKVLen))
	assertPanic(t, "too long", func() {
		m.Set(2, make([]byte, maxKVLen+1))
	})
	if m.Len() != 1 {
		t.Fatalf("unexpected len after panic; got %v want %v", m.Len(), 1)
	}
	m.SetFinished()
	if v, exist := m.Get(1); !exist || len(v) != maxKVLen {
		t.Fatalf("unexpected value; got len=%v exist=%v", len(v), exist)
	}
}

// TestIntSetAfterFinished SetFinished之后不允许再写入
func TestIntSetAfterFinished(t *testing.T) {
	m := NewInt(tempName(t))
	m.SetString(1, "v1")
	m.SetFinished()
	assertPanic(t, "can't Set after SetFinished", func() {
		m.SetString(2, "v2")
	})
	if m.Len() != 1 {
		t.Fatalf("unexpected len; got %v want %v", m.Len(), 1)
	}
}

// TestIntDataBeginPos 通过位置偏移取值的两个接口
func TestIntDataBeginPos(t *testing.T) {
	m := NewInt(tempName(t))
	m.SetString(1, "v1")
	m.SetString(2, "")
	m.SetFinished()
	pos, exist := m.GetDataBeginPosOfKVPair(1)
	if !exist {
		t.Fatal("unexpected exist; got false want true")
	}
	if v := m.GetValFromDataBeginPosOfKVPairUnSafe(int(pos)); string(v) != "v1" {
		t.Fatalf("unexpected value; got %q want %q", string(v), "v1")
	}
	posEmpty, exist := m.GetDataBeginPosOfKVPair(2)
	if !exist {
		t.Fatal("unexpected exist for empty value; got false want true")
	}
	if v := m.GetValFromDataBeginPosOfKVPairUnSafe(int(posEmpty)); len(v) != 0 {
		t.Fatalf("unexpected value; got %q want empty", string(v))
	}
	if _, exist := m.GetDataBeginPosOfKVPair(3); exist {
		t.Fatal("unexpected exist; got true want false")
	}
}
