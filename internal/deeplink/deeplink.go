package deeplink

import (
	"encoding/base64"
	"errors"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

const Scheme = "gitviewer"

var (
	ErrInvalid = errors.New("invalid gitviewer link")
	hostRE     = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)
)

type Kind string

const (
	Folder    Kind = "folder"
	File      Kind = "file"
	Workspace Kind = "workspace"
)

type Request struct {
	Host      string
	Path      string
	Kind      Kind
	NewWindow bool
	Line      int
	Column    int
	DiffPath  string
}

func ValidHost(host string) bool { return hostRE.MatchString(host) }

func EncodePath(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func DecodePath(value string) (string, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(data) == 0 {
		return "", ErrInvalid
	}
	decoded := string(data)
	if strings.ContainsRune(decoded, 0) {
		return "", ErrInvalid
	}
	return decoded, nil
}

func Build(req Request) (string, error) {
	if !ValidHost(req.Host) || !validKind(req.Kind) || !path.IsAbs(req.Path) || strings.ContainsRune(req.Path, 0) {
		return "", ErrInvalid
	}
	if req.DiffPath != "" && (!path.IsAbs(req.DiffPath) || strings.ContainsRune(req.DiffPath, 0)) {
		return "", ErrInvalid
	}
	if req.Line < 0 || req.Column < 0 {
		return "", ErrInvalid
	}
	query := url.Values{}
	query.Set("host", req.Host)
	query.Set("path64", EncodePath(path.Clean(req.Path)))
	query.Set("kind", string(req.Kind))
	if req.NewWindow {
		query.Set("newWindow", "1")
	} else {
		query.Set("newWindow", "0")
	}
	if req.Line > 0 {
		query.Set("line", strconv.Itoa(req.Line))
	}
	if req.Column > 0 {
		query.Set("column", strconv.Itoa(req.Column))
	}
	if req.DiffPath != "" {
		query.Set("diffPath64", EncodePath(path.Clean(req.DiffPath)))
	}
	return "gitviewer://open?" + query.Encode(), nil
}

func Parse(raw string) (Request, error) {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, Scheme) || !strings.EqualFold(u.Host, "open") || u.User != nil || u.Fragment != "" {
		return Request{}, ErrInvalid
	}
	query := u.Query()
	allowed := map[string]bool{
		"host": true, "path64": true, "kind": true, "newWindow": true,
		"line": true, "column": true, "diffPath64": true,
	}
	for key := range query {
		if !allowed[key] {
			return Request{}, ErrInvalid
		}
	}
	host := query.Get("host")
	if !ValidHost(host) {
		return Request{}, ErrInvalid
	}
	file, err := DecodePath(query.Get("path64"))
	if err != nil || !path.IsAbs(file) {
		return Request{}, ErrInvalid
	}
	kind := Kind(query.Get("kind"))
	if !validKind(kind) {
		return Request{}, ErrInvalid
	}
	req := Request{Host: host, Path: path.Clean(file), Kind: kind}
	switch query.Get("newWindow") {
	case "", "0":
	case "1":
		req.NewWindow = true
	default:
		return Request{}, ErrInvalid
	}
	if value := query.Get("line"); value != "" {
		req.Line, err = positiveInt(value)
		if err != nil {
			return Request{}, ErrInvalid
		}
	}
	if value := query.Get("column"); value != "" {
		req.Column, err = positiveInt(value)
		if err != nil {
			return Request{}, ErrInvalid
		}
	}
	if value := query.Get("diffPath64"); value != "" {
		req.DiffPath, err = DecodePath(value)
		if err != nil || !path.IsAbs(req.DiffPath) {
			return Request{}, ErrInvalid
		}
		req.DiffPath = path.Clean(req.DiffPath)
	}
	return req, nil
}

func validKind(kind Kind) bool {
	return kind == Folder || kind == File || kind == Workspace
}

func positiveInt(value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 0, ErrInvalid
	}
	return n, nil
}
