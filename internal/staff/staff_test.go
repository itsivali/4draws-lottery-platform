package staff_test

import (
	"context"
	"errors"
	"testing"

	"github.com/itsivali/lottery-platform/internal/staff"
)

func TestParseRoles_KnowsEveryRoleAndRefusesOthers(t *testing.T) {
	got, err := staff.ParseRoles([]string{"admin", "finance", "kyc_reviewer", "finance"})
	if err != nil || len(got) != 3 || got[0] != staff.Admin || got[1] != staff.Finance {
		t.Fatalf("roles %v %v", got, err)
	}
	for _, bad := range [][]string{{"offline_access"}, {"Admin"}, {""}} {
		if _, err := staff.ParseRoles(bad); !errors.Is(err, staff.ErrInvalid) {
			t.Errorf("%v accepted", bad)
		}
	}
}

func TestMember_OperatorNamesTheMemberAndRolesGateActions(t *testing.T) {
	m := staff.Member{ID: "0192f000-0000-7000-8000-0000000000aa", Roles: []staff.Role{staff.Finance}}
	if m.Operator() != "staff:0192f000-0000-7000-8000-0000000000aa" || !m.Has(staff.Finance) || m.Has(staff.Admin) {
		t.Fatalf("%+v %s", m, m.Operator())
	}
}

func TestUnavailable_RefusesEveryone(t *testing.T) {
	if _, err := (staff.Unavailable{}).Authenticate(context.Background(), "stf_x"); !errors.Is(err, staff.ErrUnavailable) {
		t.Fatal(err)
	}
}
