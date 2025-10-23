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
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/device-management-toolkit/go-wsman-messages/v2/pkg/wsman/cim/power"

	"github.com/device-management-toolkit/console/config"
	dto "github.com/device-management-toolkit/console/internal/entity/dto/v1"
	dtov2 "github.com/device-management-toolkit/console/internal/entity/dto/v2"
	"github.com/device-management-toolkit/console/internal/mocks"
)

// Test constants
const (
	testSystemGUID        = "test-system-guid-123"
	testInvalidGUID       = "invalid-system-guid"
	systemsBasePath       = "/redfish/v1/Systems"
	systemsInstanceURL    = systemsBasePath + "/" + testSystemGUID
	resetActionURL        = systemsInstanceURL + "/Actions/ComputerSystem.Reset"
	malformedJSONTestName = "malformed JSON"
)

// TestNewSystemsRoutes tests the NewSystemsRoutes function.
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

			// Expect the info logs (both Systems and Firmware routes)
			mockLogger.EXPECT().
				Info("Registered Redfish Systems routes under %s", gomock.Any()).
				Times(1)
			mockLogger.EXPECT().
				Info("Registered Redfish FirmwareInventory routes under %s", gomock.Any()).
				Times(1) // Setup Gin
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
}

// TestGetSystemsCollectionHandler tests the getSystemsCollectionHandler function.
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
				assert.Contains(t, body, `"@odata.type":"`+SchemaComputerSystemCollection+`"`)
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
				assert.Contains(t, body, `"`+BaseErrorMessageID+`"`)
			},
		},
		{
			name:           "upstream communication error",
			mockDevices:    nil,
			mockError:      errors.New("WSMAN connection timeout"),
			expectedStatus: http.StatusBadGateway,
			checkResponse: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, `"`+BaseErrorMessageID+`"`)
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
				assert.Contains(t, body, `"`+BaseErrorMessageID+`"`)
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

// TestGetSystemInstanceHandler tests the getSystemInstanceHandler function.
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
				assert.Contains(t, body, `"@odata.type":"`+SchemaComputerSystem+`"`)
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

// TestMethodNotAllowedHandler tests the methodNotAllowedHandler function.
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
				assert.Contains(t, body, `"`+BaseActionNotSupportedID+`"`)
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

// TestPostSystemResetHandlerSuccessfulOperations tests successful reset operations.
func TestPostSystemResetHandlerSuccessfulOperations(t *testing.T) {
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
				assert.Contains(t, body, `"TaskState":"`+TaskStateCompleted+`"`)
				assert.Contains(t, body, `"TaskStatus":"`+TaskStatusOK+`"`)
				assert.Contains(t, body, `"Name":"System Reset Task"`)
				assert.Contains(t, body, `"`+BaseSuccessMessageID+`"`)
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
				assert.Contains(t, body, `"TaskState":"`+TaskStateCompleted+`"`)
				assert.Contains(t, body, `"TaskStatus":"`+TaskStatusOK+`"`)
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
				assert.Contains(t, body, `"TaskState":"`+TaskStateCompleted+`"`)
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
				assert.Contains(t, body, `"TaskState":"`+TaskStateCompleted+`"`)
			},
		},
	}

	runResetHandlerTests(t, tests)
}

// TestPostSystemResetHandlerValidationErrors tests reset handler validation errors.
func TestPostSystemResetHandlerValidationErrors(t *testing.T) {
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
				assert.Contains(t, body, `"`+BaseMalformedJSONID+`"`)
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
				assert.Contains(t, body, `"`+BasePropertyMissingID+`"`)
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
				assert.Contains(t, body, `"`+BasePropertyValueNotInListID+`"`)
				assert.Contains(t, body, "InvalidResetType")
			},
		},
	}

	runResetHandlerTests(t, tests)
}

// TestPostSystemResetHandlerConflictErrors tests reset handler conflict errors.
func TestPostSystemResetHandlerConflictErrors(t *testing.T) {
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
				assert.Contains(t, body, `"`+BaseOperationNotAllowedID+`"`)
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
				assert.Contains(t, body, `"`+BaseOperationNotAllowedID+`"`)
			},
		},
	}

	runResetHandlerTests(t, tests)
}

// TestPostSystemResetHandlerErrorHandling tests reset handler error handling.
func TestPostSystemResetHandlerErrorHandling(t *testing.T) {
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
				assert.Contains(t, body, `"`+BaseResourceNotFoundID+`"`)
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
				assert.Contains(t, body, `"TaskState":"`+TaskStateException+`"`)
				assert.Contains(t, body, `"TaskStatus":"`+TaskStatusCritical+`"`)
				assert.Contains(t, body, `"`+BaseErrorMessageID+`"`)
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
				assert.Contains(t, body, `"`+BaseErrorMessageID+`"`)
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
				assert.Contains(t, body, `"TaskState":"`+TaskStateCompleted+`"`)
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
				assert.Contains(t, body, `"`+BaseErrorMessageID+`"`)
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
				assert.Contains(t, body, `"`+BaseErrorMessageID+`"`)
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
				assert.Contains(t, body, `"`+BaseErrorMessageID+`"`)
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
				assert.Contains(t, body, `"`+BaseErrorMessageID+`"`)
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
				assert.Contains(t, body, `"`+BaseErrorMessageID+`"`)
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
				assert.Contains(t, body, `"`+BaseErrorMessageID+`"`)
				assert.Contains(t, body, "service is temporarily unavailable")
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

// TestIsUpstreamCommunicationError tests the isUpstreamCommunicationError function.
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

// TestIsServiceTemporarilyUnavailable tests the isServiceTemporarilyUnavailable function.
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

// TestSystemsIntegrationWithFirmware tests systems integration with firmware routes.
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

		// Allow any info/warn logs
		mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any()).AnyTimes()

		gin.SetMode(gin.TestMode)
		router := gin.New()

		// Setup complete systems routes including firmware
		redfishGroup := router.Group("/redfish/v1")
		cfg := &config.Config{Auth: config.Auth{Disabled: true}}
		NewSystemsRoutes(redfishGroup, mockFeature, cfg, mockLogger)

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

		assert.Equal(t, SchemaSoftwareInventoryCollection, collection["@odata.type"])
		assert.Equal(t, "/redfish/v1/Systems/"+testSystemGUID+"/FirmwareInventory", collection["@odata.id"])
	})
}
