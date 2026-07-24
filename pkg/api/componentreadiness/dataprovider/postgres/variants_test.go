package postgres

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
)

func TestBuildVariantFilterClause(t *testing.T) {
	tests := []struct {
		name            string
		includeVariants map[string][]string
		wantClause      string
		wantArgCount    int
		wantArgs        []any
	}{
		{
			name:            "empty filter",
			includeVariants: map[string][]string{},
			wantClause:      "",
			wantArgCount:    0,
		},
		{
			name:            "single key single value",
			includeVariants: map[string][]string{"Platform": {"aws"}},
			wantClause:      "variants && ARRAY[?]::text[]",
			wantArgCount:    1,
			wantArgs:        []any{"Platform:aws"},
		},
		{
			name:            "single key multiple values",
			includeVariants: map[string][]string{"Platform": {"aws", "gcp"}},
			wantClause:      "variants && ARRAY[?, ?]::text[]",
			wantArgCount:    2,
			wantArgs:        []any{"Platform:aws", "Platform:gcp"},
		},
		{
			name: "multiple keys sorted alphabetically",
			includeVariants: map[string][]string{
				"Platform": {"aws"},
				"Network":  {"ovn"},
			},
			wantClause:   "variants && ARRAY[?]::text[] AND variants && ARRAY[?]::text[]",
			wantArgCount: 2,
			wantArgs:     []any{"Network:ovn", "Platform:aws"},
		},
		{
			name:            "key with empty values skipped",
			includeVariants: map[string][]string{"Platform": {}},
			wantClause:      "",
			wantArgCount:    0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clause, args := buildVariantFilterClause(tc.includeVariants)
			if clause != tc.wantClause {
				t.Errorf("clause = %q, want %q", clause, tc.wantClause)
			}
			if len(args) != tc.wantArgCount {
				t.Errorf("len(args) = %d, want %d", len(args), tc.wantArgCount)
			}
			if tc.wantArgs != nil {
				for i, want := range tc.wantArgs {
					if i >= len(args) {
						break
					}
					if args[i] != want {
						t.Errorf("args[%d] = %v, want %v", i, args[i], want)
					}
				}
			}
		})
	}
}

func TestBuildVariantGroupMapping(t *testing.T) {
	tests := []struct {
		name              string
		variantLookup     map[uint]map[string]string
		wantValuesClause  string
		wantGroupVariants map[int]map[string]string
	}{
		{
			name:              "empty input",
			variantLookup:     map[uint]map[string]string{},
			wantValuesClause:  "",
			wantGroupVariants: map[int]map[string]string{},
		},
		{
			name: "single vcid",
			variantLookup: map[uint]map[string]string{
				1: {"Platform": "aws"},
			},
			wantValuesClause:  "VALUES (1,0)",
			wantGroupVariants: map[int]map[string]string{0: {"Platform": "aws"}},
		},
		{
			name: "two vcids same dimensions get same group",
			variantLookup: map[uint]map[string]string{
				1: {"Platform": "aws"},
				2: {"Platform": "aws"},
			},
			wantValuesClause:  "VALUES (1,0),(2,0)",
			wantGroupVariants: map[int]map[string]string{0: {"Platform": "aws"}},
		},
		{
			name: "two vcids different dimensions get different groups",
			variantLookup: map[uint]map[string]string{
				1: {"Platform": "aws"},
				2: {"Platform": "gcp"},
			},
			wantValuesClause: "VALUES (1,0),(2,1)",
			wantGroupVariants: map[int]map[string]string{
				0: {"Platform": "aws"},
				1: {"Platform": "gcp"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := buildVariantGroupMapping(tc.variantLookup)
			if result.valuesClause != tc.wantValuesClause {
				t.Errorf("valuesClause = %q, want %q", result.valuesClause, tc.wantValuesClause)
			}
			if len(result.groupToVariants) != len(tc.wantGroupVariants) {
				t.Fatalf("group count = %d, want %d", len(result.groupToVariants), len(tc.wantGroupVariants))
			}
			for gid, wantVars := range tc.wantGroupVariants {
				gotVars, ok := result.groupToVariants[gid]
				if !ok {
					t.Errorf("missing group %d", gid)
					continue
				}
				for k, wantV := range wantVars {
					if gotV := gotVars[k]; gotV != wantV {
						t.Errorf("group %d: got[%q] = %q, want %q", gid, k, gotV, wantV)
					}
				}
			}
		})
	}
}

func TestBuildColumnGroupMapping(t *testing.T) {
	tests := []struct {
		name             string
		groupToVariants  map[int]map[string]string
		columnGroupBy    sets.Set[string]
		wantValuesClause string
		wantNewEntries   map[int]map[string]string
	}{
		{
			name:             "empty input",
			groupToVariants:  map[int]map[string]string{},
			columnGroupBy:    sets.New[string]("Platform"),
			wantValuesClause: "",
			wantNewEntries:   map[int]map[string]string{},
		},
		{
			name: "groups with same column projection collapse",
			groupToVariants: map[int]map[string]string{
				0: {"Platform": "aws", "Network": "ovn"},
				1: {"Platform": "aws", "Network": "sdn"},
			},
			columnGroupBy:    sets.New[string]("Platform"),
			wantValuesClause: "VALUES (0,2),(1,2)",
			wantNewEntries:   map[int]map[string]string{2: {"Platform": "aws"}},
		},
		{
			name: "groups with different column projection stay separate",
			groupToVariants: map[int]map[string]string{
				0: {"Platform": "aws", "Network": "ovn"},
				1: {"Platform": "gcp", "Network": "ovn"},
			},
			columnGroupBy:    sets.New[string]("Platform"),
			wantValuesClause: "VALUES (0,2),(1,3)",
			wantNewEntries: map[int]map[string]string{
				2: {"Platform": "aws"},
				3: {"Platform": "gcp"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			initialSize := len(tc.groupToVariants)
			result := buildColumnGroupMapping(tc.groupToVariants, tc.columnGroupBy)

			if result.valuesClause != tc.wantValuesClause {
				t.Errorf("valuesClause = %q, want %q", result.valuesClause, tc.wantValuesClause)
			}

			newEntryCount := len(tc.groupToVariants) - initialSize
			if newEntryCount != len(tc.wantNewEntries) {
				t.Fatalf("new synthetic entries = %d, want %d", newEntryCount, len(tc.wantNewEntries))
			}
			for gid, wantVars := range tc.wantNewEntries {
				gotVars, ok := tc.groupToVariants[gid]
				if !ok {
					t.Errorf("missing synthetic group %d", gid)
					continue
				}
				for k, wantV := range wantVars {
					if gotV := gotVars[k]; gotV != wantV {
						t.Errorf("group %d: got[%q] = %q, want %q", gid, k, gotV, wantV)
					}
				}
			}
		})
	}
}
