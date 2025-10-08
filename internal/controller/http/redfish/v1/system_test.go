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
	"go.uber.org/mock/gomock"

	"github.com/device-management-toolkit/console/config"
	dto "github.com/device-management-toolkit/console/internal/entity/dto/v1"
	"github.com/device-management-toolkit/console/internal/mocks"
	"github.com/device-management-toolkit/go-wsman-messages/v2/pkg/wsman/cim/power"
)

func TestGetSystemsCollectionHandler(t *testing.T) {
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
				assert.Contains(t, body, "database connection failed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			req, _ := http.NewRequest("GET", "/redfish/v1/Systems", nil)
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
				assert.Contains(t, body, `"PowerState":"Unknown"`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
					Do(func(ctx context.Context, guid string) {
						// Verify the correct GUID is passed
						assert.Equal(t, tt.deviceID, guid)
					})
			} else {
				mockDevice.EXPECT().
					GetPowerState(gomock.Any(), tt.deviceID).
					Return(dto.PowerState{}, tt.mockError).
					Times(1).
					Do(func(ctx context.Context, guid string) {
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
			req, _ := http.NewRequest("GET", "/redfish/v1/Systems/"+tt.deviceID, nil)
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
			// Setup Gin test context
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			req, _ := http.NewRequest("GET", "/redfish/v1/Systems/test/Actions/ComputerSystem.Reset", nil)
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

func TestPostSystemResetHandler(t *testing.T) {
	tests := []struct {
		name             string
		deviceID         string
		requestBody      map[string]interface{}
		mockPowerState   *dto.PowerState
		mockPowerError   error
		mockResetResp    power.PowerActionResponse
		mockResetError   error
		expectedStatus   int
		checkResponse    func(t *testing.T, body string)
		checkPowerCall   bool
		checkResetCall   bool
		expectedAction   int
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
			mockResetError:   nil,
			expectedStatus:   http.StatusOK,
			checkPowerCall:   true,
			checkResetCall:   true,
			expectedAction:   actionPowerUp,
			checkResponse: func(t *testing.T, body string) {
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
			mockResetError:   nil,
			expectedStatus:   http.StatusOK,
			checkPowerCall:   true,
			checkResetCall:   true,
			expectedAction:   actionPowerDown,
			checkResponse: func(t *testing.T, body string) {
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
			mockPowerError:   nil,
			mockResetResp:    power.PowerActionResponse{ReturnValue: 0},
			mockResetError:   nil,
			expectedStatus:   http.StatusOK,
			checkPowerCall:   true,
			checkResetCall:   true,
			expectedAction:   actionReset,
			checkResponse: func(t *testing.T, body string) {
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
			mockPowerError:   nil,
			mockResetResp:    power.PowerActionResponse{ReturnValue: 0},
			mockResetError:   nil,
			expectedStatus:   http.StatusOK,
			checkPowerCall:   true,
			checkResetCall:   true,
			expectedAction:   actionPowerCycle,
			checkResponse: func(t *testing.T, body string) {
				assert.Contains(t, body, `"TaskState":"Completed"`)
			},
		},
		{
			name:           "malformed JSON",
			deviceID:       "test-device-5",
			requestBody:    nil, // will send malformed JSON
			expectedStatus: http.StatusBadRequest,
			checkPowerCall: false,
			checkResetCall: false,
			checkResponse: func(t *testing.T, body string) {
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
			mockResetError:   nil,
			expectedStatus:   http.StatusOK,
			checkPowerCall:   true,
			checkResetCall:   true,
			expectedAction:   actionPowerUp,
			checkResponse: func(t *testing.T, body string) {
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
				assert.Contains(t, body, `"TaskState":"Completed"`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockDevice := mocks.NewMockDeviceManagementFeature(ctrl)
			mockLogger := mocks.NewMockLogger(ctrl)

			// Setup expectations for GetPowerState
			if tt.checkPowerCall {
				if tt.mockPowerState != nil {
					mockDevice.EXPECT().
						GetPowerState(gomock.Any(), tt.deviceID).
						Return(*tt.mockPowerState, tt.mockPowerError).
						Times(1)
				} else {
					mockDevice.EXPECT().
						GetPowerState(gomock.Any(), tt.deviceID).
						Return(dto.PowerState{}, tt.mockPowerError).
						Times(1)
				}
			}

			// Setup expectations for SendPowerAction
			if tt.checkResetCall {
				mockDevice.EXPECT().
					SendPowerAction(gomock.Any(), tt.deviceID, tt.expectedAction).
					Return(tt.mockResetResp, tt.mockResetError).
					Times(1)
			}

			// Setup error logging expectations
			if tt.mockResetError != nil {
				mockLogger.EXPECT().
					Error(tt.mockResetError, "http - redfish v1 - ComputerSystem.Reset").
					Times(1)
			}

			// Setup Gin test context
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{
				{Key: "id", Value: tt.deviceID},
			}

			// Prepare request body
			var reqBody []byte
			if tt.requestBody == nil && tt.name == "malformed JSON" {
				reqBody = []byte(`{invalid json}`)
			} else if tt.requestBody != nil {
				reqBody, _ = json.Marshal(tt.requestBody)
			} else {
				reqBody = []byte(`{}`)
			}

			req, _ := http.NewRequest("POST", "/redfish/v1/Systems/"+tt.deviceID+"/Actions/ComputerSystem.Reset", bytes.NewBuffer(reqBody))
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
		})
	}
}

func TestNewSystemsRoutes(t *testing.T) {
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
}
