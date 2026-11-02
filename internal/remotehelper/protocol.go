// Package remotehelper implements the framed protocol spoken between Git
// Viewer and the small Git helper process running on an SSH host.
//
// The wire format is a 4-byte big-endian length followed by one JSON object.
// All byte fields are declared as []byte so encoding/json base64-encodes them;
// remote paths and Git output are not required to be valid UTF-8.
package remotehelper

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"

	"gitviewer/internal/domain"
)

// Proto is the protocol version. A mismatch is fatal: the client cannot make
// assumptions about framing or request fields from another version.
const Proto = 1

const (
	// MaxFrame bounds one JSON frame in either direction.
	MaxFrame = 16 << 20
	// DefaultLimit matches process.DefaultLimit; MaxLimit is the largest
	// output budget a single request may ask for.
	DefaultLimit = 2 << 20
	MaxLimit     = 8 << 20
	// MaxInput bounds stdin (a commit message or one hunk patch).
	MaxInput = 2 << 20
	// MaxArg bounds one argv element; MaxArgs bounds the encoded command.
	MaxArg  = 16 << 10
	MaxArgs = 1 << 20
	// MaxDir bounds the repository directory sent with a request.
	MaxDir = 4096
	// MaxBatchCommands and MaxBatchOutput bound a grouped read.
	MaxBatchCommands = 4
	MaxBatchOutput   = 8 << 20
)

// Request types.
const (
	TypeHello    = "hello"
	TypeExec     = "exec"
	TypeBatch    = "batch"
	TypeCancel   = "cancel"
	TypePing     = "ping"
	TypeShutdown = "shutdown"
)

// Response types.
const (
	TypeReady       = "ready"
	TypeResult      = "result"
	TypeBatchResult = "batchResult"
	TypePong        = "pong"
	TypeFatal       = "fatal"
)

// RemoteError is the helper-side failure of one request. Code uses the same
// vocabulary as domain.ErrorCode; Message is a fixed, non-secret explanation.
type RemoteError struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

func (e *RemoteError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

// AsGitError converts a remote failure into the application's error type.
func (e *RemoteError) AsGitError() *domain.GitError {
	if e == nil {
		return nil
	}
	return &domain.GitError{Code: domain.ErrorCode(e.Code), Reason: e.Message}
}

// Request is one client frame. The Type field selects which other fields are
// meaningful. Args and Commands are base64 on the wire.
type Request struct {
	Type      string     `json:"type"`
	ID        uint64     `json:"id,omitempty"`
	Proto     int        `json:"proto,omitempty"`
	Dir       []byte     `json:"dir,omitempty"`
	Limit     int        `json:"limit,omitempty"`
	TimeoutMs int        `json:"timeoutMs,omitempty"`
	Args      [][]byte   `json:"args,omitempty"`
	Input     []byte     `json:"input,omitempty"`
	Commands  [][][]byte `json:"commands,omitempty"`
}

// CommandResult is one member of a batch response.
type CommandResult struct {
	Exit   int          `json:"exit"`
	Stdout []byte       `json:"stdout,omitempty"`
	Stderr []byte       `json:"stderr,omitempty"`
	Error  *RemoteError `json:"error,omitempty"`
}

// Response is one helper frame.
type Response struct {
	Type     string          `json:"type"`
	ID       uint64          `json:"id,omitempty"`
	Proto    int             `json:"proto,omitempty"`
	Platform string          `json:"platform,omitempty"`
	PID      int             `json:"pid,omitempty"`
	Exit     int             `json:"exit,omitempty"`
	Stdout   []byte          `json:"stdout,omitempty"`
	Stderr   []byte          `json:"stderr,omitempty"`
	Error    *RemoteError    `json:"error,omitempty"`
	Results  []CommandResult `json:"results,omitempty"`
}

var (
	ErrFrameTooLarge = errors.New("remote helper frame exceeds the size limit")
	ErrEmptyFrame    = errors.New("remote helper frame is empty")
	ErrBadJSON       = errors.New("remote helper frame is not a single JSON object")
)

// ReadFrame reads one length-prefixed frame. Truncated frames return io.ErrUnexpectedEOF.
func ReadFrame(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 {
		return nil, ErrEmptyFrame
	}
	if size > MaxFrame {
		return nil, ErrFrameTooLarge
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	return payload, nil
}

// WriteFrame writes one frame with a single full-length write sequence so a
// short write cannot silently desynchronize the stream.
func WriteFrame(w io.Writer, payload []byte) error {
	if len(payload) == 0 {
		return ErrEmptyFrame
	}
	if len(payload) > MaxFrame {
		return ErrFrameTooLarge
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if err := writeFull(w, header[:]); err != nil {
		return err
	}
	return writeFull(w, payload)
}

func writeFull(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if n > 0 {
			data = data[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

// DecodeRequest strictly decodes one request frame. Unknown fields, trailing
// data and a missing type are protocol errors.
func DecodeRequest(payload []byte) (Request, error) {
	var request Request
	if err := strictDecode(payload, &request); err != nil {
		return Request{}, err
	}
	if request.Type == "" {
		return Request{}, fmt.Errorf("%w: missing type", ErrBadJSON)
	}
	allowed := map[string]string{
		TypeHello: " type proto ", TypeExec: " type id dir limit timeoutMs args input ",
		TypeBatch: " type id dir limit timeoutMs commands ", TypeCancel: " type id ",
		TypePing: " type ", TypeShutdown: " type ",
	}[request.Type]
	if allowed == "" {
		return Request{}, ErrBadJSON
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(payload, &fields)
	for field := range fields {
		if !strings.Contains(allowed, " "+field+" ") {
			return Request{}, ErrBadJSON
		}
	}
	return request, nil
}

// DecodeResponse strictly decodes one response frame.
func DecodeResponse(payload []byte) (Response, error) {
	var response Response
	if err := strictDecode(payload, &response); err != nil {
		return Response{}, err
	}
	if response.Type == "" {
		return Response{}, fmt.Errorf("%w: missing type", ErrBadJSON)
	}
	allowed := map[string]string{
		TypeReady: " type proto platform pid ", TypeResult: " type id exit stdout stderr error ",
		TypeBatchResult: " type id results error ", TypePong: " type ", TypeFatal: " type error ",
	}[response.Type]
	if allowed == "" {
		return Response{}, ErrBadJSON
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(payload, &fields)
	for field := range fields {
		if !strings.Contains(allowed, " "+field+" ") {
			return Response{}, ErrBadJSON
		}
	}
	return response, nil
}

func strictDecode(payload []byte, target any) error {
	if !utf8.Valid(payload) {
		return ErrBadJSON
	}
	// encoding/json otherwise accepts duplicate keys, case-insensitive struct
	// fields, null scalars, and numeric arrays for []byte. None are wire forms.
	if err := checkJSONValue(payload, reflect.TypeOf(target).Elem()); err != nil {
		return ErrBadJSON
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %v", ErrBadJSON, err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing data", ErrBadJSON)
	}
	return nil
}

// checkJSONValue checks the exact wire shape before unmarshalling, including
// duplicate keys at every object depth. Values are never included in errors.
func checkJSONValue(raw []byte, typ reflect.Type) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ErrBadJSON
	}
	if typ.Kind() == reflect.Pointer {
		return checkJSONValue(raw, typ.Elem())
	}
	switch typ.Kind() {
	case reflect.Struct:
		if raw[0] != '{' {
			return ErrBadJSON
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			fields[name] = field.Type
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		if _, err := decoder.Token(); err != nil {
			return err
		}
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			field, known := fields[name]
			if !ok || !known || seen[name] {
				return ErrBadJSON
			}
			seen[name] = true
			var value json.RawMessage
			if err := decoder.Decode(&value); err != nil {
				return err
			}
			if err := checkJSONValue(value, field); err != nil {
				return err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return err
		}
		if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
			return ErrBadJSON
		}
	case reflect.Slice:
		if typ.Elem().Kind() == reflect.Uint8 {
			if raw[0] != '"' {
				return ErrBadJSON
			}
			var encoded string
			if json.Unmarshal(raw, &encoded) != nil || strings.ContainsAny(encoded, "\r\n") {
				return ErrBadJSON
			}
			if _, err := base64.StdEncoding.Strict().DecodeString(encoded); err != nil {
				return ErrBadJSON
			}
		} else {
			if raw[0] != '[' {
				return ErrBadJSON
			}
			var values []json.RawMessage
			if err := json.Unmarshal(raw, &values); err != nil {
				return err
			}
			for _, value := range values {
				if err := checkJSONValue(value, typ.Elem()); err != nil {
					return err
				}
			}
		}
	case reflect.String:
		if raw[0] != '"' {
			return ErrBadJSON
		}
	}
	return nil
}

// Encode serializes a value as a frame payload.
func Encode(value any) ([]byte, error) {
	return json.Marshal(value)
}

// WriteResponse serializes and writes one response.
func WriteResponse(w io.Writer, response Response) error {
	payload, err := Encode(response)
	if err != nil {
		return err
	}
	return WriteFrame(w, payload)
}

// WriteRequest serializes and writes one request.
func WriteRequest(w io.Writer, request Request) error {
	payload, err := Encode(request)
	if err != nil {
		return err
	}
	return WriteFrame(w, payload)
}
