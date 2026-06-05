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

func FuzzUnmarshal(f *testing.F) {
	f.Add([]byte{})
	add_all_pb_files(f)

	f.Fuzz(func(t *testing.T, pb []byte) {
		var nothing struct{}
		_ = protobuf3.Unmarshal(pb, &nothing)
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
