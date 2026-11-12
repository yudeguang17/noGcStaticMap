// Copyright 2022 rateLimit Author(https://github.com/yudeguang17/noGcStaticMap). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/yudeguang17/noGcStaticMap.
package noGcStaticMap

import (
	"path/filepath"
	"strconv"
	"testing"
)

func BenchmarkHashKey(b *testing.B) {
	k := []byte("1000000")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hashKey(k)
	}
}

func BenchmarkSetInt(b *testing.B) {
	m := NewInt(filepath.Join(b.TempDir(), "benchSet"))
	v := []byte("value")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Set(i, v)
	}
	b.StopTimer()
	m.SetFinished()
}

func BenchmarkSetAny(b *testing.B) {
	m := NewDefault(filepath.Join(b.TempDir(), "benchSetAny"))
	v := []byte("value")
	buf := make([]byte, 0, 20)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := strconv.AppendInt(buf[:0], int64(i), 10)
		m.Set(k, v)
	}
	b.StopTimer()
	m.SetFinished()
}

func prepareIntMap(b *testing.B, n int) *NoGcStaticMapInt {
	b.Helper()
	m := NewInt(filepath.Join(b.TempDir(), "benchGet"))
	for i := 0; i < n; i++ {
		m.SetString(i, strconv.Itoa(i))
	}
	m.SetFinished()
	return m
}

func BenchmarkGetInt(b *testing.B) {
	const n = 10000
	m := prepareIntMap(b, n)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, exist := m.Get(i % n); !exist {
			b.Fatalf("key %v not found", i%n)
		}
	}
}

func BenchmarkGetUnsafeInt(b *testing.B) {
	const n = 10000
	m := prepareIntMap(b, n)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, exist := m.GetUnsafe(i % n); !exist {
			b.Fatalf("key %v not found", i%n)
		}
	}
}

func BenchmarkGetStringInt(b *testing.B) {
	const n = 10000
	m := prepareIntMap(b, n)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, exist := m.GetString(i % n); !exist {
			b.Fatalf("key %v not found", i%n)
		}
	}
}

func BenchmarkBytesToStruct(b *testing.B) {
	data := []byte("1000000`abc`def`ghi")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BytesToStruct(data)
	}
}

func BenchmarkSliceToStrRoundTrip(b *testing.B) {
	ps := make([]NoGcStructExample, 100)
	for i := range ps {
		ps[i] = NoGcStructExample{Col1: i, Col2: "a", Col3: "b", Col4: "c"}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BytesToSlice([]byte(SliceToStr(ps)))
	}
}

func BenchmarkGetUnsafeAny1KB(b *testing.B) {
	m := NewDefault(filepath.Join(b.TempDir(), "benchGetUnsafeAny"))
	value := make([]byte, 1024)
	key := []byte("k")
	m.Set(key, value)
	m.SetFinished()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, exist := m.GetUnsafe(key); !exist {
			b.Fatal("key not found")
		}
	}
}

func BenchmarkGetDataBeginPosAny1KB(b *testing.B) {
	m := NewDefault(filepath.Join(b.TempDir(), "benchGetPosAny"))
	value := make([]byte, 1024)
	key := []byte("k")
	m.Set(key, value)
	m.SetFinished()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, exist := m.GetDataBeginPosOfKVPair(key); !exist {
			b.Fatal("key not found")
		}
	}
}

func BenchmarkSetBidirect(b *testing.B) {
	m := NewBidirect(filepath.Join(b.TempDir(), "benchSetBidirect"))
	buf := make([]byte, 0, 20)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := string(strconv.AppendInt(buf[:0], int64(i), 10))
		m.Set(uint32(i), s)
	}
	b.StopTimer()
	m.SetFinished()
}

func prepareBidirectMap(b *testing.B, n int) *NoGcStaticMapBidirect {
	b.Helper()
	m := NewBidirect(filepath.Join(b.TempDir(), "benchGetBidirect"))
	for i := 0; i < n; i++ {
		m.Set(uint32(i), "v"+strconv.Itoa(i))
	}
	m.SetFinished()
	return m
}

func BenchmarkGetIDBidirect(b *testing.B) {
	const n = 10000
	m := prepareBidirectMap(b, n)
	keys := make([]string, n)
	for i := range keys {
		keys[i] = "v" + strconv.Itoa(i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, exist := m.GetID(keys[i%n]); !exist {
			b.Fatalf("str %v not found", keys[i%n])
		}
	}
}

// benchSinkValue 把取到的字符串存到全局变量，避免编译器把构建过程优化掉，
// 这样测量到的分配才是调用方真正要付的代价
var benchSinkValue string

func BenchmarkGetValueBidirect(b *testing.B) {
	const n = 10000
	m := prepareBidirectMap(b, n)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, exist := m.GetValue(uint32(i % n))
		if !exist {
			b.Fatalf("id %v not found", i%n)
		}
		benchSinkValue = v
	}
}

func BenchmarkGetValueUnsafeBidirect(b *testing.B) {
	const n = 10000
	m := prepareBidirectMap(b, n)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, exist := m.GetValueUnsafe(uint32(i % n))
		if !exist {
			b.Fatalf("id %v not found", i%n)
		}
		benchSinkValue = v
	}
}

func BenchmarkSetBytesBidirect(b *testing.B) {
	m := NewBidirect(filepath.Join(b.TempDir(), "benchSetBytesBidirect"))
	buf := make([]byte, 0, 20)
	key := make([]byte, 0, 20)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key = strconv.AppendInt(buf[:0], int64(i), 10)
		m.SetBytes(uint32(i), key)
	}
	b.StopTimer()
	m.SetFinished()
}
