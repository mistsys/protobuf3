// Go support for Protocol Buffers - Google's data interchange format
//
// Copyright 2016 Mist Systems. All rights reserved.
//
// This code is derived from earlier code which was itself:
//
// Copyright 2010 The Go Authors.  All rights reserved.
// https://github.com/golang/protobuf
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

package protobuf3

/*
 * Routines for decoding protocol buffer data to construct in-memory representations.
 */

import (
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"time"
	"unsafe"

	"github.com/nsd20463/cpuendian"
)

// errOverflow is returned when an integer is too large to be represented.
var errOverflow = errors.New("protobuf3: integer overflow")

// The fundamental decoders that interpret bytes on the wire.
// Those that take integer types all return uint64 and are
// therefore of type valueDecoder.

// DecodeVarint reads a varint-encoded integer from the slice.
// It returns the integer and the number of bytes consumed, or
// zero if there is not enough.
// This is the format for the
// int32, int64, uint32, uint64, bool, and enum
// protocol buffer types.
func DecodeVarint(buf []byte) (x uint64, n int) {
	// x, n already 0
	for shift := uint(0); shift < 64 && n < len(buf); shift += 7 {
		b := uint64(buf[n])
		n++
		x |= (b & 0x7F) << shift
		if (b & 0x80) == 0 {
			return x, n
		}
	}

	// The number is truncated in some way
	return 0, 0
}

// DecodeVarint reads a varint-encoded integer from the Buffer.
// This is the format for the
// int32, int64, uint32, uint64, bool, and enum
// protocol buffer types.
func (p *Buffer) decodeVarintSlow() (x uint64, err error) {
	// x, err already 0

	i := p.index
	l := ulen(p.buf)

	for shift := uint(0); shift < 64; shift += 7 {
		if i >= l {
			err = io.ErrUnexpectedEOF
			return
		}
		b := p.buf[i]
		i++
		x |= (uint64(b) & 0x7F) << shift
		if b < 0x80 {
			p.index = i
			if shift == 9*7 {
				// the 10th byte can't have any non-zero unused bits
				if b&0xfe != 0 {
					err = errOverflow
				}
			}
			return
		}
	}

	// The number is too large to represent in a 64-bit value.
	err = errOverflow
	return
}

// DecodeVarint reads a varint-encoded integer from the Buffer.
// This is the format for the int32, int64, uint32, uint64, bool,
// and enum protocol buffer types, as well as the tags.
func (p *Buffer) DecodeVarint() (x uint64, err error) {
	i := p.index
	buf := p.buf
	n := ulen(buf)
	var b uint8

	if i >= n {
		return 0, io.ErrUnexpectedEOF
	}

	// most varints are 1 byte (because they are the protobuf tag, and most of those are 1 byte)
	// so it pays to have a special case for those
	x = uint64(buf[i])
	i++
	if x < 0x80 {
		goto done
	}

	// the longest varint we'll successfully decode is 10 bytes. so if there are more than 9 bytes
	// (since we've already read one) of buffer left we can decode it with fewer bounds checks
	if i+9 > n {
		// there are fewer than 9 bytes left; use the slower, bounds-checking code
		return p.decodeVarintSlow()
	}

	x &^= 0x80

	// note: the only way I've found to get go 1.8.1 to do bounds-check-elimination is to use constant indexes, which
	// means paying the cost of slicing buf (which is two bounds checks). That, however, ends up costing more, and
	// especially it impacts the performance of the most important 1 and 2-byte cases. So instead we leave the bounds
	// checks and index by `i`
	//_ = buf[i+8] // doesn't help (makes the code slower) in go 1.8.1 (still true with go 1.14.4)

	b = buf[i]
	i++
	x |= uint64(b) << 7
	if b < 0x80 {
		goto done
	}
	x &^= 0x80 << 7

	b = buf[i]
	i++
	x |= uint64(b) << 14
	if b < 0x80 {
		goto done
	}
	x &^= 0x80 << 14

	b = buf[i]
	i++
	x |= uint64(b) << 21
	if b < 0x80 {
		goto done
	}
	x &^= 0x80 << 21

	b = buf[i]
	i++
	x |= uint64(b) << 28
	if b < 0x80 {
		goto done
	}
	x &^= 0x80 << 28

	b = buf[i]
	i++
	x |= uint64(b) << 35
	if b < 0x80 {
		goto done
	}
	x &^= 0x80 << 35

	b = buf[i]
	i++
	x |= uint64(b) << 42
	if b < 0x80 {
		goto done
	}
	x &^= 0x80 << 42

	b = buf[i]
	i++
	x |= uint64(b) << 49
	if b < 0x80 {
		goto done
	}
	x &^= 0x80 << 49

	b = buf[i]
	i++
	x |= uint64(b) << 56
	if b < 0x80 {
		goto done
	}
	x &^= 0x80 << 56

	b = buf[i]
	i++
	x |= uint64(b) << 63
	if b < 2 {
		goto done
	}
	// x &^= 0x80 << 63 // Always zero.

	return 0, errOverflow

done:
	p.index = i
	return x, nil
}

func le64tocpu(x uint64) uint64 {
	if cpuendian.Big {
		x = ((x & 0xff) << 56) | ((x & 0xff00) << 40) | ((x & 0xff0000) << 24) | ((x & 0xff000000) << 8) |
			((x & 0xff00000000) >> 8) | ((x & 0xff0000000000) >> 24) | ((x & 0xff000000000000) >> 40) | ((x & 0xff00000000000000) >> 56)
	}
	return x
}

func le32tocpu(x uint32) uint32 {
	if cpuendian.Big {
		x = ((x & 0xff) << 24) | ((x & 0xff00) << 8) | ((x & 0xff0000) >> 8) | ((x & 0xff000000) >> 24)
	}
	return x
}

// DecodeFixed64 reads a 64-bit integer from the Buffer.
// This is the format for the
// fixed64, sfixed64, and double protocol buffer types.
func (p *Buffer) DecodeFixed64() (uint64, error) {
	// x, err already 0
	i := p.index + 8
	if i < 8 || i > ulen(p.buf) {
		return 0, io.ErrUnexpectedEOF
	}
	p.index = i

	return le64tocpu(*(*uint64)(unsafe.Pointer(&p.buf[i-8]))), nil
}

// DecodeFixed32 reads a 32-bit integer from the Buffer.
// This is the format for the
// fixed32, sfixed32, and float protocol buffer types.
func (p *Buffer) DecodeFixed32() (uint64, error) {
	// x, err already 0
	i := p.index + 4
	if i < 4 || i > ulen(p.buf) {
		return 0, io.ErrUnexpectedEOF
	}
	p.index = i

	return uint64(le32tocpu(*(*uint32)(unsafe.Pointer(&p.buf[i-4])))), nil
}

// DecodeZigzag64 reads a zigzag-encoded 64-bit integer
// from the Buffer.
// This is the format used for the sint64 protocol buffer type.
func (p *Buffer) DecodeZigzag64() (x uint64, err error) {
	x, err = p.DecodeVarint()
	if err != nil {
		return
	}
	x = (x >> 1) ^ uint64((int64(x&1)<<63)>>63)
	return
}

// DecodeZigzag32 reads a zigzag-encoded 32-bit integer
// from  the Buffer.
// This is the format used for the sint32 protocol buffer type.
// Since I might cast the result to 'int', I want this to return a signed
// 64-bit value, rather than a signed 32-bit value embedded in an
// unsigned 64-bit value. Hence the cast to int32 before extending
// to uint64 which does not appear in the proto package.
func (p *Buffer) DecodeZigzag32() (x uint64, err error) {
	x, err = p.DecodeVarint()
	if err != nil {
		return
	}
	x32 := int32((uint32(x) >> 1) ^ uint32((int32(x&1)<<31)>>31))
	x = uint64(x32)
	return
}

// CountVarints assumes p contains only varints, and counts the number of varints present
func (p *Buffer) CountVarints() int {
	n := 0
	for _, b := range p.buf[p.index:] {
		// every varint ends in  a byte with the top bit cleared, so counting the inverted top bits counts the number of varints
		n += int(b>>7) ^ 1
	}
	return n
}

// CountFixed32s assumes p contains only fixed32 values, and returns the number of values present
func (p *Buffer) CountFixed32s() int {
	return len(p.buf[p.index:]) / 4
}

// CountFixed64s assumes p contains only fixed64 values, and returns the number of values present
func (p *Buffer) CountFixed64s() int {
	return len(p.buf[p.index:]) / 8
}

// These are not ValueDecoders: they produce an array of bytes or a string.
// bytes, embedded messages

// DecodeRawBytes reads a count-delimited byte buffer from the Buffer.
// This is the format used for the bytes protocol buffer
// type and for embedded messages.
// The returned slice points to shared memory. Treat as read-only.
func (p *Buffer) DecodeRawBytes() ([]byte, error) {
	// many strings and structs are short. it pays to have a special case for these
	i := p.index
	n := ulen(p.buf)
	if i >= n {
		return nil, io.ErrUnexpectedEOF
	}
	c := uint(p.buf[i])
	i++
	if c < 0x80 {
		// 1-byte count
	} else if i < n && p.buf[i] < 0x80 {
		// 2-byte count
		c &^= 0x80
		c += uint(p.buf[i]) << 7
		i++
	} else {
		c64, err := p.DecodeVarint()
		if err != nil {
			return nil, err
		}
		c = uint(c64)
		if uint64(c) != c64 {
			return nil, fmt.Errorf("protobuf3: bad byte length %d", c64)
		}
		i = p.index
	}

	end := i + c
	if end < i || end > n {
		return nil, io.ErrUnexpectedEOF
	}

	buf := p.buf[i:end:end]
	p.index = end

	return buf, nil

}

// DecodeStringBytes reads an encoded string from the Buffer.
// This is the format used for the proto3 string type.
func (p *Buffer) DecodeStringBytes() (string, error) {
	buf, err := p.DecodeRawBytes()
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

// SkipVarint skips over a varint-encoded integer from the Buffer.
// Functionally it is similar to calling DecodeVarint and ignoring the
// value returned, except that it doesn't worry about 64-bit overflow
// of the varint value, and it runs much faster than DecodeVarint.
func (p *Buffer) SkipVarint() error {
	i := p.index
	n := ulen(p.buf)

	for {
		if i >= n {
			return io.ErrUnexpectedEOF
		}
		b := p.buf[i]
		i++
		if b < 0x80 {
			p.index = i
			return nil
		}
	}
}

// SkipFixed skips over n bytes. Useful for skipping over Fixed32 and Fixed64 with proper arguments,
// but also used to skip over arbitrary lengths.
func (p *Buffer) SkipFixed(n uint64) error {
	nb := uint(n)
	if uint64(nb) != n {
		return fmt.Errorf("protobuf3: bad skip length %d", n)
	}

	i := p.index + nb
	if i < p.index || i > ulen(p.buf) {
		return io.ErrUnexpectedEOF
	}

	p.index = i
	return nil
}

// SkipRawBytes skips over a count-delimited byte buffer from the Buffer.
// Functionally it is identical to calling DecodeRawBytes() and ignoring
// the value returned.
func (p *Buffer) SkipRawBytes() error {
	// many strings and structs are short. it pays to have a special case for these
	i := p.index
	n := ulen(p.buf)
	if i >= n {
		return io.ErrUnexpectedEOF
	}
	c := uint(p.buf[i])
	i++
	if c < 0x80 {
		// 1-byte count
	} else if i < n && p.buf[i] < 0x80 {
		// 2-byte count
		c &^= 0x80
		c += uint(p.buf[i]) << 7
		i++
	} else {
		c64, err := p.DecodeVarint()
		if err != nil {
			return err
		}
		c = uint(c64)
		if uint64(c) != c64 {
			return fmt.Errorf("protobuf3: bad byte length %d", c64)
		}
		i = p.index
	}

	end := i + c
	if end < i || end > n {
		return io.ErrUnexpectedEOF
	}

	p.index = end
	return nil
}

// Unmarshal parses the protocol buffer representation in buf and
// writes the decoded result to pb.  If the struct underlying pb does not match
// the data in buf, the results can be unpredictable.
//
// Unmarshal merges into existing data in pb. If that's not what you wanted then
// you ought to zero pb before calling Unmarshal. NOTE WELL this differs from the
// behavior of the golang/proto.Unmarshal(), but matches the standard go encoding/json.Unmarshal()
// Since we're used to json, and since having the caller do the zeroing is more efficient
// (both because they know the type (making it more efficient for the CPU), and it avoids forcing
// everyone to define a Reset() method for the Message interface (making it more efficient for
// the developer, me!)), our Unmarshal() matches the behavior of encoding/json.Unmarshal()
func Unmarshal(bytes []byte, pb Message) error {
	buf := newBuffer(bytes)
	err := buf.Unmarshal(pb)
	buf.release()
	return err
}

// Unmarshal parses the protocol buffer representation in the
// Buffer and places the decoded result in pb.  If the struct
// underlying pb does not match the data in the buffer, the results can be
// unpredictable.
func (p *Buffer) Unmarshal(pb Message) error {
	if pb == nil { // we need a non-nil interface or this won't work
		return ErrNil // NOTE this could almost qualify for a panic(), because the calling code is clearly quite confused
	}

	// If the object can unmarshal itself, let it.
	if m, ok := pb.(unmarshaler); ok {
		err := m.UnmarshalProtobuf3(p.buf[p.index:])
		p.index = ulen(p.buf)
		return err
	}

	// pb must be a pointer to a struct
	t := reflect.TypeOf(pb)
	if t.Kind() != reflect.Ptr {
		return ErrNotPointerToStruct
	}
	t = t.Elem()
	if t.Kind() != reflect.Struct {
		return ErrNotPointerToStruct
	}

	// the caller already checked that pb is a pointer-to-struct type
	base := unsafe.Pointer(reflect.ValueOf(pb).Pointer())

	prop, err := GetProperties(t)
	if err != nil {
		return err
	}

	return p.unmarshal_struct(t, prop, base)
}

// unmarshal_struct does the work of unmarshaling a structure.
func (o *Buffer) unmarshal_struct(st reflect.Type, prop *StructProperties, base unsafe.Pointer) error {
	var err error
	var pidx = 0      // index into prop.props[] where we should start searching for the next tag
	var ptag = -1     // -1, or the previous tag (matched or not, depending on whether p is nil or not)
	var p *Properties // nil, or the p where p.Tag == ptag
	for err == nil && o.index < ulen(o.buf) {
		start := o.index
		var wire WireType
		var tag int
		// most tags are one byte varints, so make that a special case and don't call DecodeVarint() and avoid error checks too
		b := uint64(o.buf[start])
		if b < 0x80 {
			o.index++
			wire = WireType(b & 0x7)
			tag = int(b >> 3)
		} else if start+1 < ulen(o.buf) && o.buf[start+1] < 0x80 {
			u := uint32(b&^0x80) + uint32(o.buf[start+1])<<7
			wire = WireType(u & 0x7)
			tag = int(u >> 3)
			o.index += 2
		} else {
			var u uint64
			u, err = o.DecodeVarint()
			if err != nil {
				break
			}
			wire = WireType(u & 0x7)
			tag = int(u >> 3)
			if tag <= 0 {
				return fmt.Errorf("protobuf3: %s: illegal tag %d (wiretype %v) at index %d of %d", st, tag, wire, start, len(o.buf))
			}
		}

		if tag != ptag {
			if tag < ptag {
				// the order on the wire has jumped around. this is legal in protobuf, but unusual. in any case we need to
				// reset the search back to the start
				pidx = 0
			}
			p = nil
			for ; pidx < len(prop.props); pidx++ {
				q := &prop.props[pidx]
				if q.Tag >= uint32(tag) { // props[] is sorted by Tag
					if q.Tag == uint32(tag) {
						p = q
					}
					break
				}
			}
			ptag = tag
		} // else re-use previous search result `p`

		if p == nil {
			err = o.skip(st, wire)
			continue
		}

		if p.dec == nil {
			fmt.Fprintf(os.Stderr, "protobuf3: no protobuf decoder for %s.%s\n", st, p.Name)
			continue
		}
		if wire != p.WireType {
			err = fmt.Errorf("protobuf3: bad wiretype for field %s.%s: got wiretype %v, wanted %v", st, p.Name, wire, p.WireType)
			break
		}
		err = p.dec(o, p, base)
	}
	return err
}

// Skip the next item in the buffer. Its wire type is decoded and presented as an argument.
// t can be nil
func (o *Buffer) skip(t reflect.Type, wire WireType) error {
	switch wire {
	case WireVarint:
		return o.SkipVarint()
	case WireBytes:
		return o.SkipRawBytes()
	case WireFixed64:
		return o.SkipFixed(8)
	case WireFixed32:
		return o.SkipFixed(4)
	default:
		return fmt.Errorf("protobuf3: can't skip unknown wiretype %v inside a %v", wire, t)
	}
}

// Get the value of the next item in the buffer. Similar to skip() but also returns the value.
// t can be nil
func (o *Buffer) get(t reflect.Type, wire WireType) ([]byte, error) {
	var err error

	start := o.index
	switch wire {
	case WireVarint:
		err = o.SkipVarint()
	case WireBytes:
		var n uint64
		n, err = o.DecodeVarint()
		start = o.index // reset the starting index to where the byte payload starts
		if err == nil {
			err = o.SkipFixed(n)
		}
	case WireFixed64:
		err = o.SkipFixed(8)
	case WireFixed32:
		err = o.SkipFixed(4)
	default:
		err = fmt.Errorf("protobuf3: can't get unknown wiretype %v for %v", wire, t)
	}
	if err != nil {
		return nil, err
	}
	return o.buf[start:o.index:o.index], nil // set slice cap out of paranoid, should someone ever append()
}

// Individual type decoders
// For each,
//	u is the decoded value,
//	v is a pointer to the field (pointer) in the struct

// Decode a *bool.
func (o *Buffer) dec_ptr_bool(p *Properties, base unsafe.Pointer) error {
	u, err := p.valDec(o)
	if err != nil {
		return err
	}
	x := u != 0
	*(**bool)(unsafe.Pointer(uintptr(base) + p.offset)) = &x
	return nil
}

// Decode a bool.
func (o *Buffer) dec_bool(p *Properties, base unsafe.Pointer) error {
	u, err := p.valDec(o)
	if err != nil {
		return err
	}
	*(*bool)(unsafe.Pointer(uintptr(base) + p.offset)) = u != 0
	return nil
}

// Decode an *int8.
func (o *Buffer) dec_ptr_int8(p *Properties, base unsafe.Pointer) error {
	u, err := p.valDec(o)
	if err != nil {
		return err
	}
	x := uint8(u)
	*(**uint8)(unsafe.Pointer(uintptr(base) + p.offset)) = &x
	return nil
}

// Decode an int8.
func (o *Buffer) dec_int8(p *Properties, base unsafe.Pointer) error {
	u, err := p.valDec(o)
	if err != nil {
		return err
	}
	*(*uint8)(unsafe.Pointer(uintptr(base) + p.offset)) = uint8(u)
	return nil
}

// Decode an *int16.
func (o *Buffer) dec_ptr_int16(p *Properties, base unsafe.Pointer) error {
	u, err := p.valDec(o)
	if err != nil {
		return err
	}
	x := uint16(u)
	*(**uint16)(unsafe.Pointer(uintptr(base) + p.offset)) = &x
	return nil
}

// Decode an int16.
func (o *Buffer) dec_int16(p *Properties, base unsafe.Pointer) error {
	u, err := p.valDec(o)
	if err != nil {
		return err
	}
	*(*uint16)(unsafe.Pointer(uintptr(base) + p.offset)) = uint16(u)
	return nil
}

// Decode an *int32.
func (o *Buffer) dec_ptr_int32(p *Properties, base unsafe.Pointer) error {
	u, err := p.valDec(o)
	if err != nil {
		return err
	}
	x := uint32(u)
	*(**uint32)(unsafe.Pointer(uintptr(base) + p.offset)) = &x
	return nil
}

// Decode an int32.
func (o *Buffer) dec_int32(p *Properties, base unsafe.Pointer) error {
	u, err := p.valDec(o)
	if err != nil {
		return err
	}
	*(*uint32)(unsafe.Pointer(uintptr(base) + p.offset)) = uint32(u)
	return nil
}

// Decode an *int.
func (o *Buffer) dec_ptr_int(p *Properties, base unsafe.Pointer) error {
	u, err := p.valDec(o)
	if err != nil {
		return err
	}
	x := uint(u)
	*(**uint)(unsafe.Pointer(uintptr(base) + p.offset)) = &x
	return nil
}

// Decode an int.
func (o *Buffer) dec_int(p *Properties, base unsafe.Pointer) error {
	u, err := p.valDec(o)
	if err != nil {
		return err
	}
	*(*uint)(unsafe.Pointer(uintptr(base) + p.offset)) = uint(u)
	return nil
}

// Decode an *int64.
func (o *Buffer) dec_ptr_int64(p *Properties, base unsafe.Pointer) error {
	u, err := p.valDec(o)
	if err != nil {
		return err
	}
	*(**uint64)(unsafe.Pointer(uintptr(base) + p.offset)) = &u
	return nil
}

// Decode an int64.
func (o *Buffer) dec_int64(p *Properties, base unsafe.Pointer) error {
	u, err := p.valDec(o)
	if err != nil {
		return err
	}
	*(*uint64)(unsafe.Pointer(uintptr(base) + p.offset)) = u
	return nil
}

// Decode a *string.
func (o *Buffer) dec_ptr_string(p *Properties, base unsafe.Pointer) error {
	s, err := o.DecodeStringBytes()
	if err != nil {
		return err
	}
	*(**string)(unsafe.Pointer(uintptr(base) + p.offset)) = &s
	return nil
}

// Decode a string.
func (o *Buffer) dec_string(p *Properties, base unsafe.Pointer) error {
	s, err := o.DecodeStringBytes()
	if err != nil {
		return err
	}
	*(*string)(unsafe.Pointer(uintptr(base) + p.offset)) = s
	return nil
}

// Decode a slice of bytes ([]byte).
func (o *Buffer) dec_slice_byte(p *Properties, base unsafe.Pointer) error {
	raw, err := o.DecodeRawBytes()
	if err != nil {
		return err
	}

	if !o.Immutable {
		copied := make([]byte, len(raw))
		copy(copied, raw)
		raw = copied
	}

	*(*[]byte)(unsafe.Pointer(uintptr(base) + p.offset)) = raw
	return nil
}

// Decode an  array of bytes ([N]byte).
func (o *Buffer) dec_array_byte(p *Properties, base unsafe.Pointer) error {
	raw, err := o.DecodeRawBytes()
	if err != nil {
		return err
	}

	n := p.length
	// NOTE WELL we assume packed bytes are encoded in one block. Thus we restart the decoding
	// at index 0 in the array. Should this not be the case then we ought to restart at an
	// index saved in a map of array->index in Buffer. However for all use cases we have that
	// is useless extra work. Should we want to decode such a field someday we can either do
	// the work, or decode into a slice, which is always variable length.
	s := ((*[maxLen]byte)(unsafe.Pointer(uintptr(base) + p.offset)))[0:n:n]

	copy(s, raw)

	return nil
}

// Decode a slice of bools ([]bool).
func (o *Buffer) dec_slice_packed_bool(p *Properties, base unsafe.Pointer) error {
	v := (*[]bool)(unsafe.Pointer(uintptr(base) + p.offset))

	nn, err := o.DecodeVarint()
	if err != nil {
		return err
	}
	nb := uint(nn) // number of bytes of encoded bools
	fin := o.index + nb
	if fin < o.index {
		return errOverflow
	}

	y := *v
	if y == nil {
		y = make([]bool, 0, p.valCnt(o))
	}

	for o.index < fin {
		u, err := p.valDec(o)
		if err != nil {
			return err
		}
		y = append(y, u != 0)
	}

	*v = y
	return nil
}

// Decode an array of bools ([N]bool).
func (o *Buffer) dec_array_packed_bool(p *Properties, base unsafe.Pointer) error {
	n := p.length
	// NOTE WELL we assume packed integers are encoded in one block. Thus we restart the decoding
	// at index 0 in the array. Should this not be the case then we ought to restart at an
	// index saved in a map of array->index in Buffer. However for all use cases we have that
	// is useless extra work. Should we want to decode such a field someday we can either do
	// the work, or decode into a slice, which is always variable length.
	s := ((*[maxLen]bool)(unsafe.Pointer(uintptr(base) + p.offset)))[0:0:n]

	nn, err := o.DecodeVarint()
	if err != nil {
		return err
	}
	nb := uint(nn) // number of bytes of encoded bools
	fin := o.index + nb
	if fin < o.index {
		return errOverflow
	}

	for o.index < fin {
		u, err := p.valDec(o)
		if err != nil {
			return err
		}
		s = append(s, u != 0)
	}

	return nil
}

// Decode a slice of int8s ([]int8) in packed format.
func (o *Buffer) dec_slice_packed_int8(p *Properties, base unsafe.Pointer) error {
	v := (*[]int8)(unsafe.Pointer(uintptr(base) + p.offset))

	nn, err := o.DecodeVarint()
	if err != nil {
		return err
	}
	nb := uint(nn) // number of bytes of encoded int8s

	fin := o.index + nb
	if fin < o.index {
		return errOverflow
	}

	y := *v
	if y == nil {
		y = make([]int8, 0, p.valCnt(o))
	}

	for o.index < fin {
		u, err := p.valDec(o)
		if err != nil {
			return err
		}
		y = append(y, int8(u))
	}
	*v = y
	return nil
}

// Decode an array of int8s ([N]int8).
func (o *Buffer) dec_array_packed_int8(p *Properties, base unsafe.Pointer) error {
	n := p.length
	// NOTE WELL we assume packed integers are encoded in one block. Thus we restart the decoding
	// at index 0 in the array. Should this not be the case then we ought to restart at an
	// index saved in a map of array->index in Buffer. However for all use cases we have that
	// is useless extra work. Should we want to decode such a field someday we can either do
	// the work, or decode into a slice, which is always variable length.
	s := ((*[maxLen]int8)(unsafe.Pointer(uintptr(base) + p.offset)))[0:0:n]

	nn, err := o.DecodeVarint()
	if err != nil {
		return err
	}
	nb := uint(nn) // number of bytes of encoded bools
	fin := o.index + nb
	if fin < o.index {
		return errOverflow
	}

	for o.index < fin {
		u, err := p.valDec(o)
		if err != nil {
			return err
		}
		if uint(len(s)) < n {
			s = append(s, int8(u))
		}
	}

	return nil
}

// Decode a slice of int16s ([]int16) in packed format.
func (o *Buffer) dec_slice_packed_int16(p *Properties, base unsafe.Pointer) error {
	v := (*[]uint16)(unsafe.Pointer(uintptr(base) + p.offset))

	nn, err := o.DecodeVarint()
	if err != nil {
		return err
	}
	nb := uint(nn) // number of bytes of encoded int16s

	fin := o.index + nb
	if fin < o.index {
		return errOverflow
	}

	y := *v
	if y == nil {
		y = make([]uint16, 0, p.valCnt(o))
	}

	for o.index < fin {
		u, err := p.valDec(o)
		if err != nil {
			return err
		}
		y = append(y, uint16(u))
	}
	*v = y
	return nil
}

// Decode an array of int16s ([N]int16).
func (o *Buffer) dec_array_packed_int16(p *Properties, base unsafe.Pointer) error {
	n := p.length
	// NOTE WELL we assume packed integers are encoded in one block. Thus we restart the decoding
	// at index 0 in the array. Should this not be the case then we ought to restart at an
	// index saved in a map of array->index in Buffer. However for all use cases we have that
	// is useless extra work. Should we want to decode such a field someday we can either do
	// the work, or decode into a slice, which is always variable length.
	s := ((*[maxLen / 2]int16)(unsafe.Pointer(uintptr(base) + p.offset)))[0:0:n]

	nn, err := o.DecodeVarint()
	if err != nil {
		return err
	}
	nb := uint(nn) // number of bytes of encoded bools
	fin := o.index + nb
	if fin < o.index {
		return errOverflow
	}

	for o.index < fin {
		u, err := p.valDec(o)
		if err != nil {
			return err
		}
		if uint(len(s)) < n {
			s = append(s, int16(u))
		}
	}

	return nil
}

// Decode a slice of int32s ([]int32) in packed format.
func (o *Buffer) dec_slice_packed_int32(p *Properties, base unsafe.Pointer) error {
	v := (*[]uint32)(unsafe.Pointer(uintptr(base) + p.offset))

	nn, err := o.DecodeVarint()
	if err != nil {
		return err
	}
	nb := uint(nn) // number of bytes of encoded int32s

	fin := o.index + nb
	if fin < o.index {
		return errOverflow
	}

	y := *v
	if y == nil {
		y = make([]uint32, 0, p.valCnt(o))
	}

	for o.index < fin {
		u, err := p.valDec(o)
		if err != nil {
			return err
		}
		y = append(y, uint32(u))
	}
	*v = y
	return nil
}

// Decode an array of int32s ([N]int32).
func (o *Buffer) dec_array_packed_int32(p *Properties, base unsafe.Pointer) error {
	n := p.length
	// NOTE WELL we assume packed integers are encoded in one block. Thus we restart the decoding
	// at index 0 in the array. Should this not be the case then we ought to restart at an
	// index saved in a map of array->index in Buffer. However for all use cases we have that
	// is useless extra work. Should we want to decode such a field someday we can either do
	// the work, or decode into a slice, which is always variable length.
	s := ((*[maxLen / 4]int32)(unsafe.Pointer(uintptr(base) + p.offset)))[0:0:n]

	nn, err := o.DecodeVarint()
	if err != nil {
		return err
	}
	nb := uint(nn) // number of bytes of encoded bools
	fin := o.index + nb
	if fin < o.index {
		return errOverflow
	}

	for o.index < fin {
		u, err := p.valDec(o)
		if err != nil {
			return err
		}
		if uint(len(s)) < n {
			s = append(s, int32(u))
		}
	}

	return nil
}

// Decode a slice of ints ([]int) in packed format.
func (o *Buffer) dec_slice_packed_int(p *Properties, base unsafe.Pointer) error {
	v := (*[]uint)(unsafe.Pointer(uintptr(base) + p.offset))

	nn, err := o.DecodeVarint()
	if err != nil {
		return err
	}
	nb := uint(nn) // number of bytes of encoded ints

	fin := o.index + nb
	if fin < o.index {
		return errOverflow
	}

	y := *v
	if y == nil {
		y = make([]uint, 0, p.valCnt(o))
	}

	for o.index < fin {
		u, err := p.valDec(o)
		if err != nil {
			return err
		}
		y = append(y, uint(u))
	}
	*v = y
	return nil
}

// Decode a slice of int64s ([]int64) in packed format.
func (o *Buffer) dec_slice_packed_int64(p *Properties, base unsafe.Pointer) error {
	v := (*[]uint64)(unsafe.Pointer(uintptr(base) + p.offset))

	nn, err := o.DecodeVarint()
	if err != nil {
		return err
	}
	nb := uint(nn) // number of bytes of encoded int64s

	fin := o.index + nb
	if fin < o.index {
		return errOverflow
	}

	y := *v
	if y == nil {
		y = make([]uint64, 0, p.valCnt(o))
	}

	for o.index < fin {
		u, err := p.valDec(o)
		if err != nil {
			return err
		}
		y = append(y, u)
	}
	*v = y
	return nil
}

// Decode an array of ints ([N]int).
func (o *Buffer) dec_array_packed_int64(p *Properties, base unsafe.Pointer) error {
	n := p.length
	// NOTE WELL we assume packed integers are encoded in one block. Thus we restart the decoding
	// at index 0 in the array. Should this not be the case then we ought to restart at an
	// index saved in a map of array->index in Buffer. However for all use cases we have that
	// is useless extra work. Should we want to decode such a field someday we can either do
	// the work, or decode into a slice, which is always variable length.
	s := ((*[maxLen / 8]int64)(unsafe.Pointer(uintptr(base) + p.offset)))[0:0:n]

	nn, err := o.DecodeVarint()
	if err != nil {
		return err
	}
	nb := uint(nn) // number of bytes of encoded bools
	fin := o.index + nb
	if fin < o.index {
		return errOverflow
	}

	for o.index < fin {
		u, err := p.valDec(o)
		if err != nil {
			return err
		}
		if uint(len(s)) < n {
			s = append(s, int64(u))
		}
	}

	return nil
}

// Decode a slice of strings ([]string).
func (o *Buffer) dec_slice_string(p *Properties, base unsafe.Pointer) error {
	s, err := o.DecodeStringBytes()
	if err != nil {
		return err
	}
	v := (*[]string)(unsafe.Pointer(uintptr(base) + p.offset))
	y := *v

	if y == nil {
		// preallocate what is probably the right sized slice immediately (if it isn't then we'll append later)
		n, _ := o.count_ahead(p.Tag, p.WireType)
		y = make([]string, 0, 1+n)
	}

	*v = append(y, s)
	return nil
}

// Decode an array of strings ([N]string).
func (o *Buffer) dec_array_string(p *Properties, base unsafe.Pointer) error {
	n := p.length
	ptr := unsafe.Pointer(uintptr(base) + p.offset) // address of 1st element of the array
	s := ((*[maxLen / 8 / 2]string)(ptr))[0:n:n]

	// the strings are encoded one at a time, each prefixed by a tag.
	str, err := o.DecodeStringBytes()
	if err != nil {
		return err
	}

	i := o.array_indexes[ptr]
	if i < n {
		s[i] = str
		i++
		o.saveIndex(ptr, i)
	}

	return nil
}

// Decode a slice of slice of bytes ([][]byte).
func (o *Buffer) dec_slice_slice_byte(p *Properties, base unsafe.Pointer) error {
	raw, err := o.DecodeRawBytes()
	if err != nil {
		return err
	}

	if !o.Immutable {
		copied := make([]byte, len(raw))
		copy(copied, raw)
		raw = copied
	}

	v := (*[][]byte)(unsafe.Pointer(uintptr(base) + p.offset))
	s := *v

	if s == nil {
		// preallocate what is probably the right sized slice immediately (if it isn't then we'll append later)
		n, _ := o.count_ahead(p.Tag, p.WireType)
		s = make([][]byte, 0, 1+n)
	}

	*v = append(s, raw)
	return nil
}

// Decode a map field.
func (o *Buffer) dec_new_map(p *Properties, base unsafe.Pointer) error {
	raw, err := o.DecodeRawBytes()
	if err != nil {
		return err
	}
	oi := o.index        // index at the end of this map entry
	o.index -= ulen(raw) // move buffer back to start of map entry

	mptr := reflect.NewAt(p.mtype, unsafe.Pointer(uintptr(base)+p.offset)) // *map[K]V
	if mptr.Elem().IsNil() {
		mptr.Elem().Set(reflect.MakeMap(mptr.Type().Elem()))
	}
	v := mptr.Elem() // map[K]V

	// Prepare addressable doubly-indirect placeholders for the key and value types.
	// See enc_new_map for why.
	keyptr := reflect.New(p.mtype.Key())                  // addressable *K
	keybase := unsafe.Pointer(keyptr.Elem().UnsafeAddr()) // *K

	valptr := reflect.New(p.mtype.Elem())                 // addressable *V
	valbase := unsafe.Pointer(valptr.Elem().UnsafeAddr()) // *V

	// Decode.
	// This parses a restricted wire format, namely the encoding of a message
	// with two fields. See enc_new_map for the format.
	// tagcode for key and value properties are always a single byte
	// because they have tags 1 and 2.
	keytag := p.mkeyprop.tagcode[0]
	valtag := p.mvalprop.tagcode[0]
	for o.index < oi {
		tagcode := o.buf[o.index]
		o.index++
		switch tagcode {
		case keytag:
			if err := p.mkeyprop.dec(o, p.mkeyprop, keybase); err != nil {
				return err
			}
		case valtag:
			if err := p.mvalprop.dec(o, p.mvalprop, valbase); err != nil {
				return err
			}
		default:
			// TODO: Should we silently skip this instead?
			return fmt.Errorf("protobuf3: bad map data tag %d", raw[0])
		}
	}
	keyelem, valelem := keyptr.Elem(), valptr.Elem()
	if !keyelem.IsValid() {
		keyelem = reflect.Zero(p.mtype.Key())
	}
	if !valelem.IsValid() {
		valelem = reflect.Zero(p.mtype.Elem())
	}

	v.SetMapIndex(keyelem, valelem)
	return nil
}

// Decode an embedded message.
func (o *Buffer) dec_struct_message(p *Properties, base unsafe.Pointer) error {
	raw, err := o.DecodeRawBytes()
	if err != nil {
		return err
	}

	ptr := unsafe.Pointer(uintptr(base) + p.offset)

	// swizzle around and reuse the buffer. less gc
	obuf, oi := o.buf, o.index
	o.buf, o.index = raw, 0

	err = o.unmarshal_struct(p.stype, p.sprop, ptr)

	o.buf, o.index = obuf, oi
	return err
}

// Decode a pointer to an embedded message.
func (o *Buffer) dec_ptr_struct_message(p *Properties, base unsafe.Pointer) error {
	raw, err := o.DecodeRawBytes()
	if err != nil {
		return err
	}

	pptr := (*unsafe.Pointer)(unsafe.Pointer(uintptr(base) + p.offset))
	ptr := *pptr
	var val reflect.Value
	if ptr == nil {
		val = reflect.New(p.stype)
		ptr = unsafe.Pointer(val.Pointer()) // Is this gc safe? it seems not to be to me, but I don't have a better solution, and it's what google's code does
		*pptr = ptr
	} // else the value is already allocated and we merge into it

	// swizzle around and reuse the buffer. less gc
	obuf, oi := o.buf, o.index
	o.buf, o.index = raw, 0

	err = o.unmarshal_struct(p.stype, p.sprop, ptr)

	o.buf, o.index = obuf, oi
	return err
}

// Decode into a slice of messages ([]struct)
func (o *Buffer) dec_slice_struct_message(p *Properties, base unsafe.Pointer) error {
	raw, err := o.DecodeRawBytes()
	if err != nil {
		return err
	}

	// build a reflect.Value of the slice
	ptr := unsafe.Pointer(uintptr(base) + p.offset)
	slice_type := reflect.SliceOf(p.stype)
	slice := reflect.NewAt(slice_type, ptr).Elem()

	if slice.IsNil() {
		// preallocate what is probably the right sized slice immediately (if it isn't then we'll append later)
		n, _ := o.count_ahead(p.Tag, p.WireType)
		slice.Set(reflect.MakeSlice(slice.Type(), 0, 1+n))
	}

	n := slice.Len()
	if n < slice.Cap() {
		slice.SetLen(n + 1)
	} else {
		// extend the slice with a new zero value
		slice.Set(reflect.Append(slice, reflect.Zero(p.stype)))
	}

	// and unmarshal into it
	val := slice.Index(n)
	if p.isAppender || p.isMarshaler {
		return val.Addr().Interface().(unmarshaler).UnmarshalProtobuf3(raw)
	}

	pval := unsafe.Pointer(val.UnsafeAddr())

	// unmarshal into pval
	obuf, oi := o.buf, o.index
	o.buf, o.index = raw, 0
	err = o.unmarshal_struct(p.stype, p.sprop, pval)
	o.buf, o.index = obuf, oi

	return err
}

// Decode into an array of messages ([N]struct)
func (o *Buffer) dec_array_struct_message(p *Properties, base unsafe.Pointer) error {
	raw, err := o.DecodeRawBytes()
	if err != nil {
		return err
	}

	// address of the start of the array
	ptr := unsafe.Pointer(uintptr(base) + p.offset)
	n := p.length
	i := o.array_indexes[ptr]
	if i < n {
		// address of element i
		ptr_elem := unsafe.Pointer(uintptr(ptr) + uintptr(i)*p.stype.Size())

		if p.isAppender || p.isMarshaler {
			err = reflect.NewAt(p.stype, ptr_elem).Interface().(unmarshaler).UnmarshalProtobuf3(raw)
		} else {
			// unmarshal into pval
			obuf, oi := o.buf, o.index
			o.buf, o.index = raw, 0
			err = o.unmarshal_struct(p.stype, p.sprop, ptr_elem)
			o.buf, o.index = obuf, oi
		}

		i++
		o.saveIndex(ptr, i)
	}

	return err
}

// Decode into a slice of pointers to messages ([]*struct)
func (o *Buffer) dec_slice_ptr_struct_message(p *Properties, base unsafe.Pointer) error {
	raw, err := o.DecodeRawBytes()
	if err != nil {
		return err
	}

	// construct a new *struct
	v := reflect.New(p.stype)
	pv := unsafe.Pointer(v.Pointer())

	// unmarshal into the new struct
	if p.isAppender || p.isMarshaler {
		err = v.Interface().(unmarshaler).UnmarshalProtobuf3(raw)
	} else {
		obuf, oi := o.buf, o.index
		o.buf, o.index = raw, 0
		err = o.unmarshal_struct(p.stype, p.sprop, pv)
		o.buf, o.index = obuf, oi
	}
	if err != nil {
		return err
	}

	// append pv to the slice []*struct
	pslice := (*[]unsafe.Pointer)(unsafe.Pointer(uintptr(base) + p.offset))
	s := *pslice

	if s == nil {
		// preallocate what is probably the right sized slice immediately (if it isn't then we'll append later)
		n, _ := o.count_ahead(p.Tag, p.WireType)
		s = make([]unsafe.Pointer, 0, 1+n)
	}

	*pslice = append(s, pv)

	return nil
}

// Decode into a array of pointers to messages ([N]*struct)
func (o *Buffer) dec_array_ptr_struct_message(p *Properties, base unsafe.Pointer) error {
	raw, err := o.DecodeRawBytes()
	if err != nil {
		return err
	}

	// construct a new *struct
	v := reflect.New(p.stype)
	pv := unsafe.Pointer(v.Pointer())

	// unmarshal into the new struct
	if p.isAppender || p.isMarshaler {
		err = v.Interface().(unmarshaler).UnmarshalProtobuf3(raw)
	} else {
		obuf, oi := o.buf, o.index
		o.buf, o.index = raw, 0
		err = o.unmarshal_struct(p.stype, p.sprop, pv)
		o.buf, o.index = obuf, oi
	}
	if err != nil {
		return err
	}

	// address of the start of the array
	ptr := unsafe.Pointer(uintptr(base) + p.offset)
	n := p.length
	i := o.array_indexes[ptr]
	if i < n {
		// address of pointer i
		*(*unsafe.Pointer)(unsafe.Pointer(uintptr(ptr) + uintptr(i)*unsafe.Sizeof(unsafe.Pointer(nil)))) = pv
		i++
		o.saveIndex(ptr, i)
	}

	return nil
}

// Decode an embedded message that can unmarshal itself
func (o *Buffer) dec_unmarshaler(p *Properties, base unsafe.Pointer) error {
	raw, err := o.get(p.stype, p.WireType)
	if err != nil {
		return err
	}

	ptr := unsafe.Pointer(uintptr(base) + p.offset)
	iv := reflect.NewAt(p.stype, ptr).Interface()
	return iv.(unmarshaler).UnmarshalProtobuf3(raw)
}

// Decode a pointer to an embedded message that can unmarshal itself
func (o *Buffer) dec_ptr_unmarshaler(p *Properties, base unsafe.Pointer) error {
	raw, err := o.get(p.stype, p.WireType)
	if err != nil {
		return err
	}

	pptr := (*unsafe.Pointer)(unsafe.Pointer(uintptr(base) + p.offset))
	var val reflect.Value
	if *pptr == nil {
		val = reflect.New(p.stype)
		*pptr = unsafe.Pointer(val.Pointer()) // Is this gc safe? it seems not to be to me, but I don't have a better solution, and it's what google's code does
	} else {
		// else the value is already allocated and we merge into it
		val = reflect.NewAt(p.stype, *pptr)
	}
	return val.Interface().(unmarshaler).UnmarshalProtobuf3(raw)
}

// Decode into slice of things which can marshal themselves
func (o *Buffer) dec_slice_unmarshaler(p *Properties, base unsafe.Pointer) error {
	raw, err := o.get(p.stype, p.WireType)
	if err != nil {
		return err
	}

	// build a reflect.Value of the slice
	ptr := unsafe.Pointer(uintptr(base) + p.offset)
	slice_type := reflect.SliceOf(p.stype)
	slice := reflect.NewAt(slice_type, ptr).Elem()

	if slice.IsNil() {
		// preallocate what is probably the right sized slice immediately (if it isn't then we'll append later)
		n, _ := o.count_ahead(p.Tag, p.WireType)
		slice.Set(reflect.MakeSlice(slice.Type(), 0, 1+n))
	}

	n := slice.Len()
	if n < slice.Cap() {
		slice.SetLen(n + 1)
	} else {
		// extend the slice with a new zero value
		slice.Set(reflect.Append(slice, reflect.Zero(p.stype)))
	}

	// and unmarshal into it
	val := slice.Index(n)
	return val.Addr().Interface().(unmarshaler).UnmarshalProtobuf3(raw)
}

// Decode into an array of unarshalers ([N]T, where T implements unarshaler)
func (o *Buffer) dec_array_unmarshaler(p *Properties, base unsafe.Pointer) error {
	raw, err := o.get(p.stype, p.WireType)
	if err != nil {
		return err
	}

	ptr := unsafe.Pointer(uintptr(base) + p.offset)
	n := p.length
	i := o.array_indexes[ptr]
	if i < n {
		// address of element i
		ptr_elem := unsafe.Pointer(uintptr(ptr) + uintptr(i)*p.stype.Size())
		err = reflect.NewAt(p.stype, ptr_elem).Interface().(unmarshaler).UnmarshalProtobuf3(raw)
		i++
		o.saveIndex(ptr, i)
	}

	return err
}

// dummy no-op decoder used for decoding 0-length array types
func (o *Buffer) dec_nothing(p *Properties, base unsafe.Pointer) error {
	return nil
}

// custom decoder for protobuf3 standard Timestamp, decoding it into the standard go time.Time
func (o *Buffer) dec_time_Time(p *Properties, base unsafe.Pointer) error {
	return o.decode_time_Time((*time.Time)(unsafe.Pointer(uintptr(base) + p.offset)))
}

// custom decoder for pointer to time.Time
func (o *Buffer) dec_ptr_time_Time(p *Properties, base unsafe.Pointer) error {
	pptr := (**time.Time)(unsafe.Pointer(uintptr(base) + p.offset))
	ptr := *pptr
	if ptr == nil {
		ptr = new(time.Time)
		*pptr = ptr
	} // else overwwrite the existing time.Time like the protobuf standard says to do
	return o.decode_time_Time(ptr)
}

// inner code for decoding protobuf3 standard Timestamp to time.Time
func (o *Buffer) decode_time_Time(t *time.Time) error {
	// first decode the byte length and limit our decoding to that (since messages are encoded in WireBytes)
	buf, err := o.DecodeRawBytes()
	if err != nil {
		return err
	}

	// swizzle buf (saves gc pressure from a new Buffer)
	obuf, oi := o.buf, o.index
	o.buf, o.index = buf, 0

	ts, err := o.DecodeTimestamp()

	o.buf, o.index = obuf, oi

	if err == nil {
		*t = ts
	}
	return err
}

// DecodeTimstamp decodes a google.protobuf.Timestamp as a time.Time
func (o *Buffer) DecodeTimestamp() (time.Time, error) {
	var secs, nanos uint64
	for o.index < ulen(o.buf) {
		tag, err := o.DecodeVarint()
		if err != nil {
			return time.Time{}, err
		}
		switch tag {
		case 1<<3 | uint64(WireVarint): // seconds
			secs, err = o.DecodeVarint()
		case 2<<3 | uint64(WireVarint): // nanoseconds
			nanos, err = o.DecodeVarint()
		default:
			// do the protobuf thing and ignore unknown tags
			err = o.skip(nil, WireType(tag)&7)
		}
		if err != nil {
			return time.Time{}, err
		}
	}

	// return whatever we got (which might even be the zero value)
	ts := time.Unix(int64(secs), int64(nanos)).UTC() // time.Unix() returns local timezone, which we usually don't use
	return ts, nil
}

// DecodeNSecTimstamp decodes a google.protobuf.Timestamp as a int64 nanosecond unix time
func (o *Buffer) DecodeNSecTimestamp() (int64, error) {
	var secs, nanos uint64
	for o.index < ulen(o.buf) {
		tag, err := o.DecodeVarint()
		if err != nil {
			return 0, err
		}
		switch tag {
		case 1<<3 | uint64(WireVarint): // seconds
			secs, err = o.DecodeVarint()
		case 2<<3 | uint64(WireVarint): // nanoseconds
			nanos, err = o.DecodeVarint()
		default:
			// do the protobuf thing and ignore unknown tags
			err = o.skip(nil, WireType(tag)&7)
		}
		if err != nil {
			return 0, err
		}
	}

	// return whatever we got (which might even be the zero value)
	return int64(secs)*1000_000_000 + int64(nanos), nil
}

// custom decoder for protobuf3 standard Duration, decoding it into the go standard time.Duration
func (o *Buffer) dec_time_Duration(p *Properties, base unsafe.Pointer) error {
	d, err := o.dec_Duration(p)
	if err != nil {
		return err
	}
	*(*time.Duration)(unsafe.Pointer(uintptr(base) + p.offset)) = d
	return nil
}

// helper function to decode a protobuf3 Duration value into a time.Duration
func (o *Buffer) dec_Duration(p *Properties) (time.Duration, error) {
	// the tag has been decoded, but not the byte length
	n, err := o.DecodeVarint()
	if err != nil {
		return 0, err
	}
	end := o.index + uint(n)

	if end < o.index || end > ulen(o.buf) {
		return 0, io.ErrUnexpectedEOF
	}

	// restrict ourselves to p.index:end
	oo := newBuffer(o.buf[o.index:end:end])

	var secs, nanos uint64
	for oo.index < ulen(oo.buf) {
		tag, err := oo.DecodeVarint()
		if err != nil {
			return 0, err
		}
		switch tag {
		case 1<<3 | uint64(WireVarint): // seconds
			secs, err = oo.DecodeVarint()
		case 2<<3 | uint64(WireVarint): // nanoseconds
			nanos, err = oo.DecodeVarint()
		default:
			// do the protobuf thing and ignore unknown tags
			err = oo.skip(nil, WireType(tag)&7)
		}
		if err != nil {
			return 0, err
		}
	}
	oo.release()

	o.index = end

	d := time.Duration(secs)*time.Second + time.Duration(nanos)*time.Nanosecond

	return d, nil
}

// custom decoder for *time.Duration, ... protobuf Duration message
func (o *Buffer) dec_ptr_time_Duration(p *Properties, base unsafe.Pointer) error {
	d, err := o.dec_Duration(p)
	if err != nil {
		return err
	}
	*(**time.Duration)(unsafe.Pointer(uintptr(base) + p.offset)) = &d
	return nil
}

// custom decode for []time.Duration
func (o *Buffer) dec_slice_time_Duration(p *Properties, base unsafe.Pointer) error {
	v := (*[]time.Duration)(unsafe.Pointer(uintptr(base) + p.offset))

	d, err := o.dec_Duration(p)
	if err != nil {
		return err
	}

	s := *v

	if s == nil {
		// preallocate what is probably the right sized slice immediately (if it isn't then we'll append later)
		n, _ := o.count_ahead(p.Tag, p.WireType)
		s = make([]time.Duration, 0, 1+n)
	}

	*v = append(s, d)

	return nil
}

// custom decode for [N]time.Duration
func (o *Buffer) dec_array_time_Duration(p *Properties, base unsafe.Pointer) error {
	// each Duration is encoded separately (since in protobuf they are a message with 2 fields)
	d, err := o.dec_Duration(p)
	if err != nil {
		return err
	}

	ptr := unsafe.Pointer(uintptr(base) + p.offset) // address of 1st element of the array
	n := p.length
	s := ((*[maxLen / 8]time.Duration)(ptr))[0:n:n]

	i := o.array_indexes[ptr]
	if i < n {
		s[i] = d
		i++
		o.saveIndex(ptr, i)
	}

	return nil
}
