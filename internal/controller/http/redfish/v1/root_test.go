package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/device-management-toolkit/console/config"
	"github.com/device-management-toolkit/console/pkg/logger"
)

func TestGenerateServiceUUID(t *testing.T) {
	t.Run("generates valid UUID format", func(t *testing.T) {
		uuid := generateServiceUUID()

		// Check that UUID is not empty
		assert.NotEmpty(t, uuid)

		// Check UUID format (8-4-4-4-12 characters)
		parts := strings.Split(uuid, "-")
		assert.Len(t, parts, 5, "UUID should have 5 parts separated by dashes")
		assert.Len(t, parts[0], 8, "First part should be 8 characters")
		assert.Len(t, parts[1], 4, "Second part should be 4 characters")
		assert.Len(t, parts[2], 4, "Third part should be 4 characters")
		assert.Len(t, parts[3], 4, "Fourth part should be 4 characters")
		assert.Len(t, parts[4], 12, "Fifth part should be 12 characters")

		// Check that version bit is set correctly (4xxx for version 4)
		assert.True(t, strings.HasPrefix(parts[2], "4"), "UUID should be version 4")

		// Generate multiple UUIDs to ensure they're different
		uuid2 := generateServiceUUID()
		assert.NotEqual(t, uuid, uuid2, "Generated UUIDs should be different")
	})
}

// TestHealthCheckFunctions tests the health check functions used for 502/503 error conditions
func TestHealthCheckFunctions(t *testing.T) {
	t.Run("isDatabaseReachable", func(t *testing.T) {
		// Test the database reachability check
		result := isDatabaseReachable()
		// Currently always returns true since we use embedded DB
		assert.True(t, result, "isDatabaseReachable should return true for embedded database")
	})

	t.Run("areSystemsReachable", func(t *testing.T) {
		// Test the systems reachability check
		result := areSystemsReachable()
		// Currently always returns true - no external systems required for service root
		assert.True(t, result, "areSystemsReachable should return true when no external systems are required")
	})

	t.Run("isExternalServiceHealthy", func(t *testing.T) {
		// Test external service health check
		result := isExternalServiceHealthy()
		// Currently always returns true - service root is self-contained
		assert.True(t, result, "isExternalServiceHealthy should return true for self-contained service")
	})

	t.Run("isServiceOverloaded", func(t *testing.T) {
		// Test service overload check
		result := isServiceOverloaded()
		// Currently always returns false - no overload detection implemented
		assert.False(t, result, "isServiceOverloaded should return false when no overload detection is implemented")
	})

	t.Run("isMaintenanceMode", func(t *testing.T) {
		// Test maintenance mode check
		result := isMaintenanceMode()
		// Currently always returns false - no maintenance mode implemented
		assert.False(t, result, "isMaintenanceMode should return false when no maintenance mode is implemented")
	})

	t.Run("hasResourceExhaustion", func(t *testing.T) {
		// Test resource exhaustion check
		result := hasResourceExhaustion()
		// Currently always returns false - no resource monitoring implemented
		assert.False(t, result, "hasResourceExhaustion should return false when no resource monitoring is implemented")
	})
}

// Test helper to create a test router with the service root routes
func createTestRouter(cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Create a logger for testing
	l := logger.New("test")

	// Create a router group for v1 API
	v1Group := router.Group("/redfish/v1")

	// Register the service root routes
	NewServiceRootRoutes(v1Group, cfg, l)

	return router
}

// Test helper to create a test config
func createTestConfig(authDisabled bool) *config.Config {
	return &config.Config{
		Auth: config.Auth{
			Disabled: authDisabled,
			JWTKey:   "test-secret-key-for-testing-purposes-only",
		},
	}
}

// TestServiceRootEndpoint tests the main service root endpoint
func TestServiceRootEndpoint(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		method         string
		acceptHeader   string
		authDisabled   bool
		expectedStatus int
		checkResponse  func(t *testing.T, body string, headers http.Header)
	}{
		{
			name:           "successful service root GET",
			path:           "/redfish/v1/",
			method:         "GET",
			acceptHeader:   "application/json",
			authDisabled:   true,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body string, headers http.Header) {
				// Check Redfish headers
				assert.Equal(t, "4.0", headers.Get("OData-Version"))
				assert.Equal(t, "application/json; charset=utf-8", headers.Get("Content-Type"))
				assert.Equal(t, "no-cache", headers.Get("Cache-Control"))

				// Check JSON structure (DMTF v1.11.0 compliant)
				assert.Contains(t, body, `"@odata.type":"#ServiceRoot.v1_11_0.ServiceRoot"`)
				assert.Contains(t, body, `"@odata.id":"/redfish/v1/"`)
				assert.Contains(t, body, `"Id":"RootService"`)
				assert.Contains(t, body, `"Name":"Redfish Root Service"`)
				assert.Contains(t, body, `"RedfishVersion":"1.11.0"`)
				assert.Contains(t, body, `"Systems":{"@odata.id":"/redfish/v1/Systems"}`)
				assert.Contains(t, body, `"SessionService":{"@odata.id":"/redfish/v1/SessionService"}`)
				assert.Contains(t, body, `"Links":{"Sessions":{"@odata.id":"/redfish/v1/SessionService/Sessions"}}`)
				assert.Contains(t, body, `"Product":"Device Management Toolkit Console"`)
				assert.Contains(t, body, `"Vendor":"Intel Corporation"`)
				assert.Contains(t, body, `"UUID":`)
			},
		},
		{
			name:           "service root with wildcard accept",
			path:           "/redfish/v1/",
			method:         "GET",
			acceptHeader:   "*/*",
			authDisabled:   true,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body string, headers http.Header) {
				assert.Contains(t, body, `"@odata.type":"#ServiceRoot.v1_11_0.ServiceRoot"`)
			},
		},
		{
			name:           "service root without accept header",
			path:           "/redfish/v1/",
			method:         "GET",
			acceptHeader:   "",
			authDisabled:   true,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body string, headers http.Header) {
				assert.Contains(t, body, `"@odata.type":"#ServiceRoot.v1_11_0.ServiceRoot"`)
			},
		},
		{
			name:           "service root with unsupported accept header",
			path:           "/redfish/v1/",
			method:         "GET",
			acceptHeader:   "text/xml",
			authDisabled:   true,
			expectedStatus: http.StatusNotAcceptable,
			checkResponse: func(t *testing.T, body string, headers http.Header) {
				assert.Contains(t, body, `"Base.1.11.0.NotAcceptable"`)
				assert.Contains(t, body, "text/xml")
			},
		},
		{
			name:           "POST method not allowed",
			path:           "/redfish/v1/",
			method:         "POST",
			acceptHeader:   "application/json",
			authDisabled:   true,
			expectedStatus: http.StatusMethodNotAllowed,
			checkResponse: func(t *testing.T, body string, headers http.Header) {
				assert.Equal(t, "GET", headers.Get("Allow"))
				assert.Contains(t, body, `"Base.1.11.0.OperationNotAllowed"`)
				assert.Contains(t, body, "POST")
				assert.Contains(t, body, "ServiceRoot")
			},
		},
		{
			name:           "PUT method not allowed",
			path:           "/redfish/v1/",
			method:         "PUT",
			acceptHeader:   "application/json",
			authDisabled:   true,
			expectedStatus: http.StatusMethodNotAllowed,
			checkResponse: func(t *testing.T, body string, headers http.Header) {
				assert.Equal(t, "GET", headers.Get("Allow"))
				assert.Contains(t, body, "PUT")
				assert.Contains(t, body, "ServiceRoot")
			},
		},
		{
			name:           "PATCH method not allowed",
			path:           "/redfish/v1/",
			method:         "PATCH",
			acceptHeader:   "application/json",
			authDisabled:   true,
			expectedStatus: http.StatusMethodNotAllowed,
			checkResponse: func(t *testing.T, body string, headers http.Header) {
				assert.Contains(t, body, "PATCH")
			},
		},
		{
			name:           "DELETE method not allowed",
			path:           "/redfish/v1/",
			method:         "DELETE",
			acceptHeader:   "application/json",
			authDisabled:   true,
			expectedStatus: http.StatusMethodNotAllowed,
			checkResponse: func(t *testing.T, body string, headers http.Header) {
				assert.Contains(t, body, "DELETE")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := createTestRouter(createTestConfig(tt.authDisabled))

			req, _ := http.NewRequest(tt.method, tt.path, nil)
			if tt.acceptHeader != "" {
				req.Header.Set("Accept", tt.acceptHeader)
			}
			if tt.method != "GET" {
				req.Header.Set("Content-Type", "application/json")
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.checkResponse != nil {
				tt.checkResponse(t, w.Body.String(), w.Header())
			}
		})
	}
}

// TestSessionServiceEndpoints tests SessionService related endpoints
func TestSessionServiceEndpoints(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		method         string
		expectedStatus int
		checkResponse  func(t *testing.T, body string, headers http.Header)
	}{
		{
			name:           "SessionService GET success",
			path:           "/redfish/v1/SessionService",
			method:         "GET",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body string, headers http.Header) {
				assert.Contains(t, body, `"@odata.type":"#SessionService.v1_0_0.SessionService"`)
				assert.Contains(t, body, `"@odata.id":"/redfish/v1/SessionService"`)
				assert.Contains(t, body, `"Id":"SessionService"`)
				assert.Contains(t, body, `"Name":"Redfish Session Service"`)
				assert.Contains(t, body, `"ServiceEnabled":true`)
				assert.Contains(t, body, `"SessionTimeout":30`)
				assert.Contains(t, body, `"Sessions":{"@odata.id":"/redfish/v1/SessionService/Sessions"}`)
			},
		},
		{
			name:           "Sessions collection GET success",
			path:           "/redfish/v1/SessionService/Sessions",
			method:         "GET",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body string, headers http.Header) {
				assert.Contains(t, body, `"@odata.type":"#SessionCollection.SessionCollection"`)
				assert.Contains(t, body, `"@odata.id":"/redfish/v1/SessionService/Sessions"`)
				assert.Contains(t, body, `"Name":"Session Collection"`)
				assert.Contains(t, body, `"Members@odata.count":0`)
				assert.Contains(t, body, `"Members":[]`)
			},
		},
		{
			name:           "SessionService POST not allowed",
			path:           "/redfish/v1/SessionService",
			method:         "POST",
			expectedStatus: http.StatusMethodNotAllowed,
			checkResponse: func(t *testing.T, body string, headers http.Header) {
				assert.Contains(t, body, `"Base.1.11.0.OperationNotAllowed"`)
				assert.Contains(t, body, "POST")
				assert.Contains(t, body, "SessionService")
			},
		},
		{
			name:           "SessionService PUT not allowed",
			path:           "/redfish/v1/SessionService",
			method:         "PUT",
			expectedStatus: http.StatusMethodNotAllowed,
			checkResponse: func(t *testing.T, body string, headers http.Header) {
				assert.Contains(t, body, "PUT")
			},
		},
		{
			name:           "Sessions collection PUT not allowed",
			path:           "/redfish/v1/SessionService/Sessions",
			method:         "PUT",
			expectedStatus: http.StatusMethodNotAllowed,
			checkResponse: func(t *testing.T, body string, headers http.Header) {
				assert.Equal(t, "GET, POST", headers.Get("Allow"))
				assert.Contains(t, body, `"Base.1.11.0.ActionNotSupported"`)
				assert.Contains(t, body, "retrieve sessions collection")
				assert.Contains(t, body, "not supported")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := createTestRouter(createTestConfig(true)) // auth disabled

			req, _ := http.NewRequest(tt.method, tt.path, nil)
			if tt.method != "GET" {
				req.Header.Set("Content-Type", "application/json")
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.checkResponse != nil {
				tt.checkResponse(t, w.Body.String(), w.Header())
			}
		})
	}
}

// TestSystemsCollectionMethods tests Systems collection method restrictions
func TestSystemsCollectionMethods(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		expectedStatus int
		checkResponse  func(t *testing.T, body string)
	}{
		{
			name:           "Systems POST not allowed",
			method:         "POST",
			expectedStatus: http.StatusMethodNotAllowed,
			checkResponse: func(t *testing.T, body string) {
				assert.Contains(t, body, "POST")
				assert.Contains(t, body, "ComputerSystemCollection")
			},
		},
		{
			name:           "Systems PUT not allowed",
			method:         "PUT",
			expectedStatus: http.StatusMethodNotAllowed,
			checkResponse: func(t *testing.T, body string) {
				assert.Contains(t, body, "PUT")
				assert.Contains(t, body, "ComputerSystemCollection")
			},
		},
		{
			name:           "Systems PATCH not allowed",
			method:         "PATCH",
			expectedStatus: http.StatusMethodNotAllowed,
			checkResponse: func(t *testing.T, body string) {
				assert.Contains(t, body, "PATCH")
				assert.Contains(t, body, "ComputerSystemCollection")
			},
		},
		{
			name:           "Systems DELETE not allowed",
			method:         "DELETE",
			expectedStatus: http.StatusMethodNotAllowed,
			checkResponse: func(t *testing.T, body string) {
				assert.Contains(t, body, "DELETE")
				assert.Contains(t, body, "ComputerSystemCollection")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := createTestRouter(createTestConfig(true))

			req, _ := http.NewRequest(tt.method, "/redfish/v1/Systems", nil)
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.checkResponse != nil {
				tt.checkResponse(t, w.Body.String())
			}
		})
	}
}

// TestMetadataEndpoint tests the OData $metadata endpoint
func TestMetadataEndpoint(t *testing.T) {
	t.Run("OData metadata XML response", func(t *testing.T) {
		router := createTestRouter(createTestConfig(true))

		req, _ := http.NewRequest("GET", "/redfish/v1/$metadata", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/xml", w.Header().Get("Content-Type"))

		body := w.Body.String()
		assert.Contains(t, body, `<?xml version="1.0" encoding="UTF-8"?>`)
		assert.Contains(t, body, `<edmx:Edmx`)
		assert.Contains(t, body, `xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx"`)
		assert.Contains(t, body, `Version="4.0"`)
		assert.Contains(t, body, `<EntityType Name="ServiceRoot">`)
		assert.Contains(t, body, `<EntityType Name="SessionService">`)
		assert.Contains(t, body, `<EntityType Name="ComputerSystem">`)
		assert.Contains(t, body, `<EntityContainer Name="Service">`)
	})
}

// TestNewServiceRootRoutes tests the route registration function
func TestNewServiceRootRoutes(t *testing.T) {
	tests := []struct {
		name         string
		authDisabled bool
	}{
		{
			name:         "routes with auth disabled",
			authDisabled: true,
		},
		{
			name:         "routes with auth enabled",
			authDisabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			l := logger.New("test")
			cfg := createTestConfig(tt.authDisabled)

			// Create router group
			v1Group := router.Group("/redfish/v1")

			// This should not panic and should register routes successfully
			assert.NotPanics(t, func() {
				NewServiceRootRoutes(v1Group, cfg, l)
			})

			// Test that routes are actually registered by making a simple request
			req, _ := http.NewRequest("GET", "/redfish/v1/", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Should get either 200 (auth disabled) or 401 (auth enabled)
			if tt.authDisabled {
				assert.Equal(t, http.StatusOK, w.Code)
			} else {
				// With auth enabled, should get 401 without proper token
				assert.Equal(t, http.StatusUnauthorized, w.Code)
			}
		})
	}
}
