package main

import (
	"encoding/csv"
	"fmt"
)

type ContingencySubject interface {
	Top() string
	Left() string
}

func WriteContingency(w *csv.Writer, top, left []string, subjects []ContingencySubject) error {
	topRow := []string{"Title"}
	for _, v := range top {
		topRow = append(topRow, v)
	}
	if err := w.Write(topRow); err != nil {
		return err
	}
	r := map[string]map[string]int{}
	for _, s := range subjects {
		tops := r[s.Left()]
		if tops == nil {
			r[s.Left()] = map[string]int{}
			tops = r[s.Left()]
		}
		tops[s.Top()]++
	}
	for _, l := range left {
		row := []string{l}
		for _, t := range top {
			val := fmt.Sprintf("%d", r[l][t])
			row = append(row, val)
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return nil
}

func IgG_MS_GKSubjects(subject []*Subject) []ContingencySubject {
	r := []ContingencySubject{}
	for _, s := range subject {
		r = append(r, IgG_MS_GKSubject{s})
	}
	return r
}

type IgG_MS_GKSubject struct {
	*Subject
}

func (s IgG_MS_GKSubject) Top() string {
	if s.Diagnosis != GK {
		return "MS"
	}
	return "GK"
}

func (s IgG_MS_GKSubject) Left() string {
	return s.IgG.String()
}
