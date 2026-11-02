package remotehelper

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"strings"
	"testing"
)

// chunkWriter accepts at most n bytes per call so writeFull must loop.
type chunkWriter struct {
	bytes.Buffer
	n int
}

func (w *chunkWriter) Write(p []byte) (int, error) {
	if len(p) > w.n {
		p = p[:w.n]
	}
	return w.Buffer.Write(p)
}

func TestFrameRoundTripNonUTF8(t *testing.T) {
	request := Request{
		Type:  TypeExec,
		ID:    7,
		Dir:   []byte("/tmp/repo\xff"),
		Args:  [][]byte{[]byte("status"), {0xff, 0xfe, 0x00, 'a'}},
		Input: []byte{0x80, 0x81},
	}
	var wire chunkWriter
	wire.n = 1
	if err := WriteRequest(&wire, request); err != nil {
		t.Fatal(err)
	}
	payload, err := ReadFrame(&wire.Buffer)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRequest(payload)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Type != request.Type || decoded.ID != request.ID || !bytes.Equal(decoded.Dir, request.Dir) {
		t.Fatalf("unexpected request: %+v", decoded)
	}
	if len(decoded.Args) != 2 || !bytes.Equal(decoded.Args[1], request.Args[1]) {
		t.Fatalf("non-UTF-8 arg was not preserved: %q", decoded.Args)
	}
	if !bytes.Equal(decoded.Input, request.Input) {
		t.Fatalf("input was not preserved: %q", decoded.Input)
	}
}

func TestFrameRejectsEmptyAndOversize(t *testing.T) {
	var empty [4]byte
	if _, err := ReadFrame(bytes.NewReader(empty[:])); !errors.Is(err, ErrEmptyFrame) {
		t.Fatalf("empty frame: %v", err)
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], MaxFrame+1)
	if _, err := ReadFrame(bytes.NewReader(header[:])); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("oversize frame: %v", err)
	}
	if err := WriteFrame(io.Discard, make([]byte, MaxFrame+1)); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("oversize write: %v", err)
	}
}

func TestFrameTruncated(t *testing.T) {
	var buf bytes.Buffer
	binary.BigEndian.PutUint32([]byte{0, 0, 0, 10}, 10)
	buf.Write([]byte{0, 0, 0, 10, '{', '}'})
	if _, err := ReadFrame(&buf); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated frame: %v", err)
	}
}

func TestStrictJSON(t *testing.T) {
	cases := []struct {
		name    string
		payload string
	}{
		{"unknown field", `{"type":"exec","bogus":1}`},
		{"trailing", `{"type":"ping"}{"type":"ping"}`},
		{"missing type", `{"id":1}`},
		{"bad base64", `{"type":"exec","args":["!!!!"]}`},
		{"not object", `[]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecodeRequest([]byte(tc.payload)); err == nil {
				t.Fatal("accepted invalid frame")
			}
		})
	}
	if _, err := DecodeResponse([]byte(`{"type":"result","stdout":"%%%"}`)); err == nil {
		t.Fatal("accepted invalid response base64")
	}
}

func TestLargePayload(t *testing.T) {
	output := bytes.Repeat([]byte("0123456789abcdef"), 1<<16)
	response := Response{Type: TypeResult, ID: 9, Exit: 0, Stdout: output}
	var wire bytes.Buffer
	if err := WriteResponse(&wire, response); err != nil {
		t.Fatal(err)
	}
	payload, err := ReadFrame(&wire)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeResponse(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded.Stdout, output) {
		t.Fatalf("large payload changed: got %d bytes", len(decoded.Stdout))
	}
}

func TestRemoteErrorMapping(t *testing.T) {
	err := (&RemoteError{Code: "UnsafeOperation", Message: "rejected"}).AsGitError()
	if err == nil || string(err.Code) != "UnsafeOperation" || err.Reason != "rejected" {
		t.Fatalf("unexpected mapping: %#v", err)
	}
	if strings.Contains(err.Error(), "rejected") {
		t.Fatal("GitError text should stay fixed")
	}
}

func TestStrictWireShapes(t *testing.T) {
	for _, raw := range []string{
		`{"type":"ping","type":"ping"}`, `{"Type":"ping"}`, `{"type":null}`,
		`{"type":"ping","id":1}`, `{"type":"exec","dir":[47,116,109,112]}`,
		`{"type":"exec","args":[[115]]}`, `{"type":"exec","args":[null]}`,
		`{"type":"exec","input":null}`, `{"type":"exec","input":"AB=="}`, `{"type":"exec","input":"YQ==\n"}`, `{"type":"hello","proto":1,"id":0}`,
		`{"type":"batch","args":[]}`, `{"type":"cancel","id":1,"commands":[]}`,
	} {
		if _, err := DecodeRequest([]byte(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	for _, raw := range []string{
		`{"type":"result","id":1,"error":{"code":"a","code":"b"}}`,
		`{"type":"result","stdout":[0]}`, `{"type":"pong","id":1}`,
		`{"type":"result","results":[]}`, `{"type":"unknown"}`,
	} {
		if _, err := DecodeResponse([]byte(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	for _, raw := range [][]byte{{0}, {0, 0, 0, 5}, {0, 0, 0, 5, '{'}} {
		if _, err := ReadFrame(bytes.NewReader(raw)); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("truncation %v: %v", raw, err)
		}
	}
}

func TestHelperJSONSchemaContracts(t *testing.T) {
	cases := []struct {
		name    string
		typ     reflect.Type
		values  []any
		invalid []string
	}{
		{"request", reflect.TypeOf(Request{}), []any{
			Request{Type: TypeHello, Proto: Proto}, Request{Type: TypeExec, ID: 1, Dir: []byte("/tmp/\xff"), Args: [][]byte{[]byte("--version")}},
			Request{Type: TypeBatch, ID: 2, Dir: []byte("/tmp"), Commands: [][][]byte{{[]byte("--version")}}},
			Request{Type: TypeCancel, ID: 2}, Request{Type: TypePing}, Request{Type: TypeShutdown},
		}, []string{`{"type":"ping","id":1}`, `{"type":"exec","dir":[47]}`, `{"type":"exec","args":[null]}`, `{"type":"hello","proto":null}`}},
		{"response", reflect.TypeOf(Response{}), []any{
			Response{Type: TypeReady, Proto: Proto, Platform: "darwin/arm64", PID: 1}, Response{Type: TypeResult, ID: 1, Stdout: []byte{255}},
			Response{Type: TypeBatchResult, ID: 2, Results: []CommandResult{{Exit: 0}, {Exit: -1, Error: &RemoteError{Code: "CommandFailed"}}}},
			Response{Type: TypePong}, Response{Type: TypeFatal, Error: &RemoteError{Code: "ProtocolError"}},
		}, []string{`{"type":"pong","exit":1}`, `{"type":"result","stdout":[0]}`, `{"type":"fatal","error":{"code":"x","extra":1}}`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("../../docs/features/git-viewer/schemas", "remote-helper-"+tc.name+".schema.json"))
			if err != nil {
				t.Fatal(err)
			}
			var spec map[string]any
			if err := json.Unmarshal(data, &spec); err != nil {
				t.Fatal(err)
			}
			fields := spec["properties"].(map[string]any)
			if len(fields) != tc.typ.NumField() {
				t.Fatal("Go/schema field count differs")
			}
			for i := 0; i < tc.typ.NumField(); i++ {
				name := strings.Split(tc.typ.Field(i).Tag.Get("json"), ",")[0]
				if _, ok := fields[name]; !ok {
					t.Fatalf("schema missing %s", name)
				}
			}
			compiler := jsonschema.NewCompiler()
			if err := compiler.AddResource("helper.json", spec); err != nil {
				t.Fatal(err)
			}
			schema, err := compiler.Compile("helper.json")
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range tc.values {
				raw, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				var wire any
				if err := json.Unmarshal(raw, &wire); err != nil {
					t.Fatal(err)
				}
				if err := schema.Validate(wire); err != nil {
					t.Fatalf("%s: %v", raw, err)
				}
				if tc.name == "request" {
					_, err = DecodeRequest(raw)
				} else {
					_, err = DecodeResponse(raw)
				}
				if err != nil {
					t.Fatalf("DTO decoder: %v", err)
				}
			}
			for _, raw := range tc.invalid {
				var wire any
				json.Unmarshal([]byte(raw), &wire)
				if schema.Validate(wire) == nil {
					t.Fatalf("schema accepted %s", raw)
				}
			}
		})
	}
}
