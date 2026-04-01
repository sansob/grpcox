package core

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fullstorydev/grpcurl"
	"github.com/jhump/protoreflect/desc"
)

func Test_prepareImport(t *testing.T) {
	type args struct {
		proto []byte
	}
	tests := []struct {
		name string
		args args
		want []byte
	}{
		{
			name: "sucess change import path to local",
			args: args{
				proto: []byte(`
				  package testing;

				  import "test.com/owner/repo/content.proto";`),
			},
			want: []byte(`
				  package testing;

				  import "content.proto";`),
		},
		{
			name: "sucess keep google import",
			args: args{
				proto: []byte(`
				  package testing;

				  import "google/proto/buf";
				  import "test.com/owner/repo/content.proto";`),
			},
			want: []byte(`
				  package testing;

				  import "google/proto/buf";
				  import "content.proto";`),
		},
		{
			name: "sucess keep local import",
			args: args{
				proto: []byte(`
				  package testing;

				  import "repo.proto";
				  import "test.com/owner/repo/content.proto";`),
			},
			want: []byte(`
				  package testing;

				  import "repo.proto";
				  import "content.proto";`),
		},
		{
			name: "success rewrite proto3 public import path to local file name",
			args: args{
				proto: []byte(`
				  syntax = "proto3";
				  package testing;

				  import public "github.com/example/common/types.proto";`),
			},
			want: []byte(`
				  syntax = "proto3";
				  package testing;

				  import public "types.proto";`),
		},
		{
			name: "success rewrite proto3 weak import path to local file name",
			args: args{
				proto: []byte(`
				  syntax = "proto3";
				  package testing;

				  import weak "example.com/owner/repo/annotations.proto";`),
			},
			want: []byte(`
				  syntax = "proto3";
				  package testing;

				  import weak "annotations.proto";`),
		},
		{
			name: "success keep google import modifiers intact",
			args: args{
				proto: []byte(`
				  syntax = "proto3";
				  package testing;

				  import public "google/protobuf/empty.proto";`),
			},
			want: []byte(`
				  syntax = "proto3";
				  package testing;

				  import public "google/protobuf/empty.proto";`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prepareImport(tt.args.proto); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("prepareImport() = %v, want %v",
					string(got),
					string(tt.want))
			}
		})
	}
}

func TestDescribeMethodInputFromSource_Proto3Features(t *testing.T) {
	const schema = `syntax = "proto3";
package testing.v1;

import "google/protobuf/any.proto";
import "google/protobuf/timestamp.proto";

message ComplexRequest {
  optional string name = 1;

  oneof payload {
    string raw = 2;
    int32 code = 3;
  }

  map<string, int32> labels = 4;
  google.protobuf.Timestamp created_at = 5;
  google.protobuf.Any details = 6;
}

message ComplexResponse {
  string status = 1;
}

service DemoService {
  rpc Submit(ComplexRequest) returns (ComplexResponse);
}
`

	dir, err := ioutil.TempDir("", "grpcox-proto3-method-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	protoPath := filepath.Join(dir, "complex.proto")
	if err := ioutil.WriteFile(protoPath, []byte(schema), 0600); err != nil {
		t.Fatalf("write proto: %v", err)
	}

	descSource, err := grpcurl.DescriptorSourceFromProtoFiles([]string{dir}, "complex.proto")
	if err != nil {
		t.Fatalf("build descriptor source: %v", err)
	}

	r := &Resource{descSource: descSource}
	gotSchema, gotTemplate, err := r.describeMethodInputFromSource("testing.v1.DemoService.Submit")
	if err != nil {
		t.Fatalf("describe method input: %v", err)
	}

	for _, want := range []string{
		"optional string name = 1;",
		"oneof payload",
		"map<string, int32> labels = 4;",
		"google.protobuf.Timestamp created_at = 5;",
		"google.protobuf.Any details = 6;",
	} {
		if !strings.Contains(gotSchema, want) {
			t.Fatalf("schema missing %q:\n%s", want, gotSchema)
		}
	}

	for _, want := range []string{
		"\"labels\"",
		"\"createdAt\"",
		"\"details\"",
	} {
		if !strings.Contains(gotTemplate, want) {
			t.Fatalf("template missing %q:\n%s", want, gotTemplate)
		}
	}
}

func TestDescribeMessage_Proto3Features(t *testing.T) {
	const schema = `syntax = "proto3";
package testing.v1;

import "google/protobuf/any.proto";

message Envelope {
  oneof body {
    string text = 1;
    bytes raw = 2;
  }

  map<string, string> metadata = 3;
  google.protobuf.Any details = 4;
}
`

	dir, err := ioutil.TempDir("", "grpcox-proto3-message-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	protoPath := filepath.Join(dir, "envelope.proto")
	if err := ioutil.WriteFile(protoPath, []byte(schema), 0600); err != nil {
		t.Fatalf("write proto: %v", err)
	}

	descSource, err := grpcurl.DescriptorSourceFromProtoFiles([]string{dir}, "envelope.proto")
	if err != nil {
		t.Fatalf("build descriptor source: %v", err)
	}

	dsc, err := descSource.FindSymbol("testing.v1.Envelope")
	if err != nil {
		t.Fatalf("find symbol: %v", err)
	}

	msg, ok := dsc.(*desc.MessageDescriptor)
	if !ok {
		t.Fatalf("symbol is %T, want *desc.MessageDescriptor", dsc)
	}

	r := &Resource{descSource: descSource}
	gotSchema, gotTemplate, err := r.describeMessage(msg)
	if err != nil {
		t.Fatalf("describe message: %v", err)
	}

	for _, want := range []string{
		"oneof body",
		"map<string, string> metadata = 3;",
		"google.protobuf.Any details = 4;",
	} {
		if !strings.Contains(gotSchema, want) {
			t.Fatalf("schema missing %q:\n%s", want, gotSchema)
		}
	}

	for _, want := range []string{
		"\"metadata\"",
		"\"details\"",
	} {
		if !strings.Contains(gotTemplate, want) {
			t.Fatalf("template missing %q:\n%s", want, gotTemplate)
		}
	}
}

func TestDescribeMethodInputFromSource_Proto2Features(t *testing.T) {
	const schema = `syntax = "proto2";
package testing.v1;

message LegacyRequest {
  required string id = 1;
  optional string alias = 2 [default = "guest"];

  enum Status {
    UNKNOWN = 0;
    ACTIVE = 1;
  }

  optional Status status = 3 [default = ACTIVE];
}

message LegacyResponse {
  optional bool ok = 1 [default = true];
}

service LegacyService {
  rpc Submit(LegacyRequest) returns (LegacyResponse);
}
`

	dir, err := ioutil.TempDir("", "grpcox-proto2-method-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	protoPath := filepath.Join(dir, "legacy.proto")
	if err := ioutil.WriteFile(protoPath, []byte(schema), 0600); err != nil {
		t.Fatalf("write proto: %v", err)
	}

	descSource, err := grpcurl.DescriptorSourceFromProtoFiles([]string{dir}, "legacy.proto")
	if err != nil {
		t.Fatalf("build descriptor source: %v", err)
	}

	r := &Resource{descSource: descSource}
	gotSchema, gotTemplate, err := r.describeMethodInputFromSource("testing.v1.LegacyService.Submit")
	if err != nil {
		t.Fatalf("describe method input: %v", err)
	}

	for _, want := range []string{
		"required string id = 1;",
		"optional string alias = 2 [default = \"guest\"]",
		"optional Status status = 3 [default = ACTIVE]",
	} {
		if !strings.Contains(gotSchema, want) {
			t.Fatalf("schema missing %q:\n%s", want, gotSchema)
		}
	}

	for _, want := range []string{
		"\"id\"",
		"\"alias\"",
		"\"status\"",
		"\"ACTIVE\"",
	} {
		if !strings.Contains(gotTemplate, want) {
			t.Fatalf("template missing %q:\n%s", want, gotTemplate)
		}
	}
}
