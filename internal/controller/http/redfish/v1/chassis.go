/*********************************************************************
 * Copyright (c) Intel Corporation 2025
 * SPDX-License-Identifier: Apache-2.0
 **********************************************************************/

// Package v1 implements Redfish API v1 Chassis resources.
package v1

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/device-management-toolkit/console/internal/entity/dto/v1"
	"github.com/device-management-toolkit/console/internal/usecase/devices"
	"github.com/device-management-toolkit/console/pkg/logger"
)

// Chassis-related constants
const (
	maxChassisList       = 1000
	maxRequestSize       = 1024 * 1024 // 1MB max request size
	chassisTypeRackMount = "RackMount"
	chassisTypeDesktop   = "Desktop"
	chassisTypeUnknown   = "Unknown"
	acceptAll            = "*/*"
	contentTypeJSON      = "application/json"
	oneMegabyte          = "1MB"
	methodGET            = "GET"
	methodHEAD           = "HEAD"
	methodPOST           = "POST"
	methodPUT            = "PUT"
	methodPATCH          = "PATCH"
	methodDELETE         = "DELETE"
)

// ChassisCollection represents a Redfish Chassis collection
type ChassisCollection struct {
	ODataContext string             `json:"@odata.context"`
	ODataID      string             `json:"@odata.id"`
	ODataType    string             `json:"@odata.type"`
	ODataEtag    string             `json:"@odata.etag,omitempty"`
	ID           string             `json:"Id"`
	Name         string             `json:"Name"`
	Description  string             `json:"Description"`
	MembersCount int                `json:"Members@odata.count"`
	Members      []ChassisReference `json:"Members"`
}

// ChassisReference represents a reference to a chassis instance
type ChassisReference struct {
	ODataID string `json:"@odata.id"`
}

// Chassis represents a Redfish Chassis instance
type Chassis struct {
	ODataContext     string            `json:"@odata.context"`
	ODataID          string            `json:"@odata.id"`
	ODataType        string            `json:"@odata.type"`
	ODataEtag        string            `json:"@odata.etag,omitempty"`
	ID               string            `json:"Id"`
	Name             string            `json:"Name"`
	Description      string            `json:"Description,omitempty"`
	ChassisType      string            `json:"ChassisType"`
	Manufacturer     string            `json:"Manufacturer,omitempty"`
	Model            string            `json:"Model,omitempty"`
	SKU              string            `json:"SKU,omitempty"`
	SerialNumber     string            `json:"SerialNumber,omitempty"`
	PartNumber       string            `json:"PartNumber,omitempty"`
	AssetTag         string            `json:"AssetTag,omitempty"`
	Status           ChassisStatus     `json:"Status"`
	PowerState       string            `json:"PowerState,omitempty"`
	IndicatorLED     string            `json:"IndicatorLED,omitempty"`
	Location         *Location         `json:"Location,omitempty"`
	Links            *ChassisLinks     `json:"Links,omitempty"`
	PhysicalSecurity *PhysicalSecurity `json:"PhysicalSecurity,omitempty"`
}

// ChassisStatus represents the status of a chassis
type ChassisStatus struct {
	State  string `json:"State"`
	Health string `json:"Health"`
}

// Location represents the physical location of a chassis
type Location struct {
	Info          string         `json:"Info,omitempty"`
	InfoFormat    string         `json:"InfoFormat,omitempty"`
	PostalAddress *PostalAddress `json:"PostalAddress,omitempty"`
	Placement     *Placement     `json:"Placement,omitempty"`
}

// PostalAddress represents a postal address
type PostalAddress struct {
	Country    string `json:"Country,omitempty"`
	Territory  string `json:"Territory,omitempty"`
	City       string `json:"City,omitempty"`
	Street     string `json:"Street,omitempty"`
	PostalCode string `json:"PostalCode,omitempty"`
	Building   string `json:"Building,omitempty"`
	Room       string `json:"Room,omitempty"`
}

// Placement represents the placement within a larger structure
type Placement struct {
	Row        string `json:"Row,omitempty"`
	Rack       string `json:"Rack,omitempty"`
	RackOffset int    `json:"RackOffset,omitempty"`
}

// ChassisLinks represents links to other resources
type ChassisLinks struct {
	ComputerSystems      []ResourceReference `json:"ComputerSystems,omitempty"`
	ComputerSystemsCount int                 `json:"ComputerSystems@odata.count,omitempty"`
	ContainedBy          *ResourceReference  `json:"ContainedBy,omitempty"`
	Contains             []ResourceReference `json:"Contains,omitempty"`
	ContainsCount        int                 `json:"Contains@odata.count,omitempty"`
	ManagedBy            []ResourceReference `json:"ManagedBy,omitempty"`
	ManagedByCount       int                 `json:"ManagedBy@odata.count,omitempty"`
}

// ResourceReference represents a reference to another resource
type ResourceReference struct {
	ODataID string `json:"@odata.id"`
}

// PhysicalSecurity represents physical security status
type PhysicalSecurity struct {
	IntrusionSensor       string `json:"IntrusionSensor,omitempty"`
	IntrusionSensorNumber int    `json:"IntrusionSensorNumber,omitempty"`
	IntrusionSensorReArm  string `json:"IntrusionSensorReArm,omitempty"`
}

// validateChassisRequest performs common request validations for chassis endpoints
func validateChassisRequest(c *gin.Context) bool {
	if !isAcceptHeaderValid(c) {
		return false
	}

	if !isContentTypeValidForNonGETMethods(c) {
		return false
	}

	return true
}

// isAcceptHeaderValid validates the Accept header
func isAcceptHeaderValid(c *gin.Context) bool {
	acceptHeader := c.GetHeader("Accept")
	if acceptHeader != "" && acceptHeader != acceptAll && acceptHeader != contentTypeJSON &&
		!strings.Contains(acceptHeader, contentTypeJSON) && !strings.Contains(acceptHeader, acceptAll) {
		NotAcceptableError(c, acceptHeader)

		return false
	}

	return true
}

// isContentTypeValidForNonGETMethods validates Content-Type for non-GET methods
func isContentTypeValidForNonGETMethods(c *gin.Context) bool {
	if c.Request.Method != methodGET && c.Request.Method != methodHEAD {
		if !isContentTypeValid(c) {
			return false
		}

		if !isRequestSizeValid(c) {
			return false
		}
	}

	return true
}

// isContentTypeValid checks if the Content-Type header is valid
func isContentTypeValid(c *gin.Context) bool {
	contentType := c.GetHeader("Content-Type")
	if contentType != "" && contentType != contentTypeJSON &&
		!strings.HasPrefix(contentType, contentTypeJSON) {
		UnsupportedMediaTypeError(c, contentType)

		return false
	}

	return true
}

// isRequestSizeValid checks if the request size is within limits
func isRequestSizeValid(c *gin.Context) bool {
	if c.Request.ContentLength > maxRequestSize {
		RequestEntityTooLargeError(c, oneMegabyte)

		return false
	}

	return true
}

// validateETag checks if the provided ETag matches the current resource ETag
func validateETag(c *gin.Context, currentETag string) bool {
	if !validateIfMatchHeader(c, currentETag) {
		return false
	}

	if !validateIfNoneMatchHeader(c, currentETag) {
		return false
	}

	return true
}

// validateIfMatchHeader validates the If-Match header
func validateIfMatchHeader(c *gin.Context, currentETag string) bool {
	ifMatch := c.GetHeader("If-Match")
	if ifMatch == "" {
		return true
	}

	// Remove quotes if present
	ifMatch = strings.Trim(ifMatch, `"`)
	currentETag = strings.Trim(currentETag, `"`)

	if ifMatch != "*" && ifMatch != currentETag {
		PreconditionFailedError(c)

		return false
	}

	return true
}

// validateIfNoneMatchHeader validates the If-None-Match header
func validateIfNoneMatchHeader(c *gin.Context, currentETag string) bool {
	ifNoneMatch := c.GetHeader("If-None-Match")
	if ifNoneMatch == "" || c.Request.Method != methodGET {
		return true
	}

	ifNoneMatch = strings.Trim(ifNoneMatch, `"`)
	currentETag = strings.Trim(currentETag, `"`)

	if ifNoneMatch == "*" || ifNoneMatch == currentETag {
		c.Status(http.StatusNotModified)

		return false
	}

	return true
}

// NewChassisRoutes registers Chassis routes with the router
func NewChassisRoutes(r *gin.RouterGroup, d devices.Feature, l logger.Interface) {
	r.GET("/Chassis", getChassisCollectionHandler(d, l))
	r.GET("/Chassis/:chassisId", getChassisInstanceHandler(d, l))
	r.PATCH("/Chassis/:chassisId", patchChassisInstanceHandler(d, l))

	// Handle unsupported methods on Chassis collection
	r.POST("/Chassis", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, methodPOST, "Chassis", methodGET)
	})
	r.PUT("/Chassis", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, methodPUT, "Chassis", methodGET)
	})
	r.PATCH("/Chassis", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, methodPATCH, "Chassis", methodGET)
	})
	r.DELETE("/Chassis", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, methodDELETE, "Chassis", methodGET)
	})

	// Handle unsupported methods on Chassis instance
	r.POST("/Chassis/:chassisId", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, methodPOST, "Chassis instance", methodGET+", "+methodPATCH)
	})
	r.PUT("/Chassis/:chassisId", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, methodPUT, "Chassis instance", methodGET+", "+methodPATCH)
	})
	r.DELETE("/Chassis/:chassisId", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, methodDELETE, "Chassis instance", methodGET+", "+methodPATCH)
	})

	l.Info("Registered Redfish v1 Chassis routes")
}

// patchChassisInstanceHandler handles PATCH requests for chassis configuration
func patchChassisInstanceHandler(d devices.Feature, l logger.Interface) gin.HandlerFunc {
	return func(c *gin.Context) {
		SetRedfishHeaders(c)

		// Perform common request validations
		if !validateChassisRequest(c) {
			return
		}

		chassisID := c.Param("chassisId")
		if chassisID == "" {
			ResourceNotFoundError(c, "Chassis", "")

			return
		}

		// Validate that the chassis exists first
		_, err := d.GetHardwareInfo(c.Request.Context(), chassisID)
		if err != nil {
			l.Error(err, "http - redfish - Chassis PATCH - hardware info lookup", "chassisId", chassisID)
			ResourceNotFoundError(c, "Chassis", chassisID)

			return
		}

		// Parse request body to check what properties are being modified
		var patchRequest map[string]interface{}
		if err := c.ShouldBindJSON(&patchRequest); err != nil {
			MalformedJSONError(c)

			return
		}

		// Check if any read-only properties are being modified
		readOnlyProperties := []string{
			"Id", "Name", "@odata.context", "@odata.id", "@odata.type", "@odata.etag",
			"ChassisType", "Manufacturer", "Model", "SKU", "SerialNumber", "PartNumber",
			"Status", "Links", "PowerState",
		}

		for _, prop := range readOnlyProperties {
			if _, exists := patchRequest[prop]; exists {
				PropertyValueNotInListError(c, fmt.Sprintf("%v", patchRequest[prop]), prop)

				return
			}
		}

		// For now, most chassis properties are read-only in this implementation
		// This would typically update writable properties like AssetTag, IndicatorLED, etc.
		// Return 501 Not Implemented for actual property updates
		NotImplementedError(c, "Chassis property modification")
	}
}

// getChassisCollectionHandler handles GET requests for the Chassis collection
func getChassisCollectionHandler(d devices.Feature, l logger.Interface) gin.HandlerFunc {
	return func(c *gin.Context) {
		SetRedfishHeaders(c)

		// Perform common request validations
		if !validateChassisRequest(c) {
			return
		}

		items, err := getChassisItems(d, l, c)
		if err != nil {
			return // Error already handled in getChassisItems
		}

		members := buildChassisMembers(items)
		etag := generateChassisCollectionETag(members)

		// Validate conditional headers (If-None-Match for caching)
		if !validateETag(c, etag) {
			return
		}

		collection := buildChassisCollection(members, etag)
		setETagHeader(c, etag)

		c.JSON(http.StatusOK, collection)
	}
}

// getChassisItems retrieves chassis items with proper error handling
func getChassisItems(d devices.Feature, l logger.Interface, c *gin.Context) ([]dto.Device, error) {
	items, err := d.Get(c.Request.Context(), maxChassisList, 0, "")
	if err != nil {
		l.Error(err, "http - redfish - Chassis collection")
		handleChassisServiceError(c, err)

		return nil, err
	}

	return items, nil
}

// handleChassisServiceError classifies and handles service errors
func handleChassisServiceError(c *gin.Context, err error) {
	errorStr := err.Error()

	if isUpstreamServiceError(errorStr) {
		BadGatewayError(c)

		return
	}

	if isRateLimitError(errorStr) {
		ServiceTemporarilyUnavailableError(c)

		return
	}

	GeneralError(c)
}

// isUpstreamServiceError checks if error is due to upstream service issues
func isUpstreamServiceError(errorStr string) bool {
	return strings.Contains(errorStr, "connection refused") ||
		strings.Contains(errorStr, "timeout") ||
		strings.Contains(errorStr, "unreachable")
}

// isRateLimitError checks if error indicates rate limiting or overload
func isRateLimitError(errorStr string) bool {
	return strings.Contains(errorStr, "too many requests") ||
		strings.Contains(errorStr, "rate limit") ||
		strings.Contains(errorStr, "overloaded")
}

// buildChassisMembers creates chassis references from device items
func buildChassisMembers(items []dto.Device) []ChassisReference {
	members := make([]ChassisReference, 0, len(items))
	for i := range items {
		it := &items[i]
		if it.GUID == "" {
			continue
		}

		members = append(members, ChassisReference{
			ODataID: "/redfish/v1/Chassis/" + it.GUID,
		})
	}

	return members
}

// buildChassisCollection creates the chassis collection response
func buildChassisCollection(members []ChassisReference, etag string) ChassisCollection {
	return ChassisCollection{
		ODataContext: "/redfish/v1/$metadata#ChassisCollection.ChassisCollection",
		ODataID:      "/redfish/v1/Chassis",
		ODataType:    "#ChassisCollection.ChassisCollection",
		ODataEtag:    etag,
		ID:           "ChassisCollection",
		Name:         "Chassis Collection",
		Description:  "Collection of Chassis instances",
		MembersCount: len(members),
		Members:      members,
	}
}

// setETagHeader sets the ETag header with proper quoting
func setETagHeader(c *gin.Context, etag string) {
	c.Header("ETag", fmt.Sprintf("%q", etag))
}

// getChassisInstanceHandler handles GET requests for a specific chassis
func getChassisInstanceHandler(d devices.Feature, l logger.Interface) gin.HandlerFunc {
	return func(c *gin.Context) {
		SetRedfishHeaders(c)

		// Perform common request validations
		if !validateChassisRequest(c) {
			return
		}

		chassisID := c.Param("chassisId")
		if chassisID == "" {
			ResourceNotFoundError(c, "Chassis", "")

			return
		}

		hwInfo, err := getHardwareInfo(d, l, c, chassisID)
		if err != nil {
			return // Error already handled in getHardwareInfo
		}

		chassisInfo, err := parseChassisInfo(hwInfo)
		if err != nil {
			l.Error(err, "http - redfish - Chassis instance - parse chassis info", "chassisId", chassisID)
			GeneralError(c)

			return
		}

		etag := generateChassisInstanceETag(chassisInfo)

		// Validate conditional headers
		if !validateETag(c, etag) {
			return
		}

		chassis := buildChassisInstance(chassisID, chassisInfo, etag)
		setETagHeader(c, etag)

		c.JSON(http.StatusOK, chassis)
	}
}

// getHardwareInfo retrieves hardware info with proper error classification
func getHardwareInfo(d devices.Feature, l logger.Interface, c *gin.Context, chassisID string) (interface{}, error) {
	hwInfo, err := d.GetHardwareInfo(c.Request.Context(), chassisID)
	if err != nil {
		l.Error(err, "http - redfish - Chassis instance - hardware info lookup", "chassisId", chassisID)
		handleHardwareInfoError(c, err, chassisID)

		return nil, err
	}

	return hwInfo, nil
}

// handleHardwareInfoError classifies and handles hardware info retrieval errors
func handleHardwareInfoError(c *gin.Context, err error, chassisID string) {
	errorStr := err.Error()

	if isResourceNotFoundError(errorStr) {
		ResourceNotFoundError(c, "Chassis", chassisID)

		return
	}

	if isUpstreamServiceError(errorStr) {
		BadGatewayError(c)

		return
	}

	if isRateLimitError(errorStr) {
		ServiceTemporarilyUnavailableError(c)

		return
	}

	// Default to resource not found for unknown device errors
	ResourceNotFoundError(c, "Chassis", chassisID)
}

// isResourceNotFoundError checks if error indicates resource not found
func isResourceNotFoundError(errorStr string) bool {
	return strings.Contains(errorStr, "not found") ||
		strings.Contains(errorStr, "invalid") ||
		strings.Contains(errorStr, "does not exist")
}

// buildChassisInstance creates the chassis instance response
func buildChassisInstance(chassisID string, chassisInfo *ChassisInfo, etag string) Chassis {
	return Chassis{
		ODataContext: "/redfish/v1/$metadata#Chassis.Chassis",
		ODataID:      "/redfish/v1/Chassis/" + chassisID,
		ODataType:    "#Chassis.v1_21_0.Chassis",
		ODataEtag:    etag,
		ID:           chassisID,
		Name:         chassisInfo.Name,
		Description:  chassisInfo.Description,
		ChassisType:  chassisInfo.ChassisType,
		Manufacturer: chassisInfo.Manufacturer,
		Model:        chassisInfo.Model,
		SKU:          chassisInfo.SKU,
		SerialNumber: chassisInfo.SerialNumber,
		PartNumber:   chassisInfo.PartNumber,
		AssetTag:     chassisInfo.AssetTag,
		Status: ChassisStatus{
			State:  chassisInfo.Status.State,
			Health: chassisInfo.Status.Health,
		},
		PowerState:   chassisInfo.PowerState,
		IndicatorLED: chassisInfo.IndicatorLED,
		Location:     chassisInfo.Location,
		Links: &ChassisLinks{
			ComputerSystems: []ResourceReference{
				{ODataID: "/redfish/v1/Systems/" + chassisID},
			},
			ComputerSystemsCount: 1,
		},
		PhysicalSecurity: chassisInfo.PhysicalSecurity,
	}
}

// ChassisInfo represents parsed chassis information from hardware info
type ChassisInfo struct {
	Name             string
	Description      string
	ChassisType      string
	Manufacturer     string
	Model            string
	SKU              string
	SerialNumber     string
	PartNumber       string
	AssetTag         string
	Status           ChassisStatus
	PowerState       string
	IndicatorLED     string
	Location         *Location
	PhysicalSecurity *PhysicalSecurity
}

// parseChassisInfo extracts chassis information from device hardware info
func parseChassisInfo(hardwareInfo interface{}) (*ChassisInfo, error) {
	if hardwareInfo == nil {
		return createDefaultChassisInfo(), nil
	}

	hardwareMap, ok := hardwareInfo.(map[string]interface{})
	if !ok {
		return createDefaultChassisInfo(), nil
	}

	chassisData, exists := hardwareMap["CIM_Chassis"]
	if !exists {
		return createDefaultChassisInfo(), nil
	}

	chassisMap, ok := chassisData.(map[string]interface{})
	if !ok {
		return createDefaultChassisInfo(), nil
	}

	response, exists := chassisMap["response"]
	if !exists {
		return createDefaultChassisInfo(), nil
	}

	return extractChassisDetails(response)
}

// createDefaultChassisInfo creates a default chassis info when no data is available
func createDefaultChassisInfo() *ChassisInfo {
	return &ChassisInfo{
		Name:         "System Chassis",
		Description:  "Computer System Chassis",
		ChassisType:  chassisTypeUnknown,
		Manufacturer: unknownValue,
		Model:        unknownValue,
		Status: ChassisStatus{
			State:  "Enabled",
			Health: "OK",
		},
		PowerState:   "On",
		IndicatorLED: "Off",
	}
}

// extractChassisDetails extracts chassis details from CIM response data
func extractChassisDetails(response interface{}) (*ChassisInfo, error) {
	chassisInfo := createDefaultChassisInfo()

	if response == nil {
		return chassisInfo, nil
	}

	// Handle both slice and single object responses
	var chassisItems []interface{}

	responseValue := reflect.ValueOf(response)
	if responseValue.Kind() == reflect.Slice {
		for i := 0; i < responseValue.Len(); i++ {
			chassisItems = append(chassisItems, responseValue.Index(i).Interface())
		}
	} else {
		chassisItems = append(chassisItems, response)
	}

	// Process the first chassis item
	if len(chassisItems) > 0 {
		chassisItem := chassisItems[0]
		populateChassisInfo(chassisInfo, chassisItem)
	}

	return chassisInfo, nil
}

// populateChassisInfo populates chassis info from a CIM chassis item
func populateChassisInfo(chassisInfo *ChassisInfo, chassisItem interface{}) {
	chassisMap := extractChassisMap(chassisItem)
	if chassisMap == nil {
		return
	}

	populateBasicInfo(chassisInfo, chassisMap)
	populateChassisType(chassisInfo, chassisMap)
	updateDescription(chassisInfo)
}

// extractChassisMap safely extracts the chassis map from the interface
func extractChassisMap(chassisItem interface{}) map[string]interface{} {
	if chassisItem == nil {
		return nil
	}

	chassisMap, ok := chassisItem.(map[string]interface{})
	if !ok {
		return nil
	}

	return chassisMap
}

// populateBasicInfo extracts basic chassis information fields
func populateBasicInfo(chassisInfo *ChassisInfo, chassisMap map[string]interface{}) {
	setStringField(chassisInfo, chassisMap, "Manufacturer", func(value string) {
		chassisInfo.Manufacturer = value
	})

	setStringField(chassisInfo, chassisMap, "Model", func(value string) {
		chassisInfo.Model = value
		chassisInfo.Name = fmt.Sprintf("%s Chassis", value)
	})

	setStringField(chassisInfo, chassisMap, "SerialNumber", func(value string) {
		chassisInfo.SerialNumber = value
	})

	setStringField(chassisInfo, chassisMap, "PartNumber", func(value string) {
		chassisInfo.PartNumber = value
	})

	setStringField(chassisInfo, chassisMap, "SKU", func(value string) {
		chassisInfo.SKU = value
	})

	setStringField(chassisInfo, chassisMap, "Tag", func(value string) {
		chassisInfo.AssetTag = value
	})
}

// setStringField safely extracts and sets a string field if it exists and is valid
func setStringField(_ *ChassisInfo, chassisMap map[string]interface{}, fieldName string, setter func(string)) {
	if value, exists := chassisMap[fieldName]; exists && value != nil {
		if valueStr, ok := value.(string); ok && valueStr != "" {
			setter(valueStr)
		}
	}
}

// populateChassisType determines and sets the chassis type
func populateChassisType(chassisInfo *ChassisInfo, chassisMap map[string]interface{}) {
	if packageType, exists := chassisMap["PackageType"]; exists && packageType != nil {
		chassisInfo.ChassisType = mapPackageTypeToChassisType(packageType)
	}
}

// updateDescription sets the description based on available manufacturer and model info
func updateDescription(chassisInfo *ChassisInfo) {
	if chassisInfo.Manufacturer != unknownValue && chassisInfo.Model != unknownValue {
		chassisInfo.Description = fmt.Sprintf("%s %s Chassis", chassisInfo.Manufacturer, chassisInfo.Model)
	}
}

// mapPackageTypeToChassisType maps CIM package types to Redfish chassis types
func mapPackageTypeToChassisType(packageType interface{}) string {
	if packageType == nil {
		return chassisTypeUnknown
	}

	packageTypeStr, ok := packageType.(string)
	if !ok {
		// Handle numeric package types
		if packageTypeNum, ok := packageType.(float64); ok {
			packageTypeStr = fmt.Sprintf("%.0f", packageTypeNum)
		} else {
			return chassisTypeUnknown
		}
	}

	// Map common package types to chassis types
	packageTypeLower := strings.ToLower(packageTypeStr)
	switch packageTypeLower {
	case "rack", "rackmount", "1u", "2u", "4u":
		return chassisTypeRackMount
	case "desktop", "tower", "minitower":
		return chassisTypeDesktop
	case "3", "4": // Common numeric values for rack mount
		return chassisTypeRackMount
	case "2": // Desktop/tower
		return chassisTypeDesktop
	default:
		return chassisTypeUnknown
	}
}

// generateChassisCollectionETag generates an ETag for the chassis collection
func generateChassisCollectionETag(members []ChassisReference) string {
	if len(members) == 0 {
		return "empty-collection"
	}

	// Create a hash based on the collection content
	hash := sha256.New()

	// Add timestamp for cache invalidation
	hash.Write([]byte(time.Now().Format("2006-01-02T15"))) // Hour-level caching

	// Add member data
	for _, member := range members {
		hash.Write([]byte(member.ODataID))
	}

	return fmt.Sprintf("%x", hash.Sum(nil))[:16]
}

// generateChassisInstanceETag generates an ETag for a chassis instance
func generateChassisInstanceETag(chassisInfo *ChassisInfo) string {
	hash := sha256.New()

	// Add chassis-specific data for ETag generation
	chassisData := struct {
		Manufacturer string
		Model        string
		SerialNumber string
		PartNumber   string
		SKU          string
		Timestamp    string
	}{
		Manufacturer: chassisInfo.Manufacturer,
		Model:        chassisInfo.Model,
		SerialNumber: chassisInfo.SerialNumber,
		PartNumber:   chassisInfo.PartNumber,
		SKU:          chassisInfo.SKU,
		Timestamp:    time.Now().Format("2006-01-02T15"), // Hour-level caching
	}

	data, err := json.Marshal(chassisData)
	if err != nil {
		return "default-etag"
	}

	hash.Write(data)

	return fmt.Sprintf("%x", hash.Sum(nil))[:16]
}
