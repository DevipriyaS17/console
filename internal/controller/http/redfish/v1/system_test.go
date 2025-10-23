/*********************************************************************
 * Copyright (c) Intel Corporation 2025
 * SPDX-License-Identifier: Apache-2.0
 **********************************************************************/

// Package v1 implements Redfish API v1 System resources tests.
package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/device-management-toolkit/go-wsman-messages/v2/pkg/wsman/cim/power"

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
