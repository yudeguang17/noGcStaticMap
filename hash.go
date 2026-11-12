// Copyright 2022 rateLimit Author(https://github.com/yudeguang17/noGcStaticMap). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/yudeguang17/noGcStaticMap.
package noGcStaticMap

import (
	"hash"
	"hash/fnv"
	"sync"
)

// fnvHasherPool 复用标准库的 hash.Hash64 实例，避免每次求哈希都新建对象。
var fnvHasherPool = sync.Pool{
	New: func() interface{} {
		return fnv.New64a()
	},
}

// hashKey 使用 Go 标准库 hash/fnv 的 FNV-1a 64 位算法计算键的哈希值，
// 也是该键探测序列的第 0 个槽位。整个工程不引入任何第三方哈希实现。
func hashKey(k []byte) uint64 {
	h := fnvHasherPool.Get().(hash.Hash64)
	h.Reset()
	// hash.Hash.Write 约定永远不返回错误
	_, _ = h.Write(k)
	sum := h.Sum64()
	fnvHasherPool.Put(h)
	return sum
}

// nextProbeHash 由上一次的哈希值派生出探测序列的下一个槽位。
//
// 采用 splitmix64 的混合步骤：它是对 uint64 的一个可逆置换，
// 因此从同一个 h0 出发的探测序列不会自我重复、且完全由键本身决定——
// 这是"只用一个 map 也能正确处理哈希冲突"的关键：
// 1)写入时，从 h0 开始逐个试探，第一个空闲槽位就是该键的位置；
// 2)查询时，从 h0 开始逐个试探，遇到空槽位即可断定键不存在
// （因为静态map没有任何删除操作，已占用的槽位永远保持占用）。
func nextProbeHash(h uint64) uint64 {
	h += 0x9e3779b97f4a7c15
	h = (h ^ (h >> 30)) * 0xbf58476d1ce4e5b9
	h = (h ^ (h >> 27)) * 0x94d049bb133111eb
	return h ^ (h >> 31)
}
