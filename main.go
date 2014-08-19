package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func main() {
	start := time.Now()
	flag.Usage = func() {
		fatalf("./main <input.csv> <outputDir>")
	}
	flag.Parse()
	inputFile := flag.Arg(0)
	if inputFile == "" {
		flag.Usage()
	}
	outputDir := flag.Arg(1)
	if outputDir == "" {
		flag.Usage()
	} else if outputDir == "." {
		fmt.Printf("Output dir must not be working directory")
	}
	readStart := time.Now()
	subjects, err := readSubjects(inputFile)
	if err != nil {
		fatalf("readSubjects: %s", err)
	}
	fmt.Printf("readSubjects: %s\n", time.Since(readStart))
	matchStart := time.Now()
	matched, matchedAgeDiffs := match(subjects)
	fmt.Printf("match: %s\n", time.Since(matchStart))
	IgG_MS_GK := func(w *csv.Writer, subjects []*Subject) error {
		if err := w.Write([]string{"Title", "GK", "MS"}); err != nil {
			return err
		}
		positive := map[string]int{}
		negative := map[string]int{}
		for _, s := range subjects {
			group := "GK"
			if s.Group != GK {
				group = "MS"
			}
			if s.IgG {
				positive[group]++
			} else {
				negative[group]++
			}
		}
		if err := w.Write([]string{"positiv", fmt.Sprintf("%d", positive["GK"]), fmt.Sprintf("%d", positive["MS"])}); err != nil {
			return err
		}
		if err := w.Write([]string{"negativ", fmt.Sprintf("%d", negative["GK"]), fmt.Sprintf("%d", negative["MS"])}); err != nil {
			return err
		}
		return nil
	}
	IgG_Titer_Lessions := func(w *csv.Writer, subjects []*Subject, kind string) error {
		header := []string{}
		groups := []Lesion{Lt6, GtEq6}
		for _, group := range groups {
			header = append(header, string(group))
		}
		if err := w.Write(header); err != nil {
			return err
		}
		results := map[Lesion][]float64{}
		for _, s := range subjects {
			var val Lesion
			switch kind {
			case "CMRT":
				val = s.CMRT_T2
			case "SMRT":
				val = s.SMRT_T2
			default:
				return fmt.Errorf("bad kind: %s", kind)
			}
			if val == LesionNA {
				continue
			}
			results[val] = append(results[val], s.IgGTiter)
		}
		i := 0
		for {
			found := false
			row := make([]string, len(groups))
			for g, group := range groups {
				if i < len(results[group]) {
					row[g] = fmt.Sprintf("%f", results[group][i])
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
	outputFiles := map[string]func(w *csv.Writer) error{
		"Patienten": func(w *csv.Writer) error {
			return writeSubjects(w, subjects)
		},
		"Patienten-Matched": func(w *csv.Writer) error {
			return writeSubjects(w, matched)
		},
		"Patienten-Matched-Altersunterschied": func(w *csv.Writer) error {
			return writeHistogram(w, matchedAgeDiffs)
		},
		"IgG-MS-GK-Unmatched": func(w *csv.Writer) error {
			return IgG_MS_GK(w, subjects)
		},
		"IgG-MS-GK-Matched": func(w *csv.Writer) error {
			return IgG_MS_GK(w, matched)
		},
		"IgG-Treatment": func(w *csv.Writer) error {
			type result struct {
				Positive int
				Negative int
			}
			results := map[TherapyGroup]result{}
			groups := []TherapyGroup{Untreated, BaseMedication, EscalationTherapy}
			for _, s := range subjects {
				g := s.TherapyGroup()
				if g == TherapyNA {
					continue
				}
				r := results[g]
				if s.IgG {
					r.Positive++
				} else {
					r.Negative++
				}
				results[g] = r
			}
			header := []string{"", "Positiv", "Negativ"}
			if err := w.Write(header); err != nil {
				return err
			}
			for _, g := range groups {
				r := results[g]
				row := []string{
					string(g),
					fmt.Sprintf("%d", r.Positive),
					fmt.Sprintf("%d", r.Negative),
				}
				if err := w.Write(row); err != nil {
					return err
				}
			}
			return nil
		},
		"EDSS": func(w *csv.Writer) error {
			header := []string{"EDSS"}
			if err := w.Write(header); err != nil {
				return err
			}
			for _, s := range subjects {
				if s.EDSS == nil {
					continue
				}
				row := []string{
					fmt.Sprintf("%f", *s.EDSS),
				}
				if err := w.Write(row); err != nil {
					return err
				}
			}
			return nil
		},
		"IgG-Titer-EDSS": func(w *csv.Writer) error {
			header := []string{"EDSS", "IgG Titer"}
			if err := w.Write(header); err != nil {
				return err
			}
			for _, s := range subjects {
				if s.EDSS == nil {
					continue
				}
				row := []string{
					fmt.Sprintf("%f", *s.EDSS),
					fmt.Sprintf("%f", s.IgGTiter),
				}
				if err := w.Write(row); err != nil {
					return err
				}
			}
			return nil
		},
		"IgG-Titer-Alter": func(w *csv.Writer) error {
			header := []string{"Alter", "IgG Titer"}
			if err := w.Write(header); err != nil {
				return err
			}
			for _, s := range subjects {
				row := []string{
					fmt.Sprintf("%f", s.Age),
					fmt.Sprintf("%f", s.IgGTiter),
				}
				if err := w.Write(row); err != nil {
					return err
				}
			}
			return nil
		},
		"IgG-Titer-CMRT-T2": func(w *csv.Writer) error {
			return IgG_Titer_Lessions(w, subjects, "CMRT")
		},
		"IgG-Titer-SMRT-T2": func(w *csv.Writer) error {
			return IgG_Titer_Lessions(w, subjects, "SMRT")
		},
		"Nikotinabusus-MS-GK-Unmatched": func(w *csv.Writer) error {
			header := []string{"Nikotinabusus", "GK", "MS"}
			if err := w.Write(header); err != nil {
				return err
			}
			type result struct {
				GK int
				MS int
			}
			results := map[YesNoNA]result{}
			for _, s := range subjects {
				r := results[s.Nikotinabusus]
				if s.Group == GK {
					r.GK++
				} else {
					r.MS++
				}
				results[s.Nikotinabusus] = r
			}
			rowTitles := []YesNoNA{Yes, No, NA}
			for _, title := range rowTitles {
				row := []string{
					string(title),
					fmt.Sprintf("%d", results[title].GK),
					fmt.Sprintf("%d", results[title].MS),
				}
				if err := w.Write(row); err != nil {
					return err
				}
			}
			return nil
		},
		"IgG-Titer-Nikotinabusus": func(w *csv.Writer) error {
			header := []string{}
			groups := []YesNoNA{Yes, No}
			for _, group := range groups {
				header = append(header, string(group))
			}
			if err := w.Write(header); err != nil {
				return err
			}
			results := map[YesNoNA][]float64{}
			for _, s := range subjects {
				if s.Nikotinabusus == NA {
					continue
				}
				results[s.Nikotinabusus] = append(results[s.Nikotinabusus], s.IgGTiter)
			}
			i := 0
			for {
				row := make([]string, len(Groups))
				found := false
				for g, group := range groups {
					if i < len(results[group]) {
						found = true
						row[g] = fmt.Sprintf("%f", results[group][i])
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
		},
		"IgG-Titer-Unmatched": func(w *csv.Writer) error {
			header := []string{}
			for _, group := range Groups {
				header = append(header, string(group))
			}
			if err := w.Write(header); err != nil {
				return err
			}
			results := map[Group][]float64{}
			for _, s := range subjects {
				results[s.Group] = append(results[s.Group], s.IgGTiter)
			}
			i := 0
			for {
				row := make([]string, len(Groups))
				found := false
				for g, group := range Groups {
					if i < len(results[group]) {
						found = true
						row[g] = fmt.Sprintf("%f", results[group][i])
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
		},
		"IgG-MS-GK-Matched-Mc-Nemar": func(w *csv.Writer) error {
			results := struct {
				NoYes  int
				YesNo  int
				YesYes int
				NoNo   int
			}{}
			i := 0
			for i < len(matched) {
				controlSubject, caseSubject := matched[i], matched[i+1]
				if controlSubject.IgG && !caseSubject.IgG {
					results.NoYes++
				} else if !controlSubject.IgG && caseSubject.IgG {
					results.YesNo++
				} else if !controlSubject.IgG && !caseSubject.IgG {
					results.YesYes++
				} else if controlSubject.IgG && caseSubject.IgG {
					results.NoNo++
				}
				i += 2
			}
			if err := w.Write([]string{"Control Risk Factor", "Case Risk Factor", "Count"}); err != nil {
				return err
			}
			rows := []struct {
				ControlRisk bool
				CaseRisk    bool
				Count       int
			}{
				{false, true, results.NoYes},
				{true, false, results.YesNo},
				{true, true, results.YesYes},
				{false, false, results.NoNo},
			}
			for _, row := range rows {
				rowStr := []string{
					fmt.Sprintf("%s", yesNo(row.ControlRisk)),
					fmt.Sprintf("%s", yesNo(row.CaseRisk)),
					fmt.Sprintf("%d", row.Count),
				}
				if err := w.Write(rowStr); err != nil {
					return err
				}
			}
			return nil
		},
	}
	for name, fn := range outputFiles {
		outputs := []string{"csv", "prism"}
		for i, output := range outputs {
			start := time.Now()
			fileName := name + ".csv"
			outPath := filepath.Join(outputDir, output, fileName)
			if err := os.MkdirAll(filepath.Dir(outPath), 0777); err != nil {
				fatalf("Could not create outPath: %s", err)
			}
			outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
			if err != nil {
				fatalf("Could not open output file: %s", err)
			}
			defer outFile.Close()
			w := csv.NewWriter(outFile)
			if output == "prism" {
				w.Comma = '\t'
			}
			if err := fn(w); err != nil {
				fmt.Printf("Failed to write %s: %s\n", outPath, err)
			}
			w.Flush()
			if i == 0 {
				fmt.Printf("%s: %s\n", name, time.Since(start))
			}
		}
	}
	fmt.Printf("Total: %s\n", time.Since(start))
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func writeHistogram(w *csv.Writer, h Histogram) error {
	header := []string{
		"Min",
		"Max",
		"Median",
		"Mittel",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	row := []string{
		fmt.Sprintf("%f", h.Min()),
		fmt.Sprintf("%f", h.Max()),
		fmt.Sprintf("%f", h.Median()),
		fmt.Sprintf("%f", h.Mean()),
	}
	return w.Write(row)
}

func writeSubjects(w *csv.Writer, subjects []*Subject) error {
	header := []string{
		"Labor- Berlin Nr.",
		"Probennummer",
		"Vorname",
		"Nachname",
		"Geschlecht",
		"Gruppe",
		"IgG",
		"Alter (PE)",
		"Alter (EM)",
		"Erkrankungsdauer",
		"EDSS",
		"Q (CSF/Serum) IgG",
		"Nikotinabusus",
		"Therapie",
		"Anzahl Schübe",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, s := range subjects {
		var ageEM string
		if s.AgeEM != nil {
			ageEM = fmt.Sprintf("%f", *s.AgeEM)
		}
		var sickDuration string
		if s.SickDuration != nil {
			sickDuration = fmt.Sprintf("%f", *s.SickDuration)
		}
		var edss string
		if s.EDSS != nil {
			edss = fmt.Sprintf("%f", *s.EDSS)
		}
		var qIgG string
		if s.QIgG != nil {
			qIgG = fmt.Sprintf("%f", *s.QIgG)
		}
		var numRelapse string
		if s.NumRelapse != nil {
			numRelapse = fmt.Sprintf("%f", *s.NumRelapse)
		}
		row := []string{
			s.LabBerlinNumber,
			s.ProbeNumber,
			s.FirstName,
			s.LastName,
			string(s.Gender),
			string(s.Group),
			fmt.Sprintf("%s", s.IgG),
			fmt.Sprintf("%f", s.Age),
			ageEM,
			sickDuration,
			edss,
			qIgG,
			string(s.Nikotinabusus),
			string(s.TherapyGroup()),
			numRelapse,
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return nil
}

func match(subjects []*Subject) (results []*Subject, ageDiffs Histogram) {
	var (
		ops      = 0
		controls []*Subject
		cases    []*Subject
	)
	for _, subject := range subjects {
		ops++
		if subject.Group == GK {
			controls = append(controls, subject)
		} else {
			cases = append(cases, subject)
		}
	}
	for _, controlSubject := range controls {
		bestMatch := -1
		for i, caseSubject := range cases {
			ops++
			if caseSubject.Gender != controlSubject.Gender {
				continue
			}
			if bestMatch == -1 {
				bestMatch = i
				continue
			}
			currentAgeDiff := math.Abs(caseSubject.Age - controlSubject.Age)
			bestAgeDiff := math.Abs(cases[bestMatch].Age - controlSubject.Age)
			if currentAgeDiff < bestAgeDiff {
				bestMatch = i
			}
		}
		ageDiff := math.Abs(cases[bestMatch].Age - controlSubject.Age)
		ageDiffs = append(ageDiffs, ageDiff)
		if bestMatch > -1 {
			results = append(results, controlSubject, cases[bestMatch])
			cases = append(cases[0:bestMatch], cases[bestMatch+1:]...)
		}
	}
	return
}

type Histogram []float64

func (h Histogram) Min() float64 {
	var min float64
	for _, val := range h {
		min = math.Min(min, val)
	}
	return min
}

func (h Histogram) Max() float64 {
	var max float64
	for _, val := range h {
		max = math.Max(max, val)
	}
	return max
}

func (h Histogram) Mean() float64 {
	var sum float64
	for _, val := range h {
		sum += val
	}
	return sum / float64(len(h))
}

func (h Histogram) Median() float64 {
	if len(h)%2 == 0 {
		return (h[len(h)/2-1] + h[len(h)/2]) / 2
	}
	return h[(len(h)-1)/2]
}

func (h Histogram) String() string {
	return fmt.Sprintf(
		"Mean: %f Median: %f Min: %f Max: %f",
		h.Mean(),
		h.Median(),
		h.Min(),
		h.Max(),
	)
}

func readSubjects(file string) ([]*Subject, error) {
	iconv := exec.Command("iconv", "-f", "utf-16", "-t", "utf-8", file)
	stdout, err := iconv.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := iconv.Start(); err != nil {
		return nil, err
	}
	defer func() {
		stdout.Close()
		iconv.Wait()
	}()
	r := csv.NewReader(stdout)
	r.Comma = '\t'
	columns, err := r.Read()
	if err != nil {
		return nil, err
	}
	subjects := []*Subject{}
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		get := func(column string) (string, error) {
			for i, c := range columns {
				if column == c {
					return row[i], nil
				}
			}
			return "", fmt.Errorf("Unknown column: %s", column)
		}
		badRow, err := get("nicht verwendbar")
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(badRow) != "" {
			continue
		}
		s := &Subject{}
		apply := func(mapping map[string]interface{}) error {
			for column, dst := range mapping {
				val, err := get(column)
				if err != nil {
					return err
				}
				val = strings.TrimSpace(val)
				switch t := dst.(type) {
				case *string:
					*t = val
				case *Status:
					switch val {
					case "positiv":
						*t = true
					case "negativ":
						*t = false
					default:
						return fmt.Errorf("Invalid Status: %s", val)
					}
				case *Group:
					found := false
					for _, group := range Groups {
						if string(group) == val {
							*t = Group(val)
							found = true
							break
						}
					}
					if !found {
						return fmt.Errorf("Invalid Group: %s", val)
					}
				case *Gender:
					found := false
					for _, gender := range Genders {
						if string(gender) == val {
							*t = Gender(val)
							found = true
							break
						}
					}
					if !found {
						return fmt.Errorf("Invalid Gender: %s", val)
					}
				case *YesNoNA:
					switch YesNoNA(strings.ToLower((val))) {
					case Yes:
						*t = Yes
					case No:
						*t = No
					default:
						*t = NA
					}
				case *Lesion:
					switch val {
					case "<6":
						*t = Lt6
					case "6":
						fallthrough
					case ">6":
						*t = GtEq6
					default:
						*t = LesionNA
					}
				case *float64:
					val = strings.Replace(val, ",", ".", -1)
					f, err := strconv.ParseFloat(val, 64)
					if err != nil {
						return err
					}
					*t = f
				case **float64:
					val = strings.Replace(val, ",", ".", -1)
					f, err := strconv.ParseFloat(val, 64)
					if err != nil {
						*t = nil
					} else {
						*t = &f
					}
				default:
					return fmt.Errorf("Bad dst type: %#v", dst)
				}
			}
			return nil
		}
		initialMapping := map[string]interface{}{
			"Probennummer":      &s.ProbeNumber,
			"Labor- Berlin Nr.": &s.LabBerlinNumber,
			"Vorname":           &s.FirstName,
			"Nachname":          &s.LastName,
		}
		if err := apply(initialMapping); err != nil {
			return nil, fmt.Errorf("%s: %s", err, row)
		}
		if s.ProbeNumber == "" && s.LabBerlinNumber == "" {
			// ignore empty row
			continue
		}
		remainingMapping := map[string]interface{}{
			"Alter (PE)":                        &s.Age,
			"Gruppe":                            &s.Group,
			"Geschlecht":                        &s.Gender,
			"IgG":                               &s.IgG,
			"IgG titer (IU/ml)":                 &s.IgGTiter,
			"Nikotinabusus":                     &s.Nikotinabusus,
			"Basismedikation":                   &s.BaseMedication,
			"Eskalationstherapie":               &s.EscalationTherapy,
			"EDSS":                              &s.EDSS,
			"Alter (EM)":                        &s.AgeEM,
			"Erkrankungsdauer (Monate)":         &s.SickDuration,
			"Q (CSF/Serum) IgG":                 &s.QIgG,
			"Anzahl der Schübe":                 &s.NumRelapse,
			"cMRT: n-Läsionen T2-Statistik neu": &s.CMRT_T2,
			"sMRT: n-Läsionen T2-Statistik neu": &s.SMRT_T2,
		}
		if err := apply(remainingMapping); err != nil {
			return nil, fmt.Errorf("%s: %s", s, err)
		}
		subjects = append(subjects, s)
	}
	return subjects, nil
}

type Gender string

const (
	Male   Gender = "m"
	Female Gender = "w"
)

var Genders = []Gender{Male, Female}

type Group string

const (
	GK   Group = "GK"
	CIS  Group = "CIS"
	RRMS Group = "RRMS"
	SPMS Group = "SPMS"
	PPMS Group = "PPMS"
)

var Groups = []Group{GK, CIS, RRMS, SPMS, PPMS}

type Status bool

func (s Status) String() string {
	if s {
		return "positiv"
	}
	return "negativ"
}

type YesNoNA string

const (
	Yes YesNoNA = "ja"
	No  YesNoNA = "nein"
	NA  YesNoNA = "n/a"
)

type TherapyGroup string

const (
	TherapyNA         TherapyGroup = "n/a"
	Untreated         TherapyGroup = "Unbehandelt"
	BaseMedication    TherapyGroup = "Basismedikation"
	EscalationTherapy TherapyGroup = "Eskalationstherapie"
)

type Lesion string

const (
	Lt6      Lesion = "<6"
	GtEq6    Lesion = ">=6"
	LesionNA Lesion = "n/a"
)

type Subject struct {
	ProbeNumber       string
	LabBerlinNumber   string
	FirstName         string
	LastName          string
	Group             Group
	Gender            Gender
	IgG               Status
	IgGTiter          float64
	QIgG              *float64
	Age               float64
	AgeEM             *float64
	SickDuration      *float64
	Nikotinabusus     YesNoNA
	BaseMedication    YesNoNA
	EscalationTherapy YesNoNA
	EDSS              *float64
	NumRelapse        *float64
	CMRT_T2           Lesion
	SMRT_T2           Lesion
}

func (s *Subject) TherapyGroup() TherapyGroup {
	if s.Group == GK {
		return TherapyNA
	}
	if s.BaseMedication == NA || s.EscalationTherapy == NA {
		return TherapyNA
	}
	if s.BaseMedication == Yes && s.EscalationTherapy == Yes {
		return TherapyNA
	}
	if s.BaseMedication == Yes {
		return BaseMedication
	} else if s.EscalationTherapy == Yes {
		return EscalationTherapy
	} else {
		return Untreated
	}
}

func (s *Subject) String() string {
	return fmt.Sprintf("<%s,%s,%s>", s.FirstName, s.LastName, s.ProbeNumber)
}

func fatalf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
	os.Exit(1)
}
