package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
)

const (
	reset = "\033[0m"

	red        = 31
	yellow     = 33
	cyan       = 36
	darkGray   = 90
	lightGreen = 92
	white      = 97

	timeFormat      = "[15:04:05.000]"
	maxInlineFields = 5
)

func colorize(colorCode int, v string) string {
	return fmt.Sprintf("\033[%sm%s%s", strconv.Itoa(colorCode), v, reset)
}

type Handler struct {
	h slog.Handler
	b *bytes.Buffer
	m *sync.Mutex
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.h.Enabled(ctx, level)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{h: h.h.WithAttrs(attrs), b: h.b, m: h.m}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{h: h.h.WithGroup(name), b: h.b, m: h.m}
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	level := r.Level.String() + ":"

	switch r.Level {
	case slog.LevelDebug:
		level = colorize(cyan, level)
	case slog.LevelInfo:
		level = colorize(lightGreen, level)
	case slog.LevelWarn:
		level = colorize(yellow, level)
	case slog.LevelError:
		level = colorize(red, level)
	}

	attrs, err := h.computeAttrs(ctx, r)
	if err != nil {
		return err
	}

	if len(attrs) <= maxInlineFields {
		var attrStr string
		// Single line format
		pairs := make([]string, 0, len(attrs))
		for k, v := range attrs {
			pairs = append(pairs, fmt.Sprintf("\"%s\": \"%v\"", k, v))
		}
		if len(pairs) > 0 {
			attrStr = fmt.Sprintf("{ %s }", strings.Join(pairs, " "))
		} else {
			attrStr = ""
		}

		fmt.Println(
			colorize(white, r.Time.Format(timeFormat)),
			level,
			colorize(white, r.Message),
			colorize(darkGray, attrStr),
		)
	} else {
		// Multi-line format for many fields
		bytes, err := json.MarshalIndent(attrs, "", " ")
		if err != nil {
			return fmt.Errorf("error when marshaling attrs: %w", err)
		}

		fmt.Println(
			colorize(white, r.Time.Format(timeFormat)),
			level,
			colorize(white, r.Message),
			colorize(darkGray, string(bytes)),
		)
	}

	return nil
}

func (h *Handler) computeAttrs(
	ctx context.Context,
	r slog.Record,
) (map[string]any, error) {
	h.m.Lock()
	defer func() {
		h.b.Reset()
		h.m.Unlock()
	}()
	if err := h.h.Handle(ctx, r); err != nil {
		return nil, fmt.Errorf("error when calling inner handler's Handle: %w", err)
	}

	var attrs map[string]any
	err := json.Unmarshal(h.b.Bytes(), &attrs)
	if err != nil {
		return nil, fmt.Errorf("error when unmarshaling inner handler's Handle result: %w", err)
	}
	return attrs, nil
}

func NewHandler(opts *slog.HandlerOptions) *Handler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	b := &bytes.Buffer{}
	return &Handler{
		b: b,
		h: slog.NewJSONHandler(b, &slog.HandlerOptions{
			Level:       opts.Level,
			AddSource:   opts.AddSource,
			ReplaceAttr: supressDefaults(opts.ReplaceAttr),
		}),
		m: &sync.Mutex{},
	}
}

func supressDefaults(
	next func([]string, slog.Attr) slog.Attr,
) func([]string, slog.Attr) slog.Attr {
	return func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey ||
			a.Key == slog.LevelKey ||
			a.Key == slog.MessageKey {
			return slog.Attr{}
		}
		if next == nil {
			return a
		}
		return next(groups, a)
	}
}

// Only called after we set the default logger
func NewWithComponent(component string) *slog.Logger {
	return slog.With("component", component)
}
