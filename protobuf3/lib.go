// Go support for Protocol Buffers - Google's data interchange format
//
// Modifications copyright 2016 Mist Systems.
//
// Original code copyright 2010 The Go Authors.  All rights reserved.
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

/*
Package protobuf3 encodes (marshals) Go structs which contain fields
tagged with 'protobuf' encoding information into protobuf version 3.

It can operate on the output generated by the protobuf compiler. However
it exists to operate on hand crafted types which are easier/more efficient
for the rest of the code to handle than the struct the protobuf compiler
emits would be.
*/
package protobuf3

import (
	"bytes"
	"errors"
	"fmt"
)

// Message is implemented by generated protocol buffer messages.
type Message interface {
	ProtoMessage() // dummy method exists only to indicate that the struct's fields ought to be decorated with protobuf="wiretype,id" tags
}

// A Buffer is a buffer manager for marshaling and unmarshaling
// protocol buffers.  It may be reused between invocations to
// reduce memory usage.  It is not necessary to use a Buffer;
// the global functions Marshal and Unmarshal create a
// temporary Buffer and are fine for most applications.
type Buffer struct {
	buf   []byte // encode/decode byte stream
	index int    // read point
	err   error  // nil, or the first error which happened during operation
}

// NewBuffer allocates a new Buffer and initializes its internal data to
// the contents of the argument slice.
func NewBuffer(e []byte) *Buffer {
	return &Buffer{buf: e}
}

// Reset resets the Buffer, ready for marshaling a new protocol buffer.
func (p *Buffer) Reset() {
	p.buf = p.buf[0:0] // for reading/writing
	p.index = 0        // for reading
	p.err = nil
}

// save the first error; toss the rest
// note: works correctly when arg err is nil
func (p *Buffer) noteError(err error) {
	if p.err == nil {
		p.err = err
	}
}

// Bytes returns the contents of the Buffer.
func (p *Buffer) Bytes() []byte { return p.buf }

// Rewind resets the read point to the start of the buffer.
func (p *Buffer) Rewind() {
	p.index = 0
}

// Find scans forward starting at 'offset', stopping and returning the next item which has id 'id'.
// The entire item is returned, including the 'tag' header and any varint byte length in the case of WireBytes.
// This way the item is itself a valid protobuf message.
// If sorted is true then this function assumes the message's fields are sorted by id, and encountering any id > 'id' short circuits the search
func (p *Buffer) Find(id uint, sorted bool) ([]byte, []byte, WireType, error) {
	for p.index < len(p.buf) {
		start := p.index
		vi, err := p.DecodeVarint()
		if err != nil {
			return nil, nil, 0, err
		}
		wt := WireType(vi) & 7
		if vi>>3 == uint64(id) {
			// it's a match. size the value and return
			var val []byte
			val_start := p.index // correct except in the case of WireBytes, where the value starts after the varint byte length

			switch wt {
			case WireBytes:
				val, err = p.DecodeRawBytes()

			case WireVarint:
				err = p.SkipVarint()

			case WireFixed32:
				err = p.SkipFixed(4)

			case WireFixed64:
				err = p.SkipFixed(8)
			}

			if val == nil {
				val = p.buf[val_start:p.index:p.index]
			} // else val is already set up

			return p.buf[start:p.index:p.index], val, wt, err

		} else if sorted && vi>>3 > uint64(id) {
			// we've advanced past the requested id, and we're assured the message is sorted by id
			// so we can stop searching now
			break
		} else {
			// skip over the ID's value
			switch wt {
			case WireBytes:
				err = p.SkipRawBytes()

			case WireVarint:
				err = p.SkipVarint()

			case WireFixed32:
				err = p.SkipFixed(4)

			case WireFixed64:
				err = p.SkipFixed(8)
			}
			if err != nil {
				return nil, nil, 0, err
			}
		}
	}

	// nothing found
	return nil, nil, 0, ErrNotFound
}

// error returned by (*Buffer).Find when the id is not present in the buffer
var ErrNotFound = errors.New("ID not found in protobuf buffer")

// DebugPrint dumps the encoded data in b in a debugging format with a header
// including the string s. Used in testing but made available for general debugging.
func DebugPrint(b []byte) string {
	var u uint64
	p := NewBuffer(b)
	depth := 0

	var out bytes.Buffer

out:
	for {
		for i := 0; i < depth; i++ {
			out.WriteString(" ")
		}

		index := p.index
		if index == len(p.buf) {
			break
		}

		op, err := p.DecodeVarint()
		if err != nil {
			out.WriteString(fmt.Sprintf("%3d: fetching op err %v\n", index, err))
			break out
		}
		tag := op >> 3
		wire := WireType(op) & 7

		switch wire {
		default:
			out.WriteString(fmt.Sprintf("%3d: t=%3d, unknown wire=%d\n", index, tag, wire))
			break out

		case WireBytes:
			var r []byte

			r, err = p.DecodeRawBytes()
			if err != nil {
				break out
			}
			out.WriteString(fmt.Sprintf("%3d: t=%3d, bytes [%d]", index, tag, len(r)))
			if len(r) <= 8 {
				for i := 0; i < len(r); i++ {
					out.WriteString(fmt.Sprintf(" %.2x", r[i]))
				}
			} else {
				for i := 0; i < 4; i++ {
					out.WriteString(fmt.Sprintf(" %.2x", r[i]))
				}
				out.WriteString(fmt.Sprintf(" .."))
				for i := len(r) - 4; i < len(r); i++ {
					out.WriteString(fmt.Sprintf(" %.2x", r[i]))
				}
			}
			out.WriteString(fmt.Sprintf("\n"))

		case WireFixed32:
			u, err = p.DecodeFixed32()
			if err != nil {
				out.WriteString(fmt.Sprintf("%3d: t=%3d, fix32 err %v\n", index, tag, err))
				break out
			}
			out.WriteString(fmt.Sprintf("%3d: t=%3d, fix32 %d\n", index, tag, u))

		case WireFixed64:
			u, err = p.DecodeFixed64()
			if err != nil {
				out.WriteString(fmt.Sprintf("%3d: t=%3d, fix64 err %v\n", index, tag, err))
				break out
			}
			out.WriteString(fmt.Sprintf("%3d: t=%3d, fix64 %d\n", index, tag, u))

		case WireVarint:
			u, err = p.DecodeVarint()
			if err != nil {
				out.WriteString(fmt.Sprintf("%3d: t=%3d, varint err %v\n", index, tag, err))
				break out
			}
			out.WriteString(fmt.Sprintf("%3d: t=%3d, varint %d\n", index, tag, u))

		case WireStartGroup:
			out.WriteString(fmt.Sprintf("%3d: t=%3d, start\n", index, tag))
			depth++

		case WireEndGroup:
			depth--
			out.WriteString(fmt.Sprintf("%3d: t=%3d, end\n", index, tag))
		}
	}

	if depth != 0 {
		out.WriteString(fmt.Sprintf("%3d: start-end not balanced %d\n", p.index, depth))
	}
	out.WriteString(fmt.Sprintf("\n"))

	return out.String()
}
