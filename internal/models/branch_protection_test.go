package models

import (
	"encoding/json"
	"testing"
)

func TestBranchProtection_JSON(t *testing.T) {
	jsonStr := `{"branch_pattern":"main","required_reviews":2,"require_linear":true}`

	var req CreateBranchProtectionRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.BranchPattern != "main" {
		t.Errorf("BranchPattern = %q", req.BranchPattern)
	}
	if req.RequiredReviews != 2 {
		t.Errorf("RequiredReviews = %d", req.RequiredReviews)
	}
	if !req.RequireLinear {
		t.Error("RequireLinear should be true")
	}
}

func TestUpdateBranchProtectionRequest_Partial(t *testing.T) {
	jsonStr := `{"required_reviews":3}`
	var req UpdateBranchProtectionRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.RequiredReviews == nil || *req.RequiredReviews != 3 {
		t.Errorf("RequiredReviews = %v", req.RequiredReviews)
	}
	if req.RequireLinear != nil {
		t.Error("RequireLinear should be nil")
	}
}
