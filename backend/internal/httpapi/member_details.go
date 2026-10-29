package httpapi

import (
	"regexp"
	"strings"
)

var uaeMobileRE = regexp.MustCompile(`^\+9715[0-9]{8}$`)

var abuDhabiAreas = map[string]bool{
	"Al Reem Island": true, "Saadiyat Island": true, "Yas Island": true,
	"Al Raha": true, "Khalifa City": true, "Mohammed Bin Zayed City": true,
	"Al Bateen": true, "Al Khalidiyah": true, "Corniche": true,
	"Downtown Abu Dhabi": true, "Al Maryah Island": true, "Al Reef": true,
	"Al Shamkha": true, "Other": true,
}

// normalizeUAEMobile accepts optional UAE mobile numbers in local or E.164
// formats, then stores only the +971 international representation.
// It never records a phone number from another country.
func normalizeUAEMobile(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", true
	}
	if strings.ContainsAny(value, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		return "", false
	}
	compact := strings.NewReplacer(" ", "", "\t", "", "-", "", "(", "", ")", "").Replace(value)
	switch {
	case strings.HasPrefix(compact, "00971"):
		compact = "+" + compact[2:]
	case strings.HasPrefix(compact, "05"):
		compact = "+971" + compact[1:]
	}
	return compact, uaeMobileRE.MatchString(compact)
}

func validAbuDhabiArea(area string) bool {
	return abuDhabiAreas[area]
}
