// Copyright 2022 rateLimit Author(https://github.com/yudeguang17/noGcStaticMap). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/yudeguang17/noGcStaticMap.
package noGcStaticMap

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"os"
	"runtime"
	"strconv"
	"unicode/utf8"
)

// NoGcStaticMapBidirect 双向索引的静态map：既可以由id查字符串，也可以由字符串查id。
//
// 与"用两个静态map各存一遍"相比，这里只有一份数据区：每条记录只写一次
// （4字节id + 2字节字符串长度 + 字符串内容），两张索引表都指向同一条记录的位置，
// 因此字符串内容不会重复存储。
type NoGcStaticMapBidirect struct {
	setFinished  bool //是否完成存储
	dataBeginPos int  //游标，记录位置
	len          int  //记录键值对个数
	bw           *bufio.Writer
	tempFile     *os.File
	tempFileName string
	data         []byte
	// indexByID 由id直接映射到记录位置，id即键，不存在冲突
	indexByID map[uint32]uint32
	// indexByStr 由字符串的哈希值(探测序列当前槽位)映射到记录位置。
	// 静态map没有删除操作，已占用的槽位永远保持占用，
	// 因此写入时取第一个空闲槽位、查询时遇到空闲槽位即可判定字符串不存在。
	indexByStr map[uint64]uint32
}

// 初始化 双向索引类型，字符串的最大长度为65535
func NewBidirect(tempFileName ...string) *NoGcStaticMapBidirect {
	var n NoGcStaticMapBidirect
	n.indexByID = make(map[uint32]uint32)
	n.indexByStr = make(map[uint64]uint32)
	n.tempFileName, n.tempFile, n.bw = createTempFile(tempFileName...)
	//兜底：未调用SetFinished就被丢弃时，关闭并删除临时文件
	runtime.SetFinalizer(&n, (*NoGcStaticMapBidirect).releaseTempFile)
	return &n
}

// releaseTempFile 关闭并删除尚未完成的临时文件，由GC终结器调用
func (n *NoGcStaticMapBidirect) releaseTempFile() {
	cleanupTempFile(n.tempFile, n.tempFileName)
	n.tempFile = nil
	n.tempFileName = ""
}

// 增加数据：id与字符串任一重复都直接panic
func (n *NoGcStaticMapBidirect) Set(id uint32, str string) {
	//string是不可变的，这里零拷贝转成[]byte交给内部实现，省掉每条记录一次复制
	n.set(id, UnsafeStringToBytes(str))
}

// 增加数据,以[]byte的方式：构建期用字节切片灌数据时不会额外复制一份字符串
func (n *NoGcStaticMapBidirect) SetBytes(id uint32, str []byte) {
	n.set(id, str)
}

// set 两个公开写入口的共同实现，全程只处理[]byte，不做字符串复制
func (n *NoGcStaticMapBidirect) set(id uint32, str []byte) {
	//键值设置完之后，不允许再添加
	if n.setFinished {
		panic("can't Set after SetFinished")
	}
	//判断字符串长度，不允许太长
	if len(str) > maxKVLen {
		panic("str is too long,The maximum is 65535")
	}
	//位置索引是uint32，这里先校验不会溢出
	pos := checkDataPos(n.dataBeginPos)
	//id即索引，重复立即报错
	if _, exist := n.indexByID[id]; exist {
		panic("can't add the id '" + strconv.FormatUint(uint64(id), 10) + "' for twice")
	}
	//在字符串索引中寻找第一个空闲槽位，同时完成重复字符串检测
	h := hashKey(str)
	for {
		dataBeginPos, exist := n.indexByStr[h]
		if !exist {
			//h即第一个空闲槽位，写完数据后再占用它
			break
		}
		if n.sameStrAtTempFile(int(dataBeginPos), str) {
			panic("can't add the str '" + abbreviateForPanic(str) + "' for twice")
		}
		h = nextProbeHash(h)
	}
	n.write(id, str)
	//数据落盘成功之后才占用索引槽位并计数
	n.indexByStr[h] = pos
	n.indexByID[id] = pos
	n.len = n.len + 1
}

// abbreviateForPanic 把可能的超长字符串截断成适合放进panic消息的形式：
// 避免panic消息动辄几十KB，也避免把整段业务数据写进日志或崩溃上报。
func abbreviateForPanic(str []byte) string {
	const maxBytes = 64
	if len(str) <= maxBytes {
		return string(str)
	}
	//截断点必须落在rune边界上，避免产生半个中文字符
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(str[cut]) {
		cut--
	}
	return string(str[:cut]) + "...(total " + strconv.Itoa(len(str)) + " bytes)"
}

// 完成键值存储,之后只允许查询。该函数可以重复调用，重复调用不会再做任何事
func (n *NoGcStaticMapBidirect) SetFinished() {
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
func (n *NoGcStaticMapBidirect) Len() int {
	return n.len
}

// 通过字符串查询其对应的id
func (n *NoGcStaticMapBidirect) GetID(str string) (id uint32, exist bool) {
	n.checkSetFinished()
	h := hashKey(UnsafeStringToBytes(str))
	for {
		dataBeginPos, ok := n.indexByStr[h]
		if !ok {
			//空槽位：静态map无删除，该字符串必然不存在
			return 0, false
		}
		if n.sameStrAtData(int(dataBeginPos), str) {
			//先转成int再做切片算术：位置索引是uint32，若直接用dataBeginPos+4，
			//当记录起点落在4GiB可寻址区的最后4字节里时加法会回绕，导致切片上下界颠倒
			p := int(dataBeginPos)
			return binary.LittleEndian.Uint32(n.data[p : p+4]), true
		}
		//该槽位被其它字符串占用，沿同一个字符串派生出的探测序列继续找
		h = nextProbeHash(h)
	}
}

// 通过id查询其对应的字符串
func (n *NoGcStaticMapBidirect) GetValue(id uint32) (v string, exist bool) {
	n.checkSetFinished()
	pos, exist := n.indexByID[id]
	if !exist {
		return "", false
	}
	dataBeginPos := int(pos)
	strLen := int(n.data[dataBeginPos+4])<<8 | int(n.data[dataBeginPos+5])
	dataBeginPos = dataBeginPos + 6
	if strLen == 0 {
		return "", true
	}
	return string(n.data[dataBeginPos : dataBeginPos+strLen]), true
}

// 通过id查询其对应的字符串 警告:返回的数据是内存中的引用，而非复制品，
// 要注意不要在外部改变该返回值，也不要长期持有（一个短字符串会把整块数据钉在堆上）
func (n *NoGcStaticMapBidirect) GetValueUnsafe(id uint32) (v string, exist bool) {
	n.checkSetFinished()
	pos, exist := n.indexByID[id]
	if !exist {
		return "", false
	}
	dataBeginPos := int(pos)
	strLen := int(n.data[dataBeginPos+4])<<8 | int(n.data[dataBeginPos+5])
	dataBeginPos = dataBeginPos + 6
	return UnsafeBytesToString(n.data[dataBeginPos : dataBeginPos+strLen]), true
}

// checkSetFinished 查询前的状态校验
func (n *NoGcStaticMapBidirect) checkSetFinished() {
	if !n.setFinished {
		panic("can't Get before SetFinished")
	}
}

// sameStrAtTempFile 判断临时文件中某个位置上的字符串是否就是str，仅用于写入阶段的冲突探测。
// 先只读2字节长度前缀：长度不同即可断定不是同一个字符串，不必读出整份内容。
func (n *NoGcStaticMapBidirect) sameStrAtTempFile(dataBeginPos int, str []byte) bool {
	var lenBuf [2]byte
	n.readTempFile(lenBuf[:], dataBeginPos+4)
	strLen := int(lenBuf[0])<<8 | int(lenBuf[1])
	if strLen != len(str) {
		return false
	}
	if strLen == 0 {
		return true
	}
	buf := make([]byte, strLen)
	n.readTempFile(buf, dataBeginPos+6)
	return bytes.Equal(buf, str)
}

// sameStrAtData 判断内存中某个位置上的字符串是否就是str，只比较内容，不复制字符串
func (n *NoGcStaticMapBidirect) sameStrAtData(dataBeginPos int, str string) bool {
	strLen := int(n.data[dataBeginPos+4])<<8 | int(n.data[dataBeginPos+5])
	if strLen != len(str) {
		return false
	}
	dataBeginPos = dataBeginPos + 6
	return bytes.Equal(n.data[dataBeginPos:dataBeginPos+strLen], UnsafeStringToBytes(str))
}

// readTempFile 从临时文件读取一段数据；只有目标字节还在写缓存里时才需要先刷盘
func (n *NoGcStaticMapBidirect) readTempFile(b []byte, dataBeginPos int) {
	if need := dataBeginPos + len(b); need > n.dataBeginPos-n.bw.Buffered() {
		haserrPanic(n.bw.Flush())
	}
	_, err := n.tempFile.ReadAt(b, int64(dataBeginPos))
	haserrPanic(err)
}

// 往文件中写入数据
func (n *NoGcStaticMapBidirect) write(id uint32, str []byte) {
	var head [6]byte //4个字节表示id，2个字节表示字符串的长度
	binary.LittleEndian.PutUint32(head[0:4], id)
	head[4] = byte(uint16(len(str)) >> 8)
	head[5] = byte(len(str))
	_, err := n.bw.Write(head[:])
	haserrPanic(err)
	if len(str) > 0 {
		_, err = n.bw.Write(str)
		haserrPanic(err)
	}
	//写完了，移动游标
	n.dataBeginPos = n.dataBeginPos + 6 + len(str)
}
