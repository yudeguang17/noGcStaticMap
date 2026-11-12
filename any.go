// Copyright 2022 rateLimit Author(https://github.com/yudeguang17/noGcStaticMap). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/yudeguang17/noGcStaticMap.
package noGcStaticMap

import (
	"bufio"
	"bytes"
	"os"
	"runtime"
)

type NoGcStaticMapAny struct {
	setFinished  bool //是否完成存储
	dataBeginPos int  //游标，记录位置
	len          int  //记录键值对个数
	bw           *bufio.Writer
	tempFile     *os.File
	tempFileName string
	data         []byte
	// index 是唯一的一张索引表：键的哈希值(探测序列当前槽位) -> 该键值对在data中的开始位置。
	// 原实现里另有 mapForHashCollision 用于存放哈希冲突的键，此处不再需要：
	// 静态map没有删除操作，已占用的槽位永远保持占用，因此可以用"键派生探测序列"在
	// 同一张表内解决冲突——写入时取第一个空闲槽位，查询时遇到空闲槽位即可判定键不存在。
	index map[uint64]uint32
}

// 初始化 默认类型,键值的最大长度为65535
func NewDefault(tempFileName ...string) *NoGcStaticMapAny {
	var n NoGcStaticMapAny
	n.index = make(map[uint64]uint32)
	n.tempFileName, n.tempFile, n.bw = createTempFile(tempFileName...)
	//兜底：未调用SetFinished就被丢弃时，关闭并删除临时文件
	runtime.SetFinalizer(&n, (*NoGcStaticMapAny).releaseTempFile)
	return &n
}

// releaseTempFile 关闭并删除尚未完成的临时文件，由GC终结器调用
func (n *NoGcStaticMapAny) releaseTempFile() {
	cleanupTempFile(n.tempFile, n.tempFileName)
	n.tempFile = nil
	n.tempFileName = ""
}

// 取出数据
func (n *NoGcStaticMapAny) Get(k []byte) (v []byte, exist bool) {
	n.checkSetFinished()
	h := hashKey(k)
	for {
		dataBeginPos, ok := n.index[h]
		if !ok {
			//空槽位：静态map无删除，该键必然不存在
			return nil, false
		}
		v, exist = n.read(k, int(dataBeginPos))
		if exist {
			return v, true
		}
		//该槽位被其它键占用，沿同一个键派生出的探测序列继续找
		h = nextProbeHash(h)
	}
}

// 取出数据 警告:返回的数据是hash表中值的引用，而非值的复制品，要注意不要在外部改变该返回值,只要不用遍历下标的方式修改值就行
func (n *NoGcStaticMapAny) GetUnsafe(k []byte) (v []byte, exist bool) {
	dataBeginPos, exist := n.GetDataBeginPosOfKVPair(k)
	if !exist {
		return nil, false
	}
	return n.GetValFromDataBeginPosOfKVPairUnSafe(int(dataBeginPos)), true
}

// 取出数据,以string的方式
func (n *NoGcStaticMapAny) GetString(k string) (v string, exist bool) {
	vbyte, exist := n.Get([]byte(k))
	if exist {
		return string(vbyte), true
	}
	return v, false
}

// 取出数据 警告:返回的数据是hash表中值的引用，而非值的复制品，要注意不要在外部改变该返回值
// 只要不使用UnsafeStringToBytes函数把它变化成[]byte,并用下标的方式修改值就行,所以这个函数基本是安全的
func (n *NoGcStaticMapAny) GetStringUnsafe(k string) (v string, exist bool) {
	dataBeginPos, exist := n.GetDataBeginPosOfKVPair([]byte(k))
	if !exist {
		return "", false
	}
	return UnsafeBytesToString(n.GetValFromDataBeginPosOfKVPairUnSafe(int(dataBeginPos))), true
}

// 取出键值对在数据中存储的开始位置
func (n *NoGcStaticMapAny) GetDataBeginPosOfKVPair(k []byte) (uint32, bool) {
	n.checkSetFinished()
	h := hashKey(k)
	for {
		dataBeginPos, ok := n.index[h]
		if !ok {
			return 0, false
		}
		if n.sameKeyAtData(int(dataBeginPos), k) {
			return dataBeginPos, true
		}
		h = nextProbeHash(h)
	}
}

// 从内存中的某个位置取出键值对中值的数据
// 警告:
// 1)传入的dataBeginPos必须是真实有效的，否则有可能会数据越界;
// 2)返回的数据是hash表中值的引用，而非值的复制品，要注意不要在外部改变该返回值
func (n *NoGcStaticMapAny) GetValFromDataBeginPosOfKVPairUnSafe(dataBeginPos int) (v []byte) {
	n.checkSetFinished()
	//读取键值的长度
	kvLenBuf := n.data[dataBeginPos : dataBeginPos+4]
	keyLen := (uint64(kvLenBuf[0]) << 8) | uint64(kvLenBuf[1])
	valLen := (uint64(kvLenBuf[2]) << 8) | uint64(kvLenBuf[3])
	//跳过键的内容
	dataBeginPos = dataBeginPos + 4 + int(keyLen)
	//空值统一返回nil，与Int/Uint32/Huge以及Get/GetString的语义保持一致
	if valLen == 0 {
		return nil
	}
	return n.data[dataBeginPos : dataBeginPos+int(valLen)]
}

// 增加数据
func (n *NoGcStaticMapAny) Set(k, v []byte) {
	//键值设置完之后，不允许再添加
	if n.setFinished {
		panic("can't Set after SetFinished")
	}
	//判断键值的长度，不允许太长
	if len(k) > maxKVLen || len(v) > maxKVLen {
		panic("k or v is too long,The maximum is 65535")
	}
	//位置索引是uint32，这里先校验不会溢出
	pos := checkDataPos(n.dataBeginPos)
	//在唯一的一张索引表中寻找本键对应的槽位，同时完成重复写入检测
	h := hashKey(k)
	for {
		dataBeginPos, exist := n.index[h]
		if !exist {
			//h即第一个空闲槽位，写完数据后再占用它
			break
		}
		if n.sameKeyAtTempFile(int(dataBeginPos), k) {
			//同一个键写入两次，直接报错，而不是悄悄丢掉后一次写入
			panic("can't add the key '" + string(k) + "' for twice")
		}
		h = nextProbeHash(h)
	}
	n.write(k, v)
	n.index[h] = pos
	//校验与写入都成功之后才计数，避免panic被recover后Len()虚高
	n.len = n.len + 1
}

// 增加数据,以string的方式
func (n *NoGcStaticMapAny) SetString(k, v string) {
	n.Set([]byte(k), []byte(v))
}

// 完成键值存储,之后只允许查询。该函数可以重复调用，重复调用不会再做任何事
func (n *NoGcStaticMapAny) SetFinished() {
	if n.setFinished {
		return
	}
	haserrPanic(n.bw.Flush())
	if n.tempFile != nil {
		haserrPanic(n.tempFile.Close())
		n.tempFile = nil
	}
	b, err := os.ReadFile(n.tempFileName)
	haserrPanic(err)
	haserrPanic(os.Remove(n.tempFileName))
	//直接接管读出来的切片，不再复制一份，避免大map在加载瞬间占用双倍内存
	n.data = b
	n.tempFileName = ""
	n.setFinished = true
}

// 键值对个数
func (n *NoGcStaticMapAny) Len() int {
	return n.len
}

// checkSetFinished 查询前的状态校验
func (n *NoGcStaticMapAny) checkSetFinished() {
	if !n.setFinished {
		panic("can't Get before SetFinished")
	}
}

// sameKeyAtData 判断内存中某个位置上的键是否就是k。
// 与read的区别是它只比较键，不会为命中项复制一份完整value——
// GetDataBeginPosOfKVPair / GetUnsafe 这类"只要位置、不要值"的路径因此不再有多余的复制。
func (n *NoGcStaticMapAny) sameKeyAtData(dataBeginPos int, k []byte) bool {
	kvLenBuf := n.data[dataBeginPos : dataBeginPos+4]
	keyLen := int(kvLenBuf[0])<<8 | int(kvLenBuf[1])
	if keyLen != len(k) {
		return false
	}
	keyBegin := dataBeginPos + 4
	return bytes.Equal(n.data[keyBegin:keyBegin+keyLen], k)
}

// sameKeyAtTempFile 判断临时文件中某个位置上的键是否就是k，仅用于写入阶段的冲突探测。
// 先只读4字节长度前缀：长度不同即可断定不是同一个键，不必读出整份键内容，
// 也就不会为每一次冲突探测都分配一份键的副本。
func (n *NoGcStaticMapAny) sameKeyAtTempFile(dataBeginPos int, k []byte) bool {
	var kvLenBuf [4]byte
	n.readTempFile(kvLenBuf[:], dataBeginPos)
	keyLen := int(kvLenBuf[0])<<8 | int(kvLenBuf[1])
	if keyLen != len(k) {
		return false
	}
	if keyLen == 0 {
		return true
	}
	key := make([]byte, keyLen)
	n.readTempFile(key, dataBeginPos+4)
	return bytes.Equal(key, k)
}

// readTempFile 从临时文件读取一段数据；只有目标字节还在写缓存里时才需要先刷盘
func (n *NoGcStaticMapAny) readTempFile(b []byte, dataBeginPos int) {
	if need := dataBeginPos + len(b); need > n.dataBeginPos-n.bw.Buffered() {
		haserrPanic(n.bw.Flush())
	}
	_, err := n.tempFile.ReadAt(b, int64(dataBeginPos))
	haserrPanic(err)
}

// 往文件中写入数据
func (n *NoGcStaticMapAny) write(k, v []byte) {
	var kvLenBuf [4]byte //2个字节表示K的长度，2个字节表示V的长度
	kvLenBuf[0] = byte(uint16(len(k)) >> 8)
	kvLenBuf[1] = byte(len(k))
	kvLenBuf[2] = byte(uint16(len(v)) >> 8)
	kvLenBuf[3] = byte(len(v))
	_, err := n.bw.Write(kvLenBuf[:])
	haserrPanic(err)
	if len(k) > 0 {
		_, err = n.bw.Write(k)
		haserrPanic(err)
	}
	if len(v) > 0 {
		_, err = n.bw.Write(v)
		haserrPanic(err)
	}
	//写完了，移动游标
	n.dataBeginPos = n.dataBeginPos + 4 + len(k) + len(v)
}

// 从内存中读取相应数据
func (n *NoGcStaticMapAny) read(k []byte, dataBeginPos int) (v []byte, exist bool) {
	kvLenBuf := n.data[dataBeginPos : dataBeginPos+4]
	keyLen := (uint64(kvLenBuf[0]) << 8) | uint64(kvLenBuf[1])
	valLen := (uint64(kvLenBuf[2]) << 8) | uint64(kvLenBuf[3])
	dataBeginPos = dataBeginPos + 4
	//读取键的内容，并判断键是否相同
	if !bytes.Equal(n.data[dataBeginPos:dataBeginPos+int(keyLen)], k) {
		return nil, false
	}
	dataBeginPos = dataBeginPos + int(keyLen)
	if valLen == 0 {
		return nil, true
	}
	v = make([]byte, valLen)
	copy(v, n.data[dataBeginPos:dataBeginPos+int(valLen)])
	return v, true
}
