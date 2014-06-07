package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"github.com/speedata/goxlsx"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

func main() {
	args := os.Args[1:]
	if len(args) < 1 {
		panic("Missing file name argument.")
	}
	rows, err := loadRows(args[0])
	if err != nil {
		panic(err)
	}

	if len(args) < 2 {
		panic("Missing command argument.")
	}
	cmd := args[1]
	switch cmd {
	case "match":
		err = cmdMatch(rows, args[2:])
	}
	if err != nil {
		panic(err)
	}
}

func cmdMatch(rows []*Row, args []string) error {
	groupedRows := map[string]map[string][]*Row{}
	for _, row := range rows {
		group := row.Get("Gruppe")
		if group == "" {
			continue
		}
		gender := row.Get("Geschlecht")
		if _, ok := groupedRows[group]; !ok {
			groupedRows[group] = map[string][]*Row{}
		}
		groupedRows[group][gender] = append(groupedRows[group][gender], row)
	}

	var csvWriter *csv.Writer
	if len(args) >= 1 && args[0] == "csv" {
		csvWriter = csv.NewWriter(os.Stdout)
		defer csvWriter.Flush()
		csvWriter.Write(append([]string{"Nr"}, rows[0].Columns()...))
	}

	groups := []string{"CIS", "RRMS", "SPMS", "PPMS"}
	for _, group := range groups {
		if csvWriter == nil {
			fmt.Printf("===== %s =====\n", group)
		}
		matchedRows := []*Row{}
		for gender, caseRows := range groupedRows[group] {
			controlRows := make([]*Row, len(groupedRows["GK"][gender]))
			copy(controlRows, groupedRows["GK"][gender])
			for len(controlRows) > 0 && len(caseRows) > 0 {
				var bestMatch *Match
				for caseIndex, caseRow := range caseRows {
					for controlIndex, controlRow := range controlRows {
						currentMatch := &Match{
							Case:         caseRow,
							CaseIndex:    caseIndex,
							Control:      controlRow,
							ControlIndex: controlIndex,
						}
						if bestMatch == nil || bestMatch.AgeDiff() > currentMatch.AgeDiff() {
							bestMatch = currentMatch
						}
					}
				}
				bestMatch.Control = bestMatch.Control.Copy()
				bestMatch.Control.Set("Gruppe", bestMatch.Control.Get("Gruppe")+"-"+group)
				matchedRows = append(matchedRows, bestMatch.Case, bestMatch.Control)
				if csvWriter != nil {
					csvWriter.Write(append([]string{fmt.Sprintf("%d", bestMatch.Case.Num())}, bestMatch.Case.values...))
					csvWriter.Write(append([]string{fmt.Sprintf("%d", bestMatch.Control.Num())}, bestMatch.Control.values...))
				}
				controlRows = append(controlRows[:bestMatch.ControlIndex], controlRows[bestMatch.ControlIndex+1:]...)
				caseRows = append(caseRows[:bestMatch.CaseIndex], caseRows[bestMatch.CaseIndex+1:]...)
			}
		}
		if csvWriter == nil {
			mustPrintRows(matchedRows)
			fmt.Printf("\n\n")
		}
	}
	return nil
}

type Match struct {
	Case         *Row
	CaseIndex    int
	Control      *Row
	ControlIndex int
}

func (m *Match) AgeDiff() float64 {
	return math.Abs(m.Case.Age() - m.Control.Age())
}

type MatchGroup struct {
	Group  string
	Gender string
}

func loadRows(file string) ([]*Row, error) {
	doc, err := goxlsx.OpenFile(file)
	if err != nil {
		return nil, err
	}
	sheet, err := doc.GetWorksheet(0)
	if err != nil {
		return nil, err
	}

	rows := []*Row{}
	columns := []string{}
	for r := sheet.MinRow; r < sheet.MaxRow; r++ {
		var row *Row
		if r > sheet.MinRow {
			row = &Row{num: r, columns: columns}
		}
		for c := sheet.MinColumn; c < sheet.MaxColumn; c++ {
			val := strings.TrimSpace(sheet.Cell(c, r))
			if r == sheet.MinRow {
				columns = append(columns, val)
			} else {
				row.values = append(row.values, val)
			}
		}
		if r > sheet.MinRow {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

type Row struct {
	columns []string
	values  []string
	num     int
}

func (r *Row) Copy() *Row {
	c := *r
	c.values = make([]string, len(r.values))
	copy(c.values, r.values)
	return &c
}

func (r *Row) Age() float64 {
	ageStr := r.Get("Alter")
	ageFloat, err := strconv.ParseFloat(ageStr, 64)
	if err != nil {
		panic(err)
	}
	return ageFloat
}

func (r *Row) Num() int {
	return r.num
}

func (r *Row) Columns() []string {
	return r.columns
}

func (r *Row) Get(column string) string {
	return r.values[r.index(column)]
}

func (r *Row) Set(column string, val string) {
	r.values[r.index(column)] = val
}

func (r *Row) index(column string) int {
	for i, val := range r.columns {
		if column == val {
			return i
		}
	}
	panic(fmt.Sprintf("Unknown column: %s", column))
}


func writeTable(w io.Writer, data [][]string) (err error) {
	lengths := make([]int, len(data[0]))
	for _, row := range data {
		for i, val := range row {
			if l := len(val); l > lengths[i] {
				lengths[i] = l
			}
		}
	}
	bw := bufio.NewWriter(w)
	defer func() {
		if err == nil {
			err = bw.Flush()
		} else {
			bw.Flush()
		}
	}()
	for _, row := range data {
		for i, val := range row {
			last := i+1 == len(row)
			if pad := lengths[i] - utf8.RuneCountInString(val); pad > 0 && !last {
				val += strings.Repeat(" ", pad)
			}
			_, err = io.WriteString(bw, val)
			if err != nil {
				return
			}
			if last {
				_, err = io.WriteString(bw, "\n")
			} else {
				_, err = io.WriteString(bw, " ")
			}
			if err != nil {
				return
			}
		}
	}
	return
}

func mustPrintfStdout(format string, args ...interface{}) {
	if _, err := fmt.Fprintf(os.Stdout, format, args...); err != nil {
		panic(err)
	}
}

func mustPrintTable(data [][]string) {
	if err := writeTable(os.Stdout, data); err != nil {
		panic(err)
	}
}

func mustPrintRows(rows []*Row) {
	data := [][]string{}
	data = append(data, append([]string{"1"}, rows[0].Columns()...))
	for _, row := range rows {
		rowVals := []string{fmt.Sprintf("%d", row.Num())}
		for _, column := range row.Columns() {
			val := row.Get(column)
			rowVals = append(rowVals, val)
		}
		data = append(data, rowVals)
	}
	mustPrintTable(data)
}
