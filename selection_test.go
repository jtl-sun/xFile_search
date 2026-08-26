package main

import "testing"

func TestNextSelectionAfterDelete(t *testing.T) {
	tests := []struct{ deleted, remain, want int }{{0,4,0},{2,4,2},{4,4,3},{0,0,-1},{-1,3,0}}
	for _, tt := range tests { if got:=nextSelectionAfterDelete(tt.deleted,tt.remain);got!=tt.want{t.Fatalf("delete=%d remain=%d got=%d want=%d",tt.deleted,tt.remain,got,tt.want)} }
}
