package recipe

import (
	"encoding/json"
	"sort"
	"testing"
)

func TestWalkStrings_FlatObject(t *testing.T) {
	raw := json.RawMessage(`{"name":"Alice","age":30,"active":true}`)
	var got []string
	WalkStrings(raw, "/input", func(pointer string, value string) {
		got = append(got, pointer+"="+value)
	})
	if len(got) != 1 {
		t.Fatalf("expected 1 string, got %d: %v", len(got), got)
	}
	if got[0] != "/input/name=Alice" {
		t.Errorf("unexpected result: %s", got[0])
	}
}

func TestWalkStrings_NestedObject(t *testing.T) {
	raw := json.RawMessage(`{"outer":{"inner":"value"}}`)
	var got []string
	WalkStrings(raw, "", func(pointer string, value string) {
		got = append(got, pointer+"="+value)
	})
	if len(got) != 1 || got[0] != "/outer/inner=value" {
		t.Errorf("unexpected: %v", got)
	}
}

func TestWalkStrings_Array(t *testing.T) {
	raw := json.RawMessage(`["a","b","c"]`)
	var got []string
	WalkStrings(raw, "/arr", func(pointer string, value string) {
		got = append(got, pointer)
	})
	sort.Strings(got)
	if len(got) != 3 {
		t.Fatalf("expected 3 strings, got %d", len(got))
	}
	expected := []string{"/arr/0", "/arr/1", "/arr/2"}
	for i, e := range expected {
		if got[i] != e {
			t.Errorf("got[%d] = %s, want %s", i, got[i], e)
		}
	}
}

func TestWalkStrings_MixedTypes(t *testing.T) {
	raw := json.RawMessage(`{"s":"hello","n":42,"b":true,"a":["x",1],"obj":{"k":"v"}}`)
	var got []string
	WalkStrings(raw, "", func(pointer string, value string) {
		got = append(got, pointer)
	})
	sort.Strings(got)
	expected := []string{"/a/0", "/obj/k", "/s"}
	if len(got) != len(expected) {
		t.Fatalf("expected %d strings, got %d: %v", len(expected), len(got), got)
	}
	for i, e := range expected {
		if got[i] != e {
			t.Errorf("got[%d] = %s, want %s", i, got[i], e)
		}
	}
}

func TestWalkStrings_NilInput(t *testing.T) {
	var called bool
	WalkStrings(nil, "", func(pointer string, value string) {
		called = true
	})
	if called {
		t.Error("visitor should not be called for nil input")
	}
}

func TestWalkStrings_EmptyInput(t *testing.T) {
	var called bool
	WalkStrings(json.RawMessage(``), "", func(pointer string, value string) {
		called = true
	})
	if called {
		t.Error("visitor should not be called for empty input")
	}
}

func TestWalkStrings_EmptyObject(t *testing.T) {
	var called bool
	WalkStrings(json.RawMessage(`{}`), "", func(pointer string, value string) {
		called = true
	})
	if called {
		t.Error("visitor should not be called for empty object")
	}
}
