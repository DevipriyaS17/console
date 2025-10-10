/*********************************************************************
 * Copyright (c) Intel Corporation 2021
 * SPDX-License-Identifier: Apache-2.0
 **********************************************************************/

package v1

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/device-management-toolkit/console/config"
	"github.com/device-management-toolkit/console/pkg/logger"
)

// generateServiceUUID generates a UUID for the Redfish service root
func generateServiceUUID() string {
	// Generate a proper UUID v4 for production use
	uuid := make([]byte, 16)
	_, err := rand.Read(uuid)
	if err != nil {
		// Fallback to a static UUID if random generation fails
		return "550e8400-e29b-41d4-a716-446655440000"
	}

	// Set version (4) and variant bits
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant 10

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}

// Future extensibility functions for upstream service health checks
// These are placeholder functions that could be implemented to check external dependencies

// isDatabaseReachable checks if the database is accessible
// Returns false if database is unreachable (would trigger 502)
func isDatabaseReachable() bool {
	// Example: ping database, check connection pool, etc.
	// if db.Ping() != nil { return false }
	return true // Currently always returns true since we use embedded DB
}

// areSystemsReachable checks if managed systems/devices are accessible
// Returns false if critical systems are unreachable (would trigger 502)
func areSystemsReachable() bool {
	// Example: check connectivity to Intel AMT devices, network accessibility
	// if !canReachManagedDevices() { return false }
	return true // Currently always returns true - no external systems required for service root
}

// isExternalServiceHealthy checks if external dependencies are available
// Returns false if required external services are down (would trigger 502)
func isExternalServiceHealthy() bool {
	// Example: check EA (Engine Activation) service, external APIs, etc.
	// if !eaService.IsHealthy() { return false }
	return true // Currently always returns true - service root is self-contained
}

// Future extensibility functions for temporary service unavailability (503 errors)

// isServiceOverloaded checks if the service is temporarily overloaded
// Returns true if too many concurrent requests are being processed (would trigger 503)
func isServiceOverloaded() bool {
	// Example: check active connection count, request queue depth, CPU/memory usage
	// if activeConnections > maxConcurrentConnections { return true }
	// if requestQueueDepth > threshold { return true }
	return false // Currently always returns false - no overload detection implemented
}

// isMaintenanceMode checks if the service is in maintenance mode
// Returns true if planned maintenance is active (would trigger 503)
func isMaintenanceMode() bool {
	// Example: check maintenance flag file, config setting, scheduled maintenance window
	// if maintenanceFlag.Exists() { return true }
	// if time.Now().In(maintenanceWindow) { return true }
	return false // Currently always returns false - no maintenance mode implemented
}

// hasResourceExhaustion checks if system resources are exhausted
// Returns true if insufficient resources are available (would trigger 503)
func hasResourceExhaustion() bool {
	// Example: check memory usage, disk space, file descriptors, database connections
	// if memoryUsage > criticalThreshold { return true }
	// if diskSpace < minRequired { return true }
	// if dbConnPool.Available() == 0 { return true }
	return false // Currently always returns false - no resource monitoring implemented
}

// NewServiceRootRoutes registers Redfish API v1 service root routes
func NewServiceRootRoutes(r *gin.RouterGroup, cfg *config.Config, l logger.Interface) {
	// Apply Redfish-compliant recovery middleware for 500 errors
	r.Use(RedfishRecoveryMiddleware())

	// Apply Redfish-compliant authentication if auth is enabled
	if !cfg.Disabled {
		r.Use(RedfishJWTAuthMiddleware(cfg))
	}

	// Redfish Service Root (main entry point)
	r.GET("/", func(c *gin.Context) {
		// Set Redfish-compliant headers
		SetRedfishHeaders(c)

		// Validate Accept header (406 Not Acceptable)
		acceptHeader := c.GetHeader("Accept")
		if acceptHeader != "" && acceptHeader != "*/*" && acceptHeader != "application/json" &&
			!strings.Contains(acceptHeader, "application/json") && !strings.Contains(acceptHeader, "*/*") {
			NotAcceptableError(c, acceptHeader)
			return
		}

		// Generate service UUID with error handling
		serviceUUID := generateServiceUUID()
		if serviceUUID == "" {
			// If UUID generation completely fails, this could indicate system issues
			BadGatewayError(c)
			return
		}

		// Future extensibility: Health checks for upstream services that could trigger 502 errors
		if !isDatabaseReachable() {
			// Database unreachable - service cannot function properly
			BadGatewayError(c)
			return
		}

		if !areSystemsReachable() {
			// Critical managed systems unreachable - service degraded
			BadGatewayError(c)
			return
		}

		if !isExternalServiceHealthy() {
			// External dependencies unavailable - service cannot provide full functionality
			BadGatewayError(c)
			return
		}

		// Check for temporary service unavailability conditions (503 errors)
		if isServiceOverloaded() {
			// Service is temporarily overloaded - too many concurrent requests
			ServiceTemporarilyUnavailableError(c)
			return
		}

		if isMaintenanceMode() {
			// Service is in maintenance mode - temporarily unavailable
			ServiceTemporarilyUnavailableError(c)
			return
		}

		if hasResourceExhaustion() {
			// Service has insufficient resources - temporarily unavailable
			ServiceTemporarilyUnavailableError(c)
			return
		}

		payload := map[string]any{
			"@odata.type":    "#ServiceRoot.v1_0_0.ServiceRoot",
			"@odata.id":      "/redfish/v1/",
			"Id":             "RootService",
			"Name":           "Redfish Root Service",
			"RedfishVersion": "1.11.0",
			"UUID":           serviceUUID,
			"Systems":        map[string]any{"@odata.id": "/redfish/v1/Systems"},
			"SessionService": map[string]any{"@odata.id": "/redfish/v1/SessionService"},
			// Mandatory Links property (DMTF requirement) - typically empty for ServiceRoot
			"Links": map[string]any{},
			// Optional but recommended properties
			"Product": "Device Management Toolkit Console",
			"Vendor":  "Intel Corporation",
		}

		c.JSON(http.StatusOK, payload)
	})

	// Handle unsupported methods on ServiceRoot with proper 405 responses
	r.POST("/", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, "POST", "ServiceRoot", "GET")
	})
	r.PUT("/", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, "PUT", "ServiceRoot", "GET")
	})
	r.PATCH("/", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, "PATCH", "ServiceRoot", "GET")
	})
	r.DELETE("/", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, "DELETE", "ServiceRoot", "GET")
	})

	// Handle unsupported methods on Systems collection with proper 405 responses
	r.POST("/Systems", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, "POST", "ComputerSystemCollection", "GET")
	})
	r.PUT("/Systems", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, "PUT", "ComputerSystemCollection", "GET")
	})
	r.PATCH("/Systems", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, "PATCH", "ComputerSystemCollection", "GET")
	})
	r.DELETE("/Systems", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, "DELETE", "ComputerSystemCollection", "GET")
	}) // OData Service Document
	r.GET("/$metadata", func(c *gin.Context) {
		c.Header("Content-Type", "application/xml")
		c.String(http.StatusOK, `<?xml version="1.0" encoding="UTF-8"?>
<edmx:Edmx xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx" Version="4.0">
	<edmx:DataServices>
		<Schema Namespace="Redfish" xmlns="http://docs.oasis-open.org/odata/ns/edm">
			<EntityType Name="ServiceRoot">
				<Key><PropertyRef Name="Id"/></Key>
				<Property Name="Id" Type="Edm.String" Nullable="false"/>
				<Property Name="Name" Type="Edm.String"/>
				<Property Name="RedfishVersion" Type="Edm.String"/>
				<NavigationProperty Name="SessionService" Type="Redfish.SessionService"/>
				<NavigationProperty Name="Systems" Type="Collection(Redfish.ComputerSystem)"/>
			</EntityType>
			<EntityType Name="SessionService">
				<Key><PropertyRef Name="Id"/></Key>
				<Property Name="Id" Type="Edm.String" Nullable="false"/>
				<Property Name="Name" Type="Edm.String"/>
				<Property Name="ServiceEnabled" Type="Edm.Boolean"/>
				<Property Name="SessionTimeout" Type="Edm.Int64"/>
				<NavigationProperty Name="Sessions" Type="Collection(Redfish.Session)"/>
			</EntityType>
			<EntityType Name="Session">
				<Key><PropertyRef Name="Id"/></Key>
				<Property Name="Id" Type="Edm.String" Nullable="false"/>
				<Property Name="Name" Type="Edm.String"/>
				<Property Name="UserName" Type="Edm.String"/>
			</EntityType>
			<EntityType Name="ComputerSystem">
				<Key><PropertyRef Name="Id"/></Key>
				<Property Name="Id" Type="Edm.String" Nullable="false"/>
				<Property Name="Name" Type="Edm.String"/>
				<Property Name="PowerState" Type="Edm.String"/>
			</EntityType>
			<EntityContainer Name="Service">
				<EntitySet Name="ServiceRoot" EntityType="Redfish.ServiceRoot"/>
				<EntitySet Name="SessionService" EntityType="Redfish.SessionService"/>
				<EntitySet Name="Sessions" EntityType="Redfish.Session"/>
				<EntitySet Name="Systems" EntityType="Redfish.ComputerSystem"/>
			</EntityContainer>
		</Schema>
	</edmx:DataServices>
</edmx:Edmx>`)
	})

	// SessionService endpoint
	r.GET("/SessionService", func(c *gin.Context) {
		// Set Redfish-compliant headers
		SetRedfishHeaders(c)

		payload := map[string]any{
			"@odata.type":    "#SessionService.v1_0_0.SessionService",
			"@odata.id":      "/redfish/v1/SessionService",
			"Id":             "SessionService",
			"Name":           "Redfish Session Service",
			"ServiceEnabled": true,
			"SessionTimeout": 30,
			"Sessions":       map[string]any{"@odata.id": "/redfish/v1/SessionService/Sessions"},
		}

		c.JSON(http.StatusOK, payload)
	})

	// Handle unsupported methods on SessionService with proper 405 responses
	r.POST("/SessionService", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, "POST", "SessionService", "GET")
	})
	r.PUT("/SessionService", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, "PUT", "SessionService", "GET")
	})
	r.PATCH("/SessionService", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, "PATCH", "SessionService", "GET")
	})
	r.DELETE("/SessionService", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, "DELETE", "SessionService", "GET")
	})

	// Sessions collection endpoint (read-only, empty list for now)
	r.GET("/SessionService/Sessions", func(c *gin.Context) {
		// Set Redfish-compliant headers
		SetRedfishHeaders(c)

		payload := map[string]any{
			"@odata.type":         "#SessionCollection.SessionCollection",
			"@odata.id":           "/redfish/v1/SessionService/Sessions",
			"Name":                "Session Collection",
			"Members@odata.count": 0,
			"Members":             []any{},
		}

		c.JSON(http.StatusOK, payload)
	})

	// Handle unsupported methods on Sessions collection with proper 405 responses
	r.PUT("/SessionService/Sessions", func(c *gin.Context) {
		MethodNotAllowedError(c, "retrieve sessions collection", "GET, POST")
	})
	r.PATCH("/SessionService/Sessions", func(c *gin.Context) {
		MethodNotAllowedError(c, "retrieve sessions collection", "GET, POST")
	})
	r.DELETE("/SessionService/Sessions", func(c *gin.Context) {
		MethodNotAllowedError(c, "retrieve sessions collection", "GET, POST")
	})

	l.Info("Registered Redfish v1 Service Root at %s", r.BasePath())
}
