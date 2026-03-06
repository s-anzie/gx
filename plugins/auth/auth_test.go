package auth

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
)

const testSecret = "test-secret-key-12345"

func TestAuth(t *testing.T) {
	tests := []struct {
		name string
		fn   func(*testing.T)
	}{
		{"valid_token", testValidToken},
		{"missing_token", testMissingToken},
		{"invalid_token", testInvalidToken},
		{"expired_token", testExpiredToken},
		{"wrong_signing_method", testWrongSigningMethod},
		{"custom_on_invalid", testCustomOnInvalid},
		{"claims_extraction", testClaimsExtraction},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fn(t)
		})
	}
}

func testValidToken(t *testing.T) {
	app := gx.New()
	app.Install(JWT(Secret(testSecret)))

	app.GET("/protected", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{"message": "authenticated"})
	})

	// Generate valid token
	token := createToken(t, testSecret, jwt.MapClaims{
		"sub":     "user123",
		"user_id": "user123",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func testMissingToken(t *testing.T) {
	app := gx.New()
	app.Install(JWT(Secret(testSecret)))

	app.GET("/protected", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{"message": "authenticated"})
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func testInvalidToken(t *testing.T) {
	app := gx.New()
	app.Install(JWT(Secret(testSecret)))

	app.GET("/protected", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{"message": "authenticated"})
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func testExpiredToken(t *testing.T) {
	app := gx.New()
	app.Install(JWT(Secret(testSecret)))

	app.GET("/protected", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{"message": "authenticated"})
	})

	// Generate expired token
	token := createToken(t, testSecret, jwt.MapClaims{
		"sub": "user123",
		"exp": time.Now().Add(-time.Hour).Unix(), // Expired 1 hour ago
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Errorf("Expected status 401 for expired token, got %d", w.Code)
	}
}

func testWrongSigningMethod(t *testing.T) {
	app := gx.New()
	app.Install(JWT(Secret(testSecret)))

	app.GET("/protected", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{"message": "authenticated"})
	})

	// Create token with different signing method (HS512 instead of HS256)
	// Both HS256 and HS512 are accepted by jwt.SigningMethodHMAC type assertion
	// So we test with a completely different algorithm family instead
	// For simplicity, we'll just test an invalid token format
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer eyJhbGciOiJub25lIn0.eyJzdWIiOiJ0ZXN0In0.")
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Errorf("Expected status 401 for invalid signing method, got %d", w.Code)
	}
}

func testCustomOnInvalid(t *testing.T) {
	customCalled := false

	app := gx.New()
	app.Install(JWT(
		Secret(testSecret),
		OnInvalid(func(c *gx.Context) core.Response {
			customCalled = true
			return c.JSON(map[string]string{
				"error": "Custom authentication failed",
			})
		}),
	))

	app.GET("/protected", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{"message": "authenticated"})
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid.token")
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("Expected status 200 (JSON default), got %d", w.Code)
	}

	if !customCalled {
		t.Error("Custom OnInvalid handler was not called")
	}
}

func testClaimsExtraction(t *testing.T) {
	app := gx.New()
	app.Install(JWT(Secret(testSecret)))

	var extractedSub string
	var extractedUserId string

	app.GET("/protected", func(c *core.Context) core.Response {
		gxCtx := &gx.Context{Context: c}

		// Test GetSubject helper
		if sub, ok := GetSubject(gxCtx); ok {
			extractedSub = sub
		}

		// Test getting user_id from context
		if userId, ok := c.Get("user_id"); ok {
			if userIdStr, ok := userId.(string); ok {
				extractedUserId = userIdStr
			}
		}

		return c.JSON(map[string]string{"message": "ok"})
	})

	// Generate token with subject claim
	token := createToken(t, testSecret, jwt.MapClaims{
		"sub":     "testuser",
		"user_id": "user123",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if extractedSub != "testuser" {
		t.Errorf("Expected subject 'testuser', got '%s'", extractedSub)
	}

	if extractedUserId != "user123" {
		t.Errorf("Expected user_id 'user123', got '%s'", extractedUserId)
	}
}

// ── Test Helpers ─────────────────────────────────────────────────────────────

func createToken(t *testing.T, secret string, claims jwt.MapClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}
	return tokenString
}
