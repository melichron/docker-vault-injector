package environment

import (
	"reflect"
	"testing"
)

func TestMergePreservesUnmanagedAndRemovesOldManagedNames(t *testing.T) {
	input := []string{"LOG_LEVEL=info", "OLD_SECRET=old", "DB_USER=old-user"}
	got := Merge(input, []string{"OLD_SECRET", "DB_USER"}, map[string]string{
		"DB_USER":     "new-user",
		"DB_PASSWORD": "new-password",
	})
	want := []string{"LOG_LEVEL=info", "DB_PASSWORD=new-password", "DB_USER=new-user"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Merge() = %v, want %v", got, want)
	}
}

func TestHashIsIndependentOfMapIterationOrder(t *testing.T) {
	one := map[string]string{"A": "one", "B": "two"}
	two := map[string]string{"B": "two", "A": "one"}
	if Hash(one) != Hash(two) {
		t.Fatal("Hash must be deterministic")
	}
}

func TestSelectExposesMissingVariableAsDrift(t *testing.T) {
	got := Select([]string{"A=one"}, []string{"A", "B"})
	want := map[string]string{"A": "one"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Select() = %v, want %v", got, want)
	}
}
