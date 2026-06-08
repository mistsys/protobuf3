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
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mistsys/protobuf3/protobuf3"
)

// add all the files matching testdata/fuzz/*.pb to the test's sole input argument
func add_all_pb_files(f *testing.F) {
	pb_files, err := filepath.Glob("testdata/fuzz/*.pb")
	if err != nil {
		f.Errorf("finding testdata/fuzz/*.pb failed: %v", err)
		return
	}
	if len(pb_files) == 0 {
		// make this an error, b/c the files are expected to exist
		f.Errorf("no files found matching testdata/fuzz/*.pb")
		return
	}
	for _, file := range pb_files {
		data, err := os.ReadFile(file)
		if err != nil {
			f.Errorf("failed to read %s: %v", file, err)
		} else {
			f.Add(data)
			f.Logf("added %d byte input from %s", len(data), file)
		}
	}
}

func FuzzDebugPrint(f *testing.F) {
	add_all_pb_files(f)
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, pb []byte) {
		_ = protobuf3.DebugPrint(pb)
	})
}

func FuzzUnmarshalEverything(f *testing.F) {
	f.Add([]byte{})
	add_all_pb_files(f)

	type Everything struct {
		Int    int  `protobuf:"varint,1"`
		Uint   uint `protobuf:"varint,2"`
		Int16  int  `protobuf:"varint,3"`
		Uint16 uint `protobuf:"varint,4"`
		Int32  int  `protobuf:"zigzag32,5"`
		Uint32 uint `protobuf:"varint,6"`
		Int64  int  `protobuf:"varint,7"`
		Uint64 uint `protobuf:"varint,8"`
		Int8   int  `protobuf:"varint,9"`
		Uint8  uint `protobuf:"varint,10"`
		Bool   bool `protobuf:"varint,11"`

		PtrInt    *int  `protobuf:"varint,21"`
		PtrUint   *uint `protobuf:"varint,22"`
		PtrInt16  *int  `protobuf:"varint,23"`
		PtrUint16 *uint `protobuf:"varint,24"`
		PtrInt32  *int  `protobuf:"varint,25"`
		PtrUint32 *uint `protobuf:"varint,26"`
		PtrInt64  *int  `protobuf:"zigzag64,27"`
		PtrUint64 *uint `protobuf:"varint,28"`
		PtrInt8   *int  `protobuf:"zigzag32,29"`
		PtrUint8  *uint `protobuf:"varint,30"`
		PtrBool   *bool `protobuf:"varint,31"`

		SliceInt    []int  `protobuf:"zigzag64,41"`
		SliceUint   []uint `protobuf:"varint,42"`
		SliceInt16  []int  `protobuf:"zigzag32,43"`
		SliceUint16 []uint `protobuf:"varint,44"`
		SliceInt32  []int  `protobuf:"varint,45"`
		SliceUint32 []uint `protobuf:"varint,46"`
		SliceInt64  []int  `protobuf:"varint,47"`
		SliceUint64 []uint `protobuf:"varint,48"`
		SliceInt8   []int  `protobuf:"varint,49"`
		SliceUint8  []uint `protobuf:"varint,50"`
		SliceBool   []bool `protobuf:"varint,51"`

		ArrayInt    [0]int   `protobuf:"varint,61"`
		ArrayUint   [1]uint  `protobuf:"varint,62"`
		ArrayInt16  [2]int   `protobuf:"varint,63"`
		ArrayUint16 [3]uint  `protobuf:"varint,64"`
		ArrayInt32  [4]int   `protobuf:"varint,65"`
		ArrayUint32 [5]uint  `protobuf:"varint,66"`
		ArrayInt64  [6]int   `protobuf:"varint,67"`
		ArrayUint64 [7]uint  `protobuf:"varint,68"`
		ArrayInt8   [8]int   `protobuf:"varint,69"`
		ArrayUint8  [9]uint  `protobuf:"varint,70"`
		ArrayBool   [10]bool `protobuf:"varint,71"`

		I32 int32   `protobuf:"fixed32,81"`
		U32 uint32  `protobuf:"fixed32,82"`
		I64 int64   `protobuf:"fixed64,83"`
		U64 uint64  `protobuf:"fixed64,84"`
		F32 float32 `protobuf:"fixed32,88"`
		F64 float64 `protobuf:"fixed64,89"`

		PI32 *int32   `protobuf:"fixed32,91"`
		PU32 *uint32  `protobuf:"fixed32,92"`
		PI64 *int64   `protobuf:"fixed64,93"`
		PU64 *uint64  `protobuf:"fixed64,94"`
		PF32 *float32 `protobuf:"fixed32,98"`
		PF64 *float64 `protobuf:"fixed64,99"`

		SI32 []int32   `protobuf:"fixed32,73,packed"`
		SU32 []uint32  `protobuf:"fixed32,74,packed"`
		SI64 []int64   `protobuf:"fixed64,75,packed"`
		SU64 []uint64  `protobuf:"fixed64,76,packed"`
		SF32 []float32 `protobuf:"fixed32,80,packed"`
		SF64 []float64 `protobuf:"fixed64,90,packed"`

		String      string   `protobuf:"bytes,100"`
		PtrString   *string  `protobuf:"bytes,101"`
		SliceString []string `protobuf:"bytes,102"`

		MapStr      map[string]struct{} `protobuf:"bytes,110" protobuf_key:"bytes,1" protobuf_val:"bytes,2"`
		MapInt      map[int]struct{}    `protobuf:"bytes,111" protobuf_key:"varint,1" protobuf_val:"bytes,2"`
		MapIntInt   map[int]int         `protobuf:"bytes,112" protobuf_key:"varint,1" protobuf_val:"varint,2"`
		MapIntBytes map[int][]byte      `protobuf:"bytes,113" protobuf_key:"varint,1" protobuf_val:"bytes,2"`
		MapStruct   map[struct {
			S string `protobuf:"bytes,1"`
		}]*struct {
			I int `protobuf:"varint,1"`
		} `protobuf:"bytes,114" protobuf_key:"bytes,1" protobuf_val:"bytes,2"`

		Struct struct {
			Bytes [12]byte `protobuf:"bytes,1"`
		} `protobuf:"bytes,120"`
		PtrStruct *struct {
			Bytes [12]byte `protobuf:"bytes,1"`
		} `protobuf:"bytes,121"`
		SliceStruct []struct {
			Bytes [12]byte `protobuf:"bytes,1"`
		} `protobuf:"bytes,122"`
		ArrayStruct [1]struct {
			Bytes [12]byte `protobuf:"bytes,1"`
		} `protobuf:"bytes,123"`
		SlicePtrStruct []*struct {
			Bytes [12]byte `protobuf:"bytes,1"`
		} `protobuf:"bytes,124"`
		ArrayPtrStruct [2]*struct {
			Bytes [12]byte `protobuf:"bytes,1"`
		} `protobuf:"bytes,125"`
	}
	_, err := protobuf3.GetProperties(reflect.TypeOf((*Everything)(nil)))
	if err != nil {
		f.Fatalf("parsing Everything's protobuf tags failed: %v", err)
	}

	f.Fuzz(func(t *testing.T, pb []byte) {
		var everything Everything
		_ = protobuf3.Unmarshal(pb, &everything)
	})
}

func FuzzNext(f *testing.F) {
	f.Add([]byte{})
	add_all_pb_files(f)

	f.Fuzz(func(t *testing.T, pb []byte) {
		scan_bytes(pb)
	})
}

func scan_bytes(pb []byte) {
	buf := protobuf3.NewBuffer(pb)
	for {
		id, full, val, wt, err := buf.Next()
		if err != nil {
			return
		}
		if id == 0 && full == nil && val == nil && wt == 0 && err == nil {
			return
		}
		if wt == protobuf3.WireBytes {
			scan_bytes(val) // even if the contents are a string, it's ok. we're fuzzing!
		}
	}
}
