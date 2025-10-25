/*********************************************************************
 * Copyright (c) Intel Corporation 2025
 * SPDX-License-Identifier: Apache-2.0
 **********************************************************************/

// Package v1 implements Redfish API v1 constants and shared values.
package v1

// Task state constants for Redfish Task responses
const (
	// TaskStateCompleted indicates the task has completed successfully
	TaskStateCompleted = "Completed"
	// TaskStateException indicates the task encountered an error
	TaskStateException = "Exception"
	// TaskStateRunning indicates the task is currently running
	TaskStateRunning = "Running"
	// TaskStatePending indicates the task is pending execution
	TaskStatePending = "Pending"
)

// Task status constants for Redfish Task responses
const (
	// TaskStatusOK indicates successful completion
	TaskStatusOK = "OK"
	// TaskStatusCritical indicates a critical error occurred
	TaskStatusCritical = "Critical"
	// TaskStatusWarning indicates a warning condition
	TaskStatusWarning = "Warning"
)

// Redfish Base Message Registry constants - used across multiple files
const (
	BaseSuccessMessageID         = "Base.1.11.0.Success"
	BaseErrorMessageID           = "Base.1.11.0.GeneralError"
	BaseMalformedJSONID          = "Base.1.11.0.MalformedJSON"
	BasePropertyMissingID        = "Base.1.11.0.PropertyMissing"
	BasePropertyValueNotInListID = "Base.1.11.0.PropertyValueNotInList"
	BaseResourceNotFoundID       = "Base.1.11.0.ResourceNotFound"
	BaseOperationNotAllowedID    = "Base.1.11.0.OperationNotAllowed"
	BaseActionNotSupportedID     = "Base.1.11.0.ActionNotSupported"
	BaseNoValidSessionID         = "Base.1.11.0.NoValidSession"
	BaseInsufficientPrivilegeID  = "Base.1.11.0.InsufficientPrivilege"
	BaseNotAcceptableID          = "Base.1.11.0.NotAcceptable"
)

// HTTP Header constants for Redfish compliance
const (
	ContentTypeJSON       = "application/json; charset=utf-8"
	ContentTypeHeaderName = "Content-Type"
	ODataVersionValue     = "4.0"
	ODataVersionHeader    = "OData-Version"
	CacheControlValue     = "no-cache"
	CacheControlHeader    = "Cache-Control"
	XFrameOptionsHeader   = "X-Frame-Options"
	XFrameOptionsValue    = "DENY"
	CSPHeader             = "Content-Security-Policy"
	CSPValue              = "default-src 'self'"
)

// Additional HTTP constants
const (
	MediaTypeWildcard = "*/*"
	HeaderAccept      = "Accept"
)

// Redfish Service Information
const (
	RedfishVersion     = "1.11.0"
	ServiceRootID      = "RootService"
	ServiceRootName    = "Redfish Root Service"
	ServiceProduct     = "Device Management Toolkit Console"
	ServiceVendor      = "Intel Corporation"
	DefaultServiceUUID = "550e8400-e29b-41d4-a716-446655440000"
)

// Common Redfish Schema Types - used across multiple files
const (
	SchemaServiceRoot                 = "#ServiceRoot.v1_11_0.ServiceRoot"
	SchemaSessionService              = "#SessionService.v1_0_0.SessionService"
	SchemaSessionCollection           = "#SessionCollection.SessionCollection"
	SchemaComputerSystem              = "#ComputerSystem.v1_0_0.ComputerSystem"
	SchemaComputerSystemCollection    = "#ComputerSystemCollection.ComputerSystemCollection"
	SchemaSoftwareInventory           = "#SoftwareInventory.v1_3_0.SoftwareInventory"
	SchemaSoftwareInventoryCollection = "#SoftwareInventoryCollection.SoftwareInventoryCollection"
	SchemaTask                        = "#Task.v1_6_0.Task"
	SchemaIntelOEM                    = "#Intel.v1_0_0.Intel"
)

// Common Redfish API Paths - used across multiple files
const (
	PathRedfishRoot            = "/redfish/v1/"
	PathSystems                = PathRedfishRoot + "Systems"
	PathSessionService         = PathRedfishRoot + "SessionService"
	PathSessionServiceSessions = PathSessionService + "/Sessions"
	PathTaskService            = PathRedfishRoot + "TaskService/Tasks"
	PathMetadata               = PathRedfishRoot + "$metadata"
)

// Common Redfish API Path patterns - for building dynamic paths
const (
	// System-specific paths
	PathSystemInstance     = PathSystems + "/"               // /redfish/v1/Systems/
	PathSystemActions      = "/Actions/ComputerSystem.Reset" // Appended to system instance
	PathSystemFirmware     = "/FirmwareInventory"            // Appended to system instance
	PathSystemFirmwareItem = PathSystemFirmware + "/"        // /FirmwareInventory/
)

// BuildSystemPath builds a path to a specific system: /redfish/v1/Systems/{systemID}
func BuildSystemPath(systemID string) string {
	return PathSystemInstance + systemID
}

// BuildSystemFirmwarePath builds a path to system firmware inventory: /redfish/v1/Systems/{systemID}/FirmwareInventory
func BuildSystemFirmwarePath(systemID string) string {
	return PathSystemInstance + systemID + PathSystemFirmware
}

// BuildSystemFirmwareItemPath builds a path to a specific firmware item: /redfish/v1/Systems/{systemID}/FirmwareInventory/{itemID}
func BuildSystemFirmwareItemPath(systemID, itemID string) string {
	return PathSystemInstance + systemID + PathSystemFirmwareItem + itemID
}

// OData Context paths for metadata
const (
	ODataContextTask                        = PathMetadata + "#Task.Task"
	ODataContextSoftwareInventory           = PathMetadata + "#SoftwareInventory.SoftwareInventory"
	ODataContextSoftwareInventoryCollection = PathMetadata + "#SoftwareInventoryCollection.SoftwareInventoryCollection"
)

// Cache control values
const (
	CacheMaxAge5Min = "max-age=300" // 5 minutes cache
)

// Common value constants
const (
	UnknownValue = "Unknown" // Default value for unknown/missing information
)
