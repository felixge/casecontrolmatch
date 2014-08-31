package main

import (
	"fmt"
	"strconv"
)
import "strings"

func ParseRelInt(val string) (RelInt, error) {
	r := RelInt{kind: Eq}
	for _, k := range []RelIntKind{GtEq, LtEq, Lt, Gt, Eq} {
		if strings.HasPrefix(val, string(k)) {
			r.kind = k
			val = strings.TrimPrefix(val, string(k))
			break
		}
	}
	intVal, err := strconv.ParseInt(val, 10, 64)
	r.val = int(intVal)
	return r, err
}

func NewRelInt(kind RelIntKind, val int) RelInt {
	return RelInt{kind: kind, val: val}
}

// RelInt is an integer, or a relative integer e.g. >5, >=3, <4, etc.
type RelInt struct {
	kind RelIntKind
	val  int
}

func (r RelInt) Kind() RelIntKind {
	if r.kind == "" {
		r.kind = Eq
	}
	return r.kind
}

func (r RelInt) Val() int {
	return r.val
}

func (r RelInt) String() string {
	if r.Kind() == Eq {
		return fmt.Sprintf("%d", r.val)
	}
	return fmt.Sprintf("%s%d", r.kind, r.val)
}

type RelIntKind string

const (
	Eq   RelIntKind = "="
	Gt   RelIntKind = ">"
	Lt   RelIntKind = "<"
	GtEq RelIntKind = ">="
	LtEq RelIntKind = "<="
)
