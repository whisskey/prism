// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

import (
	"fmt"
	"go/token"
	"log"
)

// Warning is a non-fatal fibergen analysis issue.
type Warning struct {
	Pos     token.Position
	Message string
}

// WarnLogger collects warnings and optionally writes them to the standard log.
type WarnLogger struct {
	warnings []Warning
	logSink  bool
}

// NewWarnLogger creates a warning collector. When logSink is true, warnings are
// also printed via log.Printf.
func NewWarnLogger(logSink bool) *WarnLogger {
	return &WarnLogger{logSink: logSink}
}

// Warnf records a warning at pos.
func (w *WarnLogger) Warnf(pos token.Position, format string, args ...any) {
	if w == nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	w.warnings = append(w.warnings, Warning{Pos: pos, Message: msg})
	if w.logSink {
		if pos.IsValid() {
			log.Printf("WARN: %s:%d: %s", pos.Filename, pos.Line, msg)
		} else {
			log.Printf("WARN: %s", msg)
		}
	}
}

// All returns collected warnings.
func (w *WarnLogger) All() []Warning {
	if w == nil {
		return nil
	}
	return w.warnings
}

func (ctx Context) warnf(pos token.Position, format string, args ...any) {
	if ctx.Warn != nil {
		ctx.Warn.Warnf(pos, format, args...)
	}
}

func warnMissingResponseType(ctx Context, info HandlerDetail) {
	if info.ResponseType != "" || len(info.ResponseSchemas) > 0 {
		return
	}
	pos := token.Position{Filename: info.SourceFile, Line: info.SourceLine}
	ctx.warnf(pos, "%s handler'ının response type'ı belirlenemedi — annotation gerekebilir", info.Name)
}

func warnHandlerNotFound(ctx Context, route Route) {
	if route.Handler == "" {
		return
	}
	pos := token.Position{Filename: route.SourceFile, Line: route.SourceLine}
	ctx.warnf(pos,
		"handler %q bulunamadı — route eklendi, handler boş; cross-package için go/packages ile yeniden deneyin",
		route.Handler,
	)
}

func warnInterfaceType(ctx Context, pos token.Position, handlerName, context string) {
	ctx.warnf(pos, "%s handler %s tipi interface{} — atlandı, annotation gerekebilir", handlerName, context)
}

func warnUnresolvedSchema(ctx Context, typeName string) {
	ctx.warnf(token.Position{}, "struct %q resolve edilemedi — empty schema {} eklendi", typeName)
}
