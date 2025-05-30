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
 * Routines for encoding data into the wire format for protocol buffers.
 */

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
	"unsafe"
)

// compile with true to get some debug msgs when working on this file
const debug bool = false

// XXXHack enables a backwards compatibility hack to match the canonical golang.go/protobuf error behavior for fields whose names start with XXX_
// This isn't needed unless you are dealing with old protobuf v2 generated types like some unit tests do
var XXXHack = false

// MakeFieldName is a pointer to a function which returns what should be the name of field f in the protobuf definition of type t.
// You can replace this with your own function before calling AsProtobuf[Full]() to control the field names yourself.
var MakeFieldName func(f string, t reflect.Type) string = MakeLowercaseFieldName

// MakeTypeName is a pointer to a function which returns what should be the name of the protobuf message of type t, which is the type
// of a field named f.
var MakeTypeName func(t reflect.Type, f string) string = MakeUppercaseTypeName

// MakePackageName is a pointer to a function which returns what should be the name of the protobuf package given the go package path.
// By default it simply returns the last component of the pkgpath.
var MakePackageName func(pkgpath string) string = MakeSamePackageName

// AsProtobuf3er is the interface which returns the protobuf v3 type equivalent to what the MarshalProtobuf3() method
// encodes. This is optional, but useful when using AsProtobufFull() against types implementing Marshaler.
// `definition` can be "" if the datatype doesn't need a custom definition.
// `imports` is the list of files to import when using this type. The order of imports in the slice is not important, and is not respected.
type AsProtobuf3er interface {
	AsProtobuf3() (name string, definition string, imports []string)
}

// legacy AsProtobuf3() which didn't support imports
type AsV1Protobuf3er interface {
	AsProtobuf3() (name string, definition string)
}

// maxLen is the maximum length possible for a byte array. On a 64-bit target this is (1<<50)-1. On a 32-bit target it is (1<<31)-1
// The tricky part is figuring out in a constant what flavor of target we are on. I could sure use a ?: here. It would be more
// clear than using &^uint(0) to truncate (or not) the upper 32 bits of a constant.
const maxLen = int((1 << (31 + (((50-31)<<32)&uint64(^uint(0)))>>32)) - 1) // experiments with go1.7 on amd64 show any larger size causes the compiler to error

// Constants that identify the encoding of a value on the wire.
const (
	WireVarint     = WireType(0)
	WireFixed64    = WireType(1)
	WireBytes      = WireType(2)
	WireStartGroup = WireType(3) // legacy from protobuf v2. Groups are not used in protobuf v3
	WireEndGroup   = WireType(4) // legacy...
	WireFixed32    = WireType(5)
)

type WireType byte

// mapping from WireType to string
var wireTypeNames = []string{WireVarint: "varint", WireFixed64: "fixed64", WireBytes: "bytes", WireStartGroup: "start-group", WireEndGroup: "end-group", WireFixed32: "fixed32"}

func (wt WireType) String() string {
	if int(wt) < len(wireTypeNames) {
		return wireTypeNames[wt]
	}
	return fmt.Sprintf("WireType(%d)", byte(wt))
}

// Encoders are defined in encode.go
// An encoder outputs the full representation of a field, including its
// tag and encoder type.
type encoder func(p *Buffer, prop *Properties, base unsafe.Pointer)

// A valueEncoder encodes a single integer in a particular encoding.
type valueEncoder func(o *Buffer, x uint64)

// Decoders are defined in decode.go
// A decoder creates a value from its wire representation.
// Unrecognized subelements are saved in unrec.
type decoder func(p *Buffer, prop *Properties, base unsafe.Pointer) error

// A valueDecoder decodes a single integer in a particular encoding.
type valueDecoder func(o *Buffer) (x uint64, err error)

// A valueCounter looks ahead and counts the number of values in a buffer.
type valueCounter func(o *Buffer) (n int)

// StructProperties represents properties for all the fields of a struct.
type StructProperties struct {
	props    []Properties // properties for each field encoded in protobuf, ordered by tag id
	reserved []uint32     // all the reserved tags
}

// Implement the sorting interface so we can sort the fields in tag order, as recommended by the spec.
// See encode.go, (*Buffer).enc_struct.
func (sp *StructProperties) Len() int { return len(sp.props) }
func (sp *StructProperties) Less(i, j int) bool {
	return sp.props[i].Tag < sp.props[j].Tag
}
func (sp *StructProperties) Swap(i, j int) { sp.props[i], sp.props[j] = sp.props[j], sp.props[i] }

// returns the properties into protobuf v3 format, suitable for feeding back into the protobuf compiler.
func (sp *StructProperties) asProtobuf(t reflect.Type, tname string) string {
	lines := []string{fmt.Sprintf("message %s {", tname)}
	for i := range sp.props {
		pp := &sp.props[i]
		if pp.Wire != "-" {
			lines = append(lines, fmt.Sprintf("  %s%s %s = %d;", pp.optional(), pp.asProtobuf, pp.protobufFieldName(t), pp.Tag))
		}
	}
	if len(sp.reserved) != 0 {
		var b strings.Builder
		b.WriteString("  reserved ")
		sep := ""
		for _, r := range sp.reserved {
			fmt.Fprintf(&b, "%s%d", sep, r)
			sep = ", "
		}
		b.WriteByte(';')
		lines = append(lines, b.String())
	}
	lines = append(lines, "}")
	return strings.Join(lines, "\n")
}

// Reserved is a special type used to indicate reserved protobuf IDs. Instead of the usual protobuf tag, the tag
// consists of a comma separated list of reserved protobuf IDs. Using these IDs elsewhere causes an error, and
// they are listed in the `reserved` section by asProtobuf.
// At the moment we only support reserveing IDs. If you need to reserve field names then you'll have to implement it.
type Reserved [0]byte

var reservedType = reflect.TypeOf((*Reserved)(nil)).Elem()

// parse the protobuf tag of a Reserved field
func (sp *StructProperties) parseReserved(tag string) error {
	for _, s := range strings.Split(tag, ",") {
		tag, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("protobuf3: invalid reserved tag id %q: %v", s, err)
		}
		if tag <= 0 { // catch any negative or 0 values
			return fmt.Errorf("protobuf3: reserved tag id %q out of range", s)
		}
		sp.reserved = append(sp.reserved, uint32(tag))
	}
	return nil
}

// return the name of this field in protobuf
func (p *Properties) protobufFieldName(struct_type reflect.Type) string {
	// the "name=" tag overrides any computed field name. That lets us automate any manual fixup of names we might need.
	for _, t := range strings.Split(p.Wire, ",") {
		if strings.HasPrefix(t, "name=") {
			return t[5:]
		}
	}

	return MakeFieldName(p.Name, struct_type)
}

// return the protobuf "optional" field value (with a whitespace suffix for convenience)
func (p *Properties) optional() string {
	// NOTE: we allow "optional" to be applied to all field types, even those for which, in the Go struct definition, there is no good way to tell the difference
	// between the default value and absence of the value. (an int32 for example, or pretty much nothing but pointers and maps (which are pointers underneath))
	// What isOptional does is apply
	if p.isOptional {
		return "optional "
	}
	return ""
}

// MakeLowercaseFieldName returns a reasonable lowercase field name
func MakeLowercaseFieldName(f string, t reflect.Type) string {
	// To make people who use other languages happy it would be nice if our field names were like most and were lowercase.
	// (In addition, since we use the name of fields with anonymous types as the name of the anonmymous types, we need to
	// alter those fields (or the type's name) so there isn't a collision.)
	// Converting "XxxYYzz" to "xxx_yyy_zz" seems to be reasonable for most fields names.
	// If the name already has any '_' it then I just lowercase it without inserting any more.

	if strings.ContainsRune(f, '_') {
		return strings.ToLower(f)
	}

	buf := make([]byte, 2*len(f)+4) // 2x is enough for every 2nd rune to be a '_'. +4 is enough room for anything EncodeRune() might emit
	j := 0
	prev_was_upper := true // initial condition happens to prevent the 1st rune (which is almost certainly uppercase) from getting prefixed with _
	for _, r := range f {
		if unicode.IsUpper(r) {
			// lowercase r, and prepend a '_' if this is a good place to break up the name
			if !prev_was_upper {
				buf[j] = '_'
				j++
			}
			r = unicode.ToLower(r)
			prev_was_upper = true
		} else if unicode.IsLower(r) {
			prev_was_upper = false
		} // else leave prev_was_upper alone. This rule handles some edge condition names better ("L2TP" for instance, which otherwise would be named "l2_tp")
		j += utf8.EncodeRune(buf[j:], r)
	}

	return string(buf[:j])

	// PS I tried doing things like lowercasing and inserting a '_' before each group of uppercase chars.
	// It didn't do well with field names our software was using. Yet
}

// returns the type expressed in protobuf v3 format, suitable for feeding back into the protobuf compiler.
func AsProtobuf(t reflect.Type) (string, error) {
	// dig down through any pointer types
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	prop, err := GetProperties(t)
	if err != nil {
		return "# Error: " + err.Error(), err // cause an error in the protobuf compiler if the input is used
	}
	return prop.asProtobuf(t, t.Name()), nil
}

// given the full path of the package of the 1st type passed to AsProtobufFull(), return
// the last component of the package path to be used as the package name.
func MakeSamePackageName(pkgpath string) string {
	slash := strings.LastIndexByte(pkgpath, '/')
	pkg := pkgpath
	if slash >= 0 {
		pkg = pkgpath[slash+1:]
	}
	return pkg
}

// returns the type expressed in protobuf v3 format, including all dependent types and imports
func AsProtobufFull(t reflect.Type, more ...reflect.Type) (string, error) {
	return AsProtobufFull2(t, nil, more...)
}

// returns the type expressed in protobuf v3 format, including all dependent types and imports
// extra_headers allow the caller to specify headers they want inserted after the `package` line.
func AsProtobufFull2(t reflect.Type, extra_package_headers []string, more ...reflect.Type) (string, error) {
	// dig down through any pointer types on the first type, since we'll use that one to determine the package
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	todo := make(map[reflect.Type]struct{})
	discovered := make(map[reflect.Type]struct{})

	pkgpath := t.PkgPath()

	headers := []string{
		fmt.Sprintf("// protobuf definitions generated by protobuf3.AsProtobufFull(%s.%s)", pkgpath, t.Name()),
		"",
		`syntax = "proto3";`,
		"",
	}
	imported := make(map[string]struct{}) // the set of all imported files
	var body []string

	if pkgpath != "" {
		headers = append(headers, fmt.Sprintf("package %s;", MakePackageName(pkgpath)))
	} // else the type is synthesized and lacks a path; humans need to deal with the output (after all they caused this)

	headers = append(headers, extra_package_headers...)

	// place all the arguments in the todo table to start things off
	todo[t] = struct{}{}
	for _, t := range more {
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		todo[t] = struct{}{}
	}

	// and lather/rinse/repeat until we've discovered all the types
	var first_err error
	for len(todo) != 0 {
		for t := range todo {
			// move t from todo to discovered
			delete(todo, t)
			discovered[t] = struct{}{}

			// add to todo any new, non-anonymous types used by struct t's fields
			p, err := GetProperties(t)
			if err != nil {
				if first_err == nil {
					first_err = err
				}
				body = append(body, "# Error: "+err.Error()) // cause an error in the protobuf compiler
				continue
			}
			for i := range p.props {
				pp := &p.props[i]
				tt := pp.Subtype()
				if tt != nil {
					if _, ok := discovered[tt]; !ok {
						// it's a new type of field
						switch {
						case pp.isAppender || pp.isMarshaler:
							// we can't recurse further into a custom type
							discovered[tt] = struct{}{}
						case isAsProtobuf3er(reflect.PtrTo(tt)) || isAsV1Protobuf3er(reflect.PtrTo(tt)):
							// this type has a custom protobuf definition. it presumably encodes its own types
							discovered[tt] = struct{}{}
						case tt.Kind() == reflect.Struct:
							switch tt {
							case time_Time_type:
								// the timestamp type get defined by an import of timestamp.proto
								discovered[tt] = struct{}{}
							default:
								// put this new type in the todo table if it isn't already there
								// (the duplicate insert when it is already present is a no-op)
								todo[tt] = struct{}{}
							}
						case tt == time_Duration_type:
							// the duration type get defined by an import of duration.proto
							discovered[tt] = struct{}{}
						}
					}
				}
			}

			// and we must break since todo has possibly been altered
			break
		}
	}

	// now that the types we need have all been discovered, sort their names and generate the .proto source
	// the reason we do this in 2 passes is so that the output is consistent from run to run, and diff'able
	// across runs with incremental differences.

	ordered := make(Types, 0, len(discovered))
	for t := range discovered {
		if t.Name() != "" { // skip anonymous types
			ordered = append(ordered, t)
		}
	}
	sort.Sort(ordered)

	for _, t := range ordered {
		// generate type t's protobuf definition
		ptr_t := reflect.PtrTo(t)

		var definition string
		var imports []string
		var external bool
		switch {
		case t == time_Time_type:
			// the timestamp type gets defined by an import
			imports = []string{"google/protobuf/timestamp.proto"}
			external = true

		case t == time_Duration_type:
			// the duration type gets defined by an import
			imports = []string{"google/protobuf/duration.proto"}
			external = true

		case isAppender(ptr_t) || isMarshaler(ptr_t):
			// we can't define a custom type automatically. see if it can tell us, and otherwise remind the human to do it.
			switch {
			case isAsProtobuf3er(ptr_t):
				_, definition, imports = reflect.NewAt(t, nil).Interface().(AsProtobuf3er).AsProtobuf3()
			case isAsV1Protobuf3er(ptr_t):
				_, definition = reflect.NewAt(t, nil).Interface().(AsV1Protobuf3er).AsProtobuf3()
			default:
				headers = append(headers, fmt.Sprintf("// TODO supply the definition of message %s", t.Name()))
			}
			if definition == "" {
				// the type doesn't need any additional definition (its name was sufficient)
				external = true
			}

		case isAsProtobuf3er(ptr_t):
			_, definition, imports = reflect.NewAt(t, nil).Interface().(AsProtobuf3er).AsProtobuf3()

		case isAsV1Protobuf3er(ptr_t):
			_, definition = reflect.NewAt(t, nil).Interface().(AsV1Protobuf3er).AsProtobuf3()
		}

		for _, imp := range imports {
			imported[imp] = struct{}{}
		}
		if !external {
			if definition == "" {
				var err error
				definition, err = AsProtobuf(t)
				if err != nil {
					if first_err == nil {
						first_err = err
					}
					// and definition already contains the error
				}
			}
			if definition != "" {
				body = append(body, "") // put a blank line between each message definition
				body = append(body, definition)
			}
		} // else the type doesn't need any additional definition (its name and imports are sufficient)
	}

	// generate the import header lines. to make the output reproducible, sort them
	// (if someone the order of import headers becomes important we'll have to do something fancier, but for now they are well written and independant)
	if len(imported) != 0 {
		import_headers := make([]string, 0, len(imported))
		for imp := range imported {
			import_headers = append(import_headers, fmt.Sprintf("import %q;", imp))
		}
		sort.Strings(import_headers)
		headers = append(headers, "")
		headers = append(headers, import_headers...)
	}

	return strings.Join(append(headers, body...), "\n"), first_err
}

type Types []reflect.Type

func (ts Types) Len() int           { return len(ts) }
func (ts Types) Swap(i, j int)      { ts[i], ts[j] = ts[j], ts[i] }
func (ts Types) Less(i, j int) bool { return ts[i].Name() < ts[j].Name() } // sort types by their names

// Properties represents the protocol-specific behavior of a single struct field.
type Properties struct {
	Name       string // name of the field, for error messages
	Wire       string
	asProtobuf string // protobuf v3 type for this field (or something equivalent, since we can't figure it out perfectly from the Go field type and tags)
	Tag        uint32
	WireType   WireType // the wiretype we expect to find in the messages. This is the wiretype from the protobuf: tag except in the case of repeated data, which is always packed in protobuf v3 and uses WireBytes

	enc         encoder
	valEnc      valueEncoder      // set for bool and numeric types only
	offset      uintptr           // byte offset of this field within the struct
	tagcode     string            // encoding of EncodeVarint((Tag<<3)|WireType), stored in a string for efficiency
	stype       reflect.Type      // set for struct types and time.Duration only
	sprop       *StructProperties // set for struct types only
	isMarshaler bool              // true if the type implements Marshaler and marshals/unmarshals itself
	isAppender  bool              // true if the type implements Appender and helps marshal itself into a *Buffer
	isOptional  bool              // true if the "optional" attribute was specified in the protobuf: tag. This code (for the obvious reason that it doesn't generate the structs we unmarshal into) largely ignores "optional", but it is copied into the generated .proto, and protoc or some other protobuf code generator will obey it

	mtype    reflect.Type // set for map types only
	mkeyprop *Properties  // set for map types only
	mvalprop *Properties  // set for map types only

	length uint // set for array types only

	dec    decoder
	valDec valueDecoder // set for bool and numeric types only
	valCnt valueCounter // set for bool and numeric types only
}

// String formats the properties in the protobuf struct field tag style.
func (p *Properties) String() string {
	if p.stype != nil {
		return fmt.Sprintf("%s %s (%s)", p.Wire, p.Name, p.stype.Name())
	}
	if p.mtype != nil {
		return fmt.Sprintf("%s %s (%s)", p.Wire, p.Name, p.mtype.Name())
	}
	return fmt.Sprintf("%s %s", p.Wire, p.Name)
}

// returns the inner type, or nil
func (p *Properties) Subtype() reflect.Type {
	return p.stype
}

// IntEncoder enumerates the different ways of encoding integers in Protobuf v3
type IntEncoder int

const (
	UnknownEncoder IntEncoder = iota // make the zero-value be different from any valid value so I can tell it is not set
	VarintEncoder
	Fixed32Encoder
	Fixed64Encoder
	Zigzag32Encoder
	Zigzag64Encoder
)

// Parse populates p by parsing a string in the protobuf struct field tag style.
func (p *Properties) Parse(s string) (IntEncoder, bool, error) {
	p.Wire = s

	// "bytes,49,rep,..."
	fields := strings.Split(s, ",")

	if len(fields) < 2 {
		if len(fields) > 0 && fields[0] == "-" {
			// `protobuf="-"` is used to mark fields which should be skipped by the protobuf encoder (this is same mark as is used by the std encoding/json package)
			return 0, true, nil
		}
		return 0, true, fmt.Errorf("protobuf3: tag of %q has too few fields: %q", p.Name, s)
	}

	var enc IntEncoder
	switch fields[0] {
	case "varint":
		p.valEnc = (*Buffer).EncodeVarint
		p.valDec = (*Buffer).DecodeVarint
		p.valCnt = (*Buffer).CountVarints
		p.WireType = WireVarint
		enc = VarintEncoder
	case "fixed32":
		p.valEnc = (*Buffer).EncodeFixed32
		p.valDec = (*Buffer).DecodeFixed32
		p.valCnt = (*Buffer).CountFixed32s
		p.WireType = WireFixed32
		enc = Fixed32Encoder
	case "fixed64":
		p.valEnc = (*Buffer).EncodeFixed64
		p.valDec = (*Buffer).DecodeFixed64
		p.valCnt = (*Buffer).CountFixed64s
		p.WireType = WireFixed64
		enc = Fixed64Encoder
	case "zigzag32":
		p.valEnc = (*Buffer).EncodeZigzag32
		p.valDec = (*Buffer).DecodeZigzag32
		p.valCnt = (*Buffer).CountVarints // zigzag uses varint encoding
		p.WireType = WireVarint
		enc = Zigzag32Encoder
	case "zigzag64":
		p.valEnc = (*Buffer).EncodeZigzag64
		p.valDec = (*Buffer).DecodeZigzag64
		p.valCnt = (*Buffer).CountVarints // zigzag uses varint encoding
		p.WireType = WireVarint
		enc = Zigzag64Encoder
	case "bytes":
		// no numeric converter for non-numeric types
		p.WireType = WireBytes
	default:
		return 0, false, fmt.Errorf("protobuf3: tag of %q has unknown wire type: %q", p.Name, s)
	}

	tag, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, false, fmt.Errorf("protobuf3: tag id of %q invalid: %s: %s", p.Name, s, err.Error())
	}
	if tag <= 0 { // catch any negative or 0 values
		return 0, false, fmt.Errorf("protobuf3: tag id of %q out of range: %s", p.Name, s)
	}
	p.Tag = uint32(tag)

	for _, field := range fields[2:] {
		switch field {
		case "optional":
			p.isOptional = true
			// and we don't care about any other fields
			// (if you don't mark slices/arrays/maps with ",rep" that's your own problem; this encoder always repeats those types)
		}
	}

	return enc, false, nil
}

// Initialize the fields for encoding and decoding.
func (p *Properties) setEncAndDec(t1 reflect.Type, f *reflect.StructField, name string, int_encoder IntEncoder) error {
	var err error
	p.enc = nil
	p.dec = nil
	wire := p.WireType

	// since so many cases need it, decode int_encoder into a string now
	var int32_encoder_txt, uint32_encoder_txt,
		int64_encoder_txt, uint64_encoder_txt string
	switch int_encoder {
	case VarintEncoder:
		uint32_encoder_txt = "uint32"
		int32_encoder_txt = uint32_encoder_txt[1:] // strip the 'u' off
		uint64_encoder_txt = "uint64"
		int64_encoder_txt = uint64_encoder_txt[1:] // strip the 'u' off
	case Fixed32Encoder:
		int32_encoder_txt = "sfixed32"
		uint32_encoder_txt = int32_encoder_txt[1:] // strip the 's' off
	case Fixed64Encoder:
		int64_encoder_txt = "sfixed64"
		uint64_encoder_txt = int64_encoder_txt[1:] // strip the 's' off
	case Zigzag32Encoder:
		int32_encoder_txt = "sint32"
	case Zigzag64Encoder:
		int64_encoder_txt = "sint64"
	}

	// can t1 marshal itself?
	ptr_t1 := reflect.PtrTo(t1)
	if isAppender(ptr_t1) {
		p.isAppender = true
		p.stype = t1
		p.enc = (*Buffer).enc_appender
		p.dec = (*Buffer).dec_unmarshaler
		p.asProtobuf = p.stypeAsProtobuf()
	} else if isMarshaler(ptr_t1) {
		p.isMarshaler = true
		p.stype = t1
		p.enc = (*Buffer).enc_marshaler
		p.dec = (*Buffer).dec_unmarshaler
		p.asProtobuf = p.stypeAsProtobuf()
	} else {
		switch t1.Kind() {
		default:
			return fmt.Errorf("protobuf3: no encoder/decoder for type %s", t1.Name())

		// proto3 scalar types

		case reflect.Bool:
			p.enc = (*Buffer).enc_bool
			p.dec = (*Buffer).dec_bool
			p.asProtobuf = "bool"
			if p.valEnc == nil {
				return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
			}
		case reflect.Int:
			p.enc = (*Buffer).enc_int
			p.dec = (*Buffer).dec_int
			p.asProtobuf = int32_encoder_txt
			if p.valEnc == nil {
				return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
			}
		case reflect.Uint:
			p.enc = (*Buffer).enc_uint
			p.dec = (*Buffer).dec_int // signness doesn't matter when decoding. either the top bit is set or it isn't
			p.asProtobuf = uint32_encoder_txt
			if p.valEnc == nil {
				return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
			}
		case reflect.Int8:
			p.enc = (*Buffer).enc_int8
			p.dec = (*Buffer).dec_int8
			p.asProtobuf = int32_encoder_txt
			if p.valEnc == nil {
				return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
			}
		case reflect.Uint8:
			p.enc = (*Buffer).enc_uint8
			p.dec = (*Buffer).dec_int8
			p.asProtobuf = uint32_encoder_txt
			if p.valEnc == nil {
				return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
			}
		case reflect.Int16:
			p.enc = (*Buffer).enc_int16
			p.dec = (*Buffer).dec_int16
			p.asProtobuf = int32_encoder_txt
			if p.valEnc == nil {
				return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
			}
		case reflect.Uint16:
			p.enc = (*Buffer).enc_uint16
			p.dec = (*Buffer).dec_int16
			p.asProtobuf = uint32_encoder_txt
			if p.valEnc == nil {
				return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
			}
		case reflect.Int32:
			p.enc = (*Buffer).enc_int32
			p.dec = (*Buffer).dec_int32
			p.asProtobuf = int32_encoder_txt
			if p.valEnc == nil { // note it is safe, though peculiar, for an int32 to have a wiretype of fixed64
				return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
			}
		case reflect.Uint32:
			p.enc = (*Buffer).enc_uint32
			p.dec = (*Buffer).dec_int32
			p.asProtobuf = uint32_encoder_txt
			if p.valEnc == nil {
				return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
			}
		case reflect.Int64:
			// this might be a time.Duration, or it might be an ordinary int64
			// if the caller wants a time.Duration to be encoded as a protobuf Duration then the
			// wiretype must be WireBytes. Otherwise they'll get the int64 encoding they've selected.
			if p.WireType == WireBytes && t1 == time_Duration_type {
				p.stype = time_Duration_type
				p.enc = (*Buffer).enc_time_Duration
				p.dec = (*Buffer).dec_time_Duration
				p.asProtobuf = "google.protobuf.Duration"
			} else {
				p.enc = (*Buffer).enc_int64
				p.dec = (*Buffer).dec_int64
				p.asProtobuf = int64_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			}
		case reflect.Uint64:
			p.enc = (*Buffer).enc_int64
			p.dec = (*Buffer).dec_int64
			p.asProtobuf = uint64_encoder_txt
			if p.valEnc == nil {
				return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
			}
		case reflect.Float32:
			p.enc = (*Buffer).enc_uint32 // can just treat them as bits
			p.dec = (*Buffer).dec_int32
			p.asProtobuf = "float"
			if p.valEnc == nil || wire != WireFixed32 { // the way we encode and decode float32 at the moment means we can only support fixed32
				return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
			}
		case reflect.Float64:
			p.enc = (*Buffer).enc_int64 // can just treat them as bits
			p.dec = (*Buffer).dec_int64
			p.asProtobuf = "double"
			if p.valEnc == nil || wire != WireFixed64 { // the way we encode and decode float64 at the moment means we can only support fixed64
				return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
			}
		case reflect.String:
			p.enc = (*Buffer).enc_string
			p.dec = (*Buffer).dec_string
			p.asProtobuf = "string"
			if wire != WireBytes {
				return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
			}

		case reflect.Struct:
			p.stype = t1
			p.sprop, err = getPropertiesLocked(t1)
			if err != nil {
				return err
			}
			p.asProtobuf = p.stypeAsProtobuf()
			switch t1 {
			case time_Time_type:
				p.enc = (*Buffer).enc_struct_message // time.Time encodes as a struct with 1 (made up) field
				p.dec = (*Buffer).dec_time_Time      // but it decodes with a custom function
			default:
				p.enc = (*Buffer).enc_struct_message
				p.dec = (*Buffer).dec_struct_message
			}
			if wire != WireBytes {
				return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
			}

		case reflect.Ptr:
			t2 := t1.Elem()
			// can the target of the pointer marshal itself?
			if isAppender(t1) {
				p.stype = t2
				p.isAppender = true
				p.enc = (*Buffer).enc_ptr_appender
				p.dec = (*Buffer).dec_ptr_unmarshaler
				p.asProtobuf = p.stypeAsProtobuf()
				break
			}
			if isMarshaler(t1) {
				p.stype = t2
				p.isMarshaler = true
				p.enc = (*Buffer).enc_ptr_marshaler
				p.dec = (*Buffer).dec_ptr_unmarshaler
				p.asProtobuf = p.stypeAsProtobuf()
				break
			}
			if isAsProtobuf3er(t1) || isAsV1Protobuf3er(t1) {
				p.stype = t2
			}

			switch t2.Kind() {
			default:
				return fmt.Errorf("protobuf3: no encoder function for %s -> %s", t1, t2.Name())

			case reflect.Bool:
				p.enc = (*Buffer).enc_ptr_bool
				p.dec = (*Buffer).dec_ptr_bool
				p.asProtobuf = "bool"
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Int:
				p.enc = (*Buffer).enc_ptr_int
				p.dec = (*Buffer).dec_ptr_int
				p.asProtobuf = int32_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Uint:
				p.enc = (*Buffer).enc_ptr_uint
				p.dec = (*Buffer).dec_ptr_int // signness doesn't matter when decoding. either the top bit is set or it isn't
				p.asProtobuf = uint32_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Int8:
				p.enc = (*Buffer).enc_ptr_int8
				p.dec = (*Buffer).dec_ptr_int8
				p.asProtobuf = int32_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Uint8:
				p.enc = (*Buffer).enc_ptr_uint8
				p.dec = (*Buffer).dec_ptr_int8
				p.asProtobuf = uint32_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Int16:
				p.enc = (*Buffer).enc_ptr_int16
				p.dec = (*Buffer).dec_ptr_int16
				p.asProtobuf = int32_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Uint16:
				p.enc = (*Buffer).enc_ptr_uint16
				p.dec = (*Buffer).dec_ptr_int16
				p.asProtobuf = uint32_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Int32:
				p.enc = (*Buffer).enc_ptr_int32
				p.dec = (*Buffer).dec_ptr_int32
				p.asProtobuf = int32_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Uint32:
				p.enc = (*Buffer).enc_ptr_uint32
				p.dec = (*Buffer).dec_ptr_int32
				p.asProtobuf = uint32_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Int64:
				if p.WireType == WireBytes && t2 == time_Duration_type {
					p.stype = time_Duration_type
					p.enc = (*Buffer).enc_ptr_time_Duration
					p.dec = (*Buffer).dec_ptr_time_Duration
					p.asProtobuf = "google.protobuf.Duration"
				} else {
					p.enc = (*Buffer).enc_ptr_int64
					p.dec = (*Buffer).dec_ptr_int64
					p.asProtobuf = int64_encoder_txt
					if p.valEnc == nil {
						return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
					}
				}
			case reflect.Uint64:
				p.enc = (*Buffer).enc_ptr_int64
				p.dec = (*Buffer).dec_ptr_int64
				p.asProtobuf = uint64_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Float32:
				p.enc = (*Buffer).enc_ptr_uint32 // can just treat them as bits
				p.dec = (*Buffer).dec_ptr_int32
				p.asProtobuf = "float"
				if p.valEnc == nil || wire != WireFixed32 { // the way we encode and decode float32 at the moment means we can only support fixed32
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Float64:
				p.enc = (*Buffer).enc_ptr_int64 // can just treat them as bits
				p.dec = (*Buffer).dec_ptr_int64
				p.asProtobuf = "double"
				if p.valEnc == nil || wire != WireFixed64 { // the way we encode and decode float64 at the moment means we can only support fixed64
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.String:
				p.enc = (*Buffer).enc_ptr_string
				p.dec = (*Buffer).dec_ptr_string
				p.asProtobuf = "string"
				if wire != WireBytes {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Struct:
				p.stype = t2
				p.sprop, err = getPropertiesLocked(t2)
				if err != nil {
					return err
				}
				p.asProtobuf = p.stypeAsProtobuf()
				p.enc = (*Buffer).enc_ptr_struct_message
				switch {
				case t2 == time_Time_type:
					p.dec = (*Buffer).dec_ptr_time_Time
				default:
					p.dec = (*Buffer).dec_ptr_struct_message
				}
				if wire != WireBytes {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}

				// what about *Slice and *Array types? Fill them in when we need them.
			}

		case reflect.Slice:
			// can elements of the slice marshal themselves?
			t2 := t1.Elem()
			if isAppender(reflect.PtrTo(t2)) {
				p.isAppender = true
				p.stype = t2
				p.enc = (*Buffer).enc_slice_appender
				p.dec = (*Buffer).dec_slice_unmarshaler
				p.asProtobuf = "repeated " + p.stypeAsProtobuf()
				break
			}
			if isMarshaler(reflect.PtrTo(t2)) {
				p.isMarshaler = true
				p.stype = t2
				p.enc = (*Buffer).enc_slice_marshaler
				p.dec = (*Buffer).dec_slice_unmarshaler
				p.asProtobuf = "repeated " + p.stypeAsProtobuf()
				break
			}

			switch t2.Kind() {
			default:
				return fmt.Errorf("protobuf3: no slice encoder for %s = []%s", t1.Name(), t2.Name())

			case reflect.Bool:
				p.enc = (*Buffer).enc_slice_packed_bool
				p.dec = (*Buffer).dec_slice_packed_bool
				wire = WireBytes // packed=true is implied in protobuf v3
				p.asProtobuf = "repeated bool"
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Int:
				p.enc = (*Buffer).enc_slice_packed_int
				p.dec = (*Buffer).dec_slice_packed_int
				wire = WireBytes // packed=true...
				p.asProtobuf = "repeated " + int32_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Uint:
				p.enc = (*Buffer).enc_slice_packed_uint
				p.dec = (*Buffer).dec_slice_packed_int
				wire = WireBytes // packed=true...
				p.asProtobuf = "repeated " + uint32_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Int8:
				p.enc = (*Buffer).enc_slice_packed_int8
				p.dec = (*Buffer).dec_slice_packed_int8
				wire = WireBytes // packed=true...
				p.asProtobuf = "repeated " + int32_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Uint8:
				p.enc = (*Buffer).enc_slice_byte
				p.dec = (*Buffer).dec_slice_byte
				wire = WireBytes // packed=true... even for integers
				p.asProtobuf = "bytes"
			case reflect.Int16:
				p.enc = (*Buffer).enc_slice_packed_int16
				p.dec = (*Buffer).dec_slice_packed_int16
				wire = WireBytes // packed=true...
				p.asProtobuf = "repeated " + int32_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Uint16:
				p.enc = (*Buffer).enc_slice_packed_uint16
				p.dec = (*Buffer).dec_slice_packed_int16
				wire = WireBytes // packed=true...
				p.asProtobuf = "repeated " + uint32_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Int32:
				p.enc = (*Buffer).enc_slice_packed_int32
				p.dec = (*Buffer).dec_slice_packed_int32
				wire = WireBytes // packed=true...
				p.asProtobuf = "repeated " + int32_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Uint32:
				p.enc = (*Buffer).enc_slice_packed_uint32
				p.dec = (*Buffer).dec_slice_packed_int32
				wire = WireBytes // packed=true...
				p.asProtobuf = "repeated " + uint32_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Int64:
				if p.WireType == WireBytes && t2 == time_Duration_type {
					p.stype = time_Duration_type
					p.enc = (*Buffer).enc_slice_time_Duration
					p.dec = (*Buffer).dec_slice_time_Duration
					p.asProtobuf = "repeated google.protobuf.Duration"
				} else {
					p.enc = (*Buffer).enc_slice_packed_int64
					p.dec = (*Buffer).dec_slice_packed_int64
					wire = WireBytes // packed=true...
					p.asProtobuf = "repeated " + int64_encoder_txt
					if p.valEnc == nil {
						return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
					}
				}
			case reflect.Uint64:
				p.enc = (*Buffer).enc_slice_packed_int64
				p.dec = (*Buffer).dec_slice_packed_int64
				wire = WireBytes // packed=true...
				p.asProtobuf = "repeated " + int64_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Float32:
				// can just treat them as bits
				p.enc = (*Buffer).enc_slice_packed_uint32
				p.dec = (*Buffer).dec_slice_packed_int32
				p.asProtobuf = "repeated float"
				if p.valEnc == nil || wire != WireFixed32 { // the way we encode and decode float32 at the moment means we can only support fixed32
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
				wire = WireBytes // packed=true...
			case reflect.Float64:
				// can just treat them as bits
				p.enc = (*Buffer).enc_slice_packed_int64
				p.dec = (*Buffer).dec_slice_packed_int64
				p.asProtobuf = "repeated double"
				if p.valEnc == nil || wire != WireFixed64 { // the way we encode and decode float64 at the moment means we can only support fixed64
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
				wire = WireBytes // packed=true...
			case reflect.String:
				p.enc = (*Buffer).enc_slice_string
				p.dec = (*Buffer).dec_slice_string
				p.asProtobuf = "repeated string"
				if wire != WireBytes {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Struct:
				p.stype = t2
				p.sprop, err = getPropertiesLocked(t2)
				if err != nil {
					return err
				}
				p.isAppender = isAppender(reflect.PtrTo(t2))
				p.isMarshaler = isMarshaler(reflect.PtrTo(t2))
				p.enc = (*Buffer).enc_slice_struct_message
				p.dec = (*Buffer).dec_slice_struct_message
				p.asProtobuf = "repeated " + p.stypeAsProtobuf()
				if wire != WireBytes {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Ptr:
				switch t3 := t2.Elem(); t3.Kind() {
				default:
					return fmt.Errorf("protobuf3: no ptr encoder for %s -> %s -> %s", t1.Name(), t2.Name(), t3.Name())

				case reflect.Struct:
					p.stype = t3
					p.sprop, err = getPropertiesLocked(t3)
					if err != nil {
						return err
					}
					p.isAppender = isAppender(t2)
					p.isMarshaler = isMarshaler(t2)
					p.enc = (*Buffer).enc_slice_ptr_struct_message
					p.dec = (*Buffer).dec_slice_ptr_struct_message
					p.asProtobuf = "repeated " + p.stypeAsProtobuf()
					if wire != WireBytes {
						return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
					}
				}
			case reflect.Slice:
				switch t2.Elem().Kind() {
				default:
					return fmt.Errorf("protobuf3: no slice elem encoder for %s -> %s -> %s", t1.Name(), t2.Name(), t2.Elem().Name())

				case reflect.Uint8:
					p.enc = (*Buffer).enc_slice_slice_byte
					p.dec = (*Buffer).dec_slice_slice_byte
					p.asProtobuf = "repeated bytes"
				}
			}

		case reflect.Array:
			p.length = uint(t1.Len())

			if p.length == 0 {
				// save checking the array length at encode-time by doing it now
				// a zero-length array will always encode as nothing
				p.enc = (*Buffer).enc_nothing
				p.dec = (*Buffer).dec_nothing
				break
			}

			t2 := t1.Elem()
			if isAppender(reflect.PtrTo(t2)) {
				// elements of the array can marshal themselves
				p.isAppender = true
				p.stype = t2
				p.enc = (*Buffer).enc_array_appender
				p.dec = (*Buffer).dec_array_unmarshaler
				p.asProtobuf = "repeated " + p.stypeAsProtobuf()
				break
			}
			if isMarshaler(reflect.PtrTo(t2)) {
				// elements of the array can marshal themselves
				p.isMarshaler = true
				p.stype = t2
				p.enc = (*Buffer).enc_array_marshaler
				p.dec = (*Buffer).dec_array_unmarshaler
				p.asProtobuf = "repeated " + p.stypeAsProtobuf()
				break
			}

			switch t2.Kind() {
			default:
				return fmt.Errorf("protobuf3: no array encoder for %s = %s", t1.Name(), t2.Name())

			case reflect.Bool:
				p.enc = (*Buffer).enc_array_packed_bool
				p.dec = (*Buffer).dec_array_packed_bool
				wire = WireBytes // packed=true is implied in protobuf v3
				p.asProtobuf = "repeated bool"
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Int8:
				p.enc = (*Buffer).enc_array_packed_int8
				p.dec = (*Buffer).dec_array_packed_int8
				wire = WireBytes // packed=true...
				p.asProtobuf = "repeated " + int32_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Uint8:
				// arrays of uint8 have a special type in protobuf: "bytes"
				p.enc = (*Buffer).enc_array_byte
				p.dec = (*Buffer).dec_array_byte
				p.asProtobuf = "bytes"
			case reflect.Int16:
				p.enc = (*Buffer).enc_array_packed_int16
				p.dec = (*Buffer).dec_array_packed_int16
				wire = WireBytes // packed=true...
				p.asProtobuf = "repeated " + int32_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Uint16:
				p.enc = (*Buffer).enc_array_packed_uint16
				p.dec = (*Buffer).dec_array_packed_int16
				wire = WireBytes // packed=true...
				p.asProtobuf = "repeated " + uint32_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Int32:
				p.enc = (*Buffer).enc_array_packed_int32
				p.dec = (*Buffer).dec_array_packed_int32
				wire = WireBytes // packed=true...
				p.asProtobuf = "repeated " + int32_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Uint32:
				p.enc = (*Buffer).enc_array_packed_uint32
				p.dec = (*Buffer).dec_array_packed_int32
				wire = WireBytes // packed=true...
				p.asProtobuf = "repeated " + uint32_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Int64:
				if p.WireType == WireBytes && t2 == time_Duration_type {
					p.stype = time_Duration_type
					p.enc = (*Buffer).enc_array_time_Duration
					p.dec = (*Buffer).dec_array_time_Duration
					p.asProtobuf = "repeated google.protobuf.Duration"
				} else {
					p.enc = (*Buffer).enc_array_packed_int64
					p.dec = (*Buffer).dec_array_packed_int64
					wire = WireBytes // packed=true...
					p.asProtobuf = "repeated " + int64_encoder_txt
					if p.valEnc == nil {
						return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
					}
				}
			case reflect.Uint64:
				p.enc = (*Buffer).enc_array_packed_int64
				p.dec = (*Buffer).dec_array_packed_int64
				wire = WireBytes // packed=true...
				p.asProtobuf = "repeated " + uint64_encoder_txt
				if p.valEnc == nil {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Float32:
				// can just treat them as bits
				p.enc = (*Buffer).enc_array_packed_uint32
				p.dec = (*Buffer).dec_array_packed_int32
				p.asProtobuf = "repeated float"
				if p.valEnc == nil || wire != WireFixed32 { // the way we encode and decode float32 at the moment means we can only support fixed32
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
				wire = WireBytes // packed=true...
			case reflect.Float64:
				// can just treat them as bits
				p.enc = (*Buffer).enc_array_packed_int64
				p.dec = (*Buffer).dec_array_packed_int64
				p.asProtobuf = "repeated double"
				if p.valEnc == nil || wire != WireFixed64 { // the way we encode and decode float64 at the moment means we can only support fixed64
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
				wire = WireBytes // packed=true...
			case reflect.String:
				p.enc = (*Buffer).enc_array_string
				p.dec = (*Buffer).dec_array_string
				p.asProtobuf = "repeated string"
				if wire != WireBytes {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Struct:
				p.stype = t2
				p.sprop, err = getPropertiesLocked(t2)
				if err != nil {
					return err
				}
				p.enc = (*Buffer).enc_array_struct_message
				p.dec = (*Buffer).dec_array_struct_message
				p.asProtobuf = "repeated " + p.stypeAsProtobuf()
				if wire != WireBytes {
					return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
				}
			case reflect.Ptr:
				switch t3 := t2.Elem(); t3.Kind() {
				default:
					return fmt.Errorf("protobuf3: no ptr encoder for %s -> %s -> %s", t1.Name(), t2.Name(), t3.Name())

				case reflect.Struct:
					p.stype = t3
					p.sprop, err = getPropertiesLocked(t3)
					if err != nil {
						return err
					}
					p.isAppender = isAppender(t2)
					p.isMarshaler = isMarshaler(t2)
					p.enc = (*Buffer).enc_array_ptr_struct_message
					p.dec = (*Buffer).dec_array_ptr_struct_message
					p.asProtobuf = "repeated " + p.stypeAsProtobuf()
					if wire != WireBytes {
						return fmt.Errorf("protobuf3: %q %s cannot have wiretype %s", name, t1, wire)
					}
				}
			}

		case reflect.Map:
			p.enc = (*Buffer).enc_new_map
			p.dec = (*Buffer).dec_new_map

			if p.WireType != WireBytes {
				err := fmt.Errorf("protobuf3: %s.%s wiretype is not \"bytes\"", t1.String(), name)
				fmt.Fprintln(os.Stderr, err) // print the error too
				return err
			}

			p.mtype = t1
			p.mkeyprop = &Properties{}
			key_tag := f.Tag.Get("protobuf_key")
			if key_tag == "" {
				err := fmt.Errorf("protobuf3: %s.%s lacks a protobuf_key tag", t1.String(), name)
				fmt.Fprintln(os.Stderr, err) // print the error too
				return err
			}
			skip, err := p.mkeyprop.init(p.mtype.Key(), "Key", key_tag, nil)
			if err != nil {
				return fmt.Errorf("protobuf3: while parsing the proto_key tag (%s) of %s.%s: %v", key_tag, t1.String(), name, err)
			}
			if skip {
				err := fmt.Errorf("protobuf3: %s.%s protobuf_key tag cannot be \"-\"", t1.String(), name)
				fmt.Fprintln(os.Stderr, err) // print the error too
				return err
			}
			if p.mkeyprop.Tag != 1 {
				// treat non-traditional map tags as an error since they won't be compatible with other protobuf marshalers
				err := fmt.Errorf("protobuf3: %s.%s protobuf_key tag (%s) doesn't use id 1", key_tag, t1.String(), name)
				fmt.Fprintln(os.Stderr, err) // print the error too
				return err
			}

			p.mvalprop = &Properties{}
			val_tag := f.Tag.Get("protobuf_val")
			if val_tag == "" {
				err := fmt.Errorf("protobuf3: %s.%s lacks a protobuf_val tag", t1.String(), name)
				fmt.Fprintln(os.Stderr, err) // print the error too
				return err
			}
			skip, err = p.mvalprop.init(p.mtype.Elem(), "Value", val_tag, nil)
			if err != nil {
				return fmt.Errorf("protobuf3: while parsing the proto_val tag (%s) of %s.%s: %v", val_tag, t1.String(), name, err)
			}
			if skip {
				err := fmt.Errorf("protobuf3: %s.%s protobuf_val tag cannot be \"-\"", t1.String(), name)
				fmt.Fprintln(os.Stderr, err) // print the error too
				return err
			}
			if p.mvalprop.Tag != 2 {
				// treat non-traditional map tags as an error since they won't be compatible with other protobuf marshalers
				err := fmt.Errorf("protobuf3: %s.%s protobuf_val tag (%s) doesn't use id 2", val_tag, t1.String(), name)
				fmt.Fprintln(os.Stderr, err) // print the error too
				return err
			}

			p.asProtobuf = fmt.Sprintf("map<%s, %s>", p.mkeyprop.asProtobuf, p.mvalprop.asProtobuf)
		}

		// if the type overrides the protobuf definition, use that instead
		var name, definition string
		if isAsProtobuf3er(ptr_t1) {
			name, definition, _ = reflect.NewAt(t1, nil).Interface().(AsProtobuf3er).AsProtobuf3()
		} else if isAsV1Protobuf3er(ptr_t1) {
			name, definition = reflect.NewAt(t1, nil).Interface().(AsV1Protobuf3er).AsProtobuf3()
		}
		if name != "" {
			p.asProtobuf = name
		}
		if definition != "" {
			p.stype = t1
		}
	}

	p.WireType = wire

	// precalculate tag code
	x := p.Tag<<3 | uint32(wire)
	i := 0
	var tagbuf [8]byte
	for i = 0; x > 127; i++ {
		tagbuf[i] = 0x80 | uint8(x)
		x >>= 7
	}
	tagbuf[i] = uint8(x)
	p.tagcode = string(tagbuf[0 : i+1])

	return nil
}

// using p.Name, p.stype and p.sprop, figure out the right name for the type of field p.
// if the name of the type is known, use that. Otherwise build a nested type and use it.
func (p *Properties) stypeAsProtobuf() string {
	// special case for time.Time and time.Duration (any other future special cases)
	switch p.sprop {
	case time_Time_sprop:
		return "google.protobuf.Timestamp"
		// note: there is no time.Duration case here because only struct types set .stype, and time.Duration is an int64
	}

	var name string

	// if the stype implements AsProtobuf3er and returns a type name, use that
	if isAsProtobuf3er(reflect.PtrTo(p.stype)) {
		name, _, _ = reflect.NewAt(p.stype, nil).Interface().(AsProtobuf3er).AsProtobuf3() // note AsProtobuf3() might return name "" anyway
	} else if isAsV1Protobuf3er(reflect.PtrTo(p.stype)) {
		name, _ = reflect.NewAt(p.stype, nil).Interface().(AsV1Protobuf3er).AsProtobuf3() // note AsProtobuf3() might return name "" anyway
	}

	if name == "" {
		name = MakeTypeName(p.stype, p.Name)
	}

	if p.stype.Name() == "" {
		// p.stype is an anonymous type. define it inline with the enclosing message
		// we want the type definition to preceed the type's name, so that in the end it
		// formats something like:
		//   message Outer {
		//    `message Inner { ... }
		//     Inner' inner = 1;
		//   }
		// where the section in `' is the string we need to generate.
		lines := []string{p.sprop.asProtobuf(p.stype, name)}
		lines = append(lines, name)
		str := strings.Join(lines, "\n")
		// indent str two spaces to the right. we have to do this as a search step rather than as part of Join()
		// because the strings lines are already multi-line strings. (The other solutions are to indent as a
		// reformatting step at the end, or to store Properties.asProtobuf as []string and never loose the LFs.
		// The latter makes asProtobuf expensive for all the simple types. Reformatting needs to work on all fields.
		// So the "nasty" approach here is, AFAICS, for the best.
		name = strings.Replace(str, "\n", "\n  ", -1)
	}

	return name
}

// MakeUppercaseTypeName makes an uppercase message type name for type t, which is the type of a field named f.
// Since the field is visible to us it is public, and thus it is uppercase. And since the type is similarly visible
// it is almost certainly uppercased too. So there isn't much to do except pick whichever is appropriate.
func MakeUppercaseTypeName(t reflect.Type, f string) string {
	// if the Go type is named, a good start is to use the name of the go type
	// (even if it is in a different package than the enclosing type? that can cause collisions.
	//  for now the humans can sort those out after protoc errors on the duplicate records)
	n := t.Name()
	if n != "" {
		return n
	}

	// the struct has no typename. It is an anonymous type in Go. The equivalent in Protobuf is
	// a a nested type. It would be nice to use the name of the field as the name of the type,
	// since the name of the field ought to be unique within the enclosing struct type. However
	// protoc 3.0.2 cannot handle a field and a type having the same name. So we need to make up
	// a reasonable name for this type. I didn't like the result of appending "_msg" or other
	// 'uniquifier' to p.Name. So instead I've done the non-Go thing and made fields be lowercase,
	// thus reserving uppercase names for types, and thus avoiding any collisions.
	return f
}

var (
	marshalerType        = reflect.TypeOf((*Marshaler)(nil)).Elem()
	appenderType         = reflect.TypeOf((*Appender)(nil)).Elem()
	asprotobuffer3Type   = reflect.TypeOf((*AsProtobuf3er)(nil)).Elem()
	asv1protobuffer3Type = reflect.TypeOf((*AsV1Protobuf3er)(nil)).Elem()
)

// isMarshaler reports whether type t implements Marshaler.
func isMarshaler(t reflect.Type) bool {
	return t.Implements(marshalerType)
}

func isAppender(t reflect.Type) bool {
	return t.Implements(appenderType)
}

func isAsProtobuf3er(t reflect.Type) bool {
	return t.Implements(asprotobuffer3Type)
}

func isAsV1Protobuf3er(t reflect.Type) bool {
	return t.Implements(asv1protobuffer3Type)
}

// Init populates the properties from a protocol buffer struct tag.
// returns (skip, error)
func (p *Properties) init(typ reflect.Type, name, tag string, f *reflect.StructField) (bool, error) {
	// fields without a protobuf tag are an error
	if tag == "" {
		// backwards compatibility HACK. canonical golang.org/protobuf ignores errors on fields with names that start with XXX_
		// we must do the same to pass their unit tests
		if XXXHack && strings.HasPrefix(name, "XXX_") {
			return true, nil
		}
		err := fmt.Errorf("protobuf3: %s (%s) lacks a protobuf tag. Tag it, or mark it with `protobuf:\"-\"` if it isn't intended to be marshaled to/from protobuf", name, typ.String())
		fmt.Fprintln(os.Stderr, err) // print the error too
		return true, err
	}

	p.Name = name
	if f != nil {
		p.offset = f.Offset
	}

	intencoder, skip, err := p.Parse(tag)
	if skip || err != nil {
		return skip, err
	}

	return false, p.setEncAndDec(typ, f, name, intencoder)
}

var (
	propertiesMu  sync.RWMutex
	propertiesMap = make(map[reflect.Type]*StructProperties)
)

// synthesize a StructProperties for time.Time which will encode it
// to the same as the standard protobuf3 Timestamp type.
var time_Time_type = reflect.TypeOf(time.Time{})
var time_Time_sprop = &StructProperties{
	props: []Properties{
		// we need just one made-up field with a .enc() method which we've hooked into
		Properties{
			Name:     "time.Time",
			WireType: WireBytes,
			enc:      (*Buffer).enc_time_Time,
			// note: .dec isn't used
		},
	},
}

// similarly for time.Duration ... standard protobuf3 Duration type. Note that because
// go time.Duration isn't a struct (it's a int64) there isn't a time_Duration_sprop at all.
var time_Duration_type = reflect.TypeOf(time.Duration(0))

func init() {
	propertiesMap[time_Time_type] = time_Time_sprop
}

// GetProperties returns the list of properties for the type represented by t.
// t must represent a generated struct type of a protocol message.
func GetProperties(t reflect.Type) (*StructProperties, error) {
	k := t.Kind()
	// accept a pointer-to-struct as well (but just one level)
	if k == reflect.Ptr {
		t = t.Elem()
		k = t.Kind()
	}
	if k != reflect.Struct {
		panic("protobuf3: type must have kind struct")
	}

	// Most calls to GetProperties in a long-running program will be
	// retrieving details for types we have seen before.
	propertiesMu.RLock()
	sprop, ok := propertiesMap[t]
	propertiesMu.RUnlock()
	if ok {
		return sprop, nil
	}

	propertiesMu.Lock()
	sprop, err := getPropertiesLocked(t)
	propertiesMu.Unlock()
	return sprop, err
}

// getPropertiesLocked requires that propertiesMu is held.
func getPropertiesLocked(t reflect.Type) (*StructProperties, error) {
	if prop, ok := propertiesMap[t]; ok {
		return prop, nil
	}

	prop := new(StructProperties)

	// in case of recursion, add ourselves to propertiesMap now. we'll remove ourselves if we error
	propertiesMap[t] = prop

	// build properties
	nf := t.NumField()
	prop.props = make([]Properties, 0, nf)

	for i := 0; i < nf; i++ {
		f := t.Field(i)
		name := f.Name
		if name == "" && f.Anonymous {
			// use the type's name for embedded fields, like go does
			name = f.Type.Name()
			if name == "" {
				// use the type's unamed type
				name = f.Type.String()
			}
		}
		if name == "" {
			// unnamed embedded field types have no simple name
			name = "<unnamed field>"
		}

		tag := f.Tag.Get("protobuf")

		if tag == "embedded" && f.Anonymous {
			// field f is embedded in type t and has the special `protobuf:"embedded"` tag. Get f's fields and then merge them into t's
			fprop, err := getPropertiesLocked(f.Type)
			if err != nil {
				err := fmt.Errorf("protobuf3: error preparing field %q of type %q: %v", name, t.Name(), err)
				fmt.Fprintln(os.Stderr, err) // print the error too
				delete(propertiesMap, t)
				return nil, err
			}

			// merge fprop's fields into prop
			for ii, p := range fprop.props {
				// fixup the field property as we copy them
				p.offset += f.Offset

				prop.props = append(prop.props, p)

				if debug {
					print(i, ".", ii, " ", name, " ", t.String(), " ")
					if p.Tag > 0 {
						print(p.String())
					}
					print("\n")
				}
			}

			continue
		}

		if f.Type == reservedType {
			err := prop.parseReserved(tag)
			if err != nil {
				err := fmt.Errorf("protobuf3: error parsing protobuf3.Reserved field %q of type %q: %v", name, t.Name(), err)
				fmt.Fprintln(os.Stderr, err) // print the error too
				return nil, err
			}
			continue
		}

		prop.props = append(prop.props, Properties{})
		p := &prop.props[len(prop.props)-1]

		skip, err := p.init(f.Type, name, tag, &f)
		if err != nil {
			err := fmt.Errorf("protobuf3: error preparing field %q of type %q: %v", name, t.Name(), err)
			fmt.Fprintln(os.Stderr, err) // print the error too
			delete(propertiesMap, t)
			return nil, err
		}
		if skip {
			// silently skip this field. It's not part of the protobuf encoding of this struct
			prop.props = prop.props[:len(prop.props)-1] // remove it from properties
			continue
		}

		if debug {
			print(i, " ", name, " ", t.String(), " ")
			if p.Tag > 0 {
				print(p.String())
			}
			print("\n")
		}

		if p.enc == nil || p.dec == nil {
			tname := t.Name()
			if tname == "" {
				tname = "<anonymous struct>"
			}
			err := fmt.Errorf("protobuf3: error no encoder or decoder for field %q.%q of type %q", tname, name, f.Type.Name())
			fmt.Fprintln(os.Stderr, err) // print the error too
			delete(propertiesMap, t)
			return nil, err
		}
	}

	// sort and de-dup the reserved IDs
	var reserved map[uint32]struct{}
	if len(prop.reserved) != 0 {
		reserved = make(map[uint32]struct{}, len(prop.reserved))
		for _, r := range prop.reserved {
			reserved[r] = struct{}{}
		}
		if len(reserved) != len(prop.reserved) {
			// there are duplicate reserved IDs. Is this a bug? Maybe. For now, redup the reserved list.
			prop.reserved = prop.reserved[:0]
			for r := range reserved {
				prop.reserved = append(prop.reserved, r)
			}
		}
		sort.Slice(prop.reserved, func(i, j int) bool { return prop.reserved[i] < prop.reserved[j] })
	}

	// sort prop.props by tag, so we naturally encode in tag order as suggested by protobuf documentation
	sort.Sort(prop)
	if debug {
		for i := range prop.props {
			p := &prop.props[i]
			print("| ", t.Name(), ".", p.Name, "  ", p.WireType.String(), ",", p.Tag, "  offset=", p.offset, "\n")
		}
	}

	// now that they are sorted, sanity check for duplicate or reserved tags, since some of us are hand editing the tags
	prev_tag := uint32(0)
	var err error
	for i := range prop.props {
		p := &prop.props[i]
		if prev_tag == p.Tag {
			err = fmt.Errorf("protobuf3: error duplicate tag id %d assigned to %s.%s", p.Tag, t.String(), p.Name)
		} else if _, ok := reserved[p.Tag]; ok {
			err = fmt.Errorf("protobuf3: error reserved tag id %d assigned to %s.%s", p.Tag, t.String(), p.Name)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err) // print the error too
			delete(propertiesMap, t)
			return nil, err
		}
		prev_tag = p.Tag
	}

	return prop, nil
}
