package httpapi

import "testing"

func TestNormalizeUAEMobile(t *testing.T) {
	cases := []struct {
		input string
		want  string
		valid bool
	}{
		{"", "", true},
		{"  ", "", true},
		{"+971 50 123 4567", "+971501234567", true},
		{"0501234567", "+971501234567", true},
		{"00971 55 123 4567", "+971551234567", true},
		{"+971 (58) 123-4567", "+971581234567", true},
		{"+918812345678", "", false},
		{"+97140123456", "", false},
		{"12345", "", false},
		{"050123456", "", false},
		{"+9715012345678", "", false},
		{"call me", "", false},
		{"050١٢٣٤٥٦٧", "", false},
	}
	for _, tc := range cases {
		got, ok := normalizeUAEMobile(tc.input)
		if ok != tc.valid || (ok && got != tc.want) {
			t.Errorf("normalizeUAEMobile(%q) = (%q,%t), want (%q,%t)", tc.input, got, ok, tc.want, tc.valid)
		}
	}
}

func TestAbuDhabiAreaAllowlist(t *testing.T) {
	for _, v := range []string{"Al Reem Island", "Saadiyat Island", "Khalifa City", "Other"} {
		if !validAbuDhabiArea(v) {
			t.Errorf("should allow %q", v)
		}
	}
	for _, v := range []string{"", "Dubai", "Sharjah", "Other Emirate", "Jumeirah", "Al Reem Island, Dubai"} {
		if validAbuDhabiArea(v) {
			t.Errorf("should reject %q", v)
		}
	}
}
