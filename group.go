package main

import (
	"encoding/csv"
	"fmt"
	"strings"
)

type Group interface {
	String() string
}
type GroupSubject interface {
	Group() Group
	String() string
}

func WriteGroupValues(w *csv.Writer, groups []Group, subjects []GroupSubject) error {
	header := []string{}
	for _, group := range groups {
		header = append(header, group.String())
	}
	if err := w.Write(header); err != nil {
		return err
	}
	results := map[Group][]string{}
	for _, s := range subjects {
		val := s.Group()
		match := false
		for _, group := range groups {
			if group.String() == val.String() {
				match = true
				results[group] = append(results[group], s.String())
				break
			}
		}
		if !match {
			groupStrings := []string{}
			for _, group := range groups {
				groupStrings = append(groupStrings, group.String())
			}
			groupsString := strings.Join(groupStrings, ",")
			return fmt.Errorf("%s did not match: %s", val, groupsString)
		}
	}
	i := 0
	for {
		found := false
		row := make([]string, len(groups))
		for g, group := range groups {
			if i < len(results[group]) {
				row[g] = results[group][i]
				found = true
			}
		}
		if !found {
			break
		}
		if err := w.Write(row); err != nil {
			return err
		}
		i++
	}
	return nil
}
