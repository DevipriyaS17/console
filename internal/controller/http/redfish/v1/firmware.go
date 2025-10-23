// Package v1 implements Redfish API v1 FirmwareInventory resources.
package v1

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/device-management-toolkit/console/internal/usecase/devices"
	"github.com/device-management-toolkit/console/pkg/logger"
)

// Firmware-related constants
const (
	// Common string constants
	biosID             = "BIOS"
	sleepDurationMs    = 100
	systemManufacturer = "System Manufacturer"
)

// FirmwareInventoryCollection represents a Redfish FirmwareInventory collection
type FirmwareInventoryCollection struct {
	ODataContext string                    `json:"@odata.context"`
	ODataID      string                    `json:"@odata.id"`
	ODataType    string                    `json:"@odata.type"`
	ODataEtag    string                    `json:"@odata.etag,omitempty"`
	ID           string                    `json:"Id"`
	Name         string                    `json:"Name"`
	Description  string                    `json:"Description"`
	Members      []FirmwareInventoryMember `json:"Members"`
	MembersCount int                       `json:"Members@odata.count"`
	Oem          map[string]interface{}    `json:"Oem,omitempty"`
}

// FirmwareInventoryMember represents a member reference in the collection
type FirmwareInventoryMember struct {
	ODataID string `json:"@odata.id"`
}

// FirmwareInventory represents a single firmware inventory item
type FirmwareInventory struct {
	ODataContext  string                 `json:"@odata.context"`
	ODataID       string                 `json:"@odata.id"`
	ODataType     string                 `json:"@odata.type"`
	ODataEtag     string                 `json:"@odata.etag,omitempty"`
	ID            string                 `json:"Id"`
	Name          string                 `json:"Name"`
	Description   string                 `json:"Description"`
	Version       string                 `json:"Version"`
	VersionString string                 `json:"VersionString,omitempty"`
	Manufacturer  string                 `json:"Manufacturer,omitempty"`
	ReleaseDate   string                 `json:"ReleaseDate,omitempty"`
	SoftwareID    string                 `json:"SoftwareId"`
	Updateable    bool                   `json:"Updateable"`
	Status        Status                 `json:"Status"`
	Oem           map[string]interface{} `json:"Oem,omitempty"`
}

// Status represents the health status of firmware
type Status struct {
	State  string `json:"State"`
	Health string `json:"Health"`
}

// NewFirmwareRoutes registers Redfish FirmwareInventory routes for Systems
// It exposes:
// - GET /redfish/v1/Systems/:id/FirmwareInventory
// - GET /redfish/v1/Systems/:id/FirmwareInventory/:firmwareId
func NewFirmwareRoutes(systems *gin.RouterGroup, d devices.Feature, l logger.Interface) {
	// Add firmware inventory routes to existing Systems group
	systems.GET(":id/FirmwareInventory", getFirmwareInventoryCollectionHandler(d, l))
	systems.GET(":id/FirmwareInventory/:firmwareId", getFirmwareInventoryInstanceHandler(d, l))

	// Register method-not-allowed handlers for FirmwareInventory collection
	systems.POST(":id/FirmwareInventory", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, MethodPOST, "SoftwareInventoryCollection", MethodGET)
	})
	systems.PUT(":id/FirmwareInventory", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, MethodPUT, "SoftwareInventoryCollection", MethodGET)
	})
	systems.PATCH(":id/FirmwareInventory", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, MethodPATCH, "SoftwareInventoryCollection", MethodGET)
	})
	systems.DELETE(":id/FirmwareInventory", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, MethodDELETE, "SoftwareInventoryCollection", MethodGET)
	})

	// Register method-not-allowed handlers for FirmwareInventory instances
	systems.POST(":id/FirmwareInventory/:firmwareId", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, MethodPOST, "SoftwareInventory", MethodGET)
	})
	systems.PUT(":id/FirmwareInventory/:firmwareId", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, MethodPUT, "SoftwareInventory", MethodGET)
	})
	systems.PATCH(":id/FirmwareInventory/:firmwareId", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, MethodPATCH, "SoftwareInventory", MethodGET)
	})
	systems.DELETE(":id/FirmwareInventory/:firmwareId", func(c *gin.Context) {
		HTTPMethodNotAllowedError(c, MethodDELETE, "SoftwareInventory", MethodGET)
	})

	l.Info("Registered Redfish FirmwareInventory routes under %s", systems.BasePath())
}

// generateETag creates an ETag for caching based on content
func generateETag(content string) string {
	hash := sha256.Sum256([]byte(content))

	return fmt.Sprintf(`W/"%x"`, hash)
}

// parseBIOSInfo extracts BIOS version information from hardware info structure
func parseBIOSInfo(hwInfo interface{}) (version, versionString, manufacturer, releaseDate string) {
	return extractBIOSDetails(hwInfo)
}

// extractBIOSDetails handles the complex parsing logic for BIOS information
func extractBIOSDetails(hwInfo interface{}) (version, versionString, manufacturer, releaseDate string) {
	// Set defaults
	version = UnknownValue
	versionString = UnknownValue
	manufacturer = systemManufacturer
	releaseDate = time.Now().UTC().Format("2006-01-02") // Fallback to current date if not found

	if hwInfo == nil {
		return version, versionString, manufacturer, releaseDate
	}

	// Parse the hardware info map structure
	if hwInfoMap, ok := hwInfo.(map[string]interface{}); ok {
		version, versionString, manufacturer, releaseDate = parseFromMap(hwInfoMap)
	}

	return version, versionString, manufacturer, releaseDate
}

// parseFromMap extracts BIOS info from map structure
func parseFromMap(hwInfoMap map[string]interface{}) (version, versionString, manufacturer, releaseDate string) {
	version = UnknownValue
	versionString = UnknownValue
	manufacturer = systemManufacturer
	releaseDate = time.Now().UTC().Format("2006-01-02")

	if biosElement, exists := hwInfoMap["CIM_BIOSElement"]; exists {
		if biosMap, ok := biosElement.(map[string]interface{}); ok {
			if response, exists := biosMap["response"]; exists {
				version, versionString, manufacturer, releaseDate = parseResponse(response, version, versionString, manufacturer, releaseDate)
			}
		}
	}

	return version, versionString, manufacturer, releaseDate
}

// parseResponse handles both map and struct response types
func parseResponse(response interface{}, _, _, _, _ string) (version, versionString, manufacturer, releaseDate string) {
	if responseMap, ok := response.(map[string]interface{}); ok {
		return parseFromResponseMap(responseMap)
	}

	return parseFromStruct(response)
}

// parseFromResponseMap extracts BIOS info from response map
func parseFromResponseMap(responseMap map[string]interface{}) (version, versionString, manufacturer, releaseDate string) {
	version = UnknownValue
	versionString = UnknownValue
	manufacturer = systemManufacturer
	releaseDate = time.Now().UTC().Format("2006-01-02")

	// Extract BIOS version information
	if ver, exists := responseMap["Version"]; exists {
		if verStr, ok := ver.(string); ok && verStr != "" {
			version = verStr
			versionString = verStr
		}
	}

	// Extract BIOS manufacturer
	if mfg, exists := responseMap["Manufacturer"]; exists {
		if mfgStr, ok := mfg.(string); ok && mfgStr != "" {
			manufacturer = mfgStr
		}
	}

	// Extract release date for enhanced version string
	if releaseDateObj, exists := responseMap["ReleaseDate"]; exists {
		releaseDate = extractReleaseDateFromMap(releaseDateObj, version, releaseDate)

		if version != UnknownValue {
			versionString = fmt.Sprintf("%s (Released: %s)", version, releaseDate)
		}
	}

	return version, versionString, manufacturer, releaseDate
}

// extractReleaseDateFromMap extracts release date from nested map structure
func extractReleaseDateFromMap(releaseDateObj interface{}, _, defaultReleaseDate string) string {
	releaseDateMap, ok := releaseDateObj.(map[string]interface{})
	if !ok {
		return defaultReleaseDate
	}

	dateTime, exists := releaseDateMap["DateTime"]
	if !exists {
		return defaultReleaseDate
	}

	dateStr, ok := dateTime.(string)
	if !ok || dateStr == "" {
		return defaultReleaseDate
	}

	// Parse the ISO date and extract just the date part (YYYY-MM-DD)
	parsedTime, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		return defaultReleaseDate
	}

	return parsedTime.Format("2006-01-02")
}

// parseFromStruct handles response as a struct using reflection
func parseFromStruct(response interface{}) (version, versionString, manufacturer, releaseDate string) {
	version = UnknownValue
	versionString = UnknownValue
	manufacturer = systemManufacturer
	releaseDate = time.Now().UTC().Format("2006-01-02")

	// Use reflection to extract fields from the struct
	responseValue := reflect.ValueOf(response)
	if responseValue.Kind() == reflect.Ptr {
		responseValue = responseValue.Elem()
	}

	if responseValue.Kind() == reflect.Struct {
		return extractFieldsFromStruct(responseValue)
	}

	return version, versionString, manufacturer, releaseDate
}

// extractFieldsFromStruct processes struct fields to extract BIOS information
func extractFieldsFromStruct(responseValue reflect.Value) (version, versionString, manufacturer, releaseDate string) {
	version = UnknownValue
	versionString = UnknownValue
	manufacturer = systemManufacturer
	releaseDate = time.Now().UTC().Format("2006-01-02")

	responseType := responseValue.Type()

	// Look for Version, Manufacturer, and ReleaseDate fields
	for i := 0; i < responseValue.NumField(); i++ {
		field := responseType.Field(i)
		fieldValue := responseValue.Field(i)

		switch field.Name {
		case "Version":
			version, versionString = extractVersionFromField(fieldValue)
		case "Manufacturer":
			manufacturer = extractManufacturerFromField(fieldValue)
		case "ReleaseDate":
			releaseDate = extractReleaseDateFromStruct(fieldValue, "", releaseDate)
			if version != UnknownValue {
				versionString = fmt.Sprintf("%s (Released: %s)", version, releaseDate)
			}
		}
	}

	return version, versionString, manufacturer, releaseDate
}

// extractVersionFromField extracts version from a struct field
func extractVersionFromField(fieldValue reflect.Value) (version, versionString string) {
	if fieldValue.Kind() == reflect.String {
		if verStr := fieldValue.String(); verStr != "" {
			return verStr, verStr
		}
	}

	return UnknownValue, UnknownValue
}

// extractManufacturerFromField extracts manufacturer from a struct field
func extractManufacturerFromField(fieldValue reflect.Value) string {
	if fieldValue.Kind() == reflect.String {
		if mfgStr := fieldValue.String(); mfgStr != "" {
			return mfgStr
		}
	}

	return systemManufacturer
}

// extractReleaseDateFromStruct extracts release date from struct field
func extractReleaseDateFromStruct(fieldValue reflect.Value, _, defaultReleaseDate string) string {
	if !fieldValue.IsValid() {
		return defaultReleaseDate
	}

	// The ReleaseDate is a bios.Time struct that has a DateTime field
	releaseDateStruct := fieldValue.Interface()
	if releaseDateStruct == nil {
		return defaultReleaseDate
	}

	releaseDateValue := reflect.ValueOf(releaseDateStruct)
	if releaseDateValue.Kind() != reflect.Struct {
		return defaultReleaseDate
	}

	dateTimeField := releaseDateValue.FieldByName("DateTime")
	if !dateTimeField.IsValid() || dateTimeField.Kind() != reflect.String {
		return defaultReleaseDate
	}

	dateTimeStr := dateTimeField.String()
	if dateTimeStr == "" {
		return defaultReleaseDate
	}

	// Parse the ISO date and extract just the date part (YYYY-MM-DD)
	parsedTime, err := time.Parse(time.RFC3339, dateTimeStr)
	if err != nil {
		return defaultReleaseDate
	}

	return parsedTime.Format("2006-01-02")
} // createIntelOemSection creates Intel-specific OEM extensions
func createIntelOemSection(systemID string) map[string]interface{} {
	return map[string]interface{}{
		"Intel": map[string]interface{}{
			"@odata.type": "#Intel.v1_0_0.Intel",
			"SystemGUID":  systemID,
			"LastUpdated": time.Now().UTC().Format(time.RFC3339),
			"AMTCapabilities": map[string]interface{}{
				"SupportsSOL":         true,
				"SupportsIDER":        true,
				"SupportsKVM":         true,
				"SupportsPowerAction": true,
			},
		},
	}
}

// getFirmwareInventoryCollectionHandler handles GET /Systems/{id}/FirmwareInventory
func getFirmwareInventoryCollectionHandler(d devices.Feature, l logger.Interface) gin.HandlerFunc {
	return func(c *gin.Context) {
		systemID := c.Param("id")

		// Get AMT version information for AMT firmware components
		_, versionInfo, err := d.GetVersion(c.Request.Context(), systemID)
		if err != nil {
			l.Error(err, "redfish v1 - FirmwareInventory: failed to get version for system %s", systemID)
			ResourceNotFoundError(c, "ComputerSystem", systemID)

			return
		}

		// Get hardware info and build collection
		collection := buildFirmwareCollection(d, l, c, systemID, versionInfo)

		// Set Redfish-compliant headers
		SetRedfishHeaders(c)

		// Set ETag header for HTTP caching
		c.Header("ETag", collection.ODataEtag)
		c.Header("Cache-Control", CacheMaxAge5Min) // Cache for 5 minutes

		c.JSON(http.StatusOK, collection)
	}
}

// buildFirmwareCollection creates the firmware inventory collection
func buildFirmwareCollection(d devices.Feature, l logger.Interface, c *gin.Context, systemID string, versionInfo interface{}) FirmwareInventoryCollection {
	// Get hardware information for BIOS and system firmware
	// Add small delay to avoid potential connection conflicts
	time.Sleep(sleepDurationMs * time.Millisecond)
	l.Info("redfish v1 - FirmwareInventory: attempting to get hardware info for system %s", systemID)

	hwInfo, hwErr := d.GetHardwareInfo(c.Request.Context(), systemID)
	if hwErr != nil {
		l.Warn("redfish v1 - FirmwareInventory: failed to get hardware info for system %s: %v", systemID, hwErr)
	} else {
		// Debug: Log the hwInfo structure to understand what we're getting
		if hwInfoJSON, err := json.Marshal(hwInfo); err == nil {
			l.Info("redfish v1 - FirmwareInventory: hwInfo structure: %s", string(hwInfoJSON))
		} else {
			l.Warn("redfish v1 - FirmwareInventory: failed to marshal hwInfo: %v", err)
		}
	}

	// Build firmware inventory collection from AMT version data
	collection := FirmwareInventoryCollection{
		ODataContext: ODataContextSoftwareInventoryCollection,
		ODataID:      "/redfish/v1/Systems/" + systemID + "/FirmwareInventory",
		ODataType:    SchemaSoftwareInventoryCollection,
		ID:           "FirmwareInventory",
		Name:         "Firmware Inventory Collection",
		Description:  "Collection of firmware inventory for this system",
		Members:      []FirmwareInventoryMember{},
		MembersCount: 0,
		Oem:          createIntelOemSection(systemID),
	}

	// Add firmware members based on available version info
	addFirmwareMembers(&collection, systemID, versionInfo)

	// Add system firmware from hardware info
	if hwErr == nil && hwInfo != nil {
		addBIOSMember(&collection, systemID)
	}

	collection.MembersCount = len(collection.Members)

	// Generate ETag for caching
	collectionContent := fmt.Sprintf("FirmwareInventory-%s-%d", systemID, collection.MembersCount)
	collection.ODataEtag = generateETag(collectionContent)

	return collection
}

// addFirmwareMembers adds firmware inventory members based on version info
func addFirmwareMembers(collection *FirmwareInventoryCollection, systemID string, versionInfo interface{}) {
	// Use type assertion to access version info fields
	// This assumes versionInfo has the expected structure
	v := reflect.ValueOf(versionInfo)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return
	}

	// Add AMT firmware components as inventory items
	if amt := getStringField(v, "AMT"); amt != "" {
		collection.Members = append(collection.Members, FirmwareInventoryMember{
			ODataID: "/redfish/v1/Systems/" + systemID + "/FirmwareInventory/AMT",
		})
	}

	if flash := getStringField(v, "Flash"); flash != "" {
		collection.Members = append(collection.Members, FirmwareInventoryMember{
			ODataID: "/redfish/v1/Systems/" + systemID + "/FirmwareInventory/Flash",
		})
	}

	if netstack := getStringField(v, "Netstack"); netstack != "" {
		collection.Members = append(collection.Members, FirmwareInventoryMember{
			ODataID: "/redfish/v1/Systems/" + systemID + "/FirmwareInventory/Netstack",
		})
	}

	if amtApps := getStringField(v, "AMTApps"); amtApps != "" {
		collection.Members = append(collection.Members, FirmwareInventoryMember{
			ODataID: "/redfish/v1/Systems/" + systemID + "/FirmwareInventory/AMTApps",
		})
	}
}

// getStringField safely extracts a string field from a struct using reflection
func getStringField(v reflect.Value, fieldName string) string {
	field := v.FieldByName(fieldName)
	if field.IsValid() && field.Kind() == reflect.String {
		return field.String()
	}

	return ""
}

// addBIOSMember adds BIOS firmware member to the collection
func addBIOSMember(collection *FirmwareInventoryCollection, systemID string) {
	collection.Members = append(collection.Members, FirmwareInventoryMember{
		ODataID: "/redfish/v1/Systems/" + systemID + "/FirmwareInventory/BIOS",
	})
}

// getFirmwareInventoryInstanceHandler handles GET /Systems/{id}/FirmwareInventory/{firmwareId}
func getFirmwareInventoryInstanceHandler(d devices.Feature, l logger.Interface) gin.HandlerFunc {
	return func(c *gin.Context) {
		systemID := c.Param("id")
		firmwareID := c.Param("firmwareId")

		// Get AMT version information
		_, versionInfo, err := d.GetVersion(c.Request.Context(), systemID)
		if err != nil {
			l.Error(err, "redfish v1 - FirmwareInventory: failed to get version for system %s", systemID)
			ResourceNotFoundError(c, "ComputerSystem", systemID)

			return
		}

		// Get hardware info if needed for BIOS
		hwInfo, err := getHardwareInfoIfNeeded(d, l, c, systemID, firmwareID)
		if err != nil {
			return // Error already handled in the function
		}

		// Get the specific firmware inventory item
		firmware := getFirmwareItem(systemID, firmwareID, versionInfo, hwInfo, l)
		if firmware == nil {
			ResourceNotFoundError(c, "SoftwareInventory", firmwareID)

			return
		}

		// Send response
		sendFirmwareResponse(c, firmware)
	}
}

// getHardwareInfoIfNeeded gets hardware info only for BIOS requests
func getHardwareInfoIfNeeded(d devices.Feature, l logger.Interface, c *gin.Context, systemID, firmwareID string) (interface{}, error) {
	if firmwareID != biosID {
		return nil, nil
	}

	l.Info("redfish v1 - FirmwareInventory: getting hardware info for BIOS firmware, system %s", systemID)

	hwInfo, err := d.GetHardwareInfo(c.Request.Context(), systemID)
	if err != nil {
		l.Error(err, "redfish v1 - FirmwareInventory: failed to get hardware info for system %s", systemID)
		ResourceNotFoundError(c, "SoftwareInventory", firmwareID)

		return nil, err
	}

	// Debug: Log what we actually got
	if hwInfoJSON, marshalErr := json.Marshal(hwInfo); marshalErr == nil {
		l.Info("redfish v1 - FirmwareInventory: BIOS hwInfo retrieved: %s", string(hwInfoJSON))
	} else {
		l.Warn("redfish v1 - FirmwareInventory: failed to marshal BIOS hwInfo: %v", marshalErr)
	}

	return hwInfo, nil
}

// getFirmwareItem creates the appropriate firmware inventory item based on firmware ID
func getFirmwareItem(systemID, firmwareID string, versionInfo, hwInfo interface{}, l logger.Interface) *FirmwareInventory {
	switch firmwareID {
	case "AMT":
		return createAMTFirmware(systemID, versionInfo)
	case "Flash":
		return createFlashFirmware(systemID, versionInfo)
	case "Netstack":
		return createNetstackFirmware(systemID, versionInfo)
	case "AMTApps":
		return createAMTAppsFirmware(systemID, versionInfo)
	case biosID:
		return createBIOSFirmware(systemID, hwInfo, l)
	}

	return nil
}

// sendFirmwareResponse sends the firmware inventory response
func sendFirmwareResponse(c *gin.Context, firmware *FirmwareInventory) {
	// Set Redfish-compliant headers
	SetRedfishHeaders(c)

	// Set ETag header for HTTP caching
	if firmware.ODataEtag != "" {
		c.Header("ETag", firmware.ODataEtag)
	}

	c.Header("Cache-Control", CacheMaxAge5Min) // Cache for 5 minutes

	c.JSON(http.StatusOK, firmware)
}

// createAMTFirmware creates firmware inventory for AMT
func createAMTFirmware(systemID string, versionInfo interface{}) *FirmwareInventory {
	v := reflect.ValueOf(versionInfo)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	amt := getStringField(v, "AMT")
	if amt == "" {
		return nil
	}

	return &FirmwareInventory{
		ODataContext:  ODataContextSoftwareInventory,
		ODataID:       "/redfish/v1/Systems/" + systemID + "/FirmwareInventory/AMT",
		ODataType:     SchemaSoftwareInventory,
		ODataEtag:     generateETag(fmt.Sprintf("AMT-%s-%s", systemID, amt)),
		ID:            "AMT",
		Name:          "Intel Active Management Technology",
		Description:   "Intel AMT Firmware",
		Version:       amt,
		VersionString: amt,
		Manufacturer:  ServiceVendor,
		ReleaseDate:   time.Now().UTC().Format("2006-01-02"), // Current date as placeholder
		SoftwareID:    "AMT-" + systemID,
		Updateable:    false, // AMT firmware updates typically require special procedures
		Status: Status{
			State:  "Enabled",
			Health: TaskStatusOK,
		},
		Oem: createAMTOemSection(versionInfo, systemID),
	}
}

// createFlashFirmware creates firmware inventory for Flash
func createFlashFirmware(systemID string, versionInfo interface{}) *FirmwareInventory {
	v := reflect.ValueOf(versionInfo)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	flash := getStringField(v, "Flash")
	if flash == "" {
		return nil
	}

	return &FirmwareInventory{
		ODataContext:  ODataContextSoftwareInventory,
		ODataID:       "/redfish/v1/Systems/" + systemID + "/FirmwareInventory/Flash",
		ODataType:     SchemaSoftwareInventory,
		ODataEtag:     generateETag(fmt.Sprintf("Flash-%s-%s", systemID, flash)),
		ID:            "Flash",
		Name:          "AMT Flash Firmware",
		Description:   "AMT Flash Memory Firmware",
		Version:       flash,
		VersionString: flash,
		Manufacturer:  ServiceVendor,
		ReleaseDate:   time.Now().UTC().Format("2006-01-02"),
		SoftwareID:    "Flash-" + systemID,
		Updateable:    false,
		Status: Status{
			State:  "Enabled",
			Health: TaskStatusOK,
		},
		Oem: map[string]interface{}{
			"Intel": map[string]interface{}{
				"@odata.type":  "#Intel.v1_0_0.Intel",
				"FirmwareType": "Flash",
				"Component":    "AMT Flash Memory",
				"SystemGUID":   systemID,
			},
		},
	}
}

// createNetstackFirmware creates firmware inventory for Netstack
func createNetstackFirmware(systemID string, versionInfo interface{}) *FirmwareInventory {
	v := reflect.ValueOf(versionInfo)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	netstack := getStringField(v, "Netstack")
	if netstack == "" {
		return nil
	}

	return &FirmwareInventory{
		ODataContext:  ODataContextSoftwareInventory,
		ODataID:       "/redfish/v1/Systems/" + systemID + "/FirmwareInventory/Netstack",
		ODataType:     SchemaSoftwareInventory,
		ODataEtag:     generateETag(fmt.Sprintf("Netstack-%s-%s", systemID, netstack)),
		ID:            "Netstack",
		Name:          "AMT Network Stack",
		Description:   "AMT Network Stack Firmware",
		Version:       netstack,
		VersionString: netstack,
		Manufacturer:  ServiceVendor,
		ReleaseDate:   time.Now().UTC().Format("2006-01-02"),
		SoftwareID:    "Netstack-" + systemID,
		Updateable:    false,
		Status: Status{
			State:  "Enabled",
			Health: TaskStatusOK,
		},
		Oem: map[string]interface{}{
			"Intel": map[string]interface{}{
				"@odata.type":  "#Intel.v1_0_0.Intel",
				"FirmwareType": "Netstack",
				"Component":    "AMT Network Stack",
				"SystemGUID":   systemID,
			},
		},
	}
}

// createAMTAppsFirmware creates firmware inventory for AMTApps
func createAMTAppsFirmware(systemID string, versionInfo interface{}) *FirmwareInventory {
	v := reflect.ValueOf(versionInfo)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	amtApps := getStringField(v, "AMTApps")
	if amtApps == "" {
		return nil
	}

	return &FirmwareInventory{
		ODataContext:  ODataContextSoftwareInventory,
		ODataID:       "/redfish/v1/Systems/" + systemID + "/FirmwareInventory/AMTApps",
		ODataType:     SchemaSoftwareInventory,
		ODataEtag:     generateETag(fmt.Sprintf("AMTApps-%s-%s", systemID, amtApps)),
		ID:            "AMTApps",
		Name:          "AMT Applications",
		Description:   "AMT Applications Firmware",
		Version:       amtApps,
		VersionString: amtApps,
		Manufacturer:  ServiceVendor,
		ReleaseDate:   time.Now().UTC().Format("2006-01-02"),
		SoftwareID:    "AMTApps-" + systemID,
		Updateable:    false,
		Status: Status{
			State:  "Enabled",
			Health: TaskStatusOK,
		},
		Oem: map[string]interface{}{
			"Intel": map[string]interface{}{
				"@odata.type":  "#Intel.v1_0_0.Intel",
				"FirmwareType": "AMTApps",
				"Component":    "AMT Applications",
				"SystemGUID":   systemID,
			},
		},
	}
}

// createBIOSFirmware creates firmware inventory for BIOS
func createBIOSFirmware(systemID string, hwInfo interface{}, l logger.Interface) *FirmwareInventory {
	if hwInfo == nil {
		l.Warn("BIOS firmware request - hwInfo is nil, cannot retrieve BIOS version")

		return nil
	}

	// Handle BIOS/UEFI system firmware
	l.Info("BIOS case: hwInfo type: %T, hwInfo == nil: %v", hwInfo, hwInfo == nil)

	// Parse hardware info to extract BIOS version information
	version, versionString, manufacturer, releaseDate := parseBIOSInfo(hwInfo)

	l.Info("BIOS case: parsed version=%s, manufacturer=%s, releaseDate=%s", version, manufacturer, releaseDate)

	return &FirmwareInventory{
		ODataContext:  ODataContextSoftwareInventory,
		ODataID:       "/redfish/v1/Systems/" + systemID + "/FirmwareInventory/BIOS",
		ODataType:     SchemaSoftwareInventory,
		ODataEtag:     generateETag(fmt.Sprintf("BIOS-%s-%s", systemID, version)),
		ID:            "BIOS",
		Name:          "System BIOS/UEFI",
		Description:   "System BIOS/UEFI Firmware",
		Version:       version,
		VersionString: versionString,
		Manufacturer:  manufacturer,
		ReleaseDate:   releaseDate, // Use actual BIOS release date instead of current date
		SoftwareID:    "BIOS-" + systemID,
		Updateable:    false, // BIOS updates typically require special procedures
		Status: Status{
			State:  "Enabled",
			Health: TaskStatusOK,
		},
		Oem: map[string]interface{}{
			"Intel": map[string]interface{}{
				"@odata.type":  "#Intel.v1_0_0.Intel",
				"FirmwareType": "BIOS",
				"Component":    "System BIOS/UEFI",
				"SystemGUID":   systemID,
			},
		},
	}
}

// createAMTOemSection creates the OEM section for AMT firmware
func createAMTOemSection(versionInfo interface{}, _ string) map[string]interface{} {
	v := reflect.ValueOf(versionInfo)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	return map[string]interface{}{
		"Intel": map[string]interface{}{
			"@odata.type":  "#Intel.v1_0_0.Intel",
			"FirmwareType": "AMT",
			"BuildNumber":  getStringField(v, "BuildNumber"),
			"AMTFWCore":    getStringField(v, "AMTFWCoreVersion"),
			"LegacyMode":   getStringField(v, "LegacyMode"),
			"SKU":          getStringField(v, "SKU"),
			"VendorID":     getStringField(v, "VendorID"),
		},
	}
}
