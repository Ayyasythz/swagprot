// Package schema converts protoreflect descriptors into a JSON-serialisable
// "form schema" that the swagprot web UI uses to render request forms, plus
// lightweight metadata about services and methods.
package schema

import (
	"google.golang.org/protobuf/reflect/protoreflect"
)

// maxDepth bounds how deeply nested message fields are expanded. Recursive or
// very deep types beyond this are surfaced as a raw-JSON field instead.
const maxDepth = 8

// Service is a service plus its methods, for the navigation tree.
type Service struct {
	Name     string   `json:"name"`     // short name, e.g. "Greeter"
	FullName string   `json:"fullName"` // e.g. "greet.Greeter"
	Methods  []Method `json:"methods"`
}

// Method describes a single RPC.
type Method struct {
	Name            string `json:"name"`
	FullName        string `json:"fullName"` // "pkg.Service.Method"
	InputType       string `json:"inputType"`
	OutputType      string `json:"outputType"`
	ClientStreaming bool   `json:"clientStreaming"`
	ServerStreaming bool   `json:"serverStreaming"`
}

// Field describes a single field of a request message.
type Field struct {
	Name     string `json:"name"`
	JSONName string `json:"jsonName"`
	Type     string `json:"type"` // scalar name, "enum", "message", "map", or "group"
	Repeated bool   `json:"repeated"`
	Oneof    string `json:"oneof,omitempty"` // oneof group this field belongs to
	Optional bool   `json:"optional,omitempty"`

	// Enum
	EnumValues []string `json:"enumValues,omitempty"`

	// Message (Type == "message")
	MessageType string  `json:"messageType,omitempty"`
	WellKnown   string  `json:"wellKnown,omitempty"` // e.g. "timestamp", "duration", "wrapper"
	Recursive   bool    `json:"recursive,omitempty"` // expansion stopped; UI shows raw JSON
	Fields      []Field `json:"fields,omitempty"`

	// Map (Type == "map")
	KeyType     string  `json:"keyType,omitempty"`
	ValueType   string  `json:"valueType,omitempty"`   // scalar/enum/message
	ValueFields []Field `json:"valueFields,omitempty"` // when the map value is a message
}

// MessageSchema is the expanded form schema for a request message.
type MessageSchema struct {
	Name   string  `json:"name"`
	Fields []Field `json:"fields"`
}

// MethodDetail bundles a method's metadata with its expanded request schema.
type MethodDetail struct {
	Method  Method        `json:"method"`
	Request MessageSchema `json:"request"`
}

// BuildService produces the navigation entry for a service descriptor.
func BuildService(sd protoreflect.ServiceDescriptor) Service {
	out := Service{
		Name:     string(sd.Name()),
		FullName: string(sd.FullName()),
	}
	mds := sd.Methods()
	for i := 0; i < mds.Len(); i++ {
		out.Methods = append(out.Methods, BuildMethod(mds.Get(i)))
	}
	return out
}

// BuildMethod produces metadata for a method descriptor.
func BuildMethod(md protoreflect.MethodDescriptor) Method {
	return Method{
		Name:            string(md.Name()),
		FullName:        string(md.FullName()),
		InputType:       string(md.Input().FullName()),
		OutputType:      string(md.Output().FullName()),
		ClientStreaming: md.IsStreamingClient(),
		ServerStreaming: md.IsStreamingServer(),
	}
}

// BuildMethodDetail produces method metadata plus the expanded request schema.
func BuildMethodDetail(md protoreflect.MethodDescriptor) MethodDetail {
	return MethodDetail{
		Method:  BuildMethod(md),
		Request: BuildMessage(md.Input()),
	}
}

// BuildMessage expands a message descriptor into a form schema.
func BuildMessage(msg protoreflect.MessageDescriptor) MessageSchema {
	return MessageSchema{
		Name:   string(msg.FullName()),
		Fields: buildFields(msg, 0, map[protoreflect.FullName]bool{msg.FullName(): true}),
	}
}

func buildFields(msg protoreflect.MessageDescriptor, depth int, ancestors map[protoreflect.FullName]bool) []Field {
	fields := msg.Fields()
	out := make([]Field, 0, fields.Len())
	for i := 0; i < fields.Len(); i++ {
		out = append(out, buildField(fields.Get(i), depth, ancestors))
	}
	return out
}

func buildField(fd protoreflect.FieldDescriptor, depth int, ancestors map[protoreflect.FullName]bool) Field {
	f := Field{
		Name:     string(fd.Name()),
		JSONName: fd.JSONName(),
	}
	if oneof := fd.ContainingOneof(); oneof != nil && !oneof.IsSynthetic() {
		f.Oneof = string(oneof.Name())
	}
	if fd.HasOptionalKeyword() {
		f.Optional = true
	}

	// Maps are represented in the descriptor as repeated message fields with a
	// synthetic entry type; handle them before the generic repeated case.
	if fd.IsMap() {
		f.Type = "map"
		f.KeyType = kindName(fd.MapKey())
		val := fd.MapValue()
		f.ValueType = kindName(val)
		if val.Kind() == protoreflect.MessageKind || val.Kind() == protoreflect.GroupKind {
			f.ValueType = "message"
			f.MessageType = string(val.Message().FullName())
			f.ValueFields = expandMessage(val.Message(), depth, ancestors, &f)
		} else if val.Kind() == protoreflect.EnumKind {
			f.ValueType = "enum"
			f.EnumValues = enumValues(val.Enum())
		}
		return f
	}

	f.Repeated = fd.Cardinality() == protoreflect.Repeated

	switch fd.Kind() {
	case protoreflect.EnumKind:
		f.Type = "enum"
		f.EnumValues = enumValues(fd.Enum())
	case protoreflect.MessageKind, protoreflect.GroupKind:
		f.Type = "message"
		f.MessageType = string(fd.Message().FullName())
		f.WellKnown = wellKnown(fd.Message())
		f.Fields = expandMessage(fd.Message(), depth, ancestors, &f)
	default:
		f.Type = kindName(fd)
	}
	return f
}

// expandMessage recurses into a nested message, guarding against cycles and
// excessive depth. Well-known types are not expanded; the UI renders them with
// a dedicated input based on the WellKnown hint.
func expandMessage(msg protoreflect.MessageDescriptor, depth int, ancestors map[protoreflect.FullName]bool, f *Field) []Field {
	if wellKnown(msg) != "" {
		return nil
	}
	if depth+1 >= maxDepth || ancestors[msg.FullName()] {
		f.Recursive = true
		return nil
	}
	next := make(map[protoreflect.FullName]bool, len(ancestors)+1)
	for k, v := range ancestors {
		next[k] = v
	}
	next[msg.FullName()] = true
	return buildFields(msg, depth+1, next)
}

func enumValues(ed protoreflect.EnumDescriptor) []string {
	vals := ed.Values()
	out := make([]string, 0, vals.Len())
	for i := 0; i < vals.Len(); i++ {
		out = append(out, string(vals.Get(i).Name()))
	}
	return out
}

// kindName returns the proto3 JSON-facing type name for a scalar field.
func kindName(fd protoreflect.FieldDescriptor) string {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		return "bool"
	case protoreflect.StringKind:
		return "string"
	case protoreflect.BytesKind:
		return "bytes"
	case protoreflect.FloatKind:
		return "float"
	case protoreflect.DoubleKind:
		return "double"
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return "int32"
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return "uint32"
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return "int64"
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return "uint64"
	case protoreflect.EnumKind:
		return "enum"
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return "message"
	default:
		return "string"
	}
}

// wellKnown returns a hint for well-known types that warrant a tailored input,
// or "" for ordinary messages.
func wellKnown(msg protoreflect.MessageDescriptor) string {
	switch msg.FullName() {
	case "google.protobuf.Timestamp":
		return "timestamp"
	case "google.protobuf.Duration":
		return "duration"
	case "google.protobuf.Struct", "google.protobuf.Value", "google.protobuf.ListValue":
		return "struct"
	case "google.protobuf.Any":
		return "any"
	case "google.protobuf.Empty":
		return "empty"
	case "google.protobuf.FieldMask":
		return "fieldmask"
	case "google.protobuf.DoubleValue", "google.protobuf.FloatValue",
		"google.protobuf.Int64Value", "google.protobuf.UInt64Value",
		"google.protobuf.Int32Value", "google.protobuf.UInt32Value",
		"google.protobuf.BoolValue", "google.protobuf.StringValue",
		"google.protobuf.BytesValue":
		return "wrapper"
	default:
		return ""
	}
}
