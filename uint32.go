// Copyright 2022 rateLimit Author(https://github.com/yudeguang17/noGcStaticMap). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/yudeguang17/noGcStaticMap.
package noGcStaticMap

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
)

type NoGcStaticMapUint32 struct {
	setFinished  bool //是否完成存储
	dataBeginPos int  //游标，记录位置
	len          int  //记录键值对个数
	bw           *bufio.Writer
	tempFile     *os.File
	tempFileName string
	data         []byte
	// index 唯一的一张索引表：键本身 -> 该键值对在data中的开始位置，键即索引，不存在哈希冲突
	index map[uint32]uint32
}

// 初始化 键的类型为int32,值的最大长度为65535，与默认类型相比，速度稍快，稍微节省存储空间
func NewUint32(tempFileName ...string) *NoGcStaticMapUint32 {
	var n NoGcStaticMapUint32
	n.index = make(map[uint32]uint32)
	n.tempFileName, n.tempFile, n.bw = createTempFile(tempFileName...)
	//兜底：未调用SetFinished就被丢弃时，关闭并删除临时文件
	runtime.SetFinalizer(&n, (*NoGcStaticMapUint32).releaseTempFile)
	return &n
}

// releaseTempFile 关闭并删除尚未完成的临时文件，由GC终结器调用
func (n *NoGcStaticMapUint32) releaseTempFile() {
	cleanupTempFile(n.tempFile, n.tempFileName)
	n.tempFile = nil
	n.tempFileName = ""
}

// checkSetFinished 查询前的状态校验
func (n *NoGcStaticMapUint32) checkSetFinished() {
	if !n.setFinished {
		panic("can't Get before SetFinished")
	}
}

// 取出数据
func (n *NoGcStaticMapUint32) Get(k uint32) (v []byte, exist bool) {
	n.checkSetFinished()
	dataBeginPos, exist := n.index[k]
	if exist {
		return n.read(int(dataBeginPos)), true
	}
	return v, false
}

// 取出数据 警告:返回的数据是hash表中值的引用，而非值的复制品，要注意不要在外部改变该返回值
func (n *NoGcStaticMapUint32) GetUnsafe(k uint32) (v []byte, exist bool) {
	dataBeginPos, exist := n.GetDataBeginPosOfKVPair(k)
	if !exist {
		return nil, false
	}
	return n.GetValFromDataBeginPosOfKVPairUnSafe(int(dataBeginPos)), true
}

// 取出数据,以string的方式
func (n *NoGcStaticMapUint32) GetString(k uint32) (v string, exist bool) {
	vbyte, exist := n.Get(k)
	if exist {
		return string(vbyte), true
	}
	return v, false
}

// 取出数据 警告:返回的数据是hash表中值的引用，而非值的复制品，要注意不要在外部改变该返回值
// 只要不使用UnsafeStringToBytes函数把它变化成[]byte,并用下标的方式修改值就行,所以这个函数基本是安全的
func (n *NoGcStaticMapUint32) GetStringUnsafe(k uint32) (v string, exist bool) {
	dataBeginPos, exist := n.GetDataBeginPosOfKVPair(k)
	if !exist {
		return "", false
	}
	return UnsafeBytesToString(n.GetValFromDataBeginPosOfKVPairUnSafe(int(dataBeginPos))), true
}

// 取出键值对在数据中存储的开始位置
func (n *NoGcStaticMapUint32) GetDataBeginPosOfKVPair(k uint32) (uint32, bool) {
	n.checkSetFinished()
	//这里无需校验键是否正确，故直接返回
	dataBeginPos, exist := n.index[k]
	return dataBeginPos, exist
}

// 从内存中的某个位置取出键值对中值的数据
// 警告:
// 1)传入的dataBeginPos必须是真实有效的，否则有可能会数据越界;
// 2)返回的数据是hash表中值的引用，而非值的复制品，要注意不要在外部改变该返回值
func (n *NoGcStaticMapUint32) GetValFromDataBeginPosOfKVPairUnSafe(dataBeginPos int) (v []byte) {
	n.checkSetFinished()
	//读取值的长度
	kvLenBuf := n.data[dataBeginPos : dataBeginPos+2]
	valLen := (uint64(kvLenBuf[0]) << 8) | uint64(kvLenBuf[1])
	dataBeginPos = dataBeginPos + 2
	//读取值并返回
	if valLen == 0 {
		return nil
	}
	return n.data[dataBeginPos : dataBeginPos+int(valLen)]
}

// 增加数据
func (n *NoGcStaticMapUint32) Set(k uint32, v []byte) {
	//键值设置完之后，不允许再添加
	if n.setFinished {
		panic("can't Set after SetFinished")
	}
	//判断值的长度，不允许太长
	if len(v) > maxKVLen {
		panic("k or v is too long,The maximum is 65535")
	}
	//位置索引是uint32，这里先校验不会溢出
	pos := checkDataPos(n.dataBeginPos)
	//键即索引，重复写入立即报错，而不是悄悄丢掉后一次写入
	if _, exist := n.index[k]; exist {
		panic("can't add the key '" + strconv.Itoa(int(k)) + "' for twice")
	}
	n.write(v)
	//数据落盘成功之后才占用索引槽位并计数
	n.index[k] = pos
	n.len = n.len + 1
}

// 增加数据,以string的方式
func (n *NoGcStaticMapUint32) SetString(k uint32, v string) {
	n.Set(k, []byte(v))
}

// 完成键值存储,之后只允许查询。该函数可以重复调用，重复调用不会再做任何事
func (n *NoGcStaticMapUint32) SetFinished() {
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
func (n *NoGcStaticMapUint32) Len() int {
	return n.len
}

// 从内存中读取相应数据
func (n *NoGcStaticMapUint32) read(dataBeginPos int) (v []byte) {
	//读取值的长度
	kvLenBuf := n.data[dataBeginPos : dataBeginPos+2]
	valLen := (uint64(kvLenBuf[0]) << 8) | uint64(kvLenBuf[1])
	dataBeginPos = dataBeginPos + 2
	//读取值并返回
	if valLen == 0 {
		return nil
	}
	v = make([]byte, 0, int(valLen))
	v = append(v, n.data[dataBeginPos:dataBeginPos+int(valLen)]...)
	return v
}

// 往文件中写入数据
func (n *NoGcStaticMapUint32) write(v []byte) {
	var kvLenBuf [2]byte //2个字节表示V的长度
	kvLenBuf[0] = byte(uint16(len(v)) >> 8)
	kvLenBuf[1] = byte(len(v))
	_, err := n.bw.Write(kvLenBuf[:])
	haserrPanic(err)
	if len(v) > 0 {
		_, err = n.bw.Write(v)
		haserrPanic(err)
	}
	//写完了，移动游标
	n.dataBeginPos = n.dataBeginPos + 2 + len(v)
}
