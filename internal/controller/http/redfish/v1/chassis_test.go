/*********************************************************************
 * Copyright (c) Intel Corporation 2025
 * SPDX-License-Identifier: Apache-2.0
 **********************************************************************/

// Package v1 implements Redfish API v1 Chassis resources tests.
package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/device-management-toolkit/console/internal/entity/dto/v1"
	"github.com/device-management-toolkit/console/internal/mocks"
)

func TestNewChassisRoutes(t *testing.T) {
	t.Parallel()

	t.Run("registers chassis routes correctly", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		gin.SetMode(gin.TestMode)
		router := gin.New()
		redfishGroup := router.Group("/redfish/v1")

		mockFeature := mocks.NewMockDeviceManagementFeature(ctrl)
		mockLogger := mocks.NewMockLogger(ctrl)

		mockLogger.EXPECT().
			Info("Registered Redfish v1 Chassis routes")

		// This should not panic
		require.NotPanics(t, func() {
			NewChassisRoutes(redfishGroup, mockFeature, mockLogger)
		})
	})
}

func TestGetChassisCollectionHandler(t *testing.T) {
	t.Parallel()

	t.Run("successful chassis collection retrieval", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		gin.SetMode(gin.TestMode)
		router := gin.New()

		mockFeature := mocks.NewMockDeviceManagementFeature(ctrl)
		mockLogger := mocks.NewMockLogger(ctrl)

		devices := []dto.Device{
			{GUID: "device-1", Hostname: "server-1"},
			{GUID: "device-2", Hostname: "server-2"},
		}

		mockFeature.EXPECT().
			Get(gomock.Any(), maxChassisList, 0, "").
			Return(devices, nil)

		mockLogger.EXPECT().
			Info("Registered Redfish v1 Chassis routes")

		NewChassisRoutes(router.Group("/redfish/v1"), mockFeature, mockLogger)

		req, _ := http.NewRequestWithContext(context.Background(), "GET", "/redfish/v1/Chassis", http.NoBody)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))

		var collection ChassisCollection

		err := json.Unmarshal(w.Body.Bytes(), &collection)
		require.NoError(t, err)

		assert.Equal(t, "/redfish/v1/Chassis", collection.ODataID)
		assert.Equal(t, "#ChassisCollection.ChassisCollection", collection.ODataType)
		assert.Equal(t, "ChassisCollection", collection.ID)
		assert.Equal(t, 2, collection.MembersCount)
		assert.Len(t, collection.Members, 2)
	})

	t.Run("handles service error", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		gin.SetMode(gin.TestMode)
		router := gin.New()

		mockFeature := mocks.NewMockDeviceManagementFeature(ctrl)
		mockLogger := mocks.NewMockLogger(ctrl)

		expectedErr := fmt.Errorf("service unavailable")
		mockFeature.EXPECT().
			Get(gomock.Any(), maxChassisList, 0, "").
			Return(nil, expectedErr)

		mockLogger.EXPECT().
			Error(expectedErr, "http - redfish - Chassis collection")

		mockLogger.EXPECT().
			Info("Registered Redfish v1 Chassis routes")

		NewChassisRoutes(router.Group("/redfish/v1"), mockFeature, mockLogger)

		req, _ := http.NewRequestWithContext(context.Background(), "GET", "/redfish/v1/Chassis", http.NoBody)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestGetChassisInstanceHandler(t *testing.T) {
	t.Parallel()

	t.Run("successful chassis instance retrieval", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		gin.SetMode(gin.TestMode)
		router := gin.New()

		mockFeature := mocks.NewMockDeviceManagementFeature(ctrl)
		mockLogger := mocks.NewMockLogger(ctrl)

		hwInfo := map[string]interface{}{
			"CIM_Chassis": map[string]interface{}{
				"response": []interface{}{
					map[string]interface{}{
						"Manufacturer": "Dell Inc.",
						"Model":        "PowerEdge R640",
						"SerialNumber": "ABC123456",
						"PackageType":  "3",
					},
				},
			},
		}

		mockFeature.EXPECT().
			GetHardwareInfo(gomock.Any(), "test-chassis-id").
			Return(hwInfo, nil)

		mockLogger.EXPECT().
			Info("Registered Redfish v1 Chassis routes")

		NewChassisRoutes(router.Group("/redfish/v1"), mockFeature, mockLogger)

		req, _ := http.NewRequestWithContext(context.Background(), "GET", "/redfish/v1/Chassis/test-chassis-id", http.NoBody)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var chassis Chassis

		err := json.Unmarshal(w.Body.Bytes(), &chassis)
		require.NoError(t, err)

		assert.Equal(t, "test-chassis-id", chassis.ID)
		assert.Equal(t, "Dell Inc.", chassis.Manufacturer)
		assert.Equal(t, "PowerEdge R640", chassis.Model)
		assert.Equal(t, chassisTypeRackMount, chassis.ChassisType)
	})
}

func TestParseChassisInfo(t *testing.T) {
	t.Parallel()

	t.Run("parse complete chassis info", func(t *testing.T) {
		t.Parallel()

		hwInfo := map[string]interface{}{
			"CIM_Chassis": map[string]interface{}{
				"response": []interface{}{
					map[string]interface{}{
						"Manufacturer": "HPE",
						"Model":        "ProLiant DL380",
						"SerialNumber": "SGH123XYZ",
						"PackageType":  4.0,
					},
				},
			},
		}

		chassisInfo, err := parseChassisInfo(hwInfo)
		require.NoError(t, err)
		require.NotNil(t, chassisInfo)

		assert.Equal(t, "ProLiant DL380 Chassis", chassisInfo.Name)
		assert.Equal(t, "HPE", chassisInfo.Manufacturer)
		assert.Equal(t, "ProLiant DL380", chassisInfo.Model)
		assert.Equal(t, chassisTypeRackMount, chassisInfo.ChassisType)
	})

	t.Run("handle nil hardware info", func(t *testing.T) {
		t.Parallel()

		chassisInfo, err := parseChassisInfo(nil)
		require.NoError(t, err)
		require.NotNil(t, chassisInfo)

		assert.Equal(t, "System Chassis", chassisInfo.Name)
		assert.Equal(t, chassisTypeUnknown, chassisInfo.ChassisType)
		assert.Equal(t, unknownValue, chassisInfo.Manufacturer)
	})
}

func TestMapPackageTypeToChassisType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		packageType interface{}
		expected    string
	}{
		{"rack string", "rack", chassisTypeRackMount},
		{"desktop string", "desktop", chassisTypeDesktop},
		{"numeric rack 3", 3.0, chassisTypeRackMount},
		{"numeric desktop 2", 2.0, chassisTypeDesktop},
		{"unknown", "unknown", chassisTypeUnknown},
		{"nil value", nil, chassisTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := mapPackageTypeToChassisType(tt.packageType)
			assert.Equal(t, tt.expected, result)
		})
	}
}
