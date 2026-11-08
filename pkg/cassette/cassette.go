// Copyright (c) 2015-2024 Marin Atanasov Nikolov <dnaeon@gmail.com>
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions
// are met:
// 1. Redistributions of source code must retain the above copyright
//    notice, this list of conditions and the following disclaimer
//    in this position and unchanged.
// 2. Redistributions in binary form must reproduce the above copyright
//    notice, this list of conditions and the following disclaimer in the
//    documentation and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE AUTHOR(S) ``AS IS'' AND ANY EXPRESS OR
// IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES
// OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED.
// IN NO EVENT SHALL THE AUTHOR(S) BE LIABLE FOR ANY DIRECT, INDIRECT,
// INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT
// NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF
// THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package cassette

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"go.yaml.in/yaml/v4"
)

// debugBodyLimit is the maximum number of body bytes rendered in a debug dump.
// Anything larger is truncated with a "(truncated, N bytes total)" suffix.
const debugBodyLimit = 16 * 1024

// debugSummaryBodyLimit is the maximum number of body bytes rendered in a
// compact one-line request/response summary. Keeps the per-attempt match log
// readable when there are many interactions to compare against.
const debugSummaryBodyLimit = 80

// summarizeBody returns a single-line, bounded rendering of the given body for
// inclusion in a compact request/response summary. Newlines are collapsed and
// the result is truncated to debugSummaryBodyLimit bytes.
func summarizeBody(b string) string {
	if b == "" {
		return ""
	}
	s := strings.ReplaceAll(b, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	if len(s) > debugSummaryBodyLimit {
		s = s[:debugSummaryBodyLimit] + "..."
	}

	return s
}

// formatBody returns a human-readable rendering of the given body bytes for
// inclusion in a debug dump. Binary content is replaced with a "<binary, N
// bytes>" placeholder so that the trace stays readable in a text-based slog
// handler.
func formatBody(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	if !isPrintable(b) {
		return fmt.Sprintf("<binary, %d bytes>", len(b))
	}
	if len(b) > debugBodyLimit {
		return fmt.Sprintf("%s\n(truncated, %d bytes total)", b[:debugBodyLimit], len(b))
	}

	return string(b)
}

// isPrintable reports whether b is safe to render as text in a debug trace:
// it must be valid UTF-8 and contain no control characters other than the
// whitespace runes commonly found in HTTP payloads (tab, LF, CR).
func isPrintable(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	for _, r := range string(b) {
		switch r {
		case '\t', '\n', '\r':
			continue
		}
		if !unicode.IsPrint(r) {
			return false
		}
	}

	return true
}

// dumpHTTPRequest renders an [*http.Request] in wire format for inclusion in a
// debug trace. The request body is consumed and replaced with a fresh reader
// so subsequent code (the matcher, the round-tripper) can still read it.
func dumpHTTPRequest(r *http.Request) string {
	if r.Body == nil || r.Body == http.NoBody {
		dump, err := httputil.DumpRequest(r, false)
		if err != nil {
			return fmt.Sprintf("<dump error: %v>", err)
		}

		return string(dump)
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Sprintf("<body read error: %v>", err)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	dump, err := httputil.DumpRequest(r, false)
	if err != nil {
		return fmt.Sprintf("<dump error: %v>", err)
	}

	return fmt.Sprintf("%s\n%s", dump, formatBody(body))
}

// dumpCassetteRequest renders a recorded [Request] in a wire-format-like form
// for visual diffing against an [*http.Request] dumped via [dumpHTTPRequest].
func dumpCassetteRequest(req Request) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s %s\n", req.Method, req.URL, req.Proto)
	fmt.Fprintf(&b, "Host: %s\n", req.Host)
	if err := req.Headers.Write(&b); err != nil {
		fmt.Fprintf(&b, "<header write error: %v>\n", err)
	}
	b.WriteString("\n")
	b.WriteString(formatBody([]byte(req.Body)))

	return b.String()
}

const (
	// CassetteFormatVersion is the supported cassette version.
	CassetteFormatVersion = 2
)

var (
	// ErrInteractionNotFound indicates that a requested interaction was not
	// found in the cassette file.
	ErrInteractionNotFound = errors.New("requested interaction not found")

	// ErrCassetteNotFound indicates that a requested cassette doesn't exist.
	ErrCassetteNotFound = errors.New("requested cassette not found")

	// ErrUnsupportedCassetteFormat is returned when attempting to use an
	// older and potentially unsupported format of a cassette.
	ErrUnsupportedCassetteFormat = errors.New("unsupported cassette version format")
)

// Request represents a client request as recorded in the cassette file.
type Request struct {
	Proto            string      `yaml:"proto"`
	ProtoMajor       int         `yaml:"proto_major"`
	ProtoMinor       int         `yaml:"proto_minor"`
	ContentLength    int64       `yaml:"content_length"`
	TransferEncoding []string    `yaml:"transfer_encoding,omitempty"`
	Trailer          http.Header `yaml:"trailer,omitempty"`
	Host             string      `yaml:"host"`
	RemoteAddr       string      `yaml:"remote_addr,omitempty"`
	RequestURI       string      `yaml:"request_uri,omitempty"`

	// Body of request
	Body string `yaml:"body,omitempty"`

	// Form values
	Form url.Values `yaml:"form,omitempty"`

	// Request headers
	Headers http.Header `yaml:"headers,omitempty"`

	// Request URL
	URL string `yaml:"url"`

	// Request method
	Method string `yaml:"method"`
}

// String implements [fmt.Stringer]. It renders the recorded request as a
// compact, single-line summary suitable for inclusion in a debug log
// attribute. The body is bounded and stripped of newlines so it fits cleanly
// in a key=value line.
func (r Request) String() string {
	return fmt.Sprintf("%s %s body=%q", r.Method, r.URL, summarizeBody(r.Body))
}

// Response represents a server response as recorded in the cassette file.
type Response struct {
	Proto            string      `yaml:"proto"`
	ProtoMajor       int         `yaml:"proto_major"`
	ProtoMinor       int         `yaml:"proto_minor"`
	TransferEncoding []string    `yaml:"transfer_encoding,omitempty"`
	Trailer          http.Header `yaml:"trailer,omitempty"`
	ContentLength    int64       `yaml:"content_length"`
	Uncompressed     bool        `yaml:"uncompressed,omitempty"`

	// Body of response
	Body string `yaml:"body"`

	// Response headers
	Headers http.Header `yaml:"headers"`

	// Response status message
	Status string `yaml:"status"`

	// Response status code
	Code int `yaml:"code"`

	// Response duration
	Duration time.Duration `yaml:"duration"`
}

// String implements [fmt.Stringer]. It renders the recorded response as a
// compact, single-line summary suitable for inclusion in a debug log
// attribute.
func (r Response) String() string {
	return fmt.Sprintf("%d body=%q", r.Code, summarizeBody(r.Body))
}

// Interaction type contains a pair of request/response for a single HTTP
// interaction between a client and a server.
type Interaction struct {
	// ID is the id of the interaction
	ID int `yaml:"id"`

	// Request is the recorded request
	Request Request `yaml:"request"`

	// Response is the recorded response
	Response Response `yaml:"response"`

	// DiscardOnSave if set to true will discard the interaction as a whole
	// and it will not be part of the final interactions when saving the
	// cassette on disk.
	DiscardOnSave bool `yaml:"-"`

	// replayed is true when this interaction has been played already.
	replayed bool `yaml:"-"`
}

// WasReplayed returns a boolean indicating whether the given interaction was
// already replayed.
func (i *Interaction) WasReplayed() bool {
	return i.replayed
}

// GetHTTPRequest converts the recorded interaction request to http.Request
// instance.
func (i *Interaction) GetHTTPRequest() (*http.Request, error) {
	url, err := url.Parse(i.Request.URL)
	if err != nil {
		return nil, err
	}

	req := &http.Request{
		Proto:            i.Request.Proto,
		ProtoMajor:       i.Request.ProtoMajor,
		ProtoMinor:       i.Request.ProtoMinor,
		ContentLength:    i.Request.ContentLength,
		TransferEncoding: i.Request.TransferEncoding,
		Trailer:          i.Request.Trailer,
		Host:             i.Request.Host,
		RemoteAddr:       i.Request.RemoteAddr,
		RequestURI:       i.Request.RequestURI,
		Body:             io.NopCloser(strings.NewReader(i.Request.Body)),
		Form:             i.Request.Form,
		Header:           i.Request.Headers,
		URL:              url,
		Method:           i.Request.Method,
	}

	return req, nil
}

// GetHTTPResponse converts the recorded interaction response to http.Response
// instance.
func (i *Interaction) GetHTTPResponse() (*http.Response, error) {
	req, err := i.GetHTTPRequest()
	if err != nil {
		return nil, err
	}

	resp := &http.Response{
		Status:           i.Response.Status,
		StatusCode:       i.Response.Code,
		Proto:            i.Response.Proto,
		ProtoMajor:       i.Response.ProtoMajor,
		ProtoMinor:       i.Response.ProtoMinor,
		TransferEncoding: i.Response.TransferEncoding,
		Trailer:          i.Response.Trailer,
		ContentLength:    i.Response.ContentLength,
		Uncompressed:     i.Response.Uncompressed,
		Body:             io.NopCloser(strings.NewReader(i.Response.Body)),
		Header:           i.Response.Headers,
		Close:            true,
		Request:          req,
	}

	return resp, nil
}

// MatcherFunc is a predicate, which returns true when the actual request
// matches an interaction from the cassette.
type MatcherFunc func(*http.Request, Request) bool

// MarshalFunc is a function which marshals an object to a byte slice.
type MarshalFunc func(any) ([]byte, error)

// defaultMatcher is the default matcher used to match HTTP requests with
// recorded interactions.
type defaultMatcher struct {
	// If set, the default matcher will ignore matching on any of the
	// defined headers.
	ignoreHeaders []string
}

// DefaultMatcherOption is a function which configures the default matcher.
type DefaultMatcherOption func(m *defaultMatcher)

// WithIgnoreUserAgent is a [DefaultMatcherOption], which configures the default
// matcher to ignore matching on the User-Agent HTTP header.
func WithIgnoreUserAgent() DefaultMatcherOption {
	opt := func(m *defaultMatcher) {
		m.ignoreHeaders = append(m.ignoreHeaders, "User-Agent")
	}

	return opt
}

// WithIgnoreAuthorization is a [DefaultMatcherOption], which configures the default
// matcher to ignore matching on the Authorization HTTP header.
func WithIgnoreAuthorization() DefaultMatcherOption {
	opt := func(m *defaultMatcher) {
		m.ignoreHeaders = append(m.ignoreHeaders, "Authorization")
	}

	return opt
}

// WithIgnoreHeaders is a [DefaultMatcherOption], which configures the default
// matcher to ignore matching on the defined HTTP headers.
func WithIgnoreHeaders(val ...string) DefaultMatcherOption {
	opt := func(m *defaultMatcher) {
		m.ignoreHeaders = append(m.ignoreHeaders, val...)
	}

	return opt
}

// NewDefaultMatcher returns the default matcher.
func NewDefaultMatcher(opts ...DefaultMatcherOption) MatcherFunc {
	return newDefaultMatcher(opts...).matcher
}

// newDefaultMatcher returns a default matcher configured with the given
// options.
func newDefaultMatcher(opts ...DefaultMatcherOption) *defaultMatcher {
	m := &defaultMatcher{}
	for _, opt := range opts {
		opt(m)
	}

	return m
}

// Similar to reflect.DeepEqual, but considers the contents of collections, so
// {} and nil would be considered equal. works with Array, Map, Slice, or
// pointer to Array.
func (m *defaultMatcher) deepEqualContents(x, y any) bool {
	if reflect.ValueOf(x).IsNil() {
		if reflect.ValueOf(y).IsNil() {
			return true
		} else {
			return reflect.ValueOf(y).Len() == 0
		}
	} else {
		if reflect.ValueOf(y).IsNil() {
			return reflect.ValueOf(x).Len() == 0
		} else {
			return reflect.DeepEqual(x, y)
		}
	}
}

// bodyMatches is a predicate which tests whether the bodies of the given HTTP
// request and interaction request match.
func (m *defaultMatcher) bodyMatches(r *http.Request, i Request) bool {
	return m.bodyMismatch(r, i) == nil
}

// bodyMismatch returns a [FieldMismatch] describing how the bodies of the given
// HTTP request and interaction request differ, or nil when they match.
func (m *defaultMatcher) bodyMismatch(r *http.Request, i Request) *FieldMismatch {
	if r.Body != nil {
		var buffer bytes.Buffer
		if _, err := buffer.ReadFrom(r.Body); err != nil {
			return &FieldMismatch{
				Field:    "body",
				Expected: i.Body,
				Actual:   fmt.Sprintf("<body read error: %v>", err),
			}
		}

		r.Body = io.NopCloser(bytes.NewReader(buffer.Bytes()))
		if buffer.String() != i.Body {
			return &FieldMismatch{
				Field:    "body",
				Expected: i.Body,
				Actual:   buffer.String(),
			}
		}
	} else {
		if len(i.Body) != 0 {
			return &FieldMismatch{
				Field:    "body",
				Expected: i.Body,
				Actual:   "",
			}
		}
	}

	return nil
}

// maxReportedMismatches is the maximum number of field mismatches reported for
// a single recorded interaction, so that a completely different request does
// not produce an unreadable error message.
const maxReportedMismatches = 12

// maxReportedValueLen is the maximum length of a value included in a mismatch
// report. Longer values are truncated.
const maxReportedValueLen = 200

// FieldMismatch describes a single field which differs between an actual HTTP
// request and a recorded interaction request.
type FieldMismatch struct {
	// Field is the name of the field which differs. Header names are prefixed
	// with "header:".
	Field string
	// Expected is the value recorded in the cassette.
	Expected string
	// Actual is the value of the actual HTTP request.
	Actual string
}

// String returns a human-readable representation of the field mismatch.
func (f FieldMismatch) String() string {
	return fmt.Sprintf("%s: cassette has %q, request has %q",
		f.Field, truncateValue(f.Expected), truncateValue(f.Actual))
}

// truncateValue shortens v, so that mismatch reports stay readable.
func truncateValue(v string) string {
	if len(v) <= maxReportedValueLen {
		return v
	}

	return v[:maxReportedValueLen] + "... (truncated)"
}

// InteractionMismatch describes how an actual HTTP request differs from one
// recorded interaction.
type InteractionMismatch struct {
	// ID is the ID of the recorded interaction.
	ID int
	// Request describes the recorded request, e.g. "GET https://example.com".
	Request string
	// Mismatches holds the fields which differ between the recorded request
	// and the actual HTTP request.
	Mismatches []FieldMismatch
	// Truncated reports whether Mismatches was truncated at
	// [maxReportedMismatches] entries.
	Truncated bool
}

// MatchFailureError is returned by [Cassette.GetInteraction] when the actual
// HTTP request does not match any of the recorded interactions in the cassette.
//
// It reports the recorded interaction which came closest to matching and the
// fields which differ, so that a failing test does not require diffing the
// cassette by hand.
//
// A *MatchFailureError wraps [ErrInteractionNotFound], so code which checks for
// that sentinel with [errors.Is] keeps working.
type MatchFailureError struct {
	// Cassette is the cassette file which was searched.
	Cassette string
	// Request describes the actual request which failed to match.
	Request string
	// Considered is the number of recorded interactions which were eligible
	// for matching.
	Considered int
	// Closest is the recorded interaction which came closest to matching the
	// actual request. It is nil when no interaction was eligible.
	Closest *InteractionMismatch
}

// Error implements the error interface.
func (e *MatchFailureError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s", ErrInteractionNotFound, e.Request)

	fmt.Fprintf(&b, " (cassette %q, %d interaction(s) considered)", e.Cassette, e.Considered)

	if e.Closest == nil {
		return b.String()
	}

	fmt.Fprintf(&b, "\nclosest match is interaction %d (%s), %d field(s) differ:",
		e.Closest.ID, e.Closest.Request, len(e.Closest.Mismatches))

	for _, m := range e.Closest.Mismatches {
		fmt.Fprintf(&b, "\n  - %s", m)
	}

	if e.Closest.Truncated {
		b.WriteString("\n  - ... (further differences omitted)")
	}

	return b.String()
}

// Unwrap returns [ErrInteractionNotFound], so that [errors.Is] checks against
// the sentinel error keep working.
func (e *MatchFailureError) Unwrap() error {
	return ErrInteractionNotFound
}

// diffHeaders reports the headers which differ between the recorded request
// headers and the headers of the actual HTTP request.
func diffHeaders(expected, actual map[string][]string) []FieldMismatch {
	keys := make([]string, 0, len(expected)+len(actual))
	for k := range expected {
		keys = append(keys, k)
	}

	for k := range actual {
		if _, ok := expected[k]; !ok {
			keys = append(keys, k)
		}
	}

	sort.Strings(keys)

	var mm []FieldMismatch
	for _, k := range keys {
		want := strings.Join(expected[k], ", ")
		got := strings.Join(actual[k], ", ")
		if want == got {
			continue
		}

		mm = append(mm, FieldMismatch{
			Field:    "header:" + k,
			Expected: want,
			Actual:   got,
		})
	}

	return mm
}

// formatStringMap renders a map of string slices, such as HTTP headers or form
// values, in a stable order.
func formatStringMap(m map[string][]string) string {
	if len(m) == 0 {
		return "{}"
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("{")

	for idx, k := range keys {
		if idx > 0 {
			b.WriteString(", ")
		}

		fmt.Fprintf(&b, "%s: %s", k, strings.Join(m[k], ", "))
	}

	b.WriteString("}")

	return b.String()
}

// explain is the detailed counterpart of [defaultMatcher.matcher]. It reports
// each field which differs between the actual request and the recorded
// interaction request. Its second return value is identical to the value
// returned by matcher.
func (m *defaultMatcher) explain(r *http.Request, i Request) ([]FieldMismatch, bool) {
	var mm []FieldMismatch

	add := func(field, expected, actual string) {
		mm = append(mm, FieldMismatch{
			Field:    field,
			Expected: expected,
			Actual:   actual,
		})
	}

	if r.Method != i.Method {
		add("method", i.Method, r.Method)
	}

	if r.URL.String() != i.URL {
		add("url", i.URL, r.URL.String())
	}

	if r.Proto != i.Proto {
		add("proto", i.Proto, r.Proto)
	}

	if r.ProtoMajor != i.ProtoMajor {
		add("proto_major", strconv.Itoa(i.ProtoMajor), strconv.Itoa(r.ProtoMajor))
	}

	if r.ProtoMinor != i.ProtoMinor {
		add("proto_minor", strconv.Itoa(i.ProtoMinor), strconv.Itoa(r.ProtoMinor))
	}

	requestHeader := r.Header.Clone()
	cassetteRequestHeaders := i.Headers.Clone()

	for _, header := range m.ignoreHeaders {
		delete(requestHeader, header)
		delete(cassetteRequestHeaders, header)
	}

	if !m.deepEqualContents(requestHeader, cassetteRequestHeaders) {
		headerMismatches := diffHeaders(cassetteRequestHeaders, requestHeader)
		if len(headerMismatches) == 0 {
			// deepEqualContents considers nil and empty collections equal, so
			// differing contents with no per-header difference are still
			// reported here.
			headerMismatches = append(headerMismatches, FieldMismatch{
				Field:    "headers",
				Expected: formatStringMap(cassetteRequestHeaders),
				Actual:   formatStringMap(requestHeader),
			})
		}

		mm = append(mm, headerMismatches...)
	}

	if mismatch := m.bodyMismatch(r, i); mismatch != nil {
		mm = append(mm, *mismatch)
	}

	if r.ContentLength != i.ContentLength {
		add("content_length", strconv.FormatInt(i.ContentLength, 10), strconv.FormatInt(r.ContentLength, 10))
	}

	if !m.deepEqualContents(r.TransferEncoding, i.TransferEncoding) {
		add("transfer_encoding", strings.Join(i.TransferEncoding, ", "), strings.Join(r.TransferEncoding, ", "))
	}

	if r.Host != i.Host {
		add("host", i.Host, r.Host)
	}

	if err := r.ParseForm(); err != nil {
		add("form", formatStringMap(i.Form), fmt.Sprintf("<%v>", err))
	} else if !m.deepEqualContents(r.Form, i.Form) {
		add("form", formatStringMap(i.Form), formatStringMap(r.Form))
	}

	if !m.deepEqualContents(r.Trailer, i.Trailer) {
		add("trailer", formatStringMap(i.Trailer), formatStringMap(r.Trailer))
	}

	if r.RemoteAddr != i.RemoteAddr {
		add("remote_addr", i.RemoteAddr, r.RemoteAddr)
	}

	if r.RequestURI != i.RequestURI {
		add("request_uri", i.RequestURI, r.RequestURI)
	}

	return mm, len(mm) == 0
}

// matcher is a predicate which matches the provided HTTP request again a
// recorded interaction request.
func (m *defaultMatcher) matcher(r *http.Request, i Request) bool {
	_, matched := m.explain(r, i)

	return matched
}

// defaultExplainer reports the fields which differ between an actual HTTP
// request and a recorded interaction request, as computed by the default
// matcher configured with default options.
func defaultExplainer(r *http.Request, i Request) ([]FieldMismatch, bool) {
	return newDefaultMatcher().explain(r, i)
}

// DefaultMatcher is the default matcher used to match HTTP requests with
// recorded interactions
var DefaultMatcher = NewDefaultMatcher()

// Cassette represents a cassette containing recorded interactions.
type Cassette struct {
	mu sync.Mutex `yaml:"-"`

	// Name of the cassette
	Name string `yaml:"-"`

	// File name of the cassette as written on disk
	File string `yaml:"-"`

	// Cassette format version
	Version int `yaml:"version"`

	// Interactions between client and server
	Interactions []*Interaction `yaml:"interactions"`

	// ReplayableInteractions defines whether to allow
	// interactions to be replayed or not
	ReplayableInteractions bool `yaml:"-"`

	// Matches actual request with interaction requests.
	Matcher MatcherFunc `yaml:"-"`

	// IsNew specifies whether this is a newly created cassette.
	// Returns false, when the cassette was loaded from an
	// existing source, e.g. a file.
	IsNew bool `yaml:"-"`

	nextInteractionId int `yaml:"-"`

	// MarshalFunc is a custom marshal func.
	MarshalFunc MarshalFunc `yaml:"-"`

	// DebugLogger is an [slog.Logger] which is used to emit debug events.
	// To enable debugging, see [recorder.WithDebugWriter] option.
	DebugLogger *slog.Logger `yaml:"-"`

	// matchDetails specifies whether match failures are reported as a
	// [MatchFailureError] carrying the fields which differ, instead of the
	// plain [ErrInteractionNotFound] sentinel error.
	matchDetails bool `yaml:"-"`
}

// DisableMatchDetails configures the cassette to return the plain
// [ErrInteractionNotFound] sentinel error on match failures, instead of a
// [MatchFailureError] with details.
//
// A custom [MatcherFunc] cannot describe why two requests differ, so the
// recorder disables match details when one is configured.
func (c *Cassette) DisableMatchDetails() {
	c.matchDetails = false
}

// New creates a new empty cassette
func New(name string) *Cassette {
	c := &Cassette{
		Name:                   name,
		File:                   fmt.Sprintf("%s.yaml", name),
		Version:                CassetteFormatVersion,
		Interactions:           make([]*Interaction, 0),
		Matcher:                DefaultMatcher,
		ReplayableInteractions: false,
		IsNew:                  true,
		nextInteractionId:      0,
		DebugLogger:            slog.New(slog.DiscardHandler),
		matchDetails:           true,
	}

	return c
}

// Load reads a cassette file from disk
func Load(name string) (*Cassette, error) {
	return LoadWithFS(name, NewDiskFS())
}

// Load reads a cassette file from disk
func LoadWithFS(name string, fs FS) (*Cassette, error) {
	c := New(name)
	data, err := fs.ReadFile(c.File)
	if err != nil {
		return nil, err
	}

	c.IsNew = false
	if err := yaml.Unmarshal(data, c); err != nil {
		return nil, err
	}

	if c.Version != CassetteFormatVersion {
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedCassetteFormat, CassetteFormatVersion)
	}
	c.nextInteractionId = len(c.Interactions)

	// Match details are not persisted in the cassette.
	c.matchDetails = true

	return c, err
}

// AddInteraction appends a new interaction to the cassette
func (c *Cassette) AddInteraction(i *Interaction) {
	c.mu.Lock()
	defer c.mu.Unlock()
	i.ID = c.nextInteractionId
	c.nextInteractionId += 1
	c.Interactions = append(c.Interactions, i)
	c.debug("interaction added",
		"id", i.ID,
		"request", i.Request,
		"response", i.Response,
	)
}

// GetInteraction retrieves a recorded request/response interaction
func (c *Cassette) GetInteraction(r *http.Request) (*Interaction, error) {
	return c.getInteraction(r)
}

// debug emits a debug event
func (c *Cassette) debug(msg string, args ...any) {
	c.DebugLogger.Debug(msg, args...)
}

// getInteraction searches for the interaction corresponding to the given HTTP
// request, by using the configured [MatcherFunc].
func (c *Cassette) getInteraction(r *http.Request) (*Interaction, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r.Body == nil {
		// causes an error in the matcher when we try to do r.ParseForm if r.Body is nil
		// r.ParseForm returns missing form body error
		r.Body = http.NoBody
	}
	c.debug("matching request",
		"method", r.Method,
		"url", r.URL,
		"host", r.Host,
		"dump", dumpHTTPRequest(r),
	)
	replayed := 0
	considered := 0
	var closest *InteractionMismatch

	for _, i := range c.Interactions {
		if i.replayed {
			replayed++
		}
		eligible := c.ReplayableInteractions || !i.replayed
		matched := eligible && c.Matcher(r, i.Request)
		attrs := []any{
			"id", i.ID,
			"already_replayed", i.replayed,
			"eligible", eligible,
			"matched", matched,
			"request", i.Request,
		}
		if !matched && eligible {
			attrs = append(attrs, "dump", dumpCassetteRequest(i.Request))

			considered++

			// Keep the interaction which came closest to matching, so that the
			// reported mismatch is the most useful one, instead of the first
			// one in the cassette.
			if c.matchDetails {
				mismatches, matchedByDetail := defaultExplainer(r, i.Request)
				if !matchedByDetail && (closest == nil || len(mismatches) < len(closest.Mismatches)) {
					closest = &InteractionMismatch{
						ID:         i.ID,
						Request:    fmt.Sprintf("%s %s", i.Request.Method, i.Request.URL),
						Mismatches: mismatches,
					}
				}
			}
		}
		c.debug("match attempt", attrs...)
		if matched {
			i.replayed = true
			c.debug("match found",
				"id", i.ID,
				"request", i.Request,
			)

			return i, nil
		}
	}
	c.debug("no match", "already_replayed_count", replayed)

	return nil, c.matchFailureError(r, considered, closest)
}

// matchFailureError returns the error reported when the actual request does not
// match any of the recorded interactions in the cassette.
func (c *Cassette) matchFailureError(r *http.Request, considered int, closest *InteractionMismatch) error {
	if closest == nil {
		return ErrInteractionNotFound
	}

	if len(closest.Mismatches) > maxReportedMismatches {
		closest.Mismatches = closest.Mismatches[:maxReportedMismatches]
		closest.Truncated = true
	}

	return &MatchFailureError{
		Cassette:   c.File,
		Request:    fmt.Sprintf("%s %s", r.Method, r.URL),
		Considered: considered,
		Closest:    closest,
	}
}

// Save writes the cassette data on disk for future re-use
func (c *Cassette) Save() error {
	return c.SaveWithFS(NewDiskFS())
}

// SaveWithFS writes the cassette data on abstract filesystem for future re-use
func (c *Cassette) SaveWithFS(fs FS) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.debug("saving cassette", "interaction_count", len(c.Interactions))

	// Filter out interactions which should be discarded. While discarding
	// interactions we should also fix the interaction IDs, so that we don't
	// introduce gaps in the final results.
	nextId := 0
	interactions := make([]*Interaction, 0)
	for _, i := range c.Interactions {
		if i.DiscardOnSave {
			c.debug("discarding interaction", "id", i.ID)
		} else {
			i.ID = nextId
			interactions = append(interactions, i)
			nextId += 1
		}
	}
	c.Interactions = interactions

	// Marshal to YAML and save interactions
	data, err := c.MarshalFunc(c)
	if err != nil {
		return err
	}

	// Honor the YAML structure specification
	// http://www.yaml.org/spec/1.2/spec.html#id2760395
	payload := append([]byte("---\n"), data...)
	if err := fs.WriteFile(c.File, payload); err != nil {
		return err
	}
	c.debug("cassette saved", "bytes_written", len(payload), "interactions_saved", len(c.Interactions))

	return nil
}
