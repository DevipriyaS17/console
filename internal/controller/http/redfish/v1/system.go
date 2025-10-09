// Package v1 implements Redfish API v1 ComputerSystem resources and actions.
package v1

import (
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/device-management-toolkit/console/config"
	"github.com/device-management-toolkit/console/internal/usecase/devices"
	"github.com/device-management-toolkit/console/pkg/logger"
)

// Lint constants
const (
	maxSystemsList        = 100
	powerStateUnknown     = "Unknown"
	powerStateOn          = "On"
	powerStateOff         = "Off"
	resetTypeOn           = "On"
	resetTypeForceOff     = "ForceOff"
	resetTypeForceRestart = "ForceRestart"
	resetTypePowerCycle   = "PowerCycle"
	actionPowerUp         = 2
	actionPowerCycle      = 5
	actionPowerDown       = 8
	actionReset           = 10
	// CIM PowerState enum values (Device.PowerState)
	cimPowerOn      = 2
	cimPowerSleep   = 3
	cimPowerStandby = 4
	cimPowerSoftOff = 7
	cimPowerHardOff = 8
)

// NewSystemsRoutes registers Redfish v1 ComputerSystem routes.
// It exposes:
// - GET /redfish/v1/Systems (collection)
// - GET /redfish/v1/Systems/:id (individual system)
// - POST /redfish/v1/Systems/:id/Actions/ComputerSystem.Reset (reset action)
// - GET/PUT/PATCH/DELETE /redfish/v1/Systems/:id/Actions/ComputerSystem.Reset (405 Method Not Allowed)
// The :id is expected to be the device GUID and will be mapped directly to SendPowerAction.
func NewSystemsRoutes(r *gin.RouterGroup, d devices.Feature, cfg *config.Config, l logger.Interface) {
	systems := r.Group("/Systems")

	// Apply Redfish-compliant authentication if auth is enabled
	if !cfg.Disabled {
		systems.Use(RedfishJWTAuthMiddleware(cfg))
	}

	systems.GET("", getSystemsCollectionHandler(d, l))
	systems.GET(":id", getSystemInstanceHandler(d, l))

	// ComputerSystem.Reset Action - only POST is allowed
	systems.POST(":id/Actions/ComputerSystem.Reset", postSystemResetHandler(d, l))
	systems.GET(":id/Actions/ComputerSystem.Reset", methodNotAllowedHandler("ComputerSystem.Reset", "POST"))
	systems.PUT(":id/Actions/ComputerSystem.Reset", methodNotAllowedHandler("ComputerSystem.Reset", "POST"))
	systems.PATCH(":id/Actions/ComputerSystem.Reset", methodNotAllowedHandler("ComputerSystem.Reset", "POST"))
	systems.DELETE(":id/Actions/ComputerSystem.Reset", methodNotAllowedHandler("ComputerSystem.Reset", "POST"))

	l.Info("Registered Redfish v1 Systems routes under %s", r.BasePath()+"/Systems")
}

func getSystemsCollectionHandler(d devices.Feature, l logger.Interface) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := d.Get(c.Request.Context(), maxSystemsList, 0, "")
		if err != nil {
			l.Error(err, "http - redfish v1 - Systems collection")

			if isServiceTemporarilyUnavailable(err) {
				ServiceTemporarilyUnavailableError(c)
			} else if isUpstreamCommunicationError(err) {
				ServiceUnavailableError(c)
			} else {
				GeneralError(c)
			}
			return
		}

		members := make([]any, 0, len(items))
		for i := range items { // avoid value copy
			it := &items[i]
			if it.GUID == "" {
				continue
			}

			members = append(members, map[string]any{
				"@odata.id": "/redfish/v1/Systems/" + it.GUID,
			})
		}

		payload := map[string]any{
			"@odata.type":         "#ComputerSystemCollection.ComputerSystemCollection",
			"@odata.id":           "/redfish/v1/Systems",
			"Name":                "Computer System Collection",
			"Members@odata.count": len(members),
			"Members":             members,
		}
		c.JSON(http.StatusOK, payload)
	}
}

func getSystemInstanceHandler(d devices.Feature, l logger.Interface) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		powerState := powerStateUnknown

		if ps, err := d.GetPowerState(c.Request.Context(), id); err != nil {
			l.Warn("redfish v1 - Systems instance: failed to get power state for %s: %v", id, err)
		} else {
			switch ps.PowerState { // CIM PowerState values
			case actionPowerUp: // 2 (On)
				powerState = powerStateOn
			case cimPowerSleep, cimPowerStandby: // Sleep/Standby -> treat as On
				powerState = powerStateOn
			case cimPowerSoftOff, cimPowerHardOff: // Soft Off / Hard Off
				powerState = powerStateOff
			default:
				powerState = powerStateUnknown
			}
		}

		payload := map[string]any{
			"@odata.type": "#ComputerSystem.v1_0_0.ComputerSystem",
			"@odata.id":   "/redfish/v1/Systems/" + id,
			"Id":          id,
			"Name":        "Computer System " + id,
			"PowerState":  powerState,
			"Actions": map[string]any{
				"#ComputerSystem.Reset": map[string]any{
					"target":                            "/redfish/v1/Systems/" + id + "/Actions/ComputerSystem.Reset",
					"ResetType@Redfish.AllowableValues": []string{resetTypeOn, resetTypeForceOff, resetTypeForceRestart, resetTypePowerCycle},
				},
			},
		}
		c.JSON(http.StatusOK, payload)
	}
}

// methodNotAllowedHandler returns a handler that responds with 405 Method Not Allowed for Redfish actions
func methodNotAllowedHandler(action string, allowedMethods string) gin.HandlerFunc {
	return func(c *gin.Context) {
		MethodNotAllowedError(c, action, allowedMethods)
	}
}

func postSystemResetHandler(d devices.Feature, l logger.Interface) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		var body struct {
			ResetType string `json:"ResetType"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			MalformedJSONError(c)
			return
		}

		// Check if ResetType is provided (required property)
		if body.ResetType == "" {
			PropertyMissingError(c, "ResetType")
			return
		}

		var action int

		switch body.ResetType {
		case resetTypeOn:
			action = actionPowerUp
		case resetTypeForceOff:
			action = actionPowerDown
		case resetTypeForceRestart:
			action = actionReset
		case resetTypePowerCycle:
			action = actionPowerCycle
		default:
			PropertyValueNotInListError(c, body.ResetType, "ResetType")
			return
		}

		// Check current power state to avoid redundant operations
		currentPowerState, err := d.GetPowerState(c.Request.Context(), id)
		if err == nil {
			// Only check for conflict if we can get the current state
			// Map CIM power states to determine if action would result in no change
			isCurrentlyOn := (currentPowerState.PowerState == cimPowerOn)
			isCurrentlyOff := (currentPowerState.PowerState == cimPowerSoftOff || currentPowerState.PowerState == cimPowerHardOff)

			var shouldReturnConflict bool

			switch action {
			case actionPowerUp: // Power On
				if isCurrentlyOn {
					shouldReturnConflict = true
				}
			case actionPowerDown: // Power Off
				if isCurrentlyOff {
					shouldReturnConflict = true
				}
			}

			if shouldReturnConflict {
				OperationNotAllowedError(c)
				return
			}
		}
		// If we can't get the power state, continue with the action anyway

		res, err := d.SendPowerAction(c.Request.Context(), id, action)
		if err != nil {
			l.Error(err, "http - redfish v1 - ComputerSystem.Reset")

			// Check if this is a "not found" error
			if strings.Contains(strings.ToLower(err.Error()), "not found") ||
				strings.Contains(err.Error(), "DevicesUseCase") {
				ResourceNotFoundError(c, "ComputerSystem", id)
			} else if isServiceTemporarilyUnavailable(err) {
				// 503 Service Unavailable for temporary service overload/maintenance
				ServiceTemporarilyUnavailableError(c)
			} else if isUpstreamCommunicationError(err) {
				// 502 Bad Gateway for upstream device communication failures
				ServiceUnavailableError(c)
			} else {
				// 500 Internal Server Error for other failures
				GeneralError(c)
			}
			return
		}

		// Generate a task ID for this reset operation
		taskID := fmt.Sprintf("%d", rand.Intn(999999)+100000)

		// Determine task state based on the result
		taskState := "Completed"
		taskStatus := "OK"
		messageID := BaseSuccessMessageID
		message := "The request completed successfully."

		// Check if the operation was successful based on ReturnValue
		if int(res.ReturnValue) != 0 {
			taskState = "Exception"
			taskStatus = "Critical"
			messageID = BaseErrorMessageID
			message = "A general error has occurred."
		}

		// Return Redfish-compliant Task response
		taskResponse := map[string]any{
			"@odata.context": "/redfish/v1/$metadata#Task.Task",
			"@odata.id":      "/redfish/v1/TaskService/Tasks/" + taskID,
			"@odata.type":    "#Task.v1_6_0.Task",
			"Id":             taskID,
			"Name":           "System Reset Task",
			"TaskState":      taskState,
			"TaskStatus":     taskStatus,
			"StartTime":      time.Now().UTC().Format(time.RFC3339),
			"EndTime":        time.Now().UTC().Format(time.RFC3339),
			"Messages": []map[string]any{
				{
					"MessageId": messageID,
					"Message":   message,
					"Severity":  taskStatus,
				},
			},
		}

		// Set Redfish-compliant headers
		SetRedfishHeaders(c)

		c.JSON(http.StatusOK, taskResponse)
	}
}

// isUpstreamCommunicationError determines if an error is due to upstream device communication failure
func isUpstreamCommunicationError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())

	// Check for common upstream communication error patterns
	upstreamErrors := []string{
		"connection refused",
		"connection timeout",
		"timeout",
		"network unreachable",
		"no route to host",
		"connection reset",
		"wsman",        // WSMAN-specific errors
		"amt",          // AMT-specific errors
		"unauthorized", // AMT authentication failures
		"certificate",  // TLS certificate issues
		"ssl",          // SSL/TLS errors
		"tls",          // TLS errors
		"dial tcp",     // TCP connection errors
		"i/o timeout",  // I/O timeout errors
		"connection aborted",
		"host unreachable",
	}

	for _, pattern := range upstreamErrors {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}

	return false
}

// isServiceTemporarilyUnavailable determines if the service should return 503 due to overload or maintenance
func isServiceTemporarilyUnavailable(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())

	// Check for temporary service unavailability patterns
	serviceUnavailableErrors := []string{
		"too many connections",
		"connection pool exhausted",
		"database pool full",
		"service overloaded",
		"maintenance mode",
		"rate limit exceeded",
		"too many requests",
		"resource exhausted",
		"service unavailable",
		"temporarily unavailable",
		"max connections reached",
		"server overloaded",
		"capacity exceeded",
		"throttled",
		"circuit breaker",
	}

	for _, pattern := range serviceUnavailableErrors {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}

	return false
}

// NewSystemsRoutes creates the systems routes for the given router group
