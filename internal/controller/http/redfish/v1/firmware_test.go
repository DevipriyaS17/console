/*********************************************************************
 * Copyright (c) Intel Corporation 2025
 * SPDX-License-Identifier: Apache-2.0
 **********************************************************************/

// Package v1 implements Redfish API v1 FirmwareInventory resources tests.
package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	dto "github.com/device-management-toolkit/console/internal/entity/dto/v1"
	dtov2 "github.com/device-management-toolkit/console/internal/entity/dto/v2"
	"github.com/device-management-toolkit/console/internal/mocks"
)

const testSystemID = "test-system-123"

func TestGenerateETag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "empty content",
			content:  "",
			expected: `W/"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"`,
		},
		{
			name:     "simple content",
			content:  "test",
			expected: `W/"9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"`,
		},
		{
			name:     "firmware inventory content",
			content:  "FirmwareInventory-system-123-5",
			expected: `W/"f10d99bfef532179d77a3e1229f4a1a98ae4b9bfbed03d6839442365a8d759fd"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := generateETag(tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseBIOSInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		hwInfo               interface{}
		expectedVersion      string
		expectedVersionStr   string
		expectedManufacturer string
		expectedRelease      string
	}{
		{
			name:                 "nil hardware info",
			hwInfo:               nil,
			expectedVersion:      "Unknown",
			expectedVersionStr:   "Unknown",
			expectedManufacturer: "System Manufacturer",
			expectedRelease:      time.Now().UTC().Format("2006-01-02"),
		},
		{
			name: "complete BIOS info from map",
			hwInfo: map[string]interface{}{
				"CIM_BIOSElement": map[string]interface{}{
					"response": map[string]interface{}{
						"Version":      "DNKBLi7v.86A.0082.2024.0321.1028",
						"Manufacturer": "Intel Corp.",
						"ReleaseDate": map[string]interface{}{
							"DateTime": "2024-03-21T00:00:00Z",
						},
					},
				},
			},
			expectedVersion:      "DNKBLi7v.86A.0082.2024.0321.1028",
			expectedVersionStr:   "DNKBLi7v.86A.0082.2024.0321.1028 (Released: 2024-03-21)",
			expectedManufacturer: "Intel Corp.",
			expectedRelease:      "2024-03-21",
		},
		{
			name: "BIOS info without release date",
			hwInfo: map[string]interface{}{
				"CIM_BIOSElement": map[string]interface{}{
					"response": map[string]interface{}{
						"Version":      "BIOS-1.0.0",
						"Manufacturer": "ACME Corp.",
					},
				},
			},
			expectedVersion:      "BIOS-1.0.0",
			expectedVersionStr:   "BIOS-1.0.0",
			expectedManufacturer: "ACME Corp.",
			expectedRelease:      time.Now().UTC().Format("2006-01-02"),
		},
		{
			name: "empty CIM_BIOSElement",
			hwInfo: map[string]interface{}{
				"CIM_BIOSElement": map[string]interface{}{},
			},
			expectedVersion:      "Unknown",
			expectedVersionStr:   "Unknown",
			expectedManufacturer: "System Manufacturer",
			expectedRelease:      time.Now().UTC().Format("2006-01-02"),
		},
		{
			name:                 "invalid hardware info structure",
			hwInfo:               "invalid",
			expectedVersion:      "Unknown",
			expectedVersionStr:   "Unknown",
			expectedManufacturer: "System Manufacturer",
			expectedRelease:      time.Now().UTC().Format("2006-01-02"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			version, versionStr, manufacturer, releaseDate := parseBIOSInfo(tt.hwInfo)

			assert.Equal(t, tt.expectedVersion, version)
			assert.Equal(t, tt.expectedVersionStr, versionStr)
			assert.Equal(t, tt.expectedManufacturer, manufacturer)
			assert.Equal(t, tt.expectedRelease, releaseDate)
		})
	}
}

func TestParseFromStruct(t *testing.T) {
	t.Parallel()

	// Mock BIOS struct similar to what would come from go-wsman-messages
	type MockBIOSTime struct {
		DateTime string
	}

	type MockBIOSElement struct {
		Version      string
		Manufacturer string
		ReleaseDate  MockBIOSTime
	}

	tests := []struct {
		name                 string
		response             interface{}
		expectedVersion      string
		expectedVersionStr   string
		expectedManufacturer string
		expectedRelease      string
	}{
		{
			name: "valid BIOS struct",
			response: MockBIOSElement{
				Version:      "STRUCT-BIOS-1.2.3",
				Manufacturer: "Struct Corp.",
				ReleaseDate: MockBIOSTime{
					DateTime: "2023-12-15T00:00:00Z",
				},
			},
			expectedVersion:      "STRUCT-BIOS-1.2.3",
			expectedVersionStr:   "STRUCT-BIOS-1.2.3 (Released: 2023-12-15)",
			expectedManufacturer: "Struct Corp.",
			expectedRelease:      "2023-12-15",
		},
		{
			name:                 "non-struct response",
			response:             "not a struct",
			expectedVersion:      "Unknown",
			expectedVersionStr:   "Unknown",
			expectedManufacturer: "System Manufacturer",
			expectedRelease:      time.Now().UTC().Format("2006-01-02"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			version, versionStr, manufacturer, releaseDate := parseFromStruct(tt.response)

			assert.Equal(t, tt.expectedVersion, version)
			assert.Equal(t, tt.expectedVersionStr, versionStr)
			assert.Equal(t, tt.expectedManufacturer, manufacturer)
			assert.Equal(t, tt.expectedRelease, releaseDate)
		})
	}
}

func TestExtractReleaseDateFromMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       interface{}
		defaultDate string
		expected    string
	}{
		{
			name: "valid date map",
			input: map[string]interface{}{
				"DateTime": "2024-01-15T10:30:00Z",
			},
			defaultDate: "2023-01-01",
			expected:    "2024-01-15",
		},
		{
			name:        "non-map input",
			input:       "not a map",
			defaultDate: "2023-01-01",
			expected:    "2023-01-01",
		},
		{
			name:        "map without DateTime",
			input:       map[string]interface{}{"other": "value"},
			defaultDate: "2023-01-01",
			expected:    "2023-01-01",
		},
		{
			name: "invalid DateTime format",
			input: map[string]interface{}{
				"DateTime": "invalid-date",
			},
			defaultDate: "2023-01-01",
			expected:    "2023-01-01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := extractReleaseDateFromMap(tt.input, "", tt.defaultDate)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetStringField(t *testing.T) {
	t.Parallel()

	type TestStruct struct {
		StringField string
		IntField    int
		BoolField   bool
	}

	testStruct := TestStruct{
		StringField: "test-value",
		IntField:    42,
		BoolField:   true,
	}

	v := reflect.ValueOf(testStruct)

	tests := []struct {
		name      string
		fieldName string
		expected  string
	}{
		{
			name:      "existing string field",
			fieldName: "StringField",
			expected:  "test-value",
		},
		{
			name:      "non-string field",
			fieldName: "IntField",
			expected:  "",
		},
		{
			name:      "non-existent field",
			fieldName: "NonExistent",
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := getStringField(v, tt.fieldName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateIntelOemSection(t *testing.T) {
	t.Parallel()

	systemID := testSystemID
	result := createIntelOemSection(systemID)

	assert.Contains(t, result, "Intel")
	intel, ok := result["Intel"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, "#Intel.v1_0_0.Intel", intel["@odata.type"])
	assert.Equal(t, systemID, intel["SystemGUID"])
	assert.Contains(t, intel, "LastUpdated")
	assert.Contains(t, intel, "AMTCapabilities")

	caps, ok := intel["AMTCapabilities"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, caps["SupportsSOL"])
	assert.Equal(t, true, caps["SupportsIDER"])
	assert.Equal(t, true, caps["SupportsKVM"])
	assert.Equal(t, true, caps["SupportsPowerAction"])
}

func TestGetFirmwareInventoryCollectionHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		systemID             string
		setupMocks           func(*mocks.MockDeviceManagementFeature, *mocks.MockLogger)
		expectedStatus       int
		expectedMembersCount int
		validateResponse     func(t *testing.T, body string, headers http.Header)
	}{
		{
			name:     "successful collection retrieval",
			systemID: "valid-system-id",
			setupMocks: func(mockFeature *mocks.MockDeviceManagementFeature, mockLogger *mocks.MockLogger) {
				// Mock successful GetVersion call
				mockFeature.EXPECT().
					GetVersion(gomock.Any(), "valid-system-id").
					Return(dto.Version{}, dtov2.Version{AMT: "15.0.25"}, nil)

				// Mock successful GetHardwareInfo call
				hwInfo := map[string]interface{}{
					"CIM_BIOSElement": map[string]interface{}{
						"response": map[string]interface{}{
							"Version": "BIOS-1.0.0",
						},
					},
				}
				mockFeature.EXPECT().
					GetHardwareInfo(gomock.Any(), "valid-system-id").
					Return(hwInfo, nil)

				// Logger expectations
				mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
				mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any()).AnyTimes()
			},
			expectedStatus:       http.StatusOK,
			expectedMembersCount: 2, // AMT + BIOS
			validateResponse: func(t *testing.T, body string, headers http.Header) {
				t.Helper()

				var collection FirmwareInventoryCollection

				err := json.Unmarshal([]byte(body), &collection)
				require.NoError(t, err)

				assert.Equal(t, "/redfish/v1/Systems/valid-system-id/FirmwareInventory", collection.ODataID)
				assert.Equal(t, "#SoftwareInventoryCollection.SoftwareInventoryCollection", collection.ODataType)
				assert.Equal(t, "FirmwareInventory", collection.ID)
				assert.Equal(t, "Firmware Inventory Collection", collection.Name)
				assert.Greater(t, len(collection.Members), 0)
				assert.Equal(t, len(collection.Members), collection.MembersCount)

				// Check headers
				assert.Equal(t, "application/json; charset=utf-8", headers.Get("Content-Type"))
				assert.Equal(t, "4.0", headers.Get("OData-Version"))
				assert.Contains(t, headers.Get("ETag"), `W/"`)
				assert.Equal(t, "max-age=300", headers.Get("Cache-Control"))
			},
		},
		{
			name:     "GetVersion failure - system not found",
			systemID: "invalid-system-id",
			setupMocks: func(mockFeature *mocks.MockDeviceManagementFeature, mockLogger *mocks.MockLogger) {
				mockFeature.EXPECT().
					GetVersion(gomock.Any(), "invalid-system-id").
					Return(dto.Version{}, dtov2.Version{}, fmt.Errorf("system not found"))

				mockLogger.EXPECT().Error(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
				mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
			},
			expectedStatus: http.StatusNotFound,
			validateResponse: func(t *testing.T, body string, _ http.Header) {
				t.Helper()
				assert.Contains(t, body, "Base.1.11.0.ResourceNotFound")
				assert.Contains(t, body, "ComputerSystem")
				assert.Contains(t, body, "invalid-system-id")
			},
		},
		{
			name:     "GetHardwareInfo failure but GetVersion succeeds",
			systemID: "partial-system-id",
			setupMocks: func(mockFeature *mocks.MockDeviceManagementFeature, mockLogger *mocks.MockLogger) {
				mockFeature.EXPECT().
					GetVersion(gomock.Any(), "partial-system-id").
					Return(dto.Version{}, dtov2.Version{AMT: "15.0.25", Flash: "1.2.3"}, nil)

				mockFeature.EXPECT().
					GetHardwareInfo(gomock.Any(), "partial-system-id").
					Return(nil, fmt.Errorf("hardware info not available"))

				mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
				mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any()).Times(1)
			},
			expectedStatus:       http.StatusOK,
			expectedMembersCount: 2, // AMT + Flash, no BIOS
			validateResponse: func(t *testing.T, body string, _ http.Header) {
				t.Helper()

				var collection FirmwareInventoryCollection

				err := json.Unmarshal([]byte(body), &collection)
				require.NoError(t, err)

				assert.Equal(t, 2, collection.MembersCount)
				// Should not contain BIOS member when hardware info fails
				for _, member := range collection.Members {
					assert.NotContains(t, member.ODataID, "/BIOS")
				}
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

			// Setup routes
			systems := router.Group("/redfish/v1/Systems")
			NewFirmwareRoutes(systems, mockFeature, mockLogger)

			w := httptest.NewRecorder()
			req, _ := http.NewRequestWithContext(
				context.Background(),
				"GET",
				"/redfish/v1/Systems/"+tt.systemID+"/FirmwareInventory",
				http.NoBody,
			)

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.validateResponse != nil {
				tt.validateResponse(t, w.Body.String(), w.Header())
			}
		})
	}
}

func TestGetFirmwareInventoryInstanceHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		systemID         string
		firmwareID       string
		setupMocks       func(*mocks.MockDeviceManagementFeature, *mocks.MockLogger)
		expectedStatus   int
		validateResponse func(t *testing.T, body string, headers http.Header)
	}{
		{
			name:       "successful AMT firmware retrieval",
			systemID:   "test-system",
			firmwareID: "AMT",
			setupMocks: func(mockFeature *mocks.MockDeviceManagementFeature, mockLogger *mocks.MockLogger) {
				mockFeature.EXPECT().
					GetVersion(gomock.Any(), "test-system").
					Return(dto.Version{}, dtov2.Version{AMT: "15.0.25"}, nil)

				mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body string, _ http.Header) {
				t.Helper()

				var firmware FirmwareInventory

				err := json.Unmarshal([]byte(body), &firmware)
				require.NoError(t, err)

				assert.Equal(t, "AMT", firmware.ID)
				assert.Equal(t, "Intel Active Management Technology", firmware.Name)
				assert.Equal(t, "15.0.25", firmware.Version)
				assert.Equal(t, "Intel Corporation", firmware.Manufacturer)
				assert.Equal(t, "#SoftwareInventory.v1_3_0.SoftwareInventory", firmware.ODataType)
			},
		},
		{
			name:       "successful BIOS firmware retrieval",
			systemID:   "test-system",
			firmwareID: "BIOS",
			setupMocks: func(mockFeature *mocks.MockDeviceManagementFeature, mockLogger *mocks.MockLogger) {
				mockFeature.EXPECT().
					GetVersion(gomock.Any(), "test-system").
					Return(dto.Version{}, dtov2.Version{}, nil)

				hwInfo := map[string]interface{}{
					"CIM_BIOSElement": map[string]interface{}{
						"response": map[string]interface{}{
							"Version":      "DNKBLi7v.86A.0082.2024.0321.1028",
							"Manufacturer": "Intel Corp.",
							"ReleaseDate": map[string]interface{}{
								"DateTime": "2024-03-21T00:00:00Z",
							},
						},
					},
				}
				mockFeature.EXPECT().
					GetHardwareInfo(gomock.Any(), "test-system").
					Return(hwInfo, nil)

				mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body string, _ http.Header) {
				t.Helper()

				var firmware FirmwareInventory

				err := json.Unmarshal([]byte(body), &firmware)
				require.NoError(t, err)

				assert.Equal(t, "BIOS", firmware.ID)
				assert.Equal(t, "System BIOS/UEFI", firmware.Name)
				assert.Equal(t, "DNKBLi7v.86A.0082.2024.0321.1028", firmware.Version)
				assert.Equal(t, "Intel Corp.", firmware.Manufacturer)
				assert.Equal(t, "2024-03-21", firmware.ReleaseDate)
			},
		},
		{
			name:       "firmware not found",
			systemID:   "test-system",
			firmwareID: "NonExistent",
			setupMocks: func(mockFeature *mocks.MockDeviceManagementFeature, mockLogger *mocks.MockLogger) {
				mockFeature.EXPECT().
					GetVersion(gomock.Any(), "test-system").
					Return(dto.Version{}, dtov2.Version{}, nil)

				mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
			},
			expectedStatus: http.StatusNotFound,
			validateResponse: func(t *testing.T, body string, _ http.Header) {
				t.Helper()
				assert.Contains(t, body, "Base.1.11.0.ResourceNotFound")
				assert.Contains(t, body, "SoftwareInventory")
				assert.Contains(t, body, "NonExistent")
			},
		},
		{
			name:       "system not found",
			systemID:   "invalid-system",
			firmwareID: "AMT",
			setupMocks: func(mockFeature *mocks.MockDeviceManagementFeature, mockLogger *mocks.MockLogger) {
				mockFeature.EXPECT().
					GetVersion(gomock.Any(), "invalid-system").
					Return(dto.Version{}, dtov2.Version{}, fmt.Errorf("system not found"))

				mockLogger.EXPECT().Error(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
				mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
			},
			expectedStatus: http.StatusNotFound,
			validateResponse: func(t *testing.T, body string, _ http.Header) {
				t.Helper()
				assert.Contains(t, body, "Base.1.11.0.ResourceNotFound")
				assert.Contains(t, body, "ComputerSystem")
			},
		},
		{
			name:       "BIOS hardware info failure",
			systemID:   "test-system",
			firmwareID: "BIOS",
			setupMocks: func(mockFeature *mocks.MockDeviceManagementFeature, mockLogger *mocks.MockLogger) {
				mockFeature.EXPECT().
					GetVersion(gomock.Any(), "test-system").
					Return(dto.Version{}, dtov2.Version{}, nil)

				mockFeature.EXPECT().
					GetHardwareInfo(gomock.Any(), "test-system").
					Return(nil, fmt.Errorf("hardware info not available"))

				mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
				mockLogger.EXPECT().Error(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
			},
			expectedStatus: http.StatusNotFound,
			validateResponse: func(t *testing.T, body string, _ http.Header) {
				t.Helper()
				assert.Contains(t, body, "Base.1.11.0.ResourceNotFound")
				assert.Contains(t, body, "SoftwareInventory")
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

			// Setup routes
			systems := router.Group("/redfish/v1/Systems")
			NewFirmwareRoutes(systems, mockFeature, mockLogger)

			w := httptest.NewRecorder()
			req, _ := http.NewRequestWithContext(
				context.Background(),
				"GET",
				fmt.Sprintf("/redfish/v1/Systems/%s/FirmwareInventory/%s", tt.systemID, tt.firmwareID),
				http.NoBody,
			)

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.validateResponse != nil {
				tt.validateResponse(t, w.Body.String(), w.Header())
			}
		})
	}
}

func TestCreateFirmwareFunctions(t *testing.T) {
	t.Parallel()

	systemID := testSystemID

	t.Run("createAMTFirmware", func(t *testing.T) {
		t.Parallel()

		versionInfo := dtov2.Version{AMT: "15.0.25"}

		firmware := createAMTFirmware(systemID, versionInfo)
		require.NotNil(t, firmware)

		assert.Equal(t, "AMT", firmware.ID)
		assert.Equal(t, "Intel Active Management Technology", firmware.Name)
		assert.Equal(t, "15.0.25", firmware.Version)
		assert.Equal(t, "Intel Corporation", firmware.Manufacturer)
		assert.False(t, firmware.Updateable)
		assert.Equal(t, "Enabled", firmware.Status.State)
		assert.Equal(t, "OK", firmware.Status.Health)
	})

	t.Run("createAMTFirmware with empty version", func(t *testing.T) {
		t.Parallel()

		versionInfo := dtov2.Version{AMT: ""}

		firmware := createAMTFirmware(systemID, versionInfo)
		assert.Nil(t, firmware)
	})

	t.Run("createFlashFirmware", func(t *testing.T) {
		t.Parallel()

		versionInfo := dtov2.Version{Flash: "1.2.3"}

		firmware := createFlashFirmware(systemID, versionInfo)
		require.NotNil(t, firmware)

		assert.Equal(t, "Flash", firmware.ID)
		assert.Equal(t, "AMT Flash Firmware", firmware.Name)
		assert.Equal(t, "1.2.3", firmware.Version)
		assert.Equal(t, "Intel Corporation", firmware.Manufacturer)
	})

	t.Run("createNetstackFirmware", func(t *testing.T) {
		t.Parallel()

		versionInfo := dtov2.Version{Netstack: "2.3.4"}

		firmware := createNetstackFirmware(systemID, versionInfo)
		require.NotNil(t, firmware)

		assert.Equal(t, "Netstack", firmware.ID)
		assert.Equal(t, "AMT Network Stack", firmware.Name)
		assert.Equal(t, "2.3.4", firmware.Version)
		assert.Equal(t, "Intel Corporation", firmware.Manufacturer)
	})

	t.Run("createAMTAppsFirmware", func(t *testing.T) {
		t.Parallel()

		versionInfo := dtov2.Version{AMTApps: "3.4.5"}

		firmware := createAMTAppsFirmware(systemID, versionInfo)
		require.NotNil(t, firmware)

		assert.Equal(t, "AMTApps", firmware.ID)
		assert.Equal(t, "AMT Applications", firmware.Name)
		assert.Equal(t, "3.4.5", firmware.Version)
		assert.Equal(t, "Intel Corporation", firmware.Manufacturer)
	})

	t.Run("createBIOSFirmware", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockLogger := mocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()

		hwInfo := map[string]interface{}{
			"CIM_BIOSElement": map[string]interface{}{
				"response": map[string]interface{}{
					"Version":      "BIOS-1.0.0",
					"Manufacturer": "Test Corp.",
					"ReleaseDate": map[string]interface{}{
						"DateTime": "2024-01-15T00:00:00Z",
					},
				},
			},
		}

		firmware := createBIOSFirmware(systemID, hwInfo, mockLogger)
		require.NotNil(t, firmware)

		assert.Equal(t, "BIOS", firmware.ID)
		assert.Equal(t, "System BIOS/UEFI", firmware.Name)
		assert.Equal(t, "BIOS-1.0.0", firmware.Version)
		assert.Equal(t, "Test Corp.", firmware.Manufacturer)
		assert.Equal(t, "2024-01-15", firmware.ReleaseDate)
	})

	t.Run("createBIOSFirmware with nil hwInfo", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockLogger := mocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any()).Times(1)

		firmware := createBIOSFirmware(systemID, nil, mockLogger)
		assert.Nil(t, firmware)
	})
}

func TestHTTPMethodNotAllowedHandlers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{
			name:     "POST to collection not allowed",
			method:   "POST",
			path:     "/redfish/v1/Systems/test-id/FirmwareInventory",
			expected: http.StatusMethodNotAllowed,
		},
		{
			name:     "PUT to collection not allowed",
			method:   "PUT",
			path:     "/redfish/v1/Systems/test-id/FirmwareInventory",
			expected: http.StatusMethodNotAllowed,
		},
		{
			name:     "DELETE to collection not allowed",
			method:   "DELETE",
			path:     "/redfish/v1/Systems/test-id/FirmwareInventory",
			expected: http.StatusMethodNotAllowed,
		},
		{
			name:     "PATCH to collection not allowed",
			method:   "PATCH",
			path:     "/redfish/v1/Systems/test-id/FirmwareInventory",
			expected: http.StatusMethodNotAllowed,
		},
		{
			name:     "POST to instance not allowed",
			method:   "POST",
			path:     "/redfish/v1/Systems/test-id/FirmwareInventory/AMT",
			expected: http.StatusMethodNotAllowed,
		},
		{
			name:     "PUT to instance not allowed",
			method:   "PUT",
			path:     "/redfish/v1/Systems/test-id/FirmwareInventory/AMT",
			expected: http.StatusMethodNotAllowed,
		},
		{
			name:     "DELETE to instance not allowed",
			method:   "DELETE",
			path:     "/redfish/v1/Systems/test-id/FirmwareInventory/AMT",
			expected: http.StatusMethodNotAllowed,
		},
		{
			name:     "PATCH to instance not allowed",
			method:   "PATCH",
			path:     "/redfish/v1/Systems/test-id/FirmwareInventory/AMT",
			expected: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockFeature := mocks.NewMockDeviceManagementFeature(ctrl)
			mockLogger := mocks.NewMockLogger(ctrl)

			// Add logger expectation for route registration
			mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()

			gin.SetMode(gin.TestMode)
			router := gin.New()

			// Setup routes
			systems := router.Group("/redfish/v1/Systems")
			NewFirmwareRoutes(systems, mockFeature, mockLogger)

			w := httptest.NewRecorder()
			req, _ := http.NewRequestWithContext(
				context.Background(),
				tt.method,
				tt.path,
				http.NoBody,
			)

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expected, w.Code)

			// Check that error response is Redfish compliant
			body := w.Body.String()
			assert.Contains(t, body, "Base.1.11.0.OperationNotAllowed")
			assert.Contains(t, body, "@Message.ExtendedInfo")

			// Check Allow header for method not allowed
			assert.Equal(t, "GET", w.Header().Get("Allow"))
		})
	}
}

func TestAddFirmwareMembers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		versionInfo   dtov2.Version
		expectedCount int
		expectedPaths []string
	}{
		{
			name: "all firmware components present",
			versionInfo: dtov2.Version{
				AMT:      "15.0.25",
				Flash:    "1.2.3",
				Netstack: "2.3.4",
				AMTApps:  "3.4.5",
			},
			expectedCount: 4,
			expectedPaths: []string{
				"/redfish/v1/Systems/test-id/FirmwareInventory/AMT",
				"/redfish/v1/Systems/test-id/FirmwareInventory/Flash",
				"/redfish/v1/Systems/test-id/FirmwareInventory/Netstack",
				"/redfish/v1/Systems/test-id/FirmwareInventory/AMTApps",
			},
		},
		{
			name: "partial firmware components",
			versionInfo: dtov2.Version{
				AMT:   "15.0.25",
				Flash: "1.2.3",
			},
			expectedCount: 2,
			expectedPaths: []string{
				"/redfish/v1/Systems/test-id/FirmwareInventory/AMT",
				"/redfish/v1/Systems/test-id/FirmwareInventory/Flash",
			},
		},
		{
			name:          "no firmware components",
			versionInfo:   dtov2.Version{},
			expectedCount: 0,
			expectedPaths: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			collection := &FirmwareInventoryCollection{
				Members: []FirmwareInventoryMember{},
			}

			addFirmwareMembers(collection, "test-id", tt.versionInfo)

			assert.Equal(t, tt.expectedCount, len(collection.Members))

			actualPaths := make([]string, len(collection.Members))
			for i, member := range collection.Members {
				actualPaths[i] = member.ODataID
			}

			for _, expectedPath := range tt.expectedPaths {
				assert.Contains(t, actualPaths, expectedPath)
			}
		})
	}
}

func TestGetFirmwareItem(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockLogger := mocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any()).AnyTimes()

	versionInfo := dtov2.Version{
		AMT:      "15.0.25",
		Flash:    "1.2.3",
		Netstack: "2.3.4",
		AMTApps:  "3.4.5",
	}

	hwInfo := map[string]interface{}{
		"CIM_BIOSElement": map[string]interface{}{
			"response": map[string]interface{}{
				"Version": "BIOS-1.0.0",
			},
		},
	}

	tests := []struct {
		name       string
		firmwareID string
		expectNil  bool
		expectedID string
	}{
		{
			name:       "get AMT firmware",
			firmwareID: "AMT",
			expectNil:  false,
			expectedID: "AMT",
		},
		{
			name:       "get Flash firmware",
			firmwareID: "Flash",
			expectNil:  false,
			expectedID: "Flash",
		},
		{
			name:       "get Netstack firmware",
			firmwareID: "Netstack",
			expectNil:  false,
			expectedID: "Netstack",
		},
		{
			name:       "get AMTApps firmware",
			firmwareID: "AMTApps",
			expectNil:  false,
			expectedID: "AMTApps",
		},
		{
			name:       "get BIOS firmware",
			firmwareID: "BIOS",
			expectNil:  false,
			expectedID: "BIOS",
		},
		{
			name:       "get non-existent firmware",
			firmwareID: "NonExistent",
			expectNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := getFirmwareItem("test-system", tt.firmwareID, versionInfo, hwInfo, mockLogger)

			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedID, result.ID)
			}
		})
	}
}

func TestCreateAMTOemSection(t *testing.T) {
	t.Parallel()

	versionInfo := dtov2.Version{
		BuildNumber:      "123",
		AMTFWCoreVersion: "15.0.25.1234",
		SKU:              "16392",
		VendorID:         "8086",
	}

	result := createAMTOemSection(versionInfo, "test-system")

	require.Contains(t, result, "Intel")
	intel, ok := result["Intel"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, "#Intel.v1_0_0.Intel", intel["@odata.type"])
	assert.Equal(t, "AMT", intel["FirmwareType"])
	assert.Equal(t, "123", intel["BuildNumber"])
	assert.Equal(t, "15.0.25.1234", intel["AMTFWCore"])
	assert.Equal(t, "16392", intel["SKU"])
	assert.Equal(t, "8086", intel["VendorID"])
}
