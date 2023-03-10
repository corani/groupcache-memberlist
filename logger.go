package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/mailgun/groupcache/v2"
)

type field struct {
	key   string
	value any
}

type logger struct {
	level  string
	fields []field
}

func (l *logger) Error() groupcache.Logger {
	return &logger{
		level:  "ERROR",
		fields: l.fields,
	}
}

func (l *logger) Warn() groupcache.Logger {
	return &logger{
		level:  "WARN",
		fields: l.fields,
	}
}

func (l *logger) Info() groupcache.Logger {
	return &logger{
		level:  "INFO",
		fields: l.fields,
	}
}

func (l *logger) Debug() groupcache.Logger {
	return &logger{
		level:  "DEBUG",
		fields: l.fields,
	}
}

func (l *logger) ErrorField(label string, err error) groupcache.Logger {
	return &logger{
		level:  l.level,
		fields: append(l.fields, field{label, err}),
	}
}

func (l *logger) StringField(label string, val string) groupcache.Logger {
	return &logger{
		level:  l.level,
		fields: append(l.fields, field{label, val}),
	}
}

func (l *logger) WithFields(fields map[string]interface{}) groupcache.Logger {
	res := &logger{
		level:  l.level,
		fields: l.fields,
	}

	for k, v := range fields {
		res.fields = append(res.fields, field{k, v})
	}

	return res
}

func (l *logger) Printf(format string, args ...interface{}) {
	if len(l.fields) > 0 {
		format += " {"
		for _, v := range l.fields {
			format = format + v.key + "=" + fmt.Sprint(v.value) + ","
		}
		format += "}"
	}

	format = fmt.Sprintf("[%s]", l.level) + format

	log.Printf(format, args...)
}

func newLogger() *logger {
	return &logger{}
}

type loggingTransport struct {
	http.RoundTripper
}

func (t *loggingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	log.Printf("[DEBUG] groupcache request: %v", r.URL)

	return t.RoundTripper.RoundTrip(r)
}
