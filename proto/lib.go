// Go support for Protocol Buffers - Google's data interchange format
//
// Copyright 2010 Google Inc.  All rights reserved.
// http://code.google.com/p/goprotobuf/
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
	The proto package converts data structures to and from the
	wire format of protocol buffers.  It works in concert with the
	Go source code generated for .proto files by the protocol compiler.

	A summary of the properties of the protocol buffer interface
	for a protocol buffer variable v:

	  - Names are turned from camel_case to CamelCase for export.
	  - There are no methods on v to set and get fields; just treat
	  	them as structure fields.
	  - The zero value for a struct is its correct initialization state.
		All desired fields must be set before marshaling.
	  - A Reset() method will restore a protobuf struct to its zero state.
	  - Each type T has a method NewT() to create a new instance. It
		is equivalent to new(T).
	  - Non-repeated fields are pointers to the values; nil means unset.
		That is, optional or required field int32 f becomes F *int32.
	  - Repeated fields are slices.
	  - Helper functions are available to simplify the getting and setting of fields:
	  	foo.String = proto.String("hello") // set field
	  	s := proto.GetString(foo.String)  // get field
	  - Constants are defined to hold the default values of all fields that
		have them.  They have the form Default_StructName_FieldName.
	  - Enums are given type names and maps between names to values,
	  	plus a helper function to create values.  Enum values are prefixed
	  	with the enum's type name.
	  - Nested groups and enums have type names prefixed with the name of
	  	the surrounding message type.
	  - Extensions are given descriptor names that start with E_,
		followed by an underscore-delimited list of the nested messages
		that contain it (if any) followed by the CamelCased name of the
		extension field itself.  HasExtension, ClearExtension, GetExtension
		and SetExtension are functions for manipulating extensions.
	  - Marshal and Unmarshal are functions to encode and decode the wire format.

	The simplest way to describe this is to see an example.
	Given file test.proto, containing

		package example;

		enum FOO { X = 17; };

		message Test {
		  required string label = 1;
		  optional int32 type = 2 [default=77];
		  repeated int64 reps = 3;
		  optional group OptionalGroup = 4 {
		    required string RequiredField = 5;
		  };
		}

	The resulting file, test.pb.go, is:

		package example

		import "goprotobuf.googlecode.com/hg/proto"

		type FOO int32
		const (
			FOO_X = 17
		)
		var FOO_name = map[int32] string {
			17: "X",
		}
		var FOO_value = map[string] int32 {
			"X": 17,
		}
		func NewFOO(x int32) *FOO {
			e := FOO(x)
			return &e
		}

		type Test struct {
			Label	*string	"PB(bytes,1,req,name=label)"
			Type	*int32	"PB(varint,2,opt,name=type,def=77)"
			Reps	[]int64	"PB(varint,3,rep,name=reps)"
			Optionalgroup	*Test_OptionalGroup	"PB(group,4,opt,name=optionalgroup)"
			XXX_unrecognized []byte
		}
		func (this *Test) Reset() {
			*this = Test{}
		}
		func NewTest() *Test {
			return new(Test)
		}
		const Default_Test_Type int32 = 77

		type Test_OptionalGroup struct {
			RequiredField	*string	"PB(bytes,5,req)"
			XXX_unrecognized []byte
		}
		func (this *Test_OptionalGroup) Reset() {
			*this = Test_OptionalGroup{}
		}
		func NewTest_OptionalGroup() *Test_OptionalGroup {
			return new(Test_OptionalGroup)
		}

		func init() {
			proto.RegisterEnum("example.FOO", FOO_name, FOO_value)
		}

	To create and play with a Test object:

		package main

		import (
			"log"

			"goprotobuf.googlecode.com/hg/proto"
			"./example.pb"
		)

		func main() {
			test := &example.Test {
				Label: proto.String("hello"),
				Type: proto.Int32(17),
				Optionalgroup: &example.Test_OptionalGroup {
					RequiredField: proto.String("good bye"),
				},
			}
			data, err := proto.Marshal(test)
			if err != nil {
				log.Exit("marshaling error:", err)
			}
			newTest := example.NewTest()
			err = proto.Unmarshal(data, newTest)
			if err != nil {
				log.Exit("unmarshaling error:", err)
			}
			// Now test and newTest contain the same data.
			if proto.GetString(test.Label) != proto.GetString(newTest.Label) {
				log.Exit("data mismatch %q %q", proto.GetString(test.Label), proto.GetString(newTest.Label))
			}
			// etc.
		}
*/
package proto

import (
	"fmt"
	"strconv"
)

// Stats records allocation details about the protocol buffer encoders
// and decoders.  Useful for tuning the library itself.
type Stats struct {
	Emalloc uint64 // mallocs in encode
	Dmalloc uint64 // mallocs in decode
	Encode  uint64 // number of encodes
	Decode  uint64 // number of decodes
	Chit    uint64 // number of cache hits
	Cmiss   uint64 // number of cache misses
}

var stats Stats

// GetStats returns a copy of the global Stats structure.
func GetStats() Stats { return stats }

// A Buffer is a buffer manager for marshaling and unmarshaling
// protocol buffers.  It may be reused between invocations to
// reduce memory usage.  It is not necessary to use a Buffer;
// the global functions Marshal and Unmarshal create a
// temporary Buffer and are fine for most applications.
type Buffer struct {
	buf       []byte     // encode/decode byte stream
	index     int        // write point
	freelist  [10][]byte // list of available buffers
	nfreelist int        // number of free buffers
	ptr       uintptr    // scratch area for pointers
}

// NewBuffer allocates a new Buffer and initializes its internal data to
// the contents of the argument slice.
func NewBuffer(e []byte) *Buffer {
	p := new(Buffer)
	if e == nil {
		e = p.bufalloc()
	}
	p.buf = e
	p.index = 0
	return p
}

// Reset resets the Buffer, ready for marshaling a new protocol buffer.
func (p *Buffer) Reset() {
	if p.buf == nil {
		p.buf = p.bufalloc()
	}
	p.buf = p.buf[0:0] // for reading/writing
	p.index = 0        // for reading
}

// SetBuf replaces the internal buffer with the slice,
// ready for unmarshaling the contents of the slice.
func (p *Buffer) SetBuf(s []byte) {
	p.buf = s
	p.index = 0
}

// Bytes returns the contents of the Buffer.
func (p *Buffer) Bytes() []byte { return p.buf }

// Allocate a buffer for the Buffer.
func (p *Buffer) bufalloc() []byte {
	if p.nfreelist > 0 {
		// reuse an old one
		p.nfreelist--
		s := p.freelist[p.nfreelist]
		return s[0:0]
	}
	// make a new one
	s := make([]byte, 0, 16)
	return s
}

// Free (and remember in freelist) a byte buffer for the Buffer.
func (p *Buffer) buffree(s []byte) {
	if p.nfreelist < len(p.freelist) {
		// Take next slot.
		p.freelist[p.nfreelist] = s
		p.nfreelist++
		return
	}

	// Find the smallest.
	besti := -1
	bestl := len(s)
	for i, b := range p.freelist {
		if len(b) < bestl {
			besti = i
			bestl = len(b)
		}
	}

	// Overwrite the smallest.
	if besti >= 0 {
		p.freelist[besti] = s
	}
}

/*
 * Helper routines for simplifying the creation of optional fields of basic type.
 */

// Bool is a helper routine that allocates a new bool value
// to store v and returns a pointer to it.
func Bool(v bool) *bool {
	p := new(bool)
	*p = v
	return p
}

// Int32 is a helper routine that allocates a new int32 value
// to store v and returns a pointer to it.
func Int32(v int32) *int32 {
	p := new(int32)
	*p = v
	return p
}

// Int is a helper routine that allocates a new int32 value
// to store v and returns a pointer to it, but unlike Int32
// its argument value is an int.
func Int(v int) *int32 {
	p := new(int32)
	*p = int32(v)
	return p
}

// Int64 is a helper routine that allocates a new int64 value
// to store v and returns a pointer to it.
func Int64(v int64) *int64 {
	p := new(int64)
	*p = v
	return p
}

// Float32 is a helper routine that allocates a new float32 value
// to store v and returns a pointer to it.
func Float32(v float32) *float32 {
	p := new(float32)
	*p = v
	return p
}

// Float64 is a helper routine that allocates a new float64 value
// to store v and returns a pointer to it.
func Float64(v float64) *float64 {
	p := new(float64)
	*p = v
	return p
}

// Uint32 is a helper routine that allocates a new uint32 value
// to store v and returns a pointer to it.
func Uint32(v uint32) *uint32 {
	p := new(uint32)
	*p = v
	return p
}

// Uint64 is a helper routine that allocates a new uint64 value
// to store v and returns a pointer to it.
func Uint64(v uint64) *uint64 {
	p := new(uint64)
	*p = v
	return p
}

// String is a helper routine that allocates a new string value
// to store v and returns a pointer to it.
func String(v string) *string {
	p := new(string)
	*p = v
	return p
}

/*
 * Helper routines for simplifying the fetching of optional fields of basic type.
 * If the field is missing, they return the zero for the type.
 */

// GetBool is a helper routine that returns an optional bool value.
func GetBool(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}

// GetInt32 is a helper routine that returns an optional int32 value.
func GetInt32(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}

// GetInt64 is a helper routine that returns an optional int64 value.
func GetInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// GetFloat32 is a helper routine that returns an optional float32 value.
func GetFloat32(p *float32) float32 {
	if p == nil {
		return 0
	}
	return *p
}

// GetFloat64 is a helper routine that returns an optional float64 value.
func GetFloat64(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// GetUint32 is a helper routine that returns an optional uint32 value.
func GetUint32(p *uint32) uint32 {
	if p == nil {
		return 0
	}
	return *p
}

// GetUint64 is a helper routine that returns an optional uint64 value.
func GetUint64(p *uint64) uint64 {
	if p == nil {
		return 0
	}
	return *p
}

// GetString is a helper routine that returns an optional string value.
func GetString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// EnumName is a helper function to simplify printing protocol buffer enums
// by name.  Given an enum map and a value, it returns a useful string.
func EnumName(m map[int32]string, v int32) string {
	s, ok := m[v]
	if ok {
		return s
	}
	return "unknown_enum_" + strconv.Itoa(int(v))
}

// DebugPrint dumps the encoded data in b in a debugging format with a header
// including the string s. Used in testing but made available for general debugging.
func (o *Buffer) DebugPrint(s string, b []byte) {
	var u uint64

	obuf := o.buf
	index := o.index
	o.buf = b
	o.index = 0
	depth := 0

	fmt.Printf("\n--- %s ---\n", s)

out:
	for {
		for i := 0; i < depth; i++ {
			fmt.Print("  ")
		}

		index := o.index
		if index == len(o.buf) {
			break
		}

		op, err := o.DecodeVarint()
		if err != nil {
			fmt.Printf("%3d: fetching op err %v\n", index, err)
			break out
		}
		tag := op >> 3
		wire := op & 7

		switch wire {
		default:
			fmt.Printf("%3d: t=%3d unknown wire=%d\n",
				index, tag, wire)
			break out

		case WireBytes:
			var r []byte

			r, err = o.DecodeRawBytes(false)
			if err != nil {
				break out
			}
			fmt.Printf("%3d: t=%3d bytes [%d]", index, tag, len(r))
			if len(r) <= 6 {
				for i := 0; i < len(r); i++ {
					fmt.Printf(" %.2x", r[i])
				}
			} else {
				for i := 0; i < 3; i++ {
					fmt.Printf(" %.2x", r[i])
				}
				fmt.Printf(" ..")
				for i := len(r) - 3; i < len(r); i++ {
					fmt.Printf(" %.2x", r[i])
				}
			}
			fmt.Printf("\n")

		case WireFixed32:
			u, err = o.DecodeFixed32()
			if err != nil {
				fmt.Printf("%3d: t=%3d fix32 err %v\n", index, tag, err)
				break out
			}
			fmt.Printf("%3d: t=%3d fix32 %d\n", index, tag, u)

		case WireFixed64:
			u, err = o.DecodeFixed64()
			if err != nil {
				fmt.Printf("%3d: t=%3d fix64 err %v\n", index, tag, err)
				break out
			}
			fmt.Printf("%3d: t=%3d fix64 %d\n", index, tag, u)
			break

		case WireVarint:
			u, err = o.DecodeVarint()
			if err != nil {
				fmt.Printf("%3d: t=%3d varint err %v\n", index, tag, err)
				break out
			}
			fmt.Printf("%3d: t=%3d varint %d\n", index, tag, u)

		case WireStartGroup:
			if err != nil {
				fmt.Printf("%3d: t=%3d start err %v\n", index, tag, err)
				break out
			}
			fmt.Printf("%3d: t=%3d start\n", index, tag)
			depth++

		case WireEndGroup:
			depth--
			if err != nil {
				fmt.Printf("%3d: t=%3d end err %v\n", index, tag, err)
				break out
			}
			fmt.Printf("%3d: t=%3d end\n", index, tag)
		}
	}

	if depth != 0 {
		fmt.Printf("%3d: start-end not balanced %d\n", o.index, depth)
	}
	fmt.Printf("\n")

	o.buf = obuf
	o.index = index
}