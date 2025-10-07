// Package v1 implements Redfish API v1 error handling and utilities.
package v1

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Redfish Base Message Registry v1.11.0 Message IDs
const (
	BaseSuccessMessageID         = "Base.1.11.0.Success"
	BaseErrorMessageID           = "Base.1.11.0.GeneralError"
	BaseMalformedJSONID          = "Base.1.11.0.MalformedJSON"
	BasePropertyMissingID        = "Base.1.11.0.PropertyMissing"
	BasePropertyValueNotInListID = "Base.1.11.0.PropertyValueNotInList"
	BaseResourceNotFoundID       = "Base.1.11.0.ResourceNotFound"
	BaseOperationNotAllowedID    = "Base.1.11.0.OperationNotAllowed"
)

// redfishError creates a standard Redfish error response structure
func redfishError(messageID, message, severity, resolution string, messageArgs []string) map[string]any {
	extendedInfo := map[string]any{
		"MessageId":  messageID,
		"Message":    message,
		"Severity":   severity,
		"Resolution": resolution,
	}

	// Only add MessageArgs if provided and not empty
	if len(messageArgs) > 0 {
		extendedInfo["MessageArgs"] = messageArgs
	}

	return map[string]any{
		"error": map[string]any{
			"@Message.ExtendedInfo": []map[string]any{extendedInfo},
			"code":                  messageID,
			"message":               message,
		},
	}
}

// SetRedfishHeaders sets standard Redfish-compliant HTTP headers
func SetRedfishHeaders(c *gin.Context) {
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Header("OData-Version", "4.0")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Frame-Options", "DENY")
	c.Header("Content-Security-Policy", "default-src 'self'")
}

// redfishErrorResponse sends a Redfish error response with proper headers
func redfishErrorResponse(c *gin.Context, statusCode int, messageID, message, severity, resolution string, messageArgs []string) {
	SetRedfishHeaders(c)
	c.JSON(statusCode, redfishError(messageID, message, severity, resolution, messageArgs))
}

// MalformedJSONError returns a Redfish-compliant error for malformed JSON requests
func MalformedJSONError(c *gin.Context) {
	redfishErrorResponse(c, http.StatusBadRequest,
		BaseMalformedJSONID,
		"The request body submitted was malformed JSON and could not be parsed by the receiving service.",
		"Critical",
		"Ensure that the request body is valid JSON and resubmit the request.",
		nil)
}

// PropertyMissingError returns a Redfish-compliant error for missing required properties
func PropertyMissingError(c *gin.Context, propertyName string) {
	redfishErrorResponse(c, http.StatusBadRequest,
		BasePropertyMissingID,
		fmt.Sprintf("The property %s is a required property and must be included in the request.", propertyName),
		"Warning",
		"Ensure that the property is in the request body and has a valid value and resubmit the request.",
		[]string{propertyName})
}

// PropertyValueNotInListError returns a Redfish-compliant error for invalid enum values
func PropertyValueNotInListError(c *gin.Context, value, propertyName string) {
	redfishErrorResponse(c, http.StatusBadRequest,
		BasePropertyValueNotInListID,
		fmt.Sprintf("The value '%s' for the property %s is not in the list of acceptable values.", value, propertyName),
		"Warning",
		"Choose a value from the enumeration list that the implementation can support and resubmit the request if the operation failed.",
		[]string{value, propertyName})
}

// ResourceNotFoundError returns a Redfish-compliant error for missing resources
func ResourceNotFoundError(c *gin.Context, resourceType, resourceID string) {
	redfishErrorResponse(c, http.StatusNotFound,
		BaseResourceNotFoundID,
		fmt.Sprintf("The requested resource of type %s named '%s' was not found.", resourceType, resourceID),
		"Critical",
		"Provide a valid resource identifier and resubmit the request.",
		[]string{resourceType, resourceID})
}

// OperationNotAllowedError returns a Redfish-compliant error for operations not allowed due to resource state
func OperationNotAllowedError(c *gin.Context) {
	redfishErrorResponse(c, http.StatusConflict,
		BaseOperationNotAllowedID,
		"The operation was not successful because the resource is in a state that does not allow this operation.",
		"Critical",
		"The operation was not successful because the resource is in a state that does not allow this operation.",
		nil)
}

// GeneralError returns a Redfish-compliant error for general internal errors
func GeneralError(c *gin.Context) {
	redfishErrorResponse(c, http.StatusInternalServerError,
		BaseErrorMessageID,
		"A general error has occurred. See ExtendedInfo for more information.",
		"Critical",
		"None.",
		nil)
}
