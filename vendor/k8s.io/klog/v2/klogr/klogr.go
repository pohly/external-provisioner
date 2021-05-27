// Package klogr implements github.com/go-logr/logr.Logger in terms of
// k8s.io/klog.
package klogr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"strings"

	"github.com/go-logr/logr"

	"k8s.io/klog/v2"
	"k8s.io/klog/v2/internal/serialize"
)

// Option is a functional option that reconfigures the logger created with New.
type Option func(*klogger)

// Format defines how log output is produced.
type Format string

const (
	// FormatSerialize tells klogr to turn key/value pairs into text itself
	// before invoking klog.
	FormatSerialize Format = "Serialize"

	// FormatKlog tells klogr to pass all text messages and key/value pairs
	// directly to klog. Klog itself then serializes in a human-readable
	// format and optionally passes on to a structure logging backend.
	FormatKlog Format = "Klog"
)

// WithFormat selects the output format.
func WithFormat(format Format) Option {
	return func(l *klogger) {
		l.format = format
	}
}

// New returns a logr.Logger which serializes output itself
// and writes it via klog.
func New() logr.Logger {
	return NewWithOptions(WithFormat(FormatSerialize))
}

// NewWithOptions returns a logr.Logger which serializes as determined
// by the WithFormat option and writes via klog. The default is
// FormatKlog.
func NewWithOptions(options ...Option) logr.Logger {
	l := klogger{
		level:  0,
		prefix: "",
		values: nil,
		format: FormatKlog,
	}
	for _, option := range options {
		option(&l)
	}
	return l
}

type klogger struct {
	level     int
	callDepth int
	prefix    string
	values    []interface{}
	format    Format
}

func (l klogger) clone() klogger {
	return klogger{
		level:  l.level,
		prefix: l.prefix,
		values: copySlice(l.values),
		format: l.format,
	}
}

func copySlice(in []interface{}) []interface{} {
	out := make([]interface{}, len(in))
	copy(out, in)
	return out
}

// Magic string for intermediate frames that we should ignore.
const autogeneratedFrameName = "<autogenerated>"

// Discover how many frames we need to climb to find the caller. This approach
// was suggested by Ian Lance Taylor of the Go team, so it *should* be safe
// enough (famous last words).
//
// It is needed because binding the specific klogger functions to the
// logr interface creates one additional call frame that neither we nor
// our caller know about.
func framesToCaller() int {
	// 1 is the immediate caller.  3 should be too many.
	for i := 1; i < 3; i++ {
		_, file, _, _ := runtime.Caller(i + 1) // +1 for this function's frame
		if file != autogeneratedFrameName {
			return i
		}
	}
	return 1 // something went wrong, this is safe
}

func flatten(kvList ...interface{}) string {
	keys := make([]string, 0, len(kvList))
	vals := make(map[string]interface{}, len(kvList))
	for i := 0; i < len(kvList); i += 2 {
		k, ok := kvList[i].(string)
		if !ok {
			panic(fmt.Sprintf("key is not a string: %s", pretty(kvList[i])))
		}
		var v interface{}
		if i+1 < len(kvList) {
			v = kvList[i+1]
		}
		keys = append(keys, k)
		vals[k] = v
	}
	sort.Strings(keys)
	buf := bytes.Buffer{}
	for i, k := range keys {
		v := vals[k]
		if i > 0 {
			buf.WriteRune(' ')
		}
		buf.WriteString(pretty(k))
		buf.WriteString("=")
		buf.WriteString(pretty(v))
	}
	return buf.String()
}

func pretty(value interface{}) string {
	if err, ok := value.(error); ok {
		if _, ok := value.(json.Marshaler); !ok {
			value = err.Error()
		}
	}
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	encoder.Encode(value)
	return strings.TrimSpace(string(buffer.Bytes()))
}

func (l klogger) Info(msg string, kvList ...interface{}) {
	if l.Enabled() {
		switch l.format {
		case FormatSerialize:
			msgStr := flatten("msg", msg)
			trimmed := serialize.TrimDuplicates(l.values, kvList)
			fixedStr := flatten(trimmed[0]...)
			userStr := flatten(trimmed[1]...)
			klog.InfoDepth(framesToCaller()+l.callDepth, l.prefix, " ", msgStr, " ", fixedStr, " ", userStr)
		case FormatKlog:
			trimmed := serialize.TrimDuplicates(l.values, kvList)
			if l.prefix != "" {
				msg = l.prefix + ": " + msg
			}
			klog.InfoSDepth(framesToCaller()+l.callDepth, msg, append(trimmed[0], trimmed[1]...)...)
		}
	}
}

func (l klogger) Enabled() bool {
	return bool(klog.V(klog.Level(l.level)).Enabled())
}

func (l klogger) Error(err error, msg string, kvList ...interface{}) {
	msgStr := flatten("msg", msg)
	var loggableErr interface{}
	if err != nil {
		loggableErr = err.Error()
	}
	switch l.format {
	case FormatSerialize:
		errStr := flatten("error", loggableErr)
		trimmed := serialize.TrimDuplicates(l.values, kvList)
		fixedStr := flatten(trimmed[0]...)
		userStr := flatten(trimmed[1]...)
		klog.ErrorDepth(framesToCaller()+l.callDepth, l.prefix, " ", msgStr, " ", errStr, " ", fixedStr, " ", userStr)
	case FormatKlog:
		trimmed := serialize.TrimDuplicates(l.values, kvList)
		if l.prefix != "" {
			msg = l.prefix + ": " + msg
		}
		klog.ErrorSDepth(framesToCaller()+l.callDepth, err, msg, append(trimmed[0], trimmed[1]...)...)
	}
}

func (l klogger) V(level int) logr.Logger {
	new := l.clone()
	new.level = level
	return new
}

// WithName returns a new logr.Logger with the specified name appended.  klogr
// uses '/' characters to separate name elements.  Callers should not pass '/'
// in the provided name string, but this library does not actually enforce that.
func (l klogger) WithName(name string) logr.Logger {
	new := l.clone()
	if len(l.prefix) > 0 {
		new.prefix = l.prefix + "/"
	}
	new.prefix += name
	return new
}

func (l klogger) WithValues(kvList ...interface{}) logr.Logger {
	new := l.clone()
	new.values = append(new.values, kvList...)
	return new
}

func (l klogger) WithCallDepth(depth int) logr.Logger {
	new := l.clone()
	new.callDepth += depth
	return new
}

var _ logr.Logger = klogger{}
var _ logr.CallDepthLogger = klogger{}