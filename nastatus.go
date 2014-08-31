package main

import (
	"fmt"
	"strings"
)

func ParseNAStatus(s string) (NAStatus, error) {
	s = strings.ToLower(s)
	if s == "positiv" {
		return NASPositiv, nil
	} else if s == "negativ" {
		return NASNegativ, nil
	} else if s == "keine angabe" || s == "" {
		return NASNA, nil
	}
	return "", fmt.Errorf("Bad NAStatus: %s", s)
}

type NAStatus string

const (
	NASPositiv NAStatus = "positiv"
	NASNegativ NAStatus = "negativ"
	NASNA      NAStatus = "n/a"
)

func (s NAStatus) String() string {
	return string(s)
}
