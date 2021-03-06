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

	"github.com/bradfitz/slice"
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
	msMatched := subjects.Match(func(a, b *Subject) float64 {
		if a.Diagnosis == GK || b.Diagnosis == GK {
			return 0
		}
		if a.Gender != b.Gender {
			return 0
		}
		if a.IgG == b.IgG {
			return 0
		}
		if a.SickDuration == nil || b.SickDuration == nil {
			return 0
		}
		if *a.SickDuration < 0 || *b.SickDuration < 0 {
			return 0
		}
		ageDiff := math.Abs(a.Age - b.Age)
		if ageDiff > 3 {
			return 0
		}
		sdDiff := math.Abs(*a.SickDuration - *b.SickDuration)
		if sdDiff > 1 {
			return 0
		}
		return 1 / sdDiff
	})

	outputFiles := map[string]func(w *csv.Writer) error{
		"Patienten-MS-Toxo-Matched-EDSS": func(w *csv.Writer) error {
			header := []string{"Toxo-IgG Positiv", "Toxo-IgG Negativ"}
			if err := w.Write(header); err != nil {
				return err
			}
			results := map[Status][]float64{}
			for _, m := range msMatched {
				if m.A.EDSS == nil || m.B.EDSS == nil {
					continue
				}
				for _, s := range append(Subjects{}, m.A, m.B) {
					results[s.IgG] = append(results[s.IgG], *s.EDSS)
				}
			}
			groups := []Status{true, false}
			for i := 0; ; i++ {
				row := make([]string, len(groups))
				found := false
				for j, group := range groups {
					vals := results[group]
					if i < len(vals) {
						found = true
						row[j] = fmt.Sprintf("%f", vals[i])
					}
				}
				if !found {
					break
				}
				if err := w.Write(row); err != nil {
					return err
				}
			}
			return nil
		},
		"Patienten-MS-Toxo-Matched": func(w *csv.Writer) error {
			header := []string{"Row", "Name", "Geschlecht", "Alter", "Erkrankungsdauer", "IgG", "Diagnose", "Geburtsdatum", "EDSS", "CMRT_T2", "CMRT_GD", "SMRT_T2", "SMRT_GD", "Match Score"}
			if err := w.Write(header); err != nil {
				return err
			}
			for i, match := range msMatched {
				for j, s := range append(Subjects{}, match.A, match.B) {
					row := []string{
						fmt.Sprintf("%d", i*2+j+1),
						fmt.Sprintf("%s %s", s.FirstName, s.LastName),
						fmt.Sprintf("%s", s.Gender),
						fmt.Sprintf("%f", s.Age),
						fmt.Sprintf("%f", *s.SickDuration),
						fmt.Sprintf("%s", s.IgG),
						fmt.Sprintf("%s", s.Diagnosis),
						fmt.Sprintf("%s", s.Birthday),
						fmt.Sprintf("%s", float64PtrStr(s.EDSS)),
						fmt.Sprintf("%s", s.CMRT_T2),
						fmt.Sprintf("%s", s.CMRT_GD),
						fmt.Sprintf("%s", s.SMRT_T2),
						fmt.Sprintf("%s", s.SMRT_GD),
						fmt.Sprintf("%.2f", match.Score),
					}
					if err := w.Write(row); err != nil {
						return err
					}
				}
			}
			return nil
		},
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
			top := []string{"MS", "GK"}
			left := []string{"positiv", "negativ"}
			return WriteContingency(w, top, left, IgG_MS_GKSubjects(subjects))
		},
		"IgG-MS-GK-Matched": func(w *csv.Writer) error {
			top := []string{"MS", "GK"}
			left := []string{"positiv", "negativ"}
			return WriteContingency(w, top, left, IgG_MS_GKSubjects(matched))
		},
		"IgM-MS-GK-Unmatched": func(w *csv.Writer) error {
			top := []string{"MS", "GK"}
			left := []string{"positiv", "negativ"}
			return WriteContingency(w, top, left, IgM_MS_GKSubjects(subjects))
		},
		"IgM-MS-GK-Matched": func(w *csv.Writer) error {
			top := []string{"MS", "GK"}
			left := []string{"positiv", "negativ"}
			return WriteContingency(w, top, left, IgM_MS_GKSubjects(matched))
		},
		"ANA-Nikotinabusus-MS": func(w *csv.Writer) error {
			top := []string{"ja", "nein"}
			left := []string{"positiv", "negativ"}
			return WriteContingency(w, top, left, ANA_Nikotinabusus_MS_Subjects(subjects))
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
		"IgG-Titer-IgG-Gesamt": func(w *csv.Writer) error {
			header := []string{"IgG Gesamt", "IgG Titer"}
			if err := w.Write(header); err != nil {
				return err
			}
			for _, s := range subjects {
				if s.IgGTotal == nil {
					continue
				}
				row := []string{
					fmt.Sprintf("%f", *s.IgGTotal),
					fmt.Sprintf("%f", s.IgGTiter),
				}
				if err := w.Write(row); err != nil {
					return err
				}
			}
			return nil
		},
		"IgG-Titer-Erkrankungsdauer": func(w *csv.Writer) error {
			header := []string{"Erkrankungsdauer", "IgG Titer"}
			if err := w.Write(header); err != nil {
				return err
			}
			for _, s := range subjects {
				if s.SickDuration == nil {
					continue
				}
				if *s.SickDuration < 0 {
					continue
				}
				row := []string{
					fmt.Sprintf("%f", *s.SickDuration),
					fmt.Sprintf("%f", s.IgGTiter),
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
		"IgG-Titer-SMRT-GD": func(w *csv.Writer) error {
			groups := []Group{
				NASPositiv,
				NASNegativ,
				NASNA,
			}
			groupSubjects := make([]GroupSubject, len(subjects))
			for i, s := range subjects {
				groupSubjects[i] = SMRT_GDSubject{s}
			}
			return WriteGroupValues(w, groups, groupSubjects)
		},
		"IgG-Titer-CMRT-GD": func(w *csv.Writer) error {
			groups := []Group{
				NASPositiv,
				NASNegativ,
				NASNA,
			}
			groupSubjects := make([]GroupSubject, len(subjects))
			for i, s := range subjects {
				groupSubjects[i] = CMRT_GDSubject{s}
			}
			return WriteGroupValues(w, groups, groupSubjects)
		},
		"CMRT-T2-Counts": func(w *csv.Writer) error {
			header := []string{""}
			for _, dia := range Diagnoses {
				header = append(header, string(dia))
			}
			if err := w.Write(header); err != nil {
				return err
			}
			vals := []string{"0", "<6", ">=6", "n/a"}
			for _, val := range vals {
				results := []string{val}
				for _, dia := range Diagnoses {
					count := 0
					for _, s := range matched {
						if s.Diagnosis != dia {
							continue
						}
						v := s.CMRT_T2.String()
						if v != val {
							continue
						}
						count++
					}
					results = append(results, fmt.Sprintf("%d", count))
				}
				if err := w.Write(results); err != nil {
					return err
				}
			}
			return nil
			//countS := []string{}
			//sum := 0
			//for _, count := range counts {
			//sum += count
			//countS = append(countS, fmt.Sprintf("%d", count))
			//}
			//percents := []string{}
			//for _, count := range counts {
			//percents = append(percents, fmt.Sprintf("%.2f", float64(count)/float64(sum)*100))
			//}
			//if err := w.Write(countS); err != nil {
			//return err
			//}
			//return w.Write(percents)
		},
		"IgG-Titer-CMRT-T2": func(w *csv.Writer) error {
			groups := []Group{
				NewNARelInt(NewRelInt(Eq, 0), false),
				NewNARelInt(NewRelInt(Lt, 6), false),
				NewNARelInt(NewRelInt(GtEq, 6), false),
				NewNARelInt(RelInt{}, true),
			}
			groupSubjects := make([]GroupSubject, len(subjects))
			for i, s := range subjects {
				groupSubjects[i] = CMRTSubject{s}
			}
			return WriteGroupValues(w, groups, groupSubjects)
		},
		"IgG-Titer-SMRT-T2": func(w *csv.Writer) error {
			groups := []Group{
				NewNARelInt(NewRelInt(Eq, 0), false),
				NewNARelInt(NewRelInt(Lt, 3), false),
				NewNARelInt(NewRelInt(GtEq, 3), false),
				NewNARelInt(RelInt{}, true),
			}
			groupSubjects := make([]GroupSubject, len(subjects))
			for i, s := range subjects {
				groupSubjects[i] = SMRTSubject{s}
			}
			return WriteGroupValues(w, groups, groupSubjects)
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
				if s.Diagnosis == GK {
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
				row := make([]string, len(Diagnoses))
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
			for _, group := range Diagnoses {
				header = append(header, string(group))
			}
			if err := w.Write(header); err != nil {
				return err
			}
			results := map[Diagnosis][]float64{}
			for _, s := range subjects {
				results[s.Diagnosis] = append(results[s.Diagnosis], s.IgGTiter)
			}
			i := 0
			for {
				row := make([]string, len(Diagnoses))
				found := false
				for g, group := range Diagnoses {
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
		"cMRT T2",
		"sMRT T2",
		"cMRT Gd",
		"sMRT Gd",
		"ANA",
		"IgG Gesamt",
		"IgM",
		"IgG Titer",
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
		var IgGTotal string
		if s.IgGTotal != nil {
			IgGTotal = fmt.Sprintf("%f", *s.IgGTotal)
		}
		row := []string{
			s.LabBerlinNumber,
			s.ProbeNumber,
			s.FirstName,
			s.LastName,
			string(s.Gender),
			string(s.Diagnosis),
			fmt.Sprintf("%s", s.IgG),
			fmt.Sprintf("%f", s.Age),
			ageEM,
			sickDuration,
			edss,
			qIgG,
			string(s.Nikotinabusus),
			string(s.TherapyGroup()),
			numRelapse,
			s.CMRT_T2.String(),
			s.SMRT_T2.String(),
			s.CMRT_GD.String(),
			s.SMRT_GD.String(),
			s.ANA.String(),
			IgGTotal,
			s.IgM.String(),
			fmt.Sprintf("%f", s.IgGTiter),
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
		if subject.Diagnosis == GK {
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

func readSubjects(file string) (Subjects, error) {
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
	subjects := Subjects{}
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
				case *NAStatus:
					p, err := ParseNAStatus(val)
					if err != nil {
						return err
					}
					*t = p
				case *Diagnosis:
					found := false
					for _, group := range Diagnoses {
						if string(group) == val {
							*t = Diagnosis(val)
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
				case *NARelInt:
					if val == "" {
						*t = NewNARelInt(RelInt{}, true)
						continue
					}
					p, err := ParseNARelInt(val)
					if err != nil {
						return fmt.Errorf("Bad NARelInt: %s", err)
					}
					*t = p
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
			"Geburtsdatum":      &s.Birthday,
		}
		if err := apply(initialMapping); err != nil {
			return nil, fmt.Errorf("%s: %s", err, row)
		}
		if s.ProbeNumber == "" && s.LabBerlinNumber == "" {
			// ignore empty row
			continue
		}
		remainingMapping := map[string]interface{}{
			"Alter (PE)":                          &s.Age,
			"Gruppe":                              &s.Diagnosis,
			"Geschlecht":                          &s.Gender,
			"IgG":                                 &s.IgG,
			"IgM":                                 &s.IgM,
			"IgG titer (IU/ml)":                   &s.IgGTiter,
			"Nikotinabusus":                       &s.Nikotinabusus,
			"Basismedikation":                     &s.BaseMedication,
			"Eskalationstherapie":                 &s.EscalationTherapy,
			"EDSS":                                &s.EDSS,
			"Alter (EM)":                          &s.AgeEM,
			"Erkrankungsdauer (Monate)":           &s.SickDuration,
			"Q (CSF/Serum) IgG":                   &s.QIgG,
			"Anzahl der Schübe":                   &s.NumRelapse,
			"cMRT: n-Läsionen T2-Statistik neu":   &s.CMRT_T2,
			"sMRT: n-Läsionen T2-Statistik neu 3": &s.SMRT_T2,
			"cMRT Gd":                    &s.CMRT_GD,
			"sMRT Gd":                    &s.SMRT_GD,
			"IgG mg/dl Serum (700-1600)": &s.IgGTotal,
			"ANA <1/80":                  &s.ANA,
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

type Diagnosis string

const (
	GK   Diagnosis = "GK"
	CIS  Diagnosis = "CIS"
	RRMS Diagnosis = "RRMS"
	SPMS Diagnosis = "SPMS"
	PPMS Diagnosis = "PPMS"
)

var Diagnoses = []Diagnosis{GK, CIS, RRMS, SPMS, PPMS}

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

type Match struct {
	A     *Subject
	B     *Subject
	Score float64
	i     int
	j     int
}

type Subject struct {
	ProbeNumber       string
	LabBerlinNumber   string
	FirstName         string
	LastName          string
	Diagnosis         Diagnosis
	Gender            Gender
	IgG               Status
	IgM               NAStatus
	IgGTiter          float64
	IgGTotal          *float64
	QIgG              *float64
	Age               float64
	AgeEM             *float64
	SickDuration      *float64
	Nikotinabusus     YesNoNA
	BaseMedication    YesNoNA
	EscalationTherapy YesNoNA
	EDSS              *float64
	NumRelapse        *float64
	CMRT_T2           NARelInt
	SMRT_T2           NARelInt
	CMRT_GD           NAStatus
	SMRT_GD           NAStatus
	ANA               NAStatus
	Birthday          string
}

type Subjects []*Subject

func (s Subjects) Match(scoreFn func(a, b *Subject) float64) []Match {
	removed := map[int]bool{}
	matches := []Match{}
	for i, a := range s {
		if removed[i] {
			continue
		}
		if len(s) < 2 {
			break
		}
		var best Match
		for j := i + 1; j < len(s); j++ {
			if removed[j] {
				continue
			}
			b := s[j]
			score := scoreFn(a, b)
			if score > best.Score {
				best = Match{A: a, B: b, Score: score, i: i, j: j}
			}
		}
		if best.Score > 0 {
			removed[best.j] = true
			if best.A == best.B || best.j == best.i {
				panic("bug: broken invariant")
			}
			matches = append(matches, best)
		}
	}
	slice.Sort(matches, func(i, j int) bool {
		if matches[i].A == matches[j].A || matches[i].B == matches[j].B || matches[i].A == matches[j].B {
			panic(fmt.Errorf("bug: broken invariant: %d: %#v | %d: %#v", i, matches[i], j, matches[j]))
		}
		return matches[i].Score > matches[j].Score
	})
	return matches
}

func (s *Subject) TherapyGroup() TherapyGroup {
	if s.Diagnosis == GK {
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

func float64PtrStr(v *float64) string {
	if v == nil {
		return "n/a"
	}
	return fmt.Sprintf("%f", *v)
}

func (s *Subject) String() string {
	return fmt.Sprintf("<%s,%s,%s>", s.FirstName, s.LastName, s.ProbeNumber)
}

func fatalf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
	os.Exit(1)
}

type CMRTSubject struct {
	*Subject
}

func (s CMRTSubject) String() string {
	return fmt.Sprintf("%f", s.IgGTiter)
}

func (s CMRTSubject) Group() Group {
	return s.CMRT_T2
}

type SMRTSubject struct {
	*Subject
}

func (s SMRTSubject) String() string {
	return fmt.Sprintf("%f", s.IgGTiter)
}

func (s SMRTSubject) Group() Group {
	return s.SMRT_T2
}

type SMRT_GDSubject struct {
	*Subject
}

func (s SMRT_GDSubject) String() string {
	return fmt.Sprintf("%f", s.IgGTiter)
}

func (s SMRT_GDSubject) Group() Group {
	return s.SMRT_GD
}

type CMRT_GDSubject struct {
	*Subject
}

func (s CMRT_GDSubject) String() string {
	return fmt.Sprintf("%f", s.IgGTiter)
}

func (s CMRT_GDSubject) Group() Group {
	return s.CMRT_GD
}
