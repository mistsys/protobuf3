// Go support for Protocol Buffers - Google's data interchange format
//
// Copyright 2016 Mist Systems. All rights reserved.
//
// Unlike most files, this one is entirely by Mist, and not derived
// from any earlier code.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//
//     * Redistributions of source code must retain the above copyright
// notice, this list of conditions and the following disclaimer.
//     * Redistributions in binary form must reproduce the above
// copyright notice, this list of conditions and the following disclaimer
// in the documentation and/or other materials provided with the
// distribution.
//     * Neither the name of Google Inc. nor the names of its
// contributors may be used to endorse or promote products derived from
// this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package protobuf3_test

import (
	"strconv"
	"testing"

	"github.com/mistsys/protobuf3/protobuf3"
	"github.com/mistsys/protobuf3/protobuf3/internal/unit_tests/proto"
)

func BenchmarkEncodeSmallVarint(b *testing.B) {
	buf := protobuf3.NewBuffer(make([]byte, 0, 2*128))
	for i := 0; i < b.N; i++ {
		buf.EncodeVarint(uint64(i & 16383)) // keep values under 2*7 bits
		if i&127 == 127 {
			buf.Reset() // don't keep growing, or it needs O(b.N) ram and we test realloc rather than EncodeVarint
		}
	}
}

func BenchmarkOldEncodeSmallVarint(b *testing.B) {
	buf := proto.NewBuffer(make([]byte, 0, 2*128))
	for i := 0; i < b.N; i++ {
		buf.EncodeVarint(uint64(i & 16383))
		if i&127 == 127 {
			buf.Reset()
		}
	}
}

func BenchmarkEncodeVarint(b *testing.B) {
	buf := protobuf3.NewBuffer(make([]byte, 0, 10*128))
	for i := 0; i < b.N; i++ {
		buf.EncodeVarint(uint64(i))
		if i&127 == 127 {
			buf.Reset() // don't keep growing, or it needs O(b.N) ram and we test realloc rather than EncodeVarint
		}
	}
}

func BenchmarkOldEncodeVarint(b *testing.B) {
	buf := proto.NewBuffer(make([]byte, 0, 10*128))
	for i := 0; i < b.N; i++ {
		buf.EncodeVarint(uint64(i))
		if i&127 == 127 {
			buf.Reset()
		}
	}
}

func BenchmarkDecodeSmallVarint(b *testing.B) {
	input := protobuf3.NewBuffer(nil)
	for i := 0; i < 128; i++ {
		input.EncodeVarint(uint64(i))
	}
	input.EncodeStringBytes("1234567890") // 10-byte string so we don't get near the end of the buffer and invoke the slow path
	buf := protobuf3.NewBuffer(input.Bytes())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, err := buf.DecodeVarint()
		if err != nil {
			b.Fatal(err)
			return
		}
		if v == 127 {
			// note: we could use buf.Rewind(), but that wouldn't be fair since proto package doesn't have such a method
			buf = protobuf3.NewBuffer(input.Bytes())
		}
	}
}

// decode varint at (or near) the end of the buffer (since it's a special case)
func BenchmarkDecodeSmallVarintEoB(b *testing.B) {
	input := protobuf3.NewBuffer(nil)
	input.EncodeVarint(42)
	buf := protobuf3.NewBuffer(input.Bytes())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, err := buf.DecodeVarint()
		if err != nil || v != 42 {
			b.Fatal(err)
			return
		}
		buf.Rewind()
	}
}

func BenchmarkOldDecodeSmallVarint(b *testing.B) {
	input := proto.NewBuffer(nil)
	for i := 0; i < 128; i++ {
		input.EncodeVarint(uint64(i))
	}
	input.EncodeStringBytes("1234567890") // 10-byte string so we don't get near the end of the buffer and invoke the slow path
	buf := proto.NewBuffer(input.Bytes())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, err := buf.DecodeVarint()
		if err != nil {
			b.Fatal(err)
			return
		}
		if v == 127 {
			buf = proto.NewBuffer(input.Bytes())
		}
	}
}

func BenchmarkDecode2ByteVarint(b *testing.B) {
	input := protobuf3.NewBuffer(nil)
	for i := 128; i < 128*128; i++ {
		input.EncodeVarint(uint64(i))
	}
	input.EncodeStringBytes("1234567890") // 10-byte string so we don't get near the end of the buffer and invoke the slow path
	buf := protobuf3.NewBuffer(input.Bytes())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, err := buf.DecodeVarint()
		if err != nil {
			b.Fatal(err)
			return
		}
		if v == 128*128-1 {
			// note: we could use buf.Rewind(), but that wouldn't be fair since proto package doesn't have such a method
			buf = protobuf3.NewBuffer(input.Bytes())
		}
	}
}

func BenchmarkDecode2ByteVarintEoB(b *testing.B) {
	input := protobuf3.NewBuffer(nil)
	input.EncodeVarint(128)
	buf := protobuf3.NewBuffer(input.Bytes())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, err := buf.DecodeVarint()
		if err != nil || v != 128 {
			b.Fatal(err)
			return
		}
		buf.Rewind()
	}
}

func BenchmarkOldDecode2ByteVarint(b *testing.B) {
	input := proto.NewBuffer(nil)
	for i := 128; i < 128*128; i++ {
		input.EncodeVarint(uint64(i))
	}
	input.EncodeStringBytes("1234567890") // 10-byte string so we don't get near the end of the buffer and invoke the slow path
	buf := proto.NewBuffer(input.Bytes())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, err := buf.DecodeVarint()
		if err != nil {
			b.Fatal(err)
			return
		}
		if v == 128*128-1 {
			buf = proto.NewBuffer(input.Bytes())
		}
	}
}

func BenchmarkDecode3ByteVarint(b *testing.B) {
	input := protobuf3.NewBuffer(nil)
	for i := 128 * 128; i < 128*128+1000; i++ {
		input.EncodeVarint(uint64(i))
	}
	input.EncodeStringBytes("1234567890") // 10-byte string so we don't get near the end of the buffer and invoke the slow path
	buf := protobuf3.NewBuffer(input.Bytes())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, err := buf.DecodeVarint()
		if err != nil {
			b.Fatal(err)
			return
		}
		if v == 128*128+1000-1 {
			// note: we could use buf.Rewind(), but that wouldn't be fair since proto package doesn't have such a method
			buf = protobuf3.NewBuffer(input.Bytes())
		}
	}
}

func BenchmarkDecode3ByteVarintEoB(b *testing.B) {
	input := protobuf3.NewBuffer(nil)
	input.EncodeVarint(1 << 14)
	buf := protobuf3.NewBuffer(input.Bytes())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, err := buf.DecodeVarint()
		if err != nil || v != 1<<14 {
			b.Fatal(err)
			return
		}
		buf.Rewind()
	}
}

func BenchmarkOldDecode3ByteVarint(b *testing.B) {
	input := proto.NewBuffer(nil)
	for i := 128 * 128; i < 128*128+1000; i++ {
		input.EncodeVarint(uint64(i))
	}
	input.EncodeStringBytes("1234567890") // 10-byte string so we don't get near the end of the buffer and invoke the slow path
	buf := proto.NewBuffer(input.Bytes())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, err := buf.DecodeVarint()
		if err != nil {
			b.Fatal(err)
			return
		}
		if v == 128*128+1000-1 {
			buf = proto.NewBuffer(input.Bytes())
		}
	}
}

func BenchmarkDecode4ByteVarint(b *testing.B) {
	const start = 128 * 128 * 128
	input := protobuf3.NewBuffer(nil)
	for i := start; i < start+1000; i++ {
		input.EncodeVarint(uint64(i))
	}
	input.EncodeStringBytes("1234567890") // 10-byte string so we don't get near the end of the buffer and invoke the slow path
	buf := protobuf3.NewBuffer(input.Bytes())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, err := buf.DecodeVarint()
		if err != nil {
			b.Fatal(err)
			return
		}
		if v == start+1000-1 {
			// note: we could use buf.Rewind(), but that wouldn't be fair since proto package doesn't have such a method
			buf = protobuf3.NewBuffer(input.Bytes())
		}
	}
}

func BenchmarkDecode4ByteVarintEoB(b *testing.B) {
	input := protobuf3.NewBuffer(nil)
	input.EncodeVarint(1 << 21)
	buf := protobuf3.NewBuffer(input.Bytes())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, err := buf.DecodeVarint()
		if err != nil || v != 1<<21 {
			b.Fatal(err)
			return
		}
		buf.Rewind()
	}
}

func BenchmarkDecode5ByteVarint(b *testing.B) {
	const start = 128 * 128 * 128 * 128
	input := protobuf3.NewBuffer(nil)
	for i := start; i < start+1000; i++ {
		input.EncodeVarint(uint64(i))
	}
	input.EncodeStringBytes("1234567890") // 10-byte string so we don't get near the end of the buffer and invoke the slow path
	buf := protobuf3.NewBuffer(input.Bytes())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, err := buf.DecodeVarint()
		if err != nil {
			b.Fatal(err)
			return
		}
		if v == start+1000-1 {
			// note: we could use buf.Rewind(), but that wouldn't be fair since proto package doesn't have such a method
			buf = protobuf3.NewBuffer(input.Bytes())
		}
	}
}

func BenchmarkDecode7ByteVarint(b *testing.B) {
	const start = 128 * 128 * 128 * 128 * 128 * 128
	input := protobuf3.NewBuffer(nil)
	for i := start; i < start+1000; i++ {
		input.EncodeVarint(uint64(i))
	}
	input.EncodeStringBytes("1234567890") // 10-byte string so we don't get near the end of the buffer and invoke the slow path
	buf := protobuf3.NewBuffer(input.Bytes())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, err := buf.DecodeVarint()
		if err != nil {
			b.Fatal(err)
			return
		}
		if v == start+1000-1 {
			// note: we could use buf.Rewind(), but that wouldn't be fair since proto package doesn't have such a method
			buf = protobuf3.NewBuffer(input.Bytes())
		}
	}
}

func BenchmarkDecode7ByteVarintEoB(b *testing.B) {
	input := protobuf3.NewBuffer(nil)
	input.EncodeVarint(1 << 42)
	buf := protobuf3.NewBuffer(input.Bytes())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, err := buf.DecodeVarint()
		if err != nil || v != 1<<42 {
			b.Fatal(err)
			return
		}
		buf.Rewind()
	}
}

func BenchmarkDecode9ByteVarint(b *testing.B) {
	const start = 128 * 128 * 128 * 128 * 128 * 128 * 128 * 128
	input := protobuf3.NewBuffer(nil)
	for i := start; i < start+1000; i++ {
		input.EncodeVarint(uint64(i))
	}
	input.EncodeStringBytes("1234567890") // 10-byte string so we don't get near the end of the buffer and invoke the slow path
	buf := protobuf3.NewBuffer(input.Bytes())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, err := buf.DecodeVarint()
		if err != nil {
			b.Fatal(err)
			return
		}
		if v == start+1000-1 {
			// note: we could use buf.Rewind(), but that wouldn't be fair since proto package doesn't have such a method
			buf = protobuf3.NewBuffer(input.Bytes())
		}
	}
}

func BenchmarkOldDecode9ByteVarint(b *testing.B) {
	const start = 128 * 128 * 128 * 128 * 128 * 128 * 128 * 128
	input := proto.NewBuffer(nil)
	for i := start; i < start+1000; i++ {
		input.EncodeVarint(uint64(i))
	}
	input.EncodeStringBytes("1234567890") // 10-byte string so we don't get near the end of the buffer and invoke the slow path
	buf := proto.NewBuffer(input.Bytes())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, err := buf.DecodeVarint()
		if err != nil {
			b.Fatal(err)
			return
		}
		if v == start+1000-1 {
			buf = proto.NewBuffer(input.Bytes())
		}
	}
}

func BenchmarkMarshalFixedMsg(b *testing.B) {
	i32 := int32(-10)
	u32 := uint32(11)
	i64 := int64(-12)
	u64 := uint64(13)
	f32 := float32(-14.14)
	f64 := float64(15.15)

	m := FixedMsg{
		i32: -1,
		u32: 2,
		i64: -3,
		u64: 4,
		f32: -5.5,
		f64: 6.6,

		pi32: &i32,
		pu32: &u32,
		pi64: &i64,
		pu64: &u64,
		pf32: &f32,
		pf64: &f64,

		si32: []int32{-1},
		su32: []uint32{1, 2},
		si64: []int64{-1, 3, -3},
		su64: []uint64{1, 2, 3, 4},
		sf32: []float32{-1.1, 2.2, -3.3, 4.4},
		sf64: []float64{-1.1, 2.2, -3.3, 4.4},
	}

	_, err := protobuf3.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		protobuf3.Marshal(&m)
	}
}

func BenchmarkMarshalOldFixedMsg(b *testing.B) {
	i32 := int32(-10)
	u32 := uint32(11)
	i64 := int64(-12)
	u64 := uint64(13)
	f32 := float32(-14.14)
	f64 := float64(15.15)

	m := FixedMsg{
		i32: -1,
		u32: 2,
		i64: -3,
		u64: 4,
		f32: -5.5,
		f64: 6.6,

		pi32: &i32,
		pu32: &u32,
		pi64: &i64,
		pu64: &u64,
		pf32: &f32,
		pf64: &f64,

		si32: []int32{-1},
		su32: []uint32{1, 2},
		si64: []int64{-1, 3, -3},
		su64: []uint64{1, 2, 3, 4},
		sf32: []float32{-1.1, 2.2, -3.3, 4.4},
		sf64: []float64{-1.1, 2.2, -3.3, 4.4},
	}

	_, err := proto.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proto.Marshal(&m)
	}
}

func BenchmarkUnmarshalFixedIntMsg(b *testing.B) {
	m := FixedMsg{
		i32: -1,
		u32: 2,
		i64: -3,
		u64: 4,
		f32: -5.5,
		f64: 6.6,

		/*
			si32: []int32{-1},
			su32: []uint32{1, 2},
			si64: []int64{-1, 3, -3},
			su64: []uint64{1, 2, 3, 4},
			sf32: []float32{-1.1, 2.2, -3.3, 4.4},
			sf64: []float64{-1.1, 2.2, -3.3, 4.4},
		*/
	}

	pb, err := protobuf3.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m FixedMsg
		protobuf3.Unmarshal(pb, &m)
	}
}

func BenchmarkUnmarshalOldFixedIntMsg(b *testing.B) {
	m := FixedMsg{
		i32: -1,
		u32: 2,
		i64: -3,
		u64: 4,
		f32: -5.5,
		f64: 6.6,

		/*
			si32: []int32{-1},
			su32: []uint32{1, 2},
			si64: []int64{-1, 3, -3},
			su64: []uint64{1, 2, 3, 4},
			sf32: []float32{-1.1, 2.2, -3.3, 4.4},
			sf64: []float64{-1.1, 2.2, -3.3, 4.4},
		*/
	}

	pb, err := proto.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m FixedMsg
		proto.Unmarshal(pb, &m)
	}
}

func BenchmarkUnmarshalFixedPtrIntMsg(b *testing.B) {
	i32 := int32(-10)
	u32 := uint32(11)
	i64 := int64(-12)
	u64 := uint64(13)
	f32 := float32(-14.14)
	f64 := float64(15.15)

	m := FixedMsg{
		pi32: &i32,
		pu32: &u32,
		pi64: &i64,
		pu64: &u64,
		pf32: &f32,
		pf64: &f64,
	}

	pb, err := protobuf3.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m FixedMsg
		protobuf3.Unmarshal(pb, &m)
	}
}

func BenchmarkUnmarshalOldFixedPtrIntMsg(b *testing.B) {
	i32 := int32(-10)
	u32 := uint32(11)
	i64 := int64(-12)
	u64 := uint64(13)
	f32 := float32(-14.14)
	f64 := float64(15.15)

	m := FixedMsg{
		pi32: &i32,
		pu32: &u32,
		pi64: &i64,
		pu64: &u64,
		pf32: &f32,
		pf64: &f64,
	}

	pb, err := proto.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m FixedMsg
		proto.Unmarshal(pb, &m)
	}
}

func BenchmarkUnmarshalFixedSliceMsg(b *testing.B) {
	m := FixedMsg{
		si32: []int32{-1},
		su32: []uint32{1, 2},
		si64: []int64{-1, 3, -3},
		su64: []uint64{1, 2, 3, 4},
		sf32: []float32{-1.1, 2.2, -3.3, 4.4},
		sf64: []float64{-1.1, 2.2, -3.3, 4.4},
	}

	pb, err := protobuf3.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m FixedMsg
		protobuf3.Unmarshal(pb, &m)
	}
}

func BenchmarkUnmarshalOldFixedSliceMsg(b *testing.B) {
	m := FixedMsg{
		si32: []int32{-1},
		su32: []uint32{1, 2},
		si64: []int64{-1, 3, -3},
		su64: []uint64{1, 2, 3, 4},
		sf32: []float32{-1.1, 2.2, -3.3, 4.4},
		sf64: []float64{-1.1, 2.2, -3.3, 4.4},
	}

	pb, err := proto.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m FixedMsg
		proto.Unmarshal(pb, &m)
	}
}

func BenchmarkMarshalVarMsg(b *testing.B) {
	i32 := int32(-10)
	u32 := uint32(11)
	i64 := int64(-12)
	u64 := uint64(13)

	m := VarMsg{
		i32: -1,
		u32: 2,
		i64: -3,
		u64: 4,

		pi32: &i32,
		pu32: &u32,
		pi64: &i64,
		pu64: &u64,

		si32: []int32{-1},
		su32: []uint32{1, 2},
		si64: []int64{-1, 3, -3},
		su64: []uint64{1, 2, 3, 4},
	}

	_, err := protobuf3.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		protobuf3.Marshal(&m)
	}
}

func BenchmarkMarshalOldVarMsg(b *testing.B) {
	i32 := int32(-10)
	u32 := uint32(11)
	i64 := int64(-12)
	u64 := uint64(13)

	m := VarMsg{
		i32: -1,
		u32: 2,
		i64: -3,
		u64: 4,

		pi32: &i32,
		pu32: &u32,
		pi64: &i64,
		pu64: &u64,

		si32: []int32{-1},
		su32: []uint32{1, 2},
		si64: []int64{-1, 3, -3},
		su64: []uint64{1, 2, 3, 4},
	}

	_, err := proto.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proto.Marshal(&m)
	}
}

func BenchmarkUnmarshalVarMsg(b *testing.B) {
	i32 := int32(-10)
	u32 := uint32(11)
	i64 := int64(-12)
	u64 := uint64(13)

	m := VarMsg{
		i32: -1,
		u32: 2,
		i64: -3,
		u64: 4,

		pi32: &i32,
		pu32: &u32,
		pi64: &i64,
		pu64: &u64,

		si32: []int32{-1},
		su32: []uint32{1, 2},
		si64: []int64{-1, 3, -3},
		su64: []uint64{1, 2, 3, 4},
	}

	pb, err := protobuf3.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m VarMsg
		protobuf3.Unmarshal(pb, &m)
	}
}

func BenchmarkUnmarshalOldVarMsg(b *testing.B) {
	i32 := int32(-10)
	u32 := uint32(11)
	i64 := int64(-12)
	u64 := uint64(13)

	m := VarMsg{
		i32: -1,
		u32: 2,
		i64: -3,
		u64: 4,

		pi32: &i32,
		pu32: &u32,
		pi64: &i64,
		pu64: &u64,

		si32: []int32{-1},
		su32: []uint32{1, 2},
		si64: []int64{-1, 3, -3},
		su64: []uint64{1, 2, 3, 4},
	}

	pb, err := proto.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m VarMsg
		proto.Unmarshal(pb, &m)
	}
}

func BenchmarkMarshalBytesMsg(b *testing.B) {
	s := "str"

	m := BytesMsg{
		s:  "test1",
		ps: &s,
		ss: []string{"test3", "test4"},
		sb: []byte{3, 2, 1, 0},
	}

	_, err := protobuf3.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		protobuf3.Marshal(&m)
	}
}

func BenchmarkMarshalOldBytesMsg(b *testing.B) {
	s := "str"

	m := BytesMsg{
		s:  "test1",
		ps: &s,
		ss: []string{"test3", "test4"},
		sb: []byte{3, 2, 1, 0},
	}

	_, err := proto.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proto.Marshal(&m)
	}
}

func BenchmarkUnmarshalBytesMsg(b *testing.B) {
	s := "str"

	m := BytesMsg{
		s:  "test1",
		ps: &s,
		ss: []string{"test3", "test4"},
		sb: []byte{3, 2, 1, 0},
	}

	pb, err := protobuf3.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m BytesMsg
		protobuf3.Unmarshal(pb, &m)
	}
}

func BenchmarkUnmarshalImmutableBytesMsg(b *testing.B) {
	s := "str"

	m := BytesMsg{
		s:  "test1",
		ps: &s,
		ss: []string{"test3", "test4"},
		sb: []byte{3, 2, 1, 0},
	}

	pb, err := protobuf3.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	buf := protobuf3.NewBuffer(pb)
	buf.Immutable = true

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m BytesMsg
		buf.Rewind()
		buf.Unmarshal(&m)
	}
}

func BenchmarkUnmarshalOldBytesMsg(b *testing.B) {
	s := "str"

	m := BytesMsg{
		s:  "test1",
		ps: &s,
		ss: []string{"test3", "test4"},
		sb: []byte{3, 2, 1, 0},
	}

	pb, err := proto.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m BytesMsg
		proto.Unmarshal(pb, &m)
	}
}

func BenchmarkMarshalNestedPtrStructMsg(b *testing.B) {
	// note: value is chosen so it can be compared with the equivalent non-pointer nesting
	m := NestedPtrStructMsg{
		first:  &InnerMsg{0x11},
		second: &InnerMsg{0x22},
		many:   []*InnerMsg{&InnerMsg{0x33}},
		more:   []*InnerMsg{&InnerMsg{0x44}, &InnerMsg{0x55}, &InnerMsg{0x66}},
	}

	_, err := protobuf3.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		protobuf3.Marshal(&m)
	}
}

func BenchmarkMarshalOldNestedPtrStructMsg(b *testing.B) {
	m := NestedPtrStructMsg{
		first:  &InnerMsg{0x11},
		second: &InnerMsg{0x22},
		many:   []*InnerMsg{&InnerMsg{0x33}},
		more:   []*InnerMsg{&InnerMsg{0x44}, &InnerMsg{0x55}, &InnerMsg{0x66}},
		some:   []*InnerMsg{&InnerMsg{0x77}},
	}

	_, err := proto.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proto.Marshal(&m)
	}
}

func BenchmarkMarshalNestedStructMsg(b *testing.B) {
	// note: value matches that of NestedPtrStructMsg above, for comparison between the
	// pointer-to-nested-struct and embedded-nested-struct cases
	m := NestedStructMsg{
		first:  InnerMsg{0x11},
		second: InnerMsg{0x22},
		many:   []InnerMsg{InnerMsg{0x33}},
		more:   [3]InnerMsg{InnerMsg{0x44}, InnerMsg{0x55}, InnerMsg{0x66}},
		some:   [1]*InnerMsg{&InnerMsg{0x77}},
	}

	_, err := protobuf3.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		protobuf3.Marshal(&m)
	}
}

func BenchmarkMarshalMapMsg(b *testing.B) {
	m := MapMsg{
		m: map[string]int32{
			"Nic":     0,
			"Michele": 1,
		},
	}

	_, err := protobuf3.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		protobuf3.Marshal(&m)
	}
}

func BenchmarkMarshalOldMapMsg(b *testing.B) {
	m := MapMsg{
		m: map[string]int32{
			"Nic":     0,
			"Michele": 1,
		},
	}

	_, err := proto.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proto.Marshal(&m)
	}
}

func BenchmarkUnmarshalMapMsg(b *testing.B) {
	m := MapMsg{
		m: map[string]int32{
			"Nic":     0,
			"Michele": 1,
		},
	}

	pb, err := protobuf3.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m MapMsg
		protobuf3.Unmarshal(pb, &m)
	}
}

func BenchmarkUnmarshalOldMapMsg(b *testing.B) {
	m := MapMsg{
		m: map[string]int32{
			"Nic":     0,
			"Michele": 1,
		},
	}

	pb, err := proto.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m MapMsg
		proto.Unmarshal(pb, &m)
	}
}

func BenchmarkMarshalLargeMapMsg(b *testing.B) {
	m := MapMsg{
		m: make(map[string]int32),
	}
	for i := int32(-1000); i < 1000; i++ {
		m.m[strconv.FormatInt(int64(i), 10)] = i
	}

	_, err := protobuf3.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		protobuf3.Marshal(&m)
	}
}

func BenchmarkMarshalOldLargeMapMsg(b *testing.B) {
	m := MapMsg{
		m: make(map[string]int32),
	}
	for i := int32(-1000); i < 1000; i++ {
		m.m[strconv.FormatInt(int64(i), 10)] = i
	}

	_, err := proto.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proto.Marshal(&m)
	}
}

func BenchmarkUnmarshalLargeMapMsg(b *testing.B) {
	m := MapMsg{
		m: make(map[string]int32),
	}
	for i := int32(-1000); i < 1000; i++ {
		m.m[strconv.FormatInt(int64(i), 10)] = i
	}

	pb, err := protobuf3.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m MapMsg
		protobuf3.Unmarshal(pb, &m)
	}
}

func BenchmarkUnmarshalOldLargeMapMsg(b *testing.B) {
	m := MapMsg{
		m: make(map[string]int32),
	}
	for i := int32(-1000); i < 1000; i++ {
		m.m[strconv.FormatInt(int64(i), 10)] = i
	}

	pb, err := proto.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m MapMsg
		proto.Unmarshal(pb, &m)
	}
}

type SliceOfStructMsg struct {
	Slice []StructForSliceOfStruct `protobuf:"bytes,1"`
}

type StructForSliceOfStruct struct {
	Int    int    `protobuf:"varint,3"`
	String string `protobuf:"bytes,2"`
}

func BenchmarkUnmarshalSliceOfStructMsg(b *testing.B) {
	m := SliceOfStructMsg{
		Slice: []StructForSliceOfStruct{
			StructForSliceOfStruct{
				Int:    10,
				String: "Ten",
			},
			StructForSliceOfStruct{
				Int:    9,
				String: "Nine",
			},
			StructForSliceOfStruct{
				Int:    8,
				String: "Eight",
			},
			StructForSliceOfStruct{
				Int:    7,
				String: "Seven",
			},
			StructForSliceOfStruct{
				Int:    6,
				String: "Six",
			},
			StructForSliceOfStruct{
				Int:    5,
				String: "Five",
			},
			StructForSliceOfStruct{
				Int:    4,
				String: "Four",
			},
			StructForSliceOfStruct{
				Int:    3,
				String: "Three",
			},
			StructForSliceOfStruct{
				Int:    2,
				String: "Two, main engines start",
			},
			StructForSliceOfStruct{
				Int:    1,
				String: "One",
			},
			StructForSliceOfStruct{
				Int:    0,
				String: "Liftoff",
			},
		},
	}
	for i := 0; i < 7; i++ { // x128, for a ~1000 member slice
		m.Slice = append(m.Slice, m.Slice...)
	}

	pb, err := protobuf3.Marshal(&m)
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m SliceOfStructMsg
		protobuf3.Unmarshal(pb, &m)
	}
}
