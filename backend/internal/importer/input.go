package importer

import (
	"encoding/csv"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Input is the validated content of an export directory:
//
//	members.csv     required: legacy_id,email,first_name
//	                optional: display_name,phone,area,interests (";"-separated),
//	                          status,email_confirmed,email_updates,created_at
//	gatherings.csv  optional: legacy_id,title,starts_on (YYYY-MM-DD),starts_at (HH:MM),
//	                          place,capacity [,type,description]
//	bookings.csv    optional: legacy_member_id,legacy_gathering_id [,status,created_at]
//
// Column order does not matter; unknown columns are ignored.
type Input struct {
	Members    []Member
	Gatherings []Gathering
	Bookings   []Booking
	Issues     []Issue // invalid rows (skipped)
	Warnings   []Issue // rows accepted with adjustments
}

type Member struct {
	Row                                                  int
	LegacyID, Email, FirstName, DisplayName, Phone, Area string
	Interests                                            []string
	Status                                               string
	EmailConfirmed, EmailUpdates                         bool
	CreatedAt                                            *time.Time
}

type Gathering struct {
	Row                                                           int
	LegacyID, Title, Type, Description, StartsOn, StartsAt, Place string
	Capacity                                                      int
}

type Booking struct {
	Row                               int
	MemberLegacyID, GatheringLegacyID string
	Status                            string
	CreatedAt                         time.Time
}

var (
	emailRe = regexp.MustCompile(`^[^@\s<>,;"]+@[^@\s<>,;"]+\.[^@\s<>,;"]+$`)
	phoneRe = regexp.MustCompile(`^\+?[0-9 ()\-]{7,20}$`)
)

var statusAliases = map[string]string{
	"": "pending", "pending": "pending", "applied": "pending", "new": "pending", "waiting": "pending",
	"approved": "approved", "active": "approved", "member": "approved",
	"declined": "declined", "rejected": "declined",
}

// Load reads and validates an export directory. Files are only ever opened
// for reading.
func Load(dir string) (*Input, error) {
	in := &Input{}
	members, err := readCSV(filepath.Join(dir, "members.csv"), true, "legacy_id", "email", "first_name")
	if err != nil {
		return nil, err
	}
	seenID, seenEmail := map[string]bool{}, map[string]bool{}
	for _, r := range members {
		m, issue, warns := parseMember(r)
		in.Warnings = append(in.Warnings, warns...)
		switch {
		case issue != "":
			in.Issues = append(in.Issues, Issue{"members.csv", r.row, issue})
		case seenID[m.LegacyID]:
			in.Issues = append(in.Issues, Issue{"members.csv", r.row, "duplicate_legacy_id"})
		case seenEmail[m.Email]:
			in.Issues = append(in.Issues, Issue{"members.csv", r.row, "duplicate_email_in_file"})
		default:
			seenID[m.LegacyID], seenEmail[m.Email] = true, true
			in.Members = append(in.Members, m)
		}
	}
	gatherings, err := readCSV(filepath.Join(dir, "gatherings.csv"), false, "legacy_id", "title", "starts_on", "starts_at", "place", "capacity")
	if err != nil {
		return nil, err
	}
	seenG := map[string]bool{}
	for _, r := range gatherings {
		g, issue := parseGathering(r)
		switch {
		case issue != "":
			in.Issues = append(in.Issues, Issue{"gatherings.csv", r.row, issue})
		case seenG[g.LegacyID]:
			in.Issues = append(in.Issues, Issue{"gatherings.csv", r.row, "duplicate_legacy_id"})
		default:
			seenG[g.LegacyID] = true
			in.Gatherings = append(in.Gatherings, g)
		}
	}
	bookings, err := readCSV(filepath.Join(dir, "bookings.csv"), false, "legacy_member_id", "legacy_gathering_id")
	if err != nil {
		return nil, err
	}
	seenB := map[string]bool{}
	for _, r := range bookings {
		b, issue := parseBooking(r)
		key := b.MemberLegacyID + "\x00" + b.GatheringLegacyID
		switch {
		case issue != "":
			in.Issues = append(in.Issues, Issue{"bookings.csv", r.row, issue})
		case seenB[key]:
			in.Issues = append(in.Issues, Issue{"bookings.csv", r.row, "duplicate_booking"})
		default:
			seenB[key] = true
			in.Bookings = append(in.Bookings, b)
		}
	}
	return in, nil
}

type record struct {
	row    int
	fields map[string]string
}

func (r record) get(k string) string { return strings.TrimSpace(r.fields[k]) }

func readCSV(path string, required bool, mustHave ...string) ([]record, error) {
	f, err := os.Open(path) // read-only
	if errors.Is(err, os.ErrNotExist) && !required {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cr := csv.NewReader(f)
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if err != nil {
		return nil, errors.New(filepath.Base(path) + ": cannot read header")
	}
	cols := map[int]string{}
	have := map[string]bool{}
	for i, h := range header {
		h = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))
		cols[i], have[h] = h, true
	}
	for _, c := range mustHave {
		if !have[c] {
			return nil, errors.New(filepath.Base(path) + ": missing required column " + c)
		}
	}
	var out []record
	for row := 2; ; row++ {
		rec, err := cr.Read()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, errors.New(filepath.Base(path) + ": malformed CSV at row " + strconv.Itoa(row))
		}
		fields := map[string]string{}
		for i, v := range rec {
			fields[cols[i]] = v
		}
		out = append(out, record{row, fields})
	}
}

func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "t", "yes", "y", "1":
		return true
	}
	return false
}

func parseTime(s string) (*time.Time, bool) {
	if s == "" {
		return nil, true
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999-07", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t, true
		}
	}
	return nil, false
}

func parseMember(r record) (Member, string, []Issue) {
	var warns []Issue
	m := Member{Row: r.row, LegacyID: r.get("legacy_id"), Email: strings.ToLower(r.get("email")), FirstName: r.get("first_name"),
		DisplayName: r.get("display_name"), Phone: r.get("phone"), Area: r.get("area"),
		EmailConfirmed: parseBool(r.get("email_confirmed")), EmailUpdates: parseBool(r.get("email_updates"))}
	if m.LegacyID == "" || len(m.LegacyID) > 100 {
		return m, "missing_legacy_id", nil
	}
	if !emailRe.MatchString(m.Email) || len(m.Email) > 254 {
		return m, "invalid_email", nil
	}
	if m.FirstName == "" {
		return m, "missing_first_name", nil
	}
	if len(m.FirstName) > 60 || len(m.DisplayName) > 60 || len(m.Area) > 80 {
		return m, "field_too_long", nil
	}
	status, ok := statusAliases[strings.ToLower(r.get("status"))]
	if !ok {
		return m, "unknown_status", nil
	}
	m.Status = status
	if m.Phone != "" && !phoneRe.MatchString(m.Phone) {
		m.Phone = ""
		warns = append(warns, Issue{"members.csv", r.row, "invalid_phone_dropped"})
	}
	for _, i := range strings.Split(r.get("interests"), ";") {
		if i = strings.TrimSpace(i); i != "" && len(i) <= 40 {
			m.Interests = append(m.Interests, i)
		}
	}
	if len(m.Interests) > 10 {
		m.Interests = m.Interests[:10]
		warns = append(warns, Issue{"members.csv", r.row, "interests_truncated"})
	}
	if m.Interests == nil {
		m.Interests = []string{}
	}
	t, ok := parseTime(r.get("created_at"))
	if !ok {
		return m, "invalid_created_at", nil
	}
	m.CreatedAt = t
	return m, "", warns
}

func parseGathering(r record) (Gathering, string) {
	g := Gathering{Row: r.row, LegacyID: r.get("legacy_id"), Title: r.get("title"), Type: r.get("type"), Description: r.get("description"),
		StartsOn: r.get("starts_on"), StartsAt: r.get("starts_at"), Place: r.get("place")}
	if g.LegacyID == "" {
		return g, "missing_legacy_id"
	}
	if g.Title == "" || g.Place == "" || len(g.Title) > 200 || len(g.Place) > 300 || len(g.Description) > 5000 {
		return g, "invalid_text_field"
	}
	if g.Type == "" {
		g.Type = "community"
	}
	if _, err := time.Parse("2006-01-02", g.StartsOn); err != nil {
		return g, "invalid_starts_on"
	}
	if _, err := time.Parse("15:04", g.StartsAt); err != nil {
		if _, err := time.Parse("15:04:05", g.StartsAt); err != nil {
			return g, "invalid_starts_at"
		}
	}
	c, err := strconv.Atoi(r.get("capacity"))
	if err != nil || c < 1 || c > 10000 {
		return g, "invalid_capacity"
	}
	g.Capacity = c
	return g, ""
}

func parseBooking(r record) (Booking, string) {
	b := Booking{Row: r.row, MemberLegacyID: r.get("legacy_member_id"), GatheringLegacyID: r.get("legacy_gathering_id"), Status: strings.ToLower(r.get("status"))}
	if b.MemberLegacyID == "" || b.GatheringLegacyID == "" {
		return b, "missing_reference"
	}
	switch b.Status {
	case "", "confirmed", "booked", "going":
		b.Status = "confirmed"
	case "waitlist", "waitlisted", "waiting":
		b.Status = "waitlist"
	default:
		return b, "invalid_booking_status"
	}
	t, ok := parseTime(r.get("created_at"))
	if !ok {
		return b, "invalid_created_at"
	}
	if t == nil {
		b.CreatedAt = time.Unix(0, 0).UTC().Add(time.Duration(r.row) * time.Second) // keep file order
	} else {
		b.CreatedAt = *t
	}
	return b, ""
}
