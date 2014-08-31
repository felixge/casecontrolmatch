package main

import "github.com/kylelemons/godebug/pretty"
import "testing"

func Test_NewRelInt(t *testing.T) {
	tests := []struct {
		Input string
		Want  RelInt
	}{
		{
			Input: "5",
			Want:  RelInt{kind: Eq, val: 5},
		},
		{
			Input: "<5",
			Want:  RelInt{kind: Lt, val: 5},
		},
		{
			Input: ">5",
			Want:  RelInt{kind: Gt, val: 5},
		},
		{
			Input: ">=5",
			Want:  RelInt{kind: GtEq, val: 5},
		},
		{
			Input: "<=5",
			Want:  RelInt{kind: LtEq, val: 5},
		},
		{
			Input: "<=1485",
			Want:  RelInt{kind: LtEq, val: 1485},
		},
	}
	for i, test := range tests {
		relInt, err := ParseRelInt(test.Input)
		if err != nil {
			t.Errorf("test %d: want nil, got err=%s", i, err)
			continue
		}
		if diff := pretty.Compare(relInt, test.Want); diff != "" {
			t.Errorf("test %d: %s", i, diff)
		}
	}
}
