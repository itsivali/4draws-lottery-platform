package staff_test

import (
	"context"
	"errors"
	"testing"

	"github.com/itsivali/lottery-platform/internal/staff"
)

func TestParseRoles_KnowsBothRolesAndRefusesOthers(t *testing.T) {
	got, err := staff.ParseRoles([]string{"staff", "admin", "staff"})
	if err != nil || len(got) != 2 || got[0] != staff.Staff || got[1] != staff.Admin {
		t.Fatalf("roles %v %v", got, err)
	}
	for _, bad := range [][]string{{"offline_access"}, {"Admin"}, {""}, {"draw_official"}, {"auditor"}} {
		if _, err := staff.ParseRoles(bad); !errors.Is(err, staff.ErrInvalid) {
			t.Errorf("%v accepted", bad)
		}
	}
}

func TestMember_OperatorNamesTheMemberAndAdminsMayActAsStaff(t *testing.T) {
	m := staff.Member{ID: "0192f000-0000-7000-8000-0000000000aa", Roles: []staff.Role{staff.Staff}}
	if m.Operator() != "staff:0192f000-0000-7000-8000-0000000000aa" || !m.Has(staff.Staff) || m.Has(staff.Admin) {
		t.Fatalf("%+v %s", m, m.Operator())
	}
	admin := staff.Member{ID: "0192f000-0000-7000-8000-0000000000ab", Roles: []staff.Role{staff.Admin}}
	if !admin.Has(staff.Admin) || !admin.Has(staff.Staff) {
		t.Fatalf("an admin may do everything staff do: %+v", admin)
	}
}

func TestCheckPassword_StaffRules(t *testing.T) {
	for _, tc := range []struct {
		pw, email string
		ok        bool
	}{
		{"correct horse battery", "amina@example.com", true},
		{"short-one11", "amina@example.com", false},       // 11 characters
		{"password12345", "amina@example.com", false},     // common
		{"amina@example.com", "amina@example.com", false}, // the address itself
		{"my-amina-secret-1", "amina@example.com", false}, // contains the local part
		{"aaaaaaaaaaaaaaaa", "amina@example.com", false},  // one repeated character
		{"Nairobi evening rain 2026", "ops@example.com", true},
	} {
		err := staff.CheckPassword(tc.pw, tc.email)
		if (err == nil) != tc.ok || (err != nil && !errors.Is(err, staff.ErrWeakPassword)) {
			t.Errorf("%q for %s: %v, want ok=%v", tc.pw, tc.email, err, tc.ok)
		}
	}
}

func TestUnavailable_RefusesEveryone(t *testing.T) {
	if _, err := (staff.Unavailable{}).Authenticate(context.Background(), "stf_x"); !errors.Is(err, staff.ErrUnavailable) {
		t.Fatal(err)
	}
}
