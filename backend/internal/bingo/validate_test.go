package bingo

import "testing"

func TestValidateBoard(t *testing.T) {
	// A freshly generated board is always valid.
	if err := ValidateBoard(GenerateBoard()); err != nil {
		t.Errorf("GenerateBoard produced an invalid board: %v", err)
	}

	valid := [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
	if err := ValidateBoard(valid); err != nil {
		t.Errorf("valid board rejected: %v", err)
	}

	tests := []struct {
		name  string
		mutate func(b [][]int)
	}{
		{"free centre not 0", func(b [][]int) { b[2][2] = 33 }},
		{"out of column range", func(b [][]int) { b[0][0] = 99 }}, // 99 not in B (1–15)
		{"wrong column band", func(b [][]int) { b[0][0] = 20 }},   // 20 belongs in I, not B
		{"duplicate in column", func(b [][]int) { b[1][0] = b[0][0] }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := [][]int{
				{1, 16, 31, 46, 61},
				{2, 17, 32, 47, 62},
				{3, 18, 0, 48, 63},
				{4, 19, 34, 49, 64},
				{5, 20, 35, 50, 65},
			}
			tc.mutate(b)
			if err := ValidateBoard(b); err == nil {
				t.Errorf("%s: expected an error, got nil", tc.name)
			}
		})
	}

	// Wrong shapes.
	if err := ValidateBoard([][]int{{1, 2, 3}}); err == nil {
		t.Error("non-5×5 board should be rejected")
	}
}

func TestValidateCustomID(t *testing.T) {
	if got, err := ValidateCustomID(" abc123 "); err != nil || got != "ABC123" {
		t.Errorf("ValidateCustomID(\" abc123 \") = %q,%v; want ABC123,nil", got, err)
	}
	for _, bad := range []string{"", "ABC12", "ABC1234", "ABC 12", "ABC-12", "ÄBC123"} {
		if _, err := ValidateCustomID(bad); err == nil {
			t.Errorf("ValidateCustomID(%q) should fail", bad)
		}
	}
}
