package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func main() {
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
	if err := os.RemoveAll(outputDir); err != nil {
		fatalf("Could not delete outputDir: %s", err)
	}
	if err := os.MkdirAll(outputDir, 0777); err != nil {
		fatalf("Could not create outputDir: %s", err)
	}
	start := time.Now()
	subjects, err := readSubjects(inputFile)
	if err != nil {
		fatalf("readSubjects: %s", err)
	}
	fmt.Printf("readSubjects: %s\n", time.Since(start))
	outputFiles := map[string]func(w *csv.Writer) error{
		"IgG-MS-GK-Unmatched.csv": func(w *csv.Writer) error {
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
		},
	}
	for name, fn := range outputFiles {
		start := time.Now()
		outPath := filepath.Join(outputDir, name)
		outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
		if err != nil {
			fatalf("Could not open output file: %s", err)
		}
		defer outFile.Close()
		w := csv.NewWriter(outFile)
		w.Comma = '\t'
		if err := fn(w); err != nil {
			fmt.Printf("Failed to write %s: %s\n", name, err)
		}
		w.Flush()
		fmt.Printf("%s: %s\n", name, time.Since(start))
	}
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
				case *bool:
					switch val {
					case "positiv":
						*t = true
					case "negativ":
						*t = false
					default:
						return fmt.Errorf("Invalid bool: %s", val)
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
				case *float64:
					val = strings.Replace(val, ",", ".", -1)
					f, err := strconv.ParseFloat(val, 64)
					if err != nil {
						return err
					}
					*t = f
				default:
					return fmt.Errorf("Bad dst type: %#v", dst)
				}
			}
			return nil
		}
		initialMapping := map[string]interface{}{
			"Probennummer": &s.ProbeNumber,
			"Vorname":      &s.FirstName,
			"Nachname":     &s.LastName,
		}
		if err := apply(initialMapping); err != nil {
			return nil, fmt.Errorf("%s: %s", err, row)
		}
		if s.ProbeNumber == "" {
			// ignore empty row
			continue
		}
		remainingMapping := map[string]interface{}{
			"Alter (PE)": &s.Age,
			"Gruppe":     &s.Group,
			"Geschlecht": &s.Gender,
			"IgG":        &s.IgG,
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

type Subject struct {
	ProbeNumber string
	FirstName   string
	LastName    string
	Group       Group
	Gender      Gender
	IgG         bool
	Age         float64
}

func (s *Subject) String() string {
	return fmt.Sprintf("<%s,%s,%s>", s.FirstName, s.LastName, s.ProbeNumber)
}

func fatalf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
	os.Exit(1)
}
