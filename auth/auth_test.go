package auth

import "testing"

func TestAuthenticate(t *testing.T) {
	s := NewStore()
	if err := s.AddUser("alice", "s3cret", Engineer); err != nil {
		t.Fatalf("AddUser: %v", err)
	}

	u, ok := s.Authenticate("alice", "s3cret")
	if !ok {
		t.Fatal("Authenticate with correct password: got false, want true")
	}
	if u.Name != "alice" || u.Role != Engineer {
		t.Fatalf("Authenticate returned %+v, want alice/Engineer", u)
	}

	if _, ok := s.Authenticate("alice", "wrong"); ok {
		t.Error("Authenticate with wrong password: got true, want false")
	}
	if _, ok := s.Authenticate("nobody", "s3cret"); ok {
		t.Error("Authenticate unknown user: got true, want false")
	}
}

func TestSaltDiffersForSamePassword(t *testing.T) {
	s := NewStore()
	if err := s.AddUser("alice", "same", Operator); err != nil {
		t.Fatalf("AddUser alice: %v", err)
	}
	if err := s.AddUser("bob", "same", Operator); err != nil {
		t.Fatalf("AddUser bob: %v", err)
	}

	a, b := s.users["alice"], s.users["bob"]
	if a.salt == b.salt {
		t.Error("two users share the same salt")
	}
	if a.hash == b.hash {
		t.Error("identical passwords produced identical hashes; salt not applied")
	}

	// Both still authenticate with the shared password.
	if _, ok := s.Authenticate("alice", "same"); !ok {
		t.Error("alice failed to authenticate")
	}
	if _, ok := s.Authenticate("bob", "same"); !ok {
		t.Error("bob failed to authenticate")
	}
}

func TestAddUserErrors(t *testing.T) {
	s := NewStore()
	if err := s.AddUser("alice", "pw", Operator); err != nil {
		t.Fatalf("first AddUser: %v", err)
	}
	if err := s.AddUser("alice", "other", Admin); err == nil {
		t.Error("duplicate AddUser: got nil error, want error")
	}
	if err := s.AddUser("", "pw", Operator); err == nil {
		t.Error("empty name AddUser: got nil error, want error")
	}
}

func TestSetPassword(t *testing.T) {
	s := NewStore()
	if err := s.AddUser("alice", "old", Operator); err != nil {
		t.Fatalf("AddUser: %v", err)
	}
	if err := s.SetPassword("alice", "new"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if _, ok := s.Authenticate("alice", "old"); ok {
		t.Error("old password still works after SetPassword")
	}
	if _, ok := s.Authenticate("alice", "new"); !ok {
		t.Error("new password does not work after SetPassword")
	}
	if err := s.SetPassword("ghost", "x"); err == nil {
		t.Error("SetPassword on unknown user: got nil error, want error")
	}
}

func TestCanRoleHierarchy(t *testing.T) {
	operator := &User{Name: "op", Role: Operator}
	engineer := &User{Name: "eng", Role: Engineer}
	admin := &User{Name: "adm", Role: Admin}

	// Operator may write tags but not manage users.
	if !Can(operator, ActionWriteTag) {
		t.Error("Operator should be able to WriteTag")
	}
	if Can(operator, ActionManageUsers) {
		t.Error("Operator should not be able to ManageUsers")
	}
	if Can(operator, ActionEditConfig) {
		t.Error("Operator should not be able to EditConfig")
	}

	// Engineer may edit config but not manage users.
	if !Can(engineer, ActionEditConfig) {
		t.Error("Engineer should be able to EditConfig")
	}
	if Can(engineer, ActionManageUsers) {
		t.Error("Engineer should not be able to ManageUsers")
	}

	// Admin may do everything.
	for _, a := range []Action{ActionViewScreen, ActionWriteTag, ActionEditConfig, ActionManageUsers} {
		if !Can(admin, a) {
			t.Errorf("Admin should be able to %s", a)
		}
	}

	// Nil user is always denied.
	if Can(nil, ActionViewScreen) {
		t.Error("nil user should be denied")
	}
	// Unknown action is denied even for Admin.
	if Can(admin, Action("bogus")) {
		t.Error("unknown action should be denied")
	}
}

func TestRoleString(t *testing.T) {
	cases := map[Role]string{
		Operator: "Operator",
		Engineer: "Engineer",
		Admin:    "Admin",
		Role(99): "Unknown",
	}
	for r, want := range cases {
		if got := r.String(); got != want {
			t.Errorf("Role(%d).String() = %q, want %q", int(r), got, want)
		}
	}
}
