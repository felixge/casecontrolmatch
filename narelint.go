package main

import "strings"

func ParseNARelInt(val string) (NARelInt, error) {
	if strings.ToLower(val) == "keine angabe" {
		return NARelInt{na: true}, nil
	}
	relInt, err := ParseRelInt(val)
	return NARelInt{RelInt: relInt}, err
}

func NewNARelInt(r RelInt, na bool) NARelInt {
	return NARelInt{RelInt: r, na: na}
}

type NARelInt struct {
	RelInt
	na bool
}

func (n NARelInt) NA() bool {
	return n.na
}

func (n NARelInt) String() string {
	if n.NA() {
		return "n/a"
	}
	return n.RelInt.String()
}
