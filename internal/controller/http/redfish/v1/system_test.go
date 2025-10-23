/*********************************************************************
 * Copyright (c) Intel Corporation 2025
 * SPDX-License-Identifier: Apache-2.0
 **********************************************************************/

// Package v1 implements Redfish API v1 System resources tests.
package v1

import (
  "bytes"
	"context"
	"encoding/json"
	"fmt"
  "errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/device-management-toolkit/go-wsman-messages/v2/pkg/wsman/cim/power"

	"github.com/device-management-toolkit/console/config"
	dto "github.com/device-management-toolkit/console/internal/entity/dto/v1"
	"github.com/device-management-toolkit/console/internal/mocks"
)

// Test constants
const (
	malformedJSONTestName = "malformed JSON"
)

func TestGetSystemsCollectionHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		mockDevices    []dto.Device
		mockError      error
		expectedStatus int
		expectedCount  int
		checkResponse  func(t *testing.T, body string)
	}{
		{
			name: "successful collection with devices",
			mockDevices: []dto.Device{
				{GUID: "device-1", FriendlyName: "Test Device 1"},
				{GUID: "device-2", FriendlyName: "Test Device 2"},
				{GUID: "", FriendlyName: "Device without GUID"}, // should be skipped
			},
			mockError:      nil,
			expectedStatus: http.StatusOK,
			expectedCount:  2,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"@odata.type":"#ComputerSystemCollection.ComputerSystemCollection"`)
				assert.Contains(t, body, `"@odata.id":"/redfish/v1/Systems"`)
				assert.Contains(t, body, `"Members@odata.count":2`)
				assert.Contains(t, body, `"/redfish/v1/Systems/device-1"`)
				assert.Contains(t, body, `"/redfish/v1/Systems/device-2"`)
				assert.NotContains(t, body, `"device-3"`) // device without GUID should not appear
			},
		},
		{
			name:           "empty collection",
			mockDevices:    []dto.Device{},
			mockError:      nil,
			expectedStatus: http.StatusOK,
			expectedCount:  0,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Members@odata.count":0`)
				assert.Contains(t, body, `"Members":[]`)
			},
		},
		{
			name:           "database error",
			mockDevices:    nil,
			mockError:      errors.New("database connection failed"),
			expectedStatus: http.StatusInternalServerError,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Base.1.11.0.GeneralError"`)
			},
		},
		{
			name:           "upstream communication error",
			mockDevices:    nil,
			mockError:      errors.New("WSMAN connection timeout"),
			expectedStatus: http.StatusBadGateway,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Base.1.11.0.GeneralError"`)
				assert.Contains(t, body, "upstream service or managed device is unavailable")
			},
		},
		{
			name:           "service temporarily unavailable error",
			mockDevices:    nil,
			mockError:      errors.New("too many connections to database"),
			expectedStatus: http.StatusServiceUnavailable,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Base.1.11.0.GeneralError"`)
				assert.Contains(t, body, "service is temporarily unavailable")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockDevice := mocks.NewMockDeviceManagementFeature(ctrl)
			mockLogger := mocks.NewMockLogger(ctrl)

			// Setup expectations
			mockDevice.EXPECT().
				Get(gomock.Any(), maxSystemsList, 0, "").
				Return(tt.mockDevices, tt.mockError).
				Times(1)

			if tt.mockError != nil {
				mockLogger.EXPECT().
					Error(tt.mockError, "http - redfish v1 - Systems collection").
					Times(1)
			}

			// Setup Gin test context
			gin.SetMode(gin.TestMode)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			req, _ := http.NewRequestWithContext(context.Background(), "GET", "/redfish/v1/Systems", http.NoBody)
			c.Request = req

			// Call the handler
			handler := getSystemsCollectionHandler(mockDevice, mockLogger)
			handler(c)

			// Assertions
			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.checkResponse != nil {
				tt.checkResponse(t, w.Body.String())
			}
		})
	}
}

func TestGetSystemInstanceHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		deviceID       string
		mockPowerState *dto.PowerState
		mockError      error
		expectedStatus int
		expectedPower  string
		checkResponse  func(t *testing.T, body string)
	}{
		{
			name:     "device on - CIM PowerState 2",
			deviceID: "test-device-1",
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerOn, // 2
			},
			mockError:      nil,
			expectedStatus: http.StatusOK,
			expectedPower:  powerStateOn,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"@odata.type":"#ComputerSystem.v1_0_0.ComputerSystem"`)
				assert.Contains(t, body, `"@odata.id":"/redfish/v1/Systems/test-device-1"`)
				assert.Contains(t, body, `"Id":"test-device-1"`)
				assert.Contains(t, body, `"PowerState":"On"`)
				assert.Contains(t, body, `"#ComputerSystem.Reset"`)
				assert.Contains(t, body, `"target":"/redfish/v1/Systems/test-device-1/Actions/ComputerSystem.Reset"`)
			},
		},
		{
			name:     "device off - CIM PowerState 8",
			deviceID: "test-device-2",
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerHardOff, // 8
			},
			mockError:      nil,
			expectedStatus: http.StatusOK,
			expectedPower:  powerStateOff,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"PowerState":"Off"`)
			},
		},
		{
			name:     "device sleep state - treated as on",
			deviceID: "test-device-3",
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerSleep, // 3
			},
			mockError:      nil,
			expectedStatus: http.StatusOK,
			expectedPower:  powerStateOn,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"PowerState":"On"`)
			},
		},
		{
			name:           "power state error - unknown state",
			deviceID:       "test-device-4",
			mockPowerState: nil,
			mockError:      errors.New("power state unavailable"),
			expectedStatus: http.StatusOK,
			expectedPower:  powerStateUnknown,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"PowerState":"Unknown"`)
			},
		},
		{
			name:     "unknown power state value",
			deviceID: "test-device-5",
			mockPowerState: &dto.PowerState{
				PowerState: 99, // unknown value
			},
			mockError:      nil,
			expectedStatus: http.StatusOK,
			expectedPower:  powerStateUnknown,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"PowerState":"Unknown"`)
	dto "github.com/device-management-toolkit/console/internal/entity/dto/v1"
	dtov2 "github.com/device-management-toolkit/console/internal/entity/dto/v2"
	"github.com/device-management-toolkit/console/internal/mocks"
)

const (
	testSystemGUID     = "test-system-guid-123"
	testInvalidGUID    = "invalid-system-guid"
	systemsBasePath    = "/redfish/v1/Systems"
	systemsInstanceURL = systemsBasePath + "/" + testSystemGUID
	resetActionURL     = systemsInstanceURL + "/Actions/ComputerSystem.Reset"
)

func TestNewSystemsRoutes(t *testing.T) {
	t.Parallel()

	t.Run("routes registration", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockFeature := mocks.NewMockDeviceManagementFeature(ctrl)
		mockLogger := mocks.NewMockLogger(ctrl)

		// Expect logging calls for route registration
		mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).Times(2) // Systems + Firmware routes

		gin.SetMode(gin.TestMode)
		router := gin.New()

		// Test route registration
		redfishGroup := router.Group("/redfish/v1")
		NewSystemsRoutes(redfishGroup, mockFeature, mockLogger)

		// Verify routes exist by testing them
		routes := router.Routes()

		// Check that expected routes are registered
		expectedRoutes := []string{
			"GET /redfish/v1/Systems",
			"GET /redfish/v1/Systems/:id",
			"POST /redfish/v1/Systems/:id/Actions/ComputerSystem.Reset",
			"GET /redfish/v1/Systems/:id/FirmwareInventory",
			"GET /redfish/v1/Systems/:id/FirmwareInventory/:firmwareId",
		}

		routeMap := make(map[string]bool)

		for _, route := range routes {
			key := fmt.Sprintf("%s %s", route.Method, route.Path)
			routeMap[key] = true
		}

		for _, expectedRoute := range expectedRoutes {
			assert.True(t, routeMap[expectedRoute], "Route %s should be registered", expectedRoute)
		}
	})

	t.Run("with nil dependencies", func(t *testing.T) {
		t.Parallel()

		gin.SetMode(gin.TestMode)
		router := gin.New()
		redfishGroup := router.Group("/redfish/v1")

		// This will panic due to firmware routes accessing nil logger
		// Testing that routes can be set up, but will fail on actual usage
		require.Panics(t, func() {
			NewSystemsRoutes(redfishGroup, nil, nil)
		})
	})
}

func TestGetSystemsCollectionHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		setupMocks       func(*mocks.MockDeviceManagementFeature, *mocks.MockLogger)
		expectedStatus   int
		validateResponse func(t *testing.T, body string)
	}{
		{
			name: "successful collection retrieval",
			setupMocks: func(mockFeature *mocks.MockDeviceManagementFeature, mockLogger *mocks.MockLogger) {
				devices := []dto.Device{
					{
						GUID:            "system-1",
						Hostname:        "host1",
						Tags:            []string{"tag1"},
						DNSSuffix:       "example.com",
						Username:        "admin",
						Password:        "password",
						UseTLS:          true,
						AllowSelfSigned: false,
					},
					{
						GUID:            "system-2",
						Hostname:        "host2",
						Tags:            []string{"tag2"},
						DNSSuffix:       "example.com",
						Username:        "admin",
						Password:        "password",
						UseTLS:          true,
						AllowSelfSigned: false,
					},
					{
						GUID:            "", // Should be filtered out
						Hostname:        "host3",
						Tags:            []string{},
						DNSSuffix:       "example.com",
						Username:        "admin",
						Password:        "password",
						UseTLS:          false,
						AllowSelfSigned: true,
					},
				}

				mockFeature.EXPECT().
					Get(gomock.Any(), maxSystemsList, 0, "").
					Return(devices, nil)

				mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body string) {
				t.Helper()

				var collection map[string]interface{}

				err := json.Unmarshal([]byte(body), &collection)
				require.NoError(t, err)

				assert.Equal(t, "#ComputerSystemCollection.ComputerSystemCollection", collection["@odata.type"])
				assert.Equal(t, "/redfish/v1/Systems", collection["@odata.id"])
				assert.Equal(t, "Computer System Collection", collection["Name"])

				members, ok := collection["Members"].([]interface{})
				require.True(t, ok, "Members should be a slice of interfaces")
				assert.Equal(t, 2, len(members)) // Only 2 systems with valid GUIDs

				membersCount, ok := collection["Members@odata.count"].(float64)
				require.True(t, ok, "Members@odata.count should be a float64")
				assert.Equal(t, 2.0, membersCount)

				// Check first member
				member1, ok := members[0].(map[string]interface{})
				require.True(t, ok, "First member should be a map")
				assert.Equal(t, "/redfish/v1/Systems/system-1", member1["@odata.id"])

				// Check second member
				member2, ok := members[1].(map[string]interface{})
				require.True(t, ok, "Second member should be a map")
				assert.Equal(t, "/redfish/v1/Systems/system-2", member2["@odata.id"])
			},
		},
		{
			name: "empty collection",
			setupMocks: func(mockFeature *mocks.MockDeviceManagementFeature, mockLogger *mocks.MockLogger) {
				mockFeature.EXPECT().
					Get(gomock.Any(), maxSystemsList, 0, "").
					Return([]dto.Device{}, nil)

				mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body string) {
				t.Helper()

				var collection map[string]interface{}

				err := json.Unmarshal([]byte(body), &collection)
				require.NoError(t, err)

				members, ok := collection["Members"].([]interface{})
				require.True(t, ok, "Members should be a slice of interfaces")
				assert.Equal(t, 0, len(members))

				membersCount, ok := collection["Members@odata.count"].(float64)
				require.True(t, ok, "Members@odata.count should be a float64")
				assert.Equal(t, 0.0, membersCount)
			},
		},
		{
			name: "backend error",
			setupMocks: func(mockFeature *mocks.MockDeviceManagementFeature, mockLogger *mocks.MockLogger) {
				mockFeature.EXPECT().
					Get(gomock.Any(), maxSystemsList, 0, "").
					Return(nil, fmt.Errorf("backend connection failed"))

				mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).Times(1)
			},
			expectedStatus: http.StatusInternalServerError,
			validateResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, "backend connection failed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockDevice := mocks.NewMockDeviceManagementFeature(ctrl)
			mockLogger := mocks.NewMockLogger(ctrl)

			// Setup expectations
			if tt.mockPowerState != nil {
				mockDevice.EXPECT().
					GetPowerState(gomock.Any(), tt.deviceID).
					Return(*tt.mockPowerState, tt.mockError).
					Times(1).
					Do(func(_ context.Context, guid string) {
						// Verify the correct GUID is passed
						assert.Equal(t, tt.deviceID, guid)
					})
			} else {
				mockDevice.EXPECT().
					GetPowerState(gomock.Any(), tt.deviceID).
					Return(dto.PowerState{}, tt.mockError).
					Times(1).
					Do(func(_ context.Context, guid string) {
						// Verify the correct GUID is passed
						assert.Equal(t, tt.deviceID, guid)
					})
			}

			if tt.mockError != nil {
				mockLogger.EXPECT().
					Warn("redfish v1 - Systems instance: failed to get power state for %s: %v", tt.deviceID, tt.mockError).
					Times(1)
			}

			// Setup Gin test context
			gin.SetMode(gin.TestMode)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{
				{Key: "id", Value: tt.deviceID},
			}
			req, _ := http.NewRequestWithContext(context.Background(), "GET", "/redfish/v1/Systems/"+tt.deviceID, http.NoBody)
			c.Request = req

			// Call the handler
			handler := getSystemInstanceHandler(mockDevice, mockLogger)
			handler(c)

			// Assertions
			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.checkResponse != nil {
				tt.checkResponse(t, w.Body.String())
			}
		})
	}
}

func TestMethodNotAllowedHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		action         string
		allowedMethods string
		expectedStatus int
		checkResponse  func(t *testing.T, body string, headers http.Header)
	}{
		{
			name:           "ComputerSystem.Reset action not allowed",
			action:         "ComputerSystem.Reset",
			allowedMethods: "POST",
			expectedStatus: http.StatusMethodNotAllowed,
			checkResponse: func(t *testing.T, body string, headers http.Header) {
				t.Helper()
				// Check Allow header
				assert.Equal(t, "POST", headers.Get("Allow"))

				// Check Redfish error response
				assert.Contains(t, body, `"Base.1.11.0.ActionNotSupported"`)
				assert.Contains(t, body, "ComputerSystem.Reset")
				assert.Contains(t, body, "not supported by the resource")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Setup Gin test context
			gin.SetMode(gin.TestMode)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			req, _ := http.NewRequestWithContext(context.Background(), "GET", "/redfish/v1/Systems/test/Actions/ComputerSystem.Reset", http.NoBody)
			c.Request = req

			// Call the handler
			handler := methodNotAllowedHandler(tt.action, tt.allowedMethods)
			handler(c)

			// Assertions
			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.checkResponse != nil {
				tt.checkResponse(t, w.Body.String(), w.Header())
			}
		})
	}
}

func TestPostSystemResetHandler_SuccessfulOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		deviceID       string
		requestBody    map[string]interface{}
		mockPowerState *dto.PowerState
		mockPowerError error
		mockResetResp  power.PowerActionResponse
		mockResetError error
		expectedStatus int
		checkResponse  func(t *testing.T, body string)
		checkPowerCall bool
		checkResetCall bool
		expectedAction int
	}{
		{
			name:     "successful power on",
			deviceID: "test-device-1",
			requestBody: map[string]interface{}{
				"ResetType": resetTypeOn,
			},
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerHardOff, // currently off
			},
			mockPowerError: nil,
			mockResetResp: power.PowerActionResponse{
				ReturnValue: 0, // success
			},
			mockResetError: nil,
			expectedStatus: http.StatusOK,
			checkPowerCall: true,
			checkResetCall: true,
			expectedAction: actionPowerUp,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"TaskState":"Completed"`)
				assert.Contains(t, body, `"TaskStatus":"OK"`)
				assert.Contains(t, body, `"Name":"System Reset Task"`)
				assert.Contains(t, body, `"Base.1.11.0.Success"`)
			},
		},
		{
			name:     "successful force off",
			deviceID: "test-device-2",
			requestBody: map[string]interface{}{
				"ResetType": resetTypeForceOff,
			},
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerOn, // currently on
			},
			mockPowerError: nil,
			mockResetResp: power.PowerActionResponse{
				ReturnValue: 0,
			},
			mockResetError: nil,
			expectedStatus: http.StatusOK,
			checkPowerCall: true,
			checkResetCall: true,
			expectedAction: actionPowerDown,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"TaskState":"Completed"`)
				assert.Contains(t, body, `"TaskStatus":"OK"`)
			},
		},
		{
			name:     "successful force restart",
			deviceID: "test-device-3",
			requestBody: map[string]interface{}{
				"ResetType": resetTypeForceRestart,
			},
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerOn,
			},
			mockPowerError: nil,
			mockResetResp:  power.PowerActionResponse{ReturnValue: 0},
			mockResetError: nil,
			expectedStatus: http.StatusOK,
			checkPowerCall: true,
			checkResetCall: true,
			expectedAction: actionReset,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"TaskState":"Completed"`)
			},
		},
		{
			name:     "successful power cycle",
			deviceID: "test-device-4",
			requestBody: map[string]interface{}{
				"ResetType": resetTypePowerCycle,
			},
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerOn,
			},
			mockPowerError: nil,
			mockResetResp:  power.PowerActionResponse{ReturnValue: 0},
			mockResetError: nil,
			expectedStatus: http.StatusOK,
			checkPowerCall: true,
			checkResetCall: true,
			expectedAction: actionPowerCycle,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"TaskState":"Completed"`)
			},
		},
	}

	runResetHandlerTests(t, tests)
}

func TestPostSystemResetHandler_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		deviceID       string
		requestBody    map[string]interface{}
		mockPowerState *dto.PowerState
		mockPowerError error
		mockResetResp  power.PowerActionResponse
		mockResetError error
		expectedStatus int
		checkResponse  func(t *testing.T, body string)
		checkPowerCall bool
		checkResetCall bool
		expectedAction int
	}{
		{
			name:           malformedJSONTestName,
			deviceID:       "test-device-5",
			requestBody:    nil, // will send malformed JSON
			expectedStatus: http.StatusBadRequest,
			checkPowerCall: false,
			checkResetCall: false,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Base.1.11.0.MalformedJSON"`)
			},
		},
		{
			name:     "missing ResetType property",
			deviceID: "test-device-6",
			requestBody: map[string]interface{}{
				"SomeOtherProperty": "value",
			},
			expectedStatus: http.StatusBadRequest,
			checkPowerCall: false,
			checkResetCall: false,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Base.1.11.0.PropertyMissing"`)
				assert.Contains(t, body, "ResetType")
			},
		},
		{
			name:     "invalid ResetType value",
			deviceID: "test-device-7",
			requestBody: map[string]interface{}{
				"ResetType": "InvalidResetType",
			},
			expectedStatus: http.StatusBadRequest,
			checkPowerCall: false,
			checkResetCall: false,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Base.1.11.0.PropertyValueNotInList"`)
				assert.Contains(t, body, "InvalidResetType")
			},
		},
	}

	runResetHandlerTests(t, tests)
}

func TestPostSystemResetHandler_ConflictErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		deviceID       string
		requestBody    map[string]interface{}
		mockPowerState *dto.PowerState
		mockPowerError error
		mockResetResp  power.PowerActionResponse
		mockResetError error
		expectedStatus int
		checkResponse  func(t *testing.T, body string)
		checkPowerCall bool
		checkResetCall bool
		expectedAction int
	}{
		{
			name:     "power on when already on - conflict",
			deviceID: "test-device-8",
			requestBody: map[string]interface{}{
				"ResetType": resetTypeOn,
			},
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerOn, // already on
			},
			mockPowerError: nil,
			expectedStatus: http.StatusConflict,
			checkPowerCall: true,
			checkResetCall: false,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Base.1.11.0.OperationNotAllowed"`)
			},
		},
		{
			name:     "power off when already off - conflict",
			deviceID: "test-device-9",
			requestBody: map[string]interface{}{
				"ResetType": resetTypeForceOff,
			},
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerHardOff, // already off
			},
			mockPowerError: nil,
			expectedStatus: http.StatusConflict,
			checkPowerCall: true,
			checkResetCall: false,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Base.1.11.0.OperationNotAllowed"`)
			},
		},
	}

	runResetHandlerTests(t, tests)
}

// runResetHandlerTests is a helper function to run system reset handler tests
func runResetHandlerTests(t *testing.T, tests []struct {
	name           string
	deviceID       string
	requestBody    map[string]interface{}
	mockPowerState *dto.PowerState
	mockPowerError error
	mockResetResp  power.PowerActionResponse
	mockResetError error
	expectedStatus int
	checkResponse  func(t *testing.T, body string)
	checkPowerCall bool
	checkResetCall bool
	expectedAction int
},
) {
	t.Helper()

			t.Cleanup(ctrl.Finish)

			mockFeature := mocks.NewMockDeviceManagementFeature(ctrl)
			mockLogger := mocks.NewMockLogger(ctrl)

			tt.setupMocks(mockFeature, mockLogger)

			gin.SetMode(gin.TestMode)
			router := gin.New()
			systems := router.Group("/redfish/v1/Systems")
			systems.GET("", getSystemsCollectionHandler(mockFeature, mockLogger))

			w := httptest.NewRecorder()
			req, _ := http.NewRequestWithContext(
				context.Background(),
				"GET",
				"/redfish/v1/Systems",
				http.NoBody,
			)

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			tt.validateResponse(t, w.Body.String())
		})
	}
}

func TestGetSystemInstanceHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		systemID         string
		setupMocks       func(*mocks.MockDeviceManagementFeature, *mocks.MockLogger)
		expectedStatus   int
		validateResponse func(t *testing.T, body string)
	}{
		{
			name:     "successful system retrieval with power on",
			systemID: testSystemGUID,
			setupMocks: func(mockFeature *mocks.MockDeviceManagementFeature, mockLogger *mocks.MockLogger) {
				powerState := dto.PowerState{
					PowerState: actionPowerUp, // 2 = On
				}
				mockFeature.EXPECT().
					GetPowerState(gomock.Any(), testSystemGUID).
					Return(powerState, nil)

				mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body string) {
				t.Helper()

				var system map[string]interface{}

				err := json.Unmarshal([]byte(body), &system)
				require.NoError(t, err)

				assert.Equal(t, "#ComputerSystem.v1_0_0.ComputerSystem", system["@odata.type"])
				assert.Equal(t, "/redfish/v1/Systems/"+testSystemGUID, system["@odata.id"])
				assert.Equal(t, testSystemGUID, system["Id"])
				assert.Equal(t, "Computer System "+testSystemGUID, system["Name"])
				assert.Equal(t, powerStateOn, system["PowerState"])

				// Check Actions
				actions, ok := system["Actions"].(map[string]interface{})
				require.True(t, ok, "Actions should be a map")
				resetAction, ok := actions["#ComputerSystem.Reset"].(map[string]interface{})
				require.True(t, ok, "Reset action should be a map")
				assert.Equal(t, "/redfish/v1/Systems/"+testSystemGUID+"/Actions/ComputerSystem.Reset", resetAction["target"])

				allowedValues, ok := resetAction["ResetType@Redfish.AllowableValues"].([]interface{})
				require.True(t, ok, "AllowableValues should be a slice of interfaces")

				expectedValues := []string{resetTypeOn, resetTypeForceOff, resetTypeForceRestart, resetTypePowerCycle}
				assert.Equal(t, len(expectedValues), len(allowedValues))
			},
		},
		{
			name:     "system with power off state",
			systemID: testSystemGUID,
			setupMocks: func(mockFeature *mocks.MockDeviceManagementFeature, mockLogger *mocks.MockLogger) {
				powerState := dto.PowerState{
					PowerState: cimPowerSoftOff, // 7 = Soft Off
				}
				mockFeature.EXPECT().
					GetPowerState(gomock.Any(), testSystemGUID).
					Return(powerState, nil)

				mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body string) {
				t.Helper()

				var system map[string]interface{}

				err := json.Unmarshal([]byte(body), &system)
				require.NoError(t, err)

				assert.Equal(t, powerStateOff, system["PowerState"])
			},
		},
		{
			name:     "system with sleep state (treated as on)",
			systemID: testSystemGUID,
			setupMocks: func(mockFeature *mocks.MockDeviceManagementFeature, mockLogger *mocks.MockLogger) {
				powerState := dto.PowerState{
					PowerState: cimPowerSleep, // 3 = Sleep
				}
				mockFeature.EXPECT().
					GetPowerState(gomock.Any(), testSystemGUID).
					Return(powerState, nil)

				mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body string) {
				t.Helper()

				var system map[string]interface{}

				err := json.Unmarshal([]byte(body), &system)
				require.NoError(t, err)

				assert.Equal(t, powerStateOn, system["PowerState"]) // Sleep treated as On
			},
		},
		{
			name:     "power state retrieval failure",
			systemID: testSystemGUID,
			setupMocks: func(mockFeature *mocks.MockDeviceManagementFeature, mockLogger *mocks.MockLogger) {
				mockFeature.EXPECT().
					GetPowerState(gomock.Any(), testSystemGUID).
					Return(dto.PowerState{}, fmt.Errorf("power state not available"))

				mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body string) {
				t.Helper()

				var system map[string]interface{}

				err := json.Unmarshal([]byte(body), &system)
				require.NoError(t, err)

				assert.Equal(t, powerStateUnknown, system["PowerState"]) // Default to Unknown
			},
		},
		{
			name:     "unknown power state value",
			systemID: testSystemGUID,
			setupMocks: func(mockFeature *mocks.MockDeviceManagementFeature, mockLogger *mocks.MockLogger) {
				powerState := dto.PowerState{
					PowerState: 999, // Unknown value
				}
				mockFeature.EXPECT().
					GetPowerState(gomock.Any(), testSystemGUID).
					Return(powerState, nil)

				mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body string) {
				t.Helper()

				var system map[string]interface{}

				err := json.Unmarshal([]byte(body), &system)
				require.NoError(t, err)

				assert.Equal(t, powerStateUnknown, system["PowerState"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Setup test dependencies
			ctrl, mockDevice, mockLogger := setupResetHandlerTestMocks(t)
			defer ctrl.Finish()

			// Configure mock expectations
			configurePowerStateMock(t, mockDevice, mockLogger, tt)
			configureResetActionMock(mockDevice, tt)
			configureErrorLoggingMock(mockLogger, tt)

			// Execute test
			executeResetHandlerTest(t, mockDevice, mockLogger, tt)
		})
	}
}

// setupResetHandlerTestMocks creates and returns the mock objects for testing
func setupResetHandlerTestMocks(t *testing.T) (*gomock.Controller, *mocks.MockDeviceManagementFeature, *mocks.MockLogger) {
	t.Helper()

	ctrl := gomock.NewController(t)
	mockDevice := mocks.NewMockDeviceManagementFeature(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)

	return ctrl, mockDevice, mockLogger
}

// configurePowerStateMock sets up the mock expectations for GetPowerState
func configurePowerStateMock(t *testing.T, mockDevice *mocks.MockDeviceManagementFeature, mockLogger *mocks.MockLogger, tt struct {
	name           string
	deviceID       string
	requestBody    map[string]interface{}
	mockPowerState *dto.PowerState
	mockPowerError error
	mockResetResp  power.PowerActionResponse
	mockResetError error
	expectedStatus int
	checkResponse  func(t *testing.T, body string)
	checkPowerCall bool
	checkResetCall bool
	expectedAction int
},
) {
	t.Helper()

	if !tt.checkPowerCall {
		return
	}

	if tt.mockPowerState != nil {
		mockDevice.EXPECT().
			GetPowerState(gomock.Any(), tt.deviceID).
			Return(*tt.mockPowerState, tt.mockPowerError).
			Times(1).
			Do(func(_ context.Context, guid string) {
				assert.Equal(t, tt.deviceID, guid)
			})
	} else {
		mockDevice.EXPECT().
			GetPowerState(gomock.Any(), tt.deviceID).
			Return(dto.PowerState{}, tt.mockPowerError).
			Times(1).
			Do(func(_ context.Context, guid string) {
				assert.Equal(t, tt.deviceID, guid)
			})
	}

	if tt.mockPowerError != nil {
		mockLogger.EXPECT().
			Warn("redfish v1 - Systems instance: failed to get power state for %s: %v", tt.deviceID, tt.mockPowerError).
			Times(1)
	}
}

// configureResetActionMock sets up the mock expectations for SendPowerAction
func configureResetActionMock(mockDevice *mocks.MockDeviceManagementFeature, tt struct {
	name           string
	deviceID       string
	requestBody    map[string]interface{}
	mockPowerState *dto.PowerState
	mockPowerError error
	mockResetResp  power.PowerActionResponse
	mockResetError error
	expectedStatus int
	checkResponse  func(t *testing.T, body string)
	checkPowerCall bool
	checkResetCall bool
	expectedAction int
},
) {
	if tt.checkResetCall {
		mockDevice.EXPECT().
			SendPowerAction(gomock.Any(), tt.deviceID, tt.expectedAction).
			Return(tt.mockResetResp, tt.mockResetError).
			Times(1)
	}
}

// configureErrorLoggingMock sets up the mock expectations for error logging
func configureErrorLoggingMock(mockLogger *mocks.MockLogger, tt struct {
	name           string
	deviceID       string
	requestBody    map[string]interface{}
	mockPowerState *dto.PowerState
	mockPowerError error
	mockResetResp  power.PowerActionResponse
	mockResetError error
	expectedStatus int
	checkResponse  func(t *testing.T, body string)
	checkPowerCall bool
	checkResetCall bool
	expectedAction int
},
) {
	if tt.mockResetError != nil {
		mockLogger.EXPECT().
			Error(tt.mockResetError, "http - redfish v1 - ComputerSystem.Reset").
			Times(1)
	}
}

// executeResetHandlerTest executes the actual test with the configured mocks
func executeResetHandlerTest(t *testing.T, mockDevice *mocks.MockDeviceManagementFeature, mockLogger *mocks.MockLogger, tt struct {
	name           string
	deviceID       string
	requestBody    map[string]interface{}
	mockPowerState *dto.PowerState
	mockPowerError error
	mockResetResp  power.PowerActionResponse
	mockResetError error
	expectedStatus int
	checkResponse  func(t *testing.T, body string)
	checkPowerCall bool
	checkResetCall bool
	expectedAction int
},
) {
	t.Helper()

	// Setup Gin test context
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: tt.deviceID}}

	// Prepare request body
	reqBody := prepareRequestBody(tt)

	req, _ := http.NewRequestWithContext(context.Background(),
		"POST", "/redfish/v1/Systems/"+tt.deviceID+"/Actions/ComputerSystem.Reset",
		bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	// Call the handler
	handler := postSystemResetHandler(mockDevice, mockLogger)
	handler(c)

	// Assertions
	assert.Equal(t, tt.expectedStatus, w.Code)

	if tt.checkResponse != nil {
		tt.checkResponse(t, w.Body.String())
	}
}

// prepareRequestBody prepares the request body based on test parameters
func prepareRequestBody(tt struct {
	name           string
	deviceID       string
	requestBody    map[string]interface{}
	mockPowerState *dto.PowerState
	mockPowerError error
	mockResetResp  power.PowerActionResponse
	mockResetError error
	expectedStatus int
	checkResponse  func(t *testing.T, body string)
	checkPowerCall bool
	checkResetCall bool
	expectedAction int
},
) []byte {
	switch {
	case tt.requestBody == nil && tt.name == malformedJSONTestName:
		return []byte(`{invalid json}`)
	case tt.requestBody != nil:
		reqBody, _ := json.Marshal(tt.requestBody)

		return reqBody
	default:
		return []byte(`{}`)
	}
}

func TestPostSystemResetHandler_ErrorHandling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		deviceID       string
		requestBody    map[string]interface{}
		mockPowerState *dto.PowerState
		mockPowerError error
		mockResetResp  power.PowerActionResponse
		mockResetError error
		expectedStatus int
		checkResponse  func(t *testing.T, body string)
		checkPowerCall bool
		checkResetCall bool
		expectedAction int
	}{
		{
			name:     "successful power on",
			deviceID: "test-device-1",
			requestBody: map[string]interface{}{
				"ResetType": resetTypeOn,
			},
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerHardOff, // currently off
			},
			mockPowerError: nil,
			mockResetResp: power.PowerActionResponse{
				ReturnValue: 0, // success
			},
			mockResetError: nil,
			expectedStatus: http.StatusOK,
			checkPowerCall: true,
			checkResetCall: true,
			expectedAction: actionPowerUp,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"TaskState":"Completed"`)
				assert.Contains(t, body, `"TaskStatus":"OK"`)
				assert.Contains(t, body, `"Name":"System Reset Task"`)
				assert.Contains(t, body, `"Base.1.11.0.Success"`)
			},
		},
		{
			name:     "successful force off",
			deviceID: "test-device-2",
			requestBody: map[string]interface{}{
				"ResetType": resetTypeForceOff,
			},
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerOn, // currently on
			},
			mockPowerError: nil,
			mockResetResp: power.PowerActionResponse{
				ReturnValue: 0,
			},
			mockResetError: nil,
			expectedStatus: http.StatusOK,
			checkPowerCall: true,
			checkResetCall: true,
			expectedAction: actionPowerDown,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"TaskState":"Completed"`)
				assert.Contains(t, body, `"TaskStatus":"OK"`)
			},
		},
		{
			name:     "successful force restart",
			deviceID: "test-device-3",
			requestBody: map[string]interface{}{
				"ResetType": resetTypeForceRestart,
			},
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerOn,
			},
			mockPowerError: nil,
			mockResetResp:  power.PowerActionResponse{ReturnValue: 0},
			mockResetError: nil,
			expectedStatus: http.StatusOK,
			checkPowerCall: true,
			checkResetCall: true,
			expectedAction: actionReset,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"TaskState":"Completed"`)
			},
		},
		{
			name:     "successful power cycle",
			deviceID: "test-device-4",
			requestBody: map[string]interface{}{
				"ResetType": resetTypePowerCycle,
			},
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerOn,
			},
			mockPowerError: nil,
			mockResetResp:  power.PowerActionResponse{ReturnValue: 0},
			mockResetError: nil,
			expectedStatus: http.StatusOK,
			checkPowerCall: true,
			checkResetCall: true,
			expectedAction: actionPowerCycle,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"TaskState":"Completed"`)
			},
		},
		{
			name:           malformedJSONTestName,
			deviceID:       "test-device-5",
			requestBody:    nil, // will send malformed JSON
			expectedStatus: http.StatusBadRequest,
			checkPowerCall: false,
			checkResetCall: false,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Base.1.11.0.MalformedJSON"`)
			},
		},
		{
			name:     "missing ResetType property",
			deviceID: "test-device-6",
			requestBody: map[string]interface{}{
				"SomeOtherProperty": "value",
			},
			expectedStatus: http.StatusBadRequest,
			checkPowerCall: false,
			checkResetCall: false,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Base.1.11.0.PropertyMissing"`)
				assert.Contains(t, body, "ResetType")
			},
		},
		{
			name:     "invalid ResetType value",
			deviceID: "test-device-7",
			requestBody: map[string]interface{}{
				"ResetType": "InvalidResetType",
			},
			expectedStatus: http.StatusBadRequest,
			checkPowerCall: false,
			checkResetCall: false,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Base.1.11.0.PropertyValueNotInList"`)
				assert.Contains(t, body, "InvalidResetType")
			},
		},
		{
			name:     "power on when already on - conflict",
			deviceID: "test-device-8",
			requestBody: map[string]interface{}{
				"ResetType": resetTypeOn,
			},
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerOn, // already on
			},
			mockPowerError: nil,
			expectedStatus: http.StatusConflict,
			checkPowerCall: true,
			checkResetCall: false,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Base.1.11.0.OperationNotAllowed"`)
			},
		},
		{
			name:     "power off when already off - conflict",
			deviceID: "test-device-9",
			requestBody: map[string]interface{}{
				"ResetType": resetTypeForceOff,
			},
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerHardOff, // already off
			},
			mockPowerError: nil,
			expectedStatus: http.StatusConflict,
			checkPowerCall: true,
			checkResetCall: false,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Base.1.11.0.OperationNotAllowed"`)
			},
		},
		{
			name:     "device not found error",
			deviceID: "nonexistent-device",
			requestBody: map[string]interface{}{
				"ResetType": resetTypeOn,
			},
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerHardOff,
			},
			mockPowerError: nil,
			mockResetResp:  power.PowerActionResponse{},
			mockResetError: errors.New("device not found"),
			expectedStatus: http.StatusNotFound,
			checkPowerCall: true,
			checkResetCall: true,
			expectedAction: actionPowerUp,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Base.1.11.0.ResourceNotFound"`)
				assert.Contains(t, body, "nonexistent-device")
			},
		},
		{
			name:     "power action failed",
			deviceID: "test-device-10",
			requestBody: map[string]interface{}{
				"ResetType": resetTypeOn,
			},
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerHardOff,
			},
			mockPowerError: nil,
			mockResetResp: power.PowerActionResponse{
				ReturnValue: 1, // failure
			},
			mockResetError: nil,
			expectedStatus: http.StatusOK,
			checkPowerCall: true,
			checkResetCall: true,
			expectedAction: actionPowerUp,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"TaskState":"Exception"`)
				assert.Contains(t, body, `"TaskStatus":"Critical"`)
				assert.Contains(t, body, `"Base.1.11.0.GeneralError"`)
			},
		},
		{
			name:     "general error on power action",
			deviceID: "test-device-11",
			requestBody: map[string]interface{}{
				"ResetType": resetTypeOn,
			},
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerHardOff,
			},
			mockPowerError: nil,
			mockResetResp:  power.PowerActionResponse{},
			mockResetError: errors.New("general failure"),
			expectedStatus: http.StatusInternalServerError,
			checkPowerCall: true,
			checkResetCall: true,
			expectedAction: actionPowerUp,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Base.1.11.0.GeneralError"`)
			},
		},
		{
			name:     "power state check fails - continue anyway",
			deviceID: "test-device-12",
			requestBody: map[string]interface{}{
				"ResetType": resetTypeOn,
			},
			mockPowerState: nil,
			mockPowerError: errors.New("power state check failed"),
			mockResetResp:  power.PowerActionResponse{ReturnValue: 0},
			mockResetError: nil,
			expectedStatus: http.StatusOK,
			checkPowerCall: true,
			checkResetCall: true,
			expectedAction: actionPowerUp,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"TaskState":"Completed"`)
			},
		},
		{
			name:     "upstream communication error - connection timeout",
			deviceID: "test-device-13",
			requestBody: map[string]interface{}{
				"ResetType": resetTypeOn,
			},
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerHardOff,
			},
			mockPowerError: nil,
			mockResetResp:  power.PowerActionResponse{},
			mockResetError: errors.New("connection timeout to AMT device"),
			expectedStatus: http.StatusBadGateway,
			checkPowerCall: true,
			checkResetCall: true,
			expectedAction: actionPowerUp,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Base.1.11.0.GeneralError"`)
				assert.Contains(t, body, "upstream service or managed device is unavailable")
			},
		},
		{
			name:     "upstream communication error - WSMAN failure",
			deviceID: "test-device-14",
			requestBody: map[string]interface{}{
				"ResetType": resetTypeForceOff,
			},
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerOn,
			},
			mockPowerError: nil,
			mockResetResp:  power.PowerActionResponse{},
			mockResetError: errors.New("WSMAN authentication failed"),
			expectedStatus: http.StatusBadGateway,
			checkPowerCall: true,
			checkResetCall: true,
			expectedAction: actionPowerDown,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Base.1.11.0.GeneralError"`)
				assert.Contains(t, body, "upstream service or managed device is unavailable")
			},
		},
		{
			name:     "upstream communication error - network unreachable",
			deviceID: "test-device-15",
			requestBody: map[string]interface{}{
				"ResetType": resetTypeForceRestart,
			},
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerOn,
			},
			mockPowerError: nil,
			mockResetResp:  power.PowerActionResponse{},
			mockResetError: errors.New("dial tcp: network unreachable"),
			expectedStatus: http.StatusBadGateway,
			checkPowerCall: true,
			checkResetCall: true,
			expectedAction: actionReset,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Base.1.11.0.GeneralError"`)
				assert.Contains(t, body, "upstream service or managed device is unavailable")
			},
		},
		{
			name:     "service temporarily unavailable - too many connections",
			deviceID: "test-device-16",
			requestBody: map[string]interface{}{
				"ResetType": resetTypeOn,
			},
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerHardOff,
			},
			mockPowerError: nil,
			mockResetResp:  power.PowerActionResponse{},
			mockResetError: errors.New("too many connections to service"),
			expectedStatus: http.StatusServiceUnavailable,
			checkPowerCall: true,
			checkResetCall: true,
			expectedAction: actionPowerUp,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Base.1.11.0.GeneralError"`)
				assert.Contains(t, body, "service is temporarily unavailable")
			},
		},
		{
			name:     "service temporarily unavailable - rate limit exceeded",
			deviceID: "test-device-17",
			requestBody: map[string]interface{}{
				"ResetType": resetTypeForceOff,
			},
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerOn,
			},
			mockPowerError: nil,
			mockResetResp:  power.PowerActionResponse{},
			mockResetError: errors.New("rate limit exceeded for client"),
			expectedStatus: http.StatusServiceUnavailable,
			checkPowerCall: true,
			checkResetCall: true,
			expectedAction: actionPowerDown,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Base.1.11.0.GeneralError"`)
				assert.Contains(t, body, "service is temporarily unavailable")
			},
		},
		{
			name:     "service temporarily unavailable - maintenance mode",
			deviceID: "test-device-18",
			requestBody: map[string]interface{}{
				"ResetType": resetTypePowerCycle,
			},
			mockPowerState: &dto.PowerState{
				PowerState: cimPowerOn,
			},
			mockPowerError: nil,
			mockResetResp:  power.PowerActionResponse{},
			mockResetError: errors.New("system in maintenance mode"),
			expectedStatus: http.StatusServiceUnavailable,
			checkPowerCall: true,
			checkResetCall: true,
			expectedAction: actionPowerCycle,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"Base.1.11.0.GeneralError"`)
				assert.Contains(t, body, "service is temporarily unavailable")
			},
		},
	}

	runResetHandlerTests(t, tests)
}

func TestIsUpstreamCommunicationError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "connection timeout error",
			err:      errors.New("connection timeout"),
			expected: true,
		},
		{
			name:     "WSMAN error",
			err:      errors.New("WSMAN authentication failed"),
			expected: true,
		},
		{
			name:     "AMT error",
			err:      errors.New("AMT device unreachable"),
			expected: true,
		},
		{
			name:     "network unreachable error",
			err:      errors.New("dial tcp: network unreachable"),
			expected: true,
		},
		{
			name:     "TLS certificate error",
			err:      errors.New("TLS certificate verification failed"),
			expected: true,
		},
		{
			name:     "I/O timeout error",
			err:      errors.New("i/o timeout occurred"),
			expected: true,
		},
		{
			name:     "connection refused error",
			err:      errors.New("connection refused by host"),
			expected: true,
		},
		{
			name:     "unauthorized error",
			err:      errors.New("unauthorized access to device"),
			expected: true,
		},
		{
			name:     "general database error",
			err:      errors.New("database connection failed"),
			expected: false,
		},
		{
			name:     "validation error",
			err:      errors.New("invalid parameter provided"),
			expected: false,
		},
		{
			name:     "not found error",
			err:      errors.New("device not found"),
			expected: false,
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockFeature := mocks.NewMockDeviceManagementFeature(ctrl)
			mockLogger := mocks.NewMockLogger(ctrl)

			tt.setupMocks(mockFeature, mockLogger)

			gin.SetMode(gin.TestMode)
			router := gin.New()
			systems := router.Group("/redfish/v1/Systems")
			systems.GET(":id", getSystemInstanceHandler(mockFeature, mockLogger))

			w := httptest.NewRecorder()
			req, _ := http.NewRequestWithContext(
				context.Background(),
				"GET",
				"/redfish/v1/Systems/"+tt.systemID,
				http.NoBody,
			)

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			tt.validateResponse(t, w.Body.String())
		})
	}
}

func TestPostSystemResetHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		systemID         string
		requestBody      string
		setupMocks       func(*mocks.MockDeviceManagementFeature, *mocks.MockLogger)
		expectedStatus   int
		validateResponse func(t *testing.T, body string)
	}{
		{
			name:        "successful power on",
			systemID:    testSystemGUID,
			requestBody: `{"ResetType": "On"}`,
			setupMocks: func(mockFeature *mocks.MockDeviceManagementFeature, mockLogger *mocks.MockLogger) {
				expectedResult := power.PowerActionResponse{
					ReturnValue: power.ReturnValue(0),
				}
				mockFeature.EXPECT().
					SendPowerAction(gomock.Any(), testSystemGUID, actionPowerUp).
					Return(expectedResult, nil)

				mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, "ReturnValue")
			},
		},
		{
			name:        "successful force off",
			systemID:    testSystemGUID,
			requestBody: `{"ResetType": "ForceOff"}`,
			setupMocks: func(mockFeature *mocks.MockDeviceManagementFeature, _ *mocks.MockLogger) {
				expectedResult := power.PowerActionResponse{
					ReturnValue: power.ReturnValue(0),
				}
				mockFeature.EXPECT().
					SendPowerAction(gomock.Any(), testSystemGUID, actionPowerDown).
					Return(expectedResult, nil)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, "ReturnValue")
			},
		},
		{
			name:        "successful force restart",
			systemID:    testSystemGUID,
			requestBody: `{"ResetType": "ForceRestart"}`,
			setupMocks: func(mockFeature *mocks.MockDeviceManagementFeature, _ *mocks.MockLogger) {
				expectedResult := power.PowerActionResponse{
					ReturnValue: power.ReturnValue(0),
				}
				mockFeature.EXPECT().
					SendPowerAction(gomock.Any(), testSystemGUID, actionReset).
					Return(expectedResult, nil)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, "ReturnValue")
			},
		},
		{
			name:        "successful power cycle",
			systemID:    testSystemGUID,
			requestBody: `{"ResetType": "PowerCycle"}`,
			setupMocks: func(mockFeature *mocks.MockDeviceManagementFeature, _ *mocks.MockLogger) {
				expectedResult := power.PowerActionResponse{
					ReturnValue: power.ReturnValue(0),
				}
				mockFeature.EXPECT().
					SendPowerAction(gomock.Any(), testSystemGUID, actionPowerCycle).
					Return(expectedResult, nil)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, "ReturnValue")
			},
		},
		{
			name:        "invalid reset type",
			systemID:    testSystemGUID,
			requestBody: `{"ResetType": "InvalidType"}`,
			setupMocks: func(_ *mocks.MockDeviceManagementFeature, _ *mocks.MockLogger) {
				// No mock calls expected for invalid reset type
			},
			expectedStatus: http.StatusBadRequest,
			validateResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, "unsupported ResetType")
			},
		},
		{
			name:        "malformed JSON",
			systemID:    testSystemGUID,
			requestBody: `{"ResetType": }`, // Invalid JSON
			setupMocks: func(_ *mocks.MockDeviceManagementFeature, _ *mocks.MockLogger) {
				// No mock calls expected for malformed JSON
			},
			expectedStatus: http.StatusBadRequest,
			validateResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, "error")
			},
		},
		{
			name:        "missing reset type",
			systemID:    testSystemGUID,
			requestBody: `{}`, // Missing ResetType
			setupMocks: func(_ *mocks.MockDeviceManagementFeature, _ *mocks.MockLogger) {
				// No mock calls expected - empty ResetType will be treated as unsupported
			},
			expectedStatus: http.StatusBadRequest,
			validateResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, "unsupported ResetType")
			},
		},
		{
			name:        "backend error",
			systemID:    testSystemGUID,
			requestBody: `{"ResetType": "On"}`,
			setupMocks: func(mockFeature *mocks.MockDeviceManagementFeature, mockLogger *mocks.MockLogger) {
				mockFeature.EXPECT().
					SendPowerAction(gomock.Any(), testSystemGUID, actionPowerUp).
					Return(power.PowerActionResponse{}, fmt.Errorf("system not found"))

				mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).Times(1)
			},
			expectedStatus: http.StatusInternalServerError,
			validateResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, "system not found")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockFeature := mocks.NewMockDeviceManagementFeature(ctrl)
			mockLogger := mocks.NewMockLogger(ctrl)

			tt.setupMocks(mockFeature, mockLogger)

			gin.SetMode(gin.TestMode)
			router := gin.New()
			systems := router.Group("/redfish/v1/Systems")
			systems.POST(":id/Actions/ComputerSystem.Reset", postSystemResetHandler(mockFeature, mockLogger))

			w := httptest.NewRecorder()
			req, _ := http.NewRequestWithContext(
				context.Background(),
				"POST",
				"/redfish/v1/Systems/"+tt.systemID+"/Actions/ComputerSystem.Reset",
				strings.NewReader(tt.requestBody),
			)
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			tt.validateResponse(t, w.Body.String())
		})
	}
}

func TestPowerStateMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		cimPowerState   int
		expectedRedfish string
	}{
		{
			name:            "CIM Power On maps to Redfish On",
			cimPowerState:   actionPowerUp, // 2
			expectedRedfish: powerStateOn,
		},
		{
			name:            "CIM Sleep maps to Redfish On",
			cimPowerState:   cimPowerSleep, // 3
			expectedRedfish: powerStateOn,
		},
		{
			name:            "CIM Standby maps to Redfish On",
			cimPowerState:   cimPowerStandby, // 4
			expectedRedfish: powerStateOn,
		},
		{
			name:            "CIM Soft Off maps to Redfish Off",
			cimPowerState:   cimPowerSoftOff, // 7
			expectedRedfish: powerStateOff,
		},
		{
			name:            "CIM Hard Off maps to Redfish Off",
			cimPowerState:   cimPowerHardOff, // 8
			expectedRedfish: powerStateOff,
		},
		{
			name:            "Unknown CIM state maps to Redfish Unknown",
			cimPowerState:   999,
			expectedRedfish: powerStateUnknown,
		},
		{
			name:            "Zero value maps to Redfish Unknown",
			cimPowerState:   0,
			expectedRedfish: powerStateUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := isUpstreamCommunicationError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsServiceTemporarilyUnavailable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "too many connections error",
			err:      errors.New("too many connections to database"),
			expected: true,
		},
		{
			name:     "connection pool exhausted error",
			err:      errors.New("connection pool exhausted"),
			expected: true,
		},
		{
			name:     "database pool full error",
			err:      errors.New("database pool full"),
			expected: true,
		},
		{
			name:     "service overloaded error",
			err:      errors.New("service overloaded - try again later"),
			expected: true,
		},
		{
			name:     "maintenance mode error",
			err:      errors.New("system in maintenance mode"),
			expected: true,
		},
		{
			name:     "rate limit exceeded error",
			err:      errors.New("rate limit exceeded for client"),
			expected: true,
		},
		{
			name:     "too many requests error",
			err:      errors.New("too many requests from client"),
			expected: true,
		},
		{
			name:     "resource exhausted error",
			err:      errors.New("resource exhausted - retry later"),
			expected: true,
		},
		{
			name:     "service unavailable error",
			err:      errors.New("service unavailable temporarily"),
			expected: true,
		},
		{
			name:     "max connections reached error",
			err:      errors.New("max connections reached"),
			expected: true,
		},
		{
			name:     "server overloaded error",
			err:      errors.New("server overloaded"),
			expected: true,
		},
		{
			name:     "capacity exceeded error",
			err:      errors.New("capacity exceeded"),
			expected: true,
		},
		{
			name:     "throttled error",
			err:      errors.New("request throttled"),
			expected: true,
		},
		{
			name:     "circuit breaker error",
			err:      errors.New("circuit breaker open"),
			expected: true,
		},
		{
			name:     "general database error",
			err:      errors.New("database query failed"),
			expected: false,
		},
		{
			name:     "validation error",
			err:      errors.New("invalid parameter provided"),
			expected: false,
		},
		{
			name:     "device not found error",
			err:      errors.New("device not found"),
			expected: false,
		},
		{
			name:     "WSMAN communication error",
			err:      errors.New("WSMAN connection failed"),
			expected: false, // This should be 502, not 503
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockFeature := mocks.NewMockDeviceManagementFeature(ctrl)
			mockLogger := mocks.NewMockLogger(ctrl)

			powerState := dto.PowerState{
				PowerState: tt.cimPowerState,
			}
			mockFeature.EXPECT().
				GetPowerState(gomock.Any(), testSystemGUID).
				Return(powerState, nil)

			mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()

			gin.SetMode(gin.TestMode)
			router := gin.New()
			systems := router.Group("/redfish/v1/Systems")
			systems.GET(":id", getSystemInstanceHandler(mockFeature, mockLogger))

			w := httptest.NewRecorder()
			req, _ := http.NewRequestWithContext(
				context.Background(),
				"GET",
				"/redfish/v1/Systems/"+testSystemGUID,
				http.NoBody,
			)

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var system map[string]interface{}

			err := json.Unmarshal(w.Body.Bytes(), &system)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedRedfish, system["PowerState"])
		})
	}
}

func TestResetTypeMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		redfishResetType  string
		expectedCIMAction int
	}{
		{
			name:              "On maps to PowerUp action",
			redfishResetType:  resetTypeOn,
			expectedCIMAction: actionPowerUp, // 2
		},
		{
			name:              "ForceOff maps to PowerDown action",
			redfishResetType:  resetTypeForceOff,
			expectedCIMAction: actionPowerDown, // 8
		},
		{
			name:              "ForceRestart maps to Reset action",
			redfishResetType:  resetTypeForceRestart,
			expectedCIMAction: actionReset, // 10
		},
		{
			name:              "PowerCycle maps to PowerCycle action",
			redfishResetType:  resetTypePowerCycle,
			expectedCIMAction: actionPowerCycle, // 5
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := isServiceTemporarilyUnavailable(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewSystemsRoutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		authConfig *config.Config
		checkAuth  bool
	}{
		{
			name: "auth disabled",
			authConfig: &config.Config{
				Auth: config.Auth{
					Disabled: true,
				},
			},
			checkAuth: false,
		},
		{
			name: "auth enabled",
			authConfig: &config.Config{
				Auth: config.Auth{
					Disabled: false,
					JWTKey:   "test-key",
				},
			},
			checkAuth: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockDevice := mocks.NewMockDeviceManagementFeature(ctrl)
			mockLogger := mocks.NewMockLogger(ctrl)

			// Expect the info log
			mockLogger.EXPECT().
				Info("Registered Redfish v1 Systems routes under %s", gomock.Any()).
				Times(1)

			// Setup Gin
			gin.SetMode(gin.TestMode)
			router := gin.New()
			group := router.Group("/redfish/v1")

			// Call NewSystemsRoutes
			NewSystemsRoutes(group, mockDevice, tt.authConfig, mockLogger)

			// Verify routes are registered (this is more of a smoke test)
			// In a real scenario, you might want to test actual route registration
			assert.NotNil(t, group)
		})
	}
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockFeature := mocks.NewMockDeviceManagementFeature(ctrl)
			mockLogger := mocks.NewMockLogger(ctrl)

			expectedResult := power.PowerActionResponse{
				ReturnValue: power.ReturnValue(0),
			}
			mockFeature.EXPECT().
				SendPowerAction(gomock.Any(), testSystemGUID, tt.expectedCIMAction).
				Return(expectedResult, nil)

			gin.SetMode(gin.TestMode)
			router := gin.New()
			systems := router.Group("/redfish/v1/Systems")
			systems.POST(":id/Actions/ComputerSystem.Reset", postSystemResetHandler(mockFeature, mockLogger))

			requestBody := fmt.Sprintf(`{"ResetType": %q}`, tt.redfishResetType)

			w := httptest.NewRecorder()
			req, _ := http.NewRequestWithContext(
				context.Background(),
				"POST",
				"/redfish/v1/Systems/"+testSystemGUID+"/Actions/ComputerSystem.Reset",
				strings.NewReader(requestBody),
			)
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

func TestSystemsIntegrationWithFirmware(t *testing.T) {
	t.Parallel()

	t.Run("firmware routes integrated with systems", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockFeature := mocks.NewMockDeviceManagementFeature(ctrl)
		mockLogger := mocks.NewMockLogger(ctrl)

		// Mock for firmware inventory access
		mockFeature.EXPECT().
			GetVersion(gomock.Any(), testSystemGUID).
			Return(dto.Version{}, dtov2.Version{AMT: "15.0.25"}, nil)

		// Mock for BIOS hardware info (required by firmware routes)
		mockFeature.EXPECT().
			GetHardwareInfo(gomock.Any(), testSystemGUID).
			Return(map[string]interface{}{
				"CIM_BIOSElement": map[string]interface{}{
					"Version":      "BIOS.15.25.10",
					"Manufacturer": "Intel Corp.",
				},
			}, nil)

		mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any()).AnyTimes()

		gin.SetMode(gin.TestMode)
		router := gin.New()

		// Setup complete systems routes including firmware
		redfishGroup := router.Group("/redfish/v1")
		NewSystemsRoutes(redfishGroup, mockFeature, mockLogger)

		// Test that firmware inventory endpoint is accessible via systems routes
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(
			context.Background(),
			"GET",
			"/redfish/v1/Systems/"+testSystemGUID+"/FirmwareInventory",
			http.NoBody,
		)

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var collection map[string]interface{}

		err := json.Unmarshal(w.Body.Bytes(), &collection)
		require.NoError(t, err)

		assert.Equal(t, "#SoftwareInventoryCollection.SoftwareInventoryCollection", collection["@odata.type"])
		assert.Equal(t, "/redfish/v1/Systems/"+testSystemGUID+"/FirmwareInventory", collection["@odata.id"])
	})
}

func TestConstants(t *testing.T) {
	t.Parallel()

	t.Run("power state constants", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "Unknown", powerStateUnknown)
		assert.Equal(t, "On", powerStateOn)
		assert.Equal(t, "Off", powerStateOff)
	})

	t.Run("reset type constants", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "On", resetTypeOn)
		assert.Equal(t, "ForceOff", resetTypeForceOff)
		assert.Equal(t, "ForceRestart", resetTypeForceRestart)
		assert.Equal(t, "PowerCycle", resetTypePowerCycle)
	})

	t.Run("action constants", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, 2, actionPowerUp)
		assert.Equal(t, 5, actionPowerCycle)
		assert.Equal(t, 8, actionPowerDown)
		assert.Equal(t, 10, actionReset)
	})

	t.Run("CIM power state constants", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, 2, cimPowerOn)
		assert.Equal(t, 3, cimPowerSleep)
		assert.Equal(t, 4, cimPowerStandby)
		assert.Equal(t, 7, cimPowerSoftOff)
		assert.Equal(t, 8, cimPowerHardOff)
	})

	t.Run("limits constants", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, 100, maxSystemsList)
	})
}

func TestSystemResponseStructure(t *testing.T) {
	t.Parallel()

	t.Run("system response contains all required fields", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockFeature := mocks.NewMockDeviceManagementFeature(ctrl)
		mockLogger := mocks.NewMockLogger(ctrl)

		powerState := dto.PowerState{PowerState: actionPowerUp}
		mockFeature.EXPECT().
			GetPowerState(gomock.Any(), testSystemGUID).
			Return(powerState, nil)

		mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()

		gin.SetMode(gin.TestMode)
		router := gin.New()
		systems := router.Group("/redfish/v1/Systems")
		systems.GET(":id", getSystemInstanceHandler(mockFeature, mockLogger))

		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(
			context.Background(),
			"GET",
			"/redfish/v1/Systems/"+testSystemGUID,
			http.NoBody,
		)

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var system map[string]interface{}

		err := json.Unmarshal(w.Body.Bytes(), &system)
		require.NoError(t, err)

		// Check required Redfish properties
		requiredFields := []string{
			"@odata.type",
			"@odata.id",
			"Id",
			"Name",
			"PowerState",
			"Actions",
		}

		for _, field := range requiredFields {
			assert.Contains(t, system, field, "System response missing required field: %s", field)
		}

		// Check Actions structure
		actions, ok := system["Actions"].(map[string]interface{})
		require.True(t, ok, "Actions should be a map")
		assert.Contains(t, actions, "#ComputerSystem.Reset")

		resetAction, ok := actions["#ComputerSystem.Reset"].(map[string]interface{})
		require.True(t, ok, "Reset action should be a map")
		assert.Contains(t, resetAction, "target")
		assert.Contains(t, resetAction, "ResetType@Redfish.AllowableValues")

		allowedValues, ok := resetAction["ResetType@Redfish.AllowableValues"].([]interface{})
		require.True(t, ok, "AllowableValues should be a slice of interfaces")
		assert.Equal(t, 4, len(allowedValues)) // On, ForceOff, ForceRestart, PowerCycle
	})
}

func TestErrorHandling(t *testing.T) {
	t.Parallel()

	t.Run("handles context cancellation", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockFeature := mocks.NewMockDeviceManagementFeature(ctrl)
		mockLogger := mocks.NewMockLogger(ctrl)

		mockFeature.EXPECT().
			Get(gomock.Any(), maxSystemsList, 0, "").
			Return(nil, context.Canceled)

		mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).Times(1)

		gin.SetMode(gin.TestMode)
		router := gin.New()
		systems := router.Group("/redfish/v1/Systems")
		systems.GET("", getSystemsCollectionHandler(mockFeature, mockLogger))

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(ctx, "GET", "/redfish/v1/Systems", http.NoBody)

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("handles nil device feature gracefully", func(t *testing.T) {
		t.Parallel()

		gin.SetMode(gin.TestMode)
		router := gin.New()
		systems := router.Group("/redfish/v1/Systems")

		// This should not panic, but will result in a runtime error when called
		require.NotPanics(t, func() {
			systems.GET("", getSystemsCollectionHandler(nil, nil))
		})
	})
}
