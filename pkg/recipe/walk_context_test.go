package recipe

import (
	"encoding/json"
	"sort"
	"testing"
)

func TestWalkStringsWithContext_BasicStrings(t *testing.T) {
	raw := json.RawMessage(`{"name":"Alice","city":"NYC"}`)
	var got []string
	WalkStringsWithContext(raw, "/input", func(ctx StringContext) {
		got = append(got, ctx.Pointer+"="+ctx.Value)
	})
	sort.Strings(got)
	if len(got) != 2 {
		t.Fatalf("expected 2 strings, got %d: %v", len(got), got)
	}
}

func TestWalkStringsWithContext_CondLHS(t *testing.T) {
	raw := json.RawMessage(`{"conditions":[{"lhs":"=_dp('...')","rhs":"ok","op":"equals"}]}`)
	var lhsFound bool
	var rhsIsLHS bool
	WalkStringsWithContext(raw, "/input", func(ctx StringContext) {
		if ctx.Value == "=_dp('...')" {
			lhsFound = ctx.IsCondLHS
		}
		if ctx.Value == "ok" {
			rhsIsLHS = ctx.IsCondLHS
		}
	})
	if !lhsFound {
		t.Error("expected IsCondLHS=true for conditions/0/lhs")
	}
	if rhsIsLHS {
		t.Error("expected IsCondLHS=false for conditions/0/rhs")
	}
}

func TestWalkStringsWithContext_NestedCondLHS(t *testing.T) {
	raw := json.RawMessage(`{"conditions":[{"lhs":"val1"},{"lhs":"val2"}]}`)
	var count int
	WalkStringsWithContext(raw, "/input", func(ctx StringContext) {
		if ctx.IsCondLHS {
			count++
		}
	})
	if count != 2 {
		t.Errorf("expected 2 condLHS strings, got %d", count)
	}
}

func TestWalkStringsWithContext_NotCondLHS(t *testing.T) {
	raw := json.RawMessage(`{"lhs":"not_in_conditions"}`)
	var isLHS bool
	WalkStringsWithContext(raw, "/input", func(ctx StringContext) {
		isLHS = ctx.IsCondLHS
	})
	if isLHS {
		t.Error("expected IsCondLHS=false for top-level lhs not under conditions")
	}
}

func TestWalkStringsWithContext_NilInput(t *testing.T) {
	var called bool
	WalkStringsWithContext(nil, "", func(ctx StringContext) {
		called = true
	})
	if called {
		t.Error("visitor should not be called for nil input")
	}
}

func TestWalkStringsWithContext_EmptyInput(t *testing.T) {
	var called bool
	WalkStringsWithContext(json.RawMessage(``), "", func(ctx StringContext) {
		called = true
	})
	if called {
		t.Error("visitor should not be called for empty input")
	}
}

func TestWalkStringsWithContext_Array(t *testing.T) {
	raw := json.RawMessage(`["a","b"]`)
	var got []string
	WalkStringsWithContext(raw, "/arr", func(ctx StringContext) {
		got = append(got, ctx.Pointer)
	})
	sort.Strings(got)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0] != "/arr/0" || got[1] != "/arr/1" {
		t.Errorf("unexpected pointers: %v", got)
	}
}
