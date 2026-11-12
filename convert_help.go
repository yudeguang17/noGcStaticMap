// Copyright 2022 rateLimit Author(https://github.com/yudeguang17/noGcStaticMap). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/yudeguang17/noGcStaticMap.
package noGcStaticMap

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unsafe"
)

// 结构体案例
type NoGcStructExample struct {
	Col1 int
	Col2 string
	Col3 string
	Col4 string
}

// 缓存池1024
var bytePoolForConvert1024 = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 1024)
		return b
	},
}

// 缓存池10240
var bytePoolForConvert10240 = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 10240)
		return b
	},
}

// 缓存池102400
var bytePoolForConvert102400 = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 102400)
		return b
	},
}

// 缓存池1024000
var bytePoolForConvert1024000 = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 1024000)
		return b
	},
}

// 缓存池10240000
var bytePoolForConvert10240000 = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 10240000)
		return b
	},
}

// 默认的文本分隔符，一般文本中不会有这个字符。
// 注意：这是一个包级变量，BytesToStruct/StructToStr/checkStr在运行期读取它，
// 因此只允许在任何并发使用之前（一般是程序初始化阶段）设置，运行期修改会构成数据竞争。
var SplitSep = []byte("`")

// acquireConvertBuf 按数据长度挑选合适的缓存池，并返回该池以便用完归还
func acquireConvertBuf(dataLen int) ([]byte, *sync.Pool) {
	switch {
	case dataLen <= 1024:
		return bytePoolForConvert1024.Get().([]byte), &bytePoolForConvert1024
	case dataLen <= 10240:
		return bytePoolForConvert10240.Get().([]byte), &bytePoolForConvert10240
	case dataLen <= 102400:
		return bytePoolForConvert102400.Get().([]byte), &bytePoolForConvert102400
	case dataLen <= 1024000:
		return bytePoolForConvert1024000.Get().([]byte), &bytePoolForConvert1024000
	case dataLen <= 10240000:
		return bytePoolForConvert10240000.Get().([]byte), &bytePoolForConvert10240000
	}
	panic("data is too long, the maximum is 10240000")
}

// 高效的对数据进行分割，相比于用系统的Split减少gc的产生,速度更快
func BytesToStruct(data []byte) (p NoGcStructExample) {
	dataLen := len(data)
	if dataLen == 0 {
		panic("data err")
	}
	sep := SplitSep
	sepLen := len(sep)
	if sepLen == 0 {
		panic("SplitSep can't be empty")
	}
	//因为要经常分配内存，所以要用缓存池,并且要用合理的缓存池大小
	byteBuf, pool := acquireConvertBuf(dataLen)
	defer pool.Put(byteBuf)
	byteBufDataLen := 0
	curFieldLocation := 1
	for i := 0; i < dataLen; {
		//按完整的分隔符比较，而不是只比较首个字节，这样多字节分隔符也能正确工作
		if sepLen <= dataLen-i && bytes.Equal(data[i:i+sepLen], sep) {
			p.assignField(curFieldLocation, string(byteBuf[0:byteBufDataLen]))
			curFieldLocation = curFieldLocation + 1
			//清空此byteBuf,通过移动offset实现
			byteBufDataLen = 0
			i = i + sepLen
			continue
		}
		//数据添加到byteBuf，并移动offset
		byteBuf[byteBufDataLen] = data[i]
		byteBufDataLen = byteBufDataLen + 1
		i++
	}
	//收尾：末尾正好是分隔符时既不产生额外字段，也不会把分隔符算进最后一个字段
	if byteBufDataLen > 0 {
		p.assignField(curFieldLocation, string(byteBuf[0:byteBufDataLen]))
	}
	return p
}

// assignField 把分割出来的一段内容赋给结构体对应字段
func (p *NoGcStructExample) assignField(curFieldLocation int, curField string) {
	switch curFieldLocation {
	case 1:
		col, err := strconv.Atoi(curField)
		if err != nil {
			panic(err)
		}
		p.Col1 = col
	case 2:
		p.Col2 = curField
	case 3:
		p.Col3 = curField
	case 4:
		p.Col4 = curField
	default:
		panic("too many fields, the maximum is 4, the extra field is:" + curField)
	}
}

// 数据转换为切片
func BytesToSlice(data []byte) (ps []NoGcStructExample) {
	pos := 0
	for pos < len(data) {
		if len(data)-pos < 4 {
			panic(fmt.Sprintf("data err: truncated length prefix at offset %v", pos))
		}
		//前置4字节小端序的记录长度
		dataLen := int(binary.LittleEndian.Uint32(data[pos : pos+4]))
		pos = pos + 4
		if dataLen < 0 || len(data)-pos < dataLen {
			panic(fmt.Sprintf("data err: invalid record length %v at offset %v", dataLen, pos-4))
		}
		ps = append(ps, BytesToStruct(data[pos:pos+dataLen]))
		pos = pos + dataLen
	}
	return ps
}

// 把结构体切片转换成字符串
func SliceToStr(p []NoGcStructExample) string {
	var b strings.Builder
	var lenBuf [4]byte
	for i := range p {
		s := StructToStr(p[i])
		binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(s)))
		b.Write(lenBuf[:])
		b.WriteString(s)
	}
	return b.String()
}

// 把结构体转换成字符串
func StructToStr(p NoGcStructExample) string {
	return JoinInterface(string(SplitSep),
		p.Col1,
		checkStr(p.Col2),
		checkStr(p.Col3),
		checkStr(p.Col4))
}

// JoinInterface 用分隔符把若干元素拼接成一个字符串
func JoinInterface(sep string, elems ...interface{}) string {
	switch len(elems) {
	case 0:
		return ""
	case 1:
		return string(appendInterface(nil, elems[0]))
	}
	//拼接结果的长度无法事先精确算出，先按一个保守估计分配，避免为统计长度而把每个元素转换两遍
	capHint := len(sep) * (len(elems) - 1)
	for i := 0; i < len(elems); i++ {
		capHint += interfaceLenHint(elems[i])
	}
	buf := make([]byte, 0, capHint)
	for i := 0; i < len(elems); i++ {
		if i > 0 {
			buf = append(buf, sep...)
		}
		buf = appendInterface(buf, elems[i])
	}
	return string(buf)
}

// appendInterface 把单个元素追加到字节缓冲。
// 只对本包实际会用到的类型(string/int/int64/uint32/nil)走零分配快路径，
// 其余类型一律交给fmt.Sprint，从而与逐元素fmt.Sprint拼接的输出完全一致：
// 1)同时实现error和fmt.Stringer的类型，fmt会优先调用Error()；
// 2)String()内部panic时，fmt会捕获并输出%!v(PANIC=String method: ...)，而不是把panic抛出去。
// 这两点与直接调用v.String()的行为不同，所以这里刻意不给fmt.Stringer开辟快路径。
func appendInterface(buf []byte, elem interface{}) []byte {
	switch v := elem.(type) {
	case nil:
		return append(buf, "<nil>"...)
	case string:
		return append(buf, v...)
	case int:
		return strconv.AppendInt(buf, int64(v), 10)
	case int64:
		return strconv.AppendInt(buf, v, 10)
	case uint32:
		return strconv.AppendUint(buf, uint64(v), 10)
	default:
		return append(buf, fmt.Sprint(elem)...)
	}
}

// interfaceLenHint 给容量预估用：只对长度能直接算出来的类型做预估。
// 实测中这条分支也评估过 fmt.Stringer，但那样会让 String() 被调用两次
// （一次预估值、一次真正拼接），String() 昂贵或有副作用时会被放大，
// 所以未知类型一律返回0，交给append时的自然扩容。
func interfaceLenHint(elem interface{}) int {
	switch v := elem.(type) {
	case string:
		return len(v)
	case int, int64, uint32:
		return 20 //整数的最长长度，只是容量预估
	case nil:
		return 5
	default:
		//未知类型（含实现了fmt.Stringer的类型）不在预估阶段做任何转换，返回0即可
		return 0
	}
}

// checkStr 校验字段中是否包含分隔符
func checkStr(s string) string {
	if strings.Contains(s, string(SplitSep)) {
		panic("the field can't contain the SplitSep:" + string(SplitSep))
	}
	return s
}

// uint32ToByte 把uint32转换为小端序的4字节
func uint32ToByte(num uint32) []byte {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], num)
	return b[:]
}

// 把[]byte零拷贝转换为string
// 前提：b 在整个string生命周期内不能被修改，且不能被释放
func UnsafeBytesToString(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// 把string零拷贝转换为[]byte
// 前提：返回的切片指向只读内存，不允许通过下标修改，否则会触发段错误
func UnsafeStringToBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
