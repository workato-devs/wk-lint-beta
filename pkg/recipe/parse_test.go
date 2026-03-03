package recipe

import (
	"sort"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		wantErr     bool
		errContains string
		check       func(t *testing.T, pr *ParsedRecipe)
	}{
		{
			name: "valid minimal recipe with one action",
			json: `{
				"name": "Test recipe",
				"version": 1,
				"private": true,
				"concurrency": 1,
				"code": {
					"number": 0,
					"provider": "workato_recipe_function",
					"name": "execute",
					"as": "trigger",
					"keyword": "trigger",
					"input": {},
					"uuid": "trigger-001",
					"block": [
						{
							"number": 1,
							"provider": "salesforce",
							"name": "update_sobject",
							"as": "update_record",
							"keyword": "action",
							"input": {},
							"uuid": "action-001"
						}
					]
				},
				"config": [
					{"keyword": "application", "provider": "workato_recipe_function"},
					{"keyword": "application", "provider": "salesforce"}
				]
			}`,
			check: func(t *testing.T, pr *ParsedRecipe) {
				// Raw fields
				if pr.Raw.Name != "Test recipe" {
					t.Errorf("Raw.Name = %q, want %q", pr.Raw.Name, "Test recipe")
				}
				if pr.Raw.Version == nil || *pr.Raw.Version != 1 {
					t.Errorf("Raw.Version = %v, want 1", pr.Raw.Version)
				}
				if pr.Raw.Private == nil || *pr.Raw.Private != true {
					t.Errorf("Raw.Private = %v, want true", pr.Raw.Private)
				}

				// Trigger
				if pr.Trigger.Keyword != "trigger" {
					t.Errorf("Trigger.Keyword = %q, want %q", pr.Trigger.Keyword, "trigger")
				}
				if pr.Trigger.Provider == nil || *pr.Trigger.Provider != "workato_recipe_function" {
					t.Errorf("Trigger.Provider = %v, want %q", pr.Trigger.Provider, "workato_recipe_function")
				}
				if pr.Trigger.UUID != "trigger-001" {
					t.Errorf("Trigger.UUID = %q, want %q", pr.Trigger.UUID, "trigger-001")
				}
				if pr.Trigger.Number == nil || *pr.Trigger.Number != 0 {
					t.Errorf("Trigger.Number = %v, want 0", pr.Trigger.Number)
				}

				// Steps: trigger + 1 action = 2
				if len(pr.Steps) != 2 {
					t.Fatalf("len(Steps) = %d, want 2", len(pr.Steps))
				}

				// Step 0: trigger at /code, depth 0
				s0 := pr.Steps[0]
				if s0.JSONPointer != "/code" {
					t.Errorf("Steps[0].JSONPointer = %q, want %q", s0.JSONPointer, "/code")
				}
				if s0.Depth != 0 {
					t.Errorf("Steps[0].Depth = %d, want 0", s0.Depth)
				}
				if s0.Code.Keyword != "trigger" {
					t.Errorf("Steps[0].Code.Keyword = %q, want %q", s0.Code.Keyword, "trigger")
				}

				// Step 1: action at /code/block/0, depth 1
				s1 := pr.Steps[1]
				if s1.JSONPointer != "/code/block/0" {
					t.Errorf("Steps[1].JSONPointer = %q, want %q", s1.JSONPointer, "/code/block/0")
				}
				if s1.Depth != 1 {
					t.Errorf("Steps[1].Depth = %d, want 1", s1.Depth)
				}
				if s1.Code.Keyword != "action" {
					t.Errorf("Steps[1].Code.Keyword = %q, want %q", s1.Code.Keyword, "action")
				}
				if s1.Code.Provider == nil || *s1.Code.Provider != "salesforce" {
					t.Errorf("Steps[1].Code.Provider = %v, want %q", s1.Code.Provider, "salesforce")
				}
				if s1.Code.Number == nil || *s1.Code.Number != 1 {
					t.Errorf("Steps[1].Code.Number = %v, want 1", s1.Code.Number)
				}
				if s1.Code.UUID != "action-001" {
					t.Errorf("Steps[1].Code.UUID = %q, want %q", s1.Code.UUID, "action-001")
				}
				if s1.Code.Name != "update_sobject" {
					t.Errorf("Steps[1].Code.Name = %q, want %q", s1.Code.Name, "update_sobject")
				}
				if s1.Code.As != "update_record" {
					t.Errorf("Steps[1].Code.As = %q, want %q", s1.Code.As, "update_record")
				}

				// Config
				if len(pr.Config) != 2 {
					t.Fatalf("len(Config) = %d, want 2", len(pr.Config))
				}
				if pr.Config[0].Keyword != "application" {
					t.Errorf("Config[0].Keyword = %q, want %q", pr.Config[0].Keyword, "application")
				}
				if pr.Config[0].Provider != "workato_recipe_function" {
					t.Errorf("Config[0].Provider = %q, want %q", pr.Config[0].Provider, "workato_recipe_function")
				}
				if pr.Config[1].Provider != "salesforce" {
					t.Errorf("Config[1].Provider = %q, want %q", pr.Config[1].Provider, "salesforce")
				}

				// Providers (order not guaranteed by map iteration)
				if len(pr.Providers) != 2 {
					t.Fatalf("len(Providers) = %d, want 2", len(pr.Providers))
				}
				sort.Strings(pr.Providers)
				if pr.Providers[0] != "salesforce" || pr.Providers[1] != "workato_recipe_function" {
					t.Errorf("Providers = %v, want [salesforce workato_recipe_function]", pr.Providers)
				}
			},
		},
		{
			name: "nested blocks - try/catch with depth 2",
			json: `{
				"name": "Try-catch recipe",
				"version": 1,
				"private": true,
				"concurrency": 1,
				"code": {
					"number": 0,
					"provider": "workato_recipe_function",
					"name": "execute",
					"keyword": "trigger",
					"uuid": "trigger-001",
					"block": [
						{
							"number": 1,
							"provider": "workato",
							"name": "try",
							"keyword": "action",
							"uuid": "try-001",
							"block": [
								{
									"number": 2,
									"provider": "salesforce",
									"name": "create_sobject",
									"keyword": "action",
									"uuid": "create-001"
								}
							]
						},
						{
							"number": 3,
							"provider": "workato",
							"name": "catch",
							"keyword": "action",
							"uuid": "catch-001",
							"block": [
								{
									"number": 4,
									"provider": "workato",
									"name": "log_message",
									"keyword": "action",
									"uuid": "log-001"
								}
							]
						}
					]
				},
				"config": []
			}`,
			check: func(t *testing.T, pr *ParsedRecipe) {
				// trigger + try + create + catch + log = 5 steps
				if len(pr.Steps) != 5 {
					t.Fatalf("len(Steps) = %d, want 5", len(pr.Steps))
				}

				expected := []struct {
					pointer string
					depth   int
					keyword string
					uuid    string
				}{
					{"/code", 0, "trigger", "trigger-001"},
					{"/code/block/0", 1, "action", "try-001"},
					{"/code/block/0/block/0", 2, "action", "create-001"},
					{"/code/block/1", 1, "action", "catch-001"},
					{"/code/block/1/block/0", 2, "action", "log-001"},
				}

				for i, exp := range expected {
					s := pr.Steps[i]
					if s.JSONPointer != exp.pointer {
						t.Errorf("Steps[%d].JSONPointer = %q, want %q", i, s.JSONPointer, exp.pointer)
					}
					if s.Depth != exp.depth {
						t.Errorf("Steps[%d].Depth = %d, want %d", i, s.Depth, exp.depth)
					}
					if s.Code.Keyword != exp.keyword {
						t.Errorf("Steps[%d].Code.Keyword = %q, want %q", i, s.Code.Keyword, exp.keyword)
					}
					if s.Code.UUID != exp.uuid {
						t.Errorf("Steps[%d].Code.UUID = %q, want %q", i, s.Code.UUID, exp.uuid)
					}
				}
			},
		},
		{
			name: "if/else blocks",
			json: `{
				"name": "If-else recipe",
				"version": 1,
				"private": true,
				"concurrency": 1,
				"code": {
					"number": 0,
					"provider": "workato_recipe_function",
					"name": "execute",
					"keyword": "trigger",
					"uuid": "trigger-001",
					"block": [
						{
							"number": 1,
							"provider": "workato",
							"name": "if",
							"keyword": "action",
							"uuid": "if-001",
							"conditions": {"all": []},
							"block": [
								{
									"number": 2,
									"provider": "salesforce",
									"name": "update_sobject",
									"keyword": "action",
									"uuid": "update-001"
								}
							]
						},
						{
							"number": 3,
							"provider": "workato",
							"name": "elsif",
							"keyword": "action",
							"uuid": "elsif-001",
							"block": [
								{
									"number": 4,
									"provider": "salesforce",
									"name": "delete_sobject",
									"keyword": "action",
									"uuid": "delete-001"
								}
							]
						}
					]
				},
				"config": [
					{"keyword": "application", "provider": "salesforce"}
				]
			}`,
			check: func(t *testing.T, pr *ParsedRecipe) {
				// trigger + if + update + elsif + delete = 5
				if len(pr.Steps) != 5 {
					t.Fatalf("len(Steps) = %d, want 5", len(pr.Steps))
				}

				expected := []struct {
					pointer string
					depth   int
					name    string
				}{
					{"/code", 0, "execute"},
					{"/code/block/0", 1, "if"},
					{"/code/block/0/block/0", 2, "update_sobject"},
					{"/code/block/1", 1, "elsif"},
					{"/code/block/1/block/0", 2, "delete_sobject"},
				}

				for i, exp := range expected {
					s := pr.Steps[i]
					if s.JSONPointer != exp.pointer {
						t.Errorf("Steps[%d].JSONPointer = %q, want %q", i, s.JSONPointer, exp.pointer)
					}
					if s.Depth != exp.depth {
						t.Errorf("Steps[%d].Depth = %d, want %d", i, s.Depth, exp.depth)
					}
					if s.Code.Name != exp.name {
						t.Errorf("Steps[%d].Code.Name = %q, want %q", i, s.Code.Name, exp.name)
					}
				}

				// Verify the if step has conditions populated
				ifStep := pr.Steps[1]
				if ifStep.Code.Conditions == nil {
					t.Error("if step should have non-nil Conditions")
				}

				// Config / providers
				if len(pr.Providers) != 1 {
					t.Fatalf("len(Providers) = %d, want 1", len(pr.Providers))
				}
				if pr.Providers[0] != "salesforce" {
					t.Errorf("Providers[0] = %q, want %q", pr.Providers[0], "salesforce")
				}
			},
		},
		{
			name:        "invalid JSON",
			json:        `{not valid json at all!!!`,
			wantErr:     true,
			errContains: "invalid JSON",
		},
		{
			name: "code as array should fail",
			json: `{
				"name": "Bad recipe",
				"version": 1,
				"code": [
					{"keyword": "trigger", "number": 0}
				],
				"config": []
			}`,
			wantErr:     true,
			errContains: "cannot parse code block",
		},
		{
			name: "empty block - trigger only with no actions",
			json: `{
				"name": "Empty block recipe",
				"version": 1,
				"private": false,
				"concurrency": 1,
				"code": {
					"number": 0,
					"provider": "workato_recipe_function",
					"name": "execute",
					"keyword": "trigger",
					"uuid": "trigger-only"
				},
				"config": []
			}`,
			check: func(t *testing.T, pr *ParsedRecipe) {
				// Only the trigger step
				if len(pr.Steps) != 1 {
					t.Fatalf("len(Steps) = %d, want 1", len(pr.Steps))
				}
				s := pr.Steps[0]
				if s.JSONPointer != "/code" {
					t.Errorf("Steps[0].JSONPointer = %q, want %q", s.JSONPointer, "/code")
				}
				if s.Depth != 0 {
					t.Errorf("Steps[0].Depth = %d, want 0", s.Depth)
				}
				if s.Code.UUID != "trigger-only" {
					t.Errorf("Steps[0].Code.UUID = %q, want %q", s.Code.UUID, "trigger-only")
				}
				if len(s.Code.Block) != 0 {
					t.Errorf("Steps[0].Code.Block length = %d, want 0", len(s.Code.Block))
				}
			},
		},
		{
			name: "config parsing with duplicate and empty providers",
			json: `{
				"name": "Config test",
				"version": 1,
				"code": {
					"number": 0,
					"provider": "workato_recipe_function",
					"keyword": "trigger",
					"uuid": "trigger-001"
				},
				"config": [
					{"keyword": "application", "provider": "salesforce"},
					{"keyword": "application", "provider": "salesforce"},
					{"keyword": "application", "provider": "slack"},
					{"keyword": "property", "name": "some_prop"}
				]
			}`,
			check: func(t *testing.T, pr *ParsedRecipe) {
				// All 4 config entries should be present
				if len(pr.Config) != 4 {
					t.Fatalf("len(Config) = %d, want 4", len(pr.Config))
				}

				// Check the property entry (no provider)
				if pr.Config[3].Keyword != "property" {
					t.Errorf("Config[3].Keyword = %q, want %q", pr.Config[3].Keyword, "property")
				}
				if pr.Config[3].Name != "some_prop" {
					t.Errorf("Config[3].Name = %q, want %q", pr.Config[3].Name, "some_prop")
				}
				if pr.Config[3].Provider != "" {
					t.Errorf("Config[3].Provider = %q, want empty", pr.Config[3].Provider)
				}

				// Providers should be deduplicated: salesforce and slack
				if len(pr.Providers) != 2 {
					t.Fatalf("len(Providers) = %d, want 2", len(pr.Providers))
				}
				sort.Strings(pr.Providers)
				if pr.Providers[0] != "salesforce" || pr.Providers[1] != "slack" {
					t.Errorf("Providers = %v, want [salesforce slack]", pr.Providers)
				}
			},
		},
		{
			name: "no config field",
			json: `{
				"name": "No config",
				"version": 1,
				"code": {
					"number": 0,
					"provider": "workato_recipe_function",
					"keyword": "trigger",
					"uuid": "trigger-001"
				}
			}`,
			check: func(t *testing.T, pr *ParsedRecipe) {
				if len(pr.Config) != 0 {
					t.Errorf("len(Config) = %d, want 0", len(pr.Config))
				}
				if len(pr.Providers) != 0 {
					t.Errorf("len(Providers) = %d, want 0", len(pr.Providers))
				}
			},
		},
		{
			name: "deeply nested - 3+ levels",
			json: `{
				"name": "Deeply nested recipe",
				"version": 1,
				"private": true,
				"concurrency": 1,
				"code": {
					"number": 0,
					"provider": "workato_recipe_function",
					"keyword": "trigger",
					"uuid": "t-001",
					"block": [
						{
							"number": 1,
							"provider": "workato",
							"name": "repeat",
							"keyword": "action",
							"uuid": "repeat-001",
							"block": [
								{
									"number": 2,
									"provider": "workato",
									"name": "if",
									"keyword": "action",
									"uuid": "if-001",
									"block": [
										{
											"number": 3,
											"provider": "workato",
											"name": "try",
											"keyword": "action",
											"uuid": "try-001",
											"block": [
												{
													"number": 4,
													"provider": "salesforce",
													"name": "create_sobject",
													"keyword": "action",
													"uuid": "create-001"
												}
											]
										}
									]
								}
							]
						}
					]
				},
				"config": []
			}`,
			check: func(t *testing.T, pr *ParsedRecipe) {
				// trigger + repeat + if + try + create = 5
				if len(pr.Steps) != 5 {
					t.Fatalf("len(Steps) = %d, want 5", len(pr.Steps))
				}

				expected := []struct {
					pointer string
					depth   int
					uuid    string
				}{
					{"/code", 0, "t-001"},
					{"/code/block/0", 1, "repeat-001"},
					{"/code/block/0/block/0", 2, "if-001"},
					{"/code/block/0/block/0/block/0", 3, "try-001"},
					{"/code/block/0/block/0/block/0/block/0", 4, "create-001"},
				}

				for i, exp := range expected {
					s := pr.Steps[i]
					if s.JSONPointer != exp.pointer {
						t.Errorf("Steps[%d].JSONPointer = %q, want %q", i, s.JSONPointer, exp.pointer)
					}
					if s.Depth != exp.depth {
						t.Errorf("Steps[%d].Depth = %d, want %d", i, s.Depth, exp.depth)
					}
					if s.Code.UUID != exp.uuid {
						t.Errorf("Steps[%d].Code.UUID = %q, want %q", i, s.Code.UUID, exp.uuid)
					}
				}
			},
		},
		{
			name: "multiple actions at same level",
			json: `{
				"name": "Multi-action recipe",
				"version": 1,
				"code": {
					"number": 0,
					"provider": "workato_recipe_function",
					"keyword": "trigger",
					"uuid": "t-001",
					"block": [
						{
							"number": 1,
							"provider": "salesforce",
							"name": "search_sobjects",
							"keyword": "action",
							"uuid": "search-001"
						},
						{
							"number": 2,
							"provider": "salesforce",
							"name": "update_sobject",
							"keyword": "action",
							"uuid": "update-001"
						},
						{
							"number": 3,
							"provider": "slack",
							"name": "post_message",
							"keyword": "action",
							"uuid": "slack-001"
						}
					]
				},
				"config": []
			}`,
			check: func(t *testing.T, pr *ParsedRecipe) {
				// trigger + 3 actions = 4
				if len(pr.Steps) != 4 {
					t.Fatalf("len(Steps) = %d, want 4", len(pr.Steps))
				}

				// All actions should be at depth 1
				for i := 1; i < len(pr.Steps); i++ {
					if pr.Steps[i].Depth != 1 {
						t.Errorf("Steps[%d].Depth = %d, want 1", i, pr.Steps[i].Depth)
					}
				}

				// Verify sequential pointers
				if pr.Steps[1].JSONPointer != "/code/block/0" {
					t.Errorf("Steps[1].JSONPointer = %q, want %q", pr.Steps[1].JSONPointer, "/code/block/0")
				}
				if pr.Steps[2].JSONPointer != "/code/block/1" {
					t.Errorf("Steps[2].JSONPointer = %q, want %q", pr.Steps[2].JSONPointer, "/code/block/1")
				}
				if pr.Steps[3].JSONPointer != "/code/block/2" {
					t.Errorf("Steps[3].JSONPointer = %q, want %q", pr.Steps[3].JSONPointer, "/code/block/2")
				}
			},
		},
		{
			name: "mixed nested and flat actions",
			json: `{
				"name": "Mixed recipe",
				"version": 1,
				"code": {
					"number": 0,
					"provider": "workato_recipe_function",
					"keyword": "trigger",
					"uuid": "t-001",
					"block": [
						{
							"number": 1,
							"provider": "salesforce",
							"name": "search_sobjects",
							"keyword": "action",
							"uuid": "search-001"
						},
						{
							"number": 2,
							"provider": "workato",
							"name": "if",
							"keyword": "action",
							"uuid": "if-001",
							"block": [
								{
									"number": 3,
									"provider": "salesforce",
									"name": "update_sobject",
									"keyword": "action",
									"uuid": "update-001"
								}
							]
						},
						{
							"number": 4,
							"provider": "slack",
							"name": "post_message",
							"keyword": "action",
							"uuid": "slack-001"
						}
					]
				},
				"config": []
			}`,
			check: func(t *testing.T, pr *ParsedRecipe) {
				// trigger + search + if + update + slack = 5
				if len(pr.Steps) != 5 {
					t.Fatalf("len(Steps) = %d, want 5", len(pr.Steps))
				}

				expected := []struct {
					pointer string
					depth   int
					uuid    string
				}{
					{"/code", 0, "t-001"},
					{"/code/block/0", 1, "search-001"},
					{"/code/block/1", 1, "if-001"},
					{"/code/block/1/block/0", 2, "update-001"},
					{"/code/block/2", 1, "slack-001"},
				}

				for i, exp := range expected {
					s := pr.Steps[i]
					if s.JSONPointer != exp.pointer {
						t.Errorf("Steps[%d].JSONPointer = %q, want %q", i, s.JSONPointer, exp.pointer)
					}
					if s.Depth != exp.depth {
						t.Errorf("Steps[%d].Depth = %d, want %d", i, s.Depth, exp.depth)
					}
					if s.Code.UUID != exp.uuid {
						t.Errorf("Steps[%d].Code.UUID = %q, want %q", i, s.Code.UUID, exp.uuid)
					}
				}
			},
		},
		{
			name: "provider is null in code",
			json: `{
				"name": "Null provider recipe",
				"version": 1,
				"code": {
					"number": 0,
					"provider": null,
					"keyword": "trigger",
					"uuid": "t-001"
				},
				"config": []
			}`,
			check: func(t *testing.T, pr *ParsedRecipe) {
				if pr.Trigger.Provider != nil {
					t.Errorf("Trigger.Provider = %v, want nil", pr.Trigger.Provider)
				}
			},
		},
		{
			name: "description field is preserved",
			json: `{
				"name": "Described recipe",
				"version": 1,
				"description": "This does something useful",
				"code": {
					"number": 0,
					"provider": "workato_recipe_function",
					"keyword": "trigger",
					"uuid": "t-001"
				},
				"config": []
			}`,
			check: func(t *testing.T, pr *ParsedRecipe) {
				if pr.Raw.Description != "This does something useful" {
					t.Errorf("Raw.Description = %q, want %q", pr.Raw.Description, "This does something useful")
				}
			},
		},
		{
			name: "empty block array",
			json: `{
				"name": "Empty block array",
				"version": 1,
				"code": {
					"number": 0,
					"provider": "workato_recipe_function",
					"keyword": "trigger",
					"uuid": "t-001",
					"block": []
				},
				"config": []
			}`,
			check: func(t *testing.T, pr *ParsedRecipe) {
				// Only the trigger
				if len(pr.Steps) != 1 {
					t.Fatalf("len(Steps) = %d, want 1", len(pr.Steps))
				}
				if pr.Steps[0].JSONPointer != "/code" {
					t.Errorf("Steps[0].JSONPointer = %q, want %q", pr.Steps[0].JSONPointer, "/code")
				}
			},
		},
		{
			name:        "completely empty input",
			json:        ``,
			wantErr:     true,
			errContains: "invalid JSON",
		},
		{
			name:        "truncated JSON",
			json:        `{"name": "test", "code": {"keyword": "trigger"`,
			wantErr:     true,
			errContains: "invalid JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr, err := Parse([]byte(tt.json))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, pr)
			}
		})
	}
}
