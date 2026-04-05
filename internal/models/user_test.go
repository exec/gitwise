package models

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestUser_PasswordNotSerialized(t *testing.T) {
	u := User{
		ID:       uuid.New(),
		Username: "alice",
		Email:    "alice@example.com",
		Password: "supersecret",
		FullName: "Alice Smith",
	}

	data, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	s := string(data)
	if contains(s, "supersecret") {
		t.Error("password should not appear in JSON output")
	}
	if contains(s, "password") {
		t.Error("password field should not appear in JSON output")
	}

	// Verify other fields are present
	var decoded map[string]any
	json.Unmarshal(data, &decoded)
	if decoded["username"] != "alice" {
		t.Errorf("username = %v", decoded["username"])
	}
	if decoded["email"] != "alice@example.com" {
		t.Errorf("email = %v", decoded["email"])
	}
}

func TestCreateUserRequest_JSON(t *testing.T) {
	jsonStr := `{"username":"bob","email":"bob@example.com","password":"pass1234","full_name":"Bob Jones"}`
	var req CreateUserRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.Username != "bob" {
		t.Errorf("Username = %q", req.Username)
	}
	if req.Password != "pass1234" {
		t.Errorf("Password = %q", req.Password)
	}
	if req.FullName != "Bob Jones" {
		t.Errorf("FullName = %q", req.FullName)
	}
}

func TestLoginRequest_JSON(t *testing.T) {
	jsonStr := `{"login":"alice","password":"secret"}`
	var req LoginRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.Login != "alice" {
		t.Errorf("Login = %q", req.Login)
	}
}

func TestUpdateUserRequest_JSON_Partial(t *testing.T) {
	jsonStr := `{"full_name":"New Name"}`
	var req UpdateUserRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.FullName == nil {
		t.Fatal("FullName should not be nil")
	}
	if *req.FullName != "New Name" {
		t.Errorf("FullName = %q", *req.FullName)
	}
	if req.Bio != nil {
		t.Error("Bio should be nil for partial update")
	}
	if req.AvatarURL != nil {
		t.Error("AvatarURL should be nil for partial update")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
