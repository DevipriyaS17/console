# Redfish Support

## Table of Contents

- [Introduction](#introduction)
- [Redfish usage from DMT](#redfish-usage-from-dmt)
- [Architecture and Design](#architecture-and-design)
- [Redfish Resource Mapping](#redfish-resource-mapping)
- [Redfish Authentication Architecture](#redfish-authentication-architecture)

## Introduction

**[Redfish](https://www.dmtf.org/standards/redfish)** is a modern, REST-based API standard developed by the **Distributed Management Task Force (DMTF)** for managing servers, storage systems, networking equipment, and other computing infrastructure. It provides a secure, scalable approach to systems management that replaces older protocols like IPMI (Intelligent Platform Management Interface) and traditional proprietary management interfaces. Key Characteristics of Redfish

1. RESTful Architecture - Uses standard HTTP/HTTPS methods (GET, POST, PATCH, DELETE)
1. Standardized Data Models - Common schema for representing hardware components with consistent naming conventions across vendors and well-defined resource relationships.
1. Security-First Design - Built-in authentication and authorization mechanisms, HTTPS/TLS encryption for all communications, Role-based access control (RBAC) and Event-driven security notifications

### Redfish API Structure

Redfish organizes system resources in a hierarchical tree structure:

```text
/redfish/v1/                             # Service Root
├── Systems/                             # Computer Systems
│   ├── 1/                               # Specific System
│   │   ├── Processors/                  # CPU Information
│   │   ├── Memory/                      # Memory Modules
│   │   ├── Storage/                     # Storage Controllers
│   │   ├── NetworkInterfaces/           # Network Adapters
│   │   └── Actions/                     # Available Actions
├── Chassis/                             # Physical Chassis
├── Managers/                            # Management Controllers
├── AccountService/                      # User Account Management
├── SessionService/                      # Session Management
├── EventService/                        # Event Subscriptions
└── UpdateService/                       # Firmware Updates
```

### Redfish in the DMT Context

The **DMT** leverages Redfish to provide unified management capabilities for Intel Active Management Technology (AMT) enabled devices. This integration offers several advantages:

1. Standardized Device Management -  Common API interface for all managed devices and simplified client integration for third-party tools
1. Enterprise Integration - Integration with existing enterprise management tools, Support for bulk operations across multiple devices, Standardized reporting and monitoring capabilities
1. Future-Proof Architecture - Built on industry standards that continue to evolve, Support for emerging hardware features through schema extensions, Compatibility with next-generation management tools.

## Redfish usage from DMT

The DMT Console exposes **Redfish APIs as additional endpoints** alongside the existing DMT REST APIs, effectively acting as a **Redfish Aggregator** for all Intel Active Management Technology (AMT) enabled devices under its management. This architecture provides a standardized Redfish interface while maintaining backward compatibility with existing DMT workflows.

### Device Onboarding Prerequisite

> **Important**: Before a device can be accessed via Redfish APIs, it **must be onboarded** to the DMT Console using existing DMT procedures as described in the [DMT Enterprise Device Activation, Addition](https://device-management-toolkit.github.io/docs/2.28/GetStarted/Enterprise/activateDevice/).

Once onboarded through DMT, the device becomes available via both:

- **DMT REST APIs** - Immediate availability for existing workflows
- **Redfish APIs** - Standards-compliant access through `/redfish/v1/` endpoints

Both API sets operate on the same underlying device infrastructure, with DMT serving as an intelligent **Redfish Aggregator** that:

- **Translates** between AMT device capabilities and Redfish data models
- **Aggregates** multiple AMT devices into a unified Redfish service root
- **Normalizes** device responses to conform to DMTF Redfish schemas
- **Proxies** Redfish requests to appropriate AMT devices
- **Maintains** consistent authentication and authorization across both API sets

### Redfish Workflow Sequence

The following sequence diagram illustrates the generic workflow for any Redfish API request through the DMT Console aggregator:

```mermaid
sequenceDiagram
    autonumber
    participant Client as Redfish Client
    participant DMT as DMT Console
    participant AMT1 as AMT Device 1
    participant AMT2 as AMT Device 2

    Note over DMT,AMT2: Prerequisites: Device Onboarding (must be completed first)

    Note over Client,AMT2: Generic Redfish API Operations (after onboarding)

    Client->>DMT: Redfish API Request
    DMT->>DMT: Authenticate and authorize request
    DMT->>DMT: Parse Redfish resource path

    alt Device-specific resources (Systems, Chassis, Managers)
        DMT->>DMT: Identify target devices
        DMT->>AMT1: Retrieve device data (WS-MAN Requests)
        DMT->>AMT2: Retrieve device data (WS-MAN Requests)
        AMT1-->>DMT: Device-specific response (WS-MAN Responses)
        AMT2-->>DMT: Device-specific response (WS-MAN Responses)
        DMT->>DMT: Transform device data to DMTF Redfish schema
        DMT->>DMT: Aggregate device collection
    else Service-level resources (SessionService, AccountService, EventService)
        DMT->>DMT: Process service-level operation
        DMT->>DMT: Generate service response from DMT configuration
        Note over DMT: No device interaction required
    end

    DMT->>DMT: Apply Redfish resource formatting
    DMT-->>Client: DMTF-compliant Redfish response
```

#### Device Mapping Example

When a device is onboarded to DMT, it becomes accessible via both API paradigms:

**DMT REST API Access:**

```bash
# Get device information via DMT API
GET /api/v1/devices/{device-uuid}
Authorization: Bearer {jwt-token}
```

**Redfish API Access:**

```bash
# Get same device via Redfish API
GET /redfish/v1/Systems/{device-uuid}
Authorization: Bearer {jwt-token} [TBD on how to support Authorization header]
```

Both endpoints provide access to the same AMT device, but with different data representations:

- **DMT API** returns DMT-specific JSON structure
- **Redfish API** returns DMTF-compliant ComputerSystem schema

#### Redfish Aggregation Workflow Example

The following sequence diagram illustrates how the DMT Console aggregates multiple onboarded AMT devices into a unified Redfish Systems collection:

```mermaid
sequenceDiagram
    autonumber
    participant Client as Redfish Client
    participant DMT as DMT Console
    participant AMT1 as AMT Device 1
    participant AMT2 as AMT Device 2

    Note over DMT,AMT2: Prerequisites: Device Onboarding (must be completed first)

    Note over Client,AMT2: Redfish API Operations (after onboarding)

    Client->>DMT: GET /redfish/v1/Systems/
    DMT->>DMT: Query onboarded devices
    DMT->>AMT1: Get system information (WS-MAN APIs)
    DMT->>AMT2: Get system information (WS-MAN APIs)
    AMT1-->>DMT: AMT-specific response (WS-MAN APIs)
    AMT2-->>DMT: AMT-specific response (WS-MAN APIs)

    DMT->>DMT: Transform to Redfish schema
    DMT->>DMT: Aggregate device collection
    DMT-->>Client: Redfish Systems Collection
```

### **Prerequisites Summary**

Before utilizing Redfish APIs for any AMT device:

✅ **Required Steps:**

1. Device must be **discoverable** on the network
2. Device must have **AMT enabled** and properly configured
3. Device must be **onboarded** through DMT's existing procedures
4. Device must be **healthy** and responsive in DMT console
5. User must have **valid authentication** to DMT console

This approach ensures that **all existing DMT investments and procedures remain valuable** while providing the additional benefit of standards-compliant Redfish access to the same managed infrastructure.

## Architecture and Design

### High-Level Architecture Overview

In the following diagram, we present the high-level architecture of the DMT Console's Redfish implementation.

```mermaid
flowchart LR
  subgraph Clients[Clients]
    UI[Sample Web UI]
    Scripts[DMT Automation Scripts]
    Redfish[Redfish Tools]
    RedfishSuites[Redfish Interop Suites]
  end
  APIGW[API Gateway]
  subgraph Backend[DMT Console]
    Router[HTTP / WS Router]
    Config[Configuration Management]
    Middleware[JWT / CORS Middleware]
    HTTPControllers[HTTP Controllers v1/v2]
    WSControllers[WebSocket Controllers]
    UseCases[Use Case Layer]
    Translator[WS-MAN Translator]
    Repositories[Repository Layer]
    Entities[Entity Layer]
    RedfishControllers[Redfish Controllers v1]
    RedfishTranslator[Redfish to WS-MAN Translator]

  end
  DB[(PostgreSQL / SQLite)]
  Migrations[[DB Migrations]]
  WSMan[WS-MAN]
  Device[Intel AMT Device]
  OIDC[OIDC Providers]


  UI --> APIGW
  Redfish --> APIGW
  RedfishSuites --> APIGW
  Scripts --> APIGW
  APIGW --> Router
  Router --> Middleware
  Router --> Config
  Middleware --> HTTPControllers
  Middleware --> WSControllers
  Middleware --> RedfishControllers
  HTTPControllers --> UseCases
  WSControllers --> UseCases
  UseCases --> Repositories --> DB
  UseCases --> Entities
  UseCases --> Translator --> WSMan --> Device
  RedfishControllers --> RedfishTranslator --> WSMan
  Migrations --> DB
  Middleware --> OIDC

  %% Styling for Redfish components
  classDef redfishComponents fill:#e1f5fe,stroke:#0277bd,stroke-width:3px,color:#000
  class RedfishControllers,RedfishTranslator redfishComponents
```

**Topics to be covered:**

- Component responsibilities and interfaces
- Integration points with existing DMT services
- Error handling and fault tolerance mechanisms

## Redfish Resource Mapping

The following table provides a comprehensive overview of each Redfish resource type implemented within the DMT Console. Each resource type links to documentation covering implementation specifics and capabilities.

| **Resource Type** | **Category** | **Description** |
|---|---|---|
| **[Systems](02-redfish-systems.md)** | Device-Specific | Represents computer systems with ComputerSystem schema mapping from AMT data, including processor, memory, storage, network, power, thermal, and boot management capabilities |
| **[Chassis](03-redfish-chassis.md)** | Device-Specific | Provides physical hardware inventory including chassis information, thermal sensors, power supplies, fan control, physical security, asset tracking, and environmental monitoring |
| **[Managers](04-redfish-managers.md)** | Device-Specific | Manages AMT controller capabilities including network services, certificate management, logging, virtual media, remote console access, and firmware update mechanisms |
| **[AccountService](05-redfish-accountservice.md)** | Service-Level | Handles user account management within DMT context, including RBAC implementation, password policies, security settings, and account lifecycle management |
| **[SessionService](06-redfish-sessionservice.md)** | Service-Level | Manages authentication sessions, JWT token handling, session timeouts, cleanup procedures, and concurrent session limitations |
| **[EventService](07-redfish-eventservice.md)** | Service-Level | Provides event handling capabilities including subscription management, filtering, routing, push notifications, and event history logging |
| **[UpdateService](08-redfish-updateservice.md)** | Service-Level | Coordinates firmware updates across devices with scheduling, orchestration, progress tracking, status reporting, and rollback mechanisms |

## Redfish Authentication Architecture

### Overview

The DMT Console implements a unified, simple authentication architecture using configuration-based credentials and JWT tokens for all client types, maintaining DMTF Redfish specification compliance.

### Unified Authentication Flow

```text
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Web Browsers  │    │   API Clients   │    │ Redfish Clients │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │ JWT Token (Authorization: Bearer)
                                 ▼
               ┌─────────────────────────────────────┐
               │         Authentication Layer        │
               │    ┌───────────────────────────┐    │
               │    │      JWT Validator        │    │
               │    │   (config.yml creds)      │    │
               │    └───────────────────────────┘    │
               └─────────────────────────────────────┘
                                 │
                                 ▼
               ┌─────────────────────────────────────┐
               │         Protected Resources         │
               │  • Web UI          • Redfish APIs   │
               │  • REST APIs       • Device Mgmt    │
               └─────────────────────────────────────┘
```

### Authentication Sequence Diagrams

#### 1. Current Login Flow (Configuration-Based Authentication)

```mermaid
sequenceDiagram
    participant Client as Web/API Client
    participant Gateway as Auth Gateway
    participant Config as Configuration
    participant JWT as JWT Service
    participant Protected as Protected Resource

    Client->>Gateway: 1. POST /api/v1/authorize {username, password}
    Gateway->>Config: 2. Check adminUsername/adminPassword
    Config-->>Gateway: 3. Credentials validated
    Gateway->>JWT: 4. Generate JWT token
    JWT-->>Gateway: 5. Signed JWT token
    Gateway-->>Client: 6. 200 OK {token: "eyJhbGc..."}

    Note over Client,Gateway: Subsequent authenticated requests
    Client->>Gateway: 7. GET /api/v1/devices (Authorization: Bearer eyJhbGc...)
    Gateway->>JWT: 8. Validate JWT token
    JWT-->>Gateway: 9. Token valid
    Gateway->>Protected: 10. Forward request
    Protected-->>Gateway: 11. Response
    Gateway-->>Client: 12. 200 OK {response}
```

**Current Implementation Notes:**

- No user database table exists
- Single admin user from configuration: `standalone/G@ppm0ym`
- Same JWT token used for both web UI and API access
- No session management - stateless JWT only

#### 2. Future Enhanced Authentication Flow (Design Goal)

```mermaid
sequenceDiagram
    participant Redfish as Redfish Client
    participant Console as DMT Console
    participant Config as Configuration
    participant JWT as JWT Service
    participant Systems as Systems Endpoint

    Note over Redfish,Console: DMTF Redfish compliant authentication

    Redfish->>Console: 1. POST /api/v1/authorize {username, password}
    Console->>Config: 2. Validate against adminUsername/adminPassword
    Config-->>Console: 3. Credentials valid
    Console->>JWT: 4. Generate JWT token
    JWT-->>Console: 5. Signed JWT token
    Console-->>Redfish: 6. 200 OK {token: "eyJhbGc..."}

    Note over Redfish,Console: Redfish API calls use same JWT
    Redfish->>Console: 7. GET /redfish/v1/Systems/1 (Authorization: Bearer eyJhbGc...)
    Console->>JWT: 8. Validate JWT token
    JWT-->>Console: 9. Token valid
    Console->>Systems: 10. Get system information
    Systems-->>Console: 11. DMTF-compliant JSON response
    Console-->>Redfish: 12. 200 OK {Redfish ComputerSystem}
```

**Key Benefits:**

- Same authentication endpoint for all clients
- DMTF Redfish specification compliance
- No separate authentication systems to maintain
- Universal JWT token works everywhere

#### 4. Unified Configuration Example

**config.yml:**

```yaml
auth:
  disabled: false
  adminUsername: admin
  adminPassword: your-secure-password
  jwtKey: your-256-bit-secret-key
  jwtExpiration: 24h0m0s
```

**Environment Variables (Production):**

```bash
AUTH_ADMIN_USERNAME=admin
AUTH_ADMIN_PASSWORD=your-secure-password
AUTH_JWT_KEY=your-256-bit-secret-key
AUTH_JWT_EXPIRATION=24h
```

### 5. Client Examples

**Web UI Login:**

```javascript
// Same endpoint for all clients
const response = await fetch('/api/v1/authorize', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    username: 'admin',
    password: 'your-secure-password'
  })
});
const { token } = await response.json();

// Use token for all subsequent requests
localStorage.setItem('jwt_token', token);
```

**Redfish Client:**

```bash
# Get JWT token (same endpoint)
TOKEN=$(curl -s -X POST /api/v1/authorize \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-secure-password"}' \
  | jq -r '.token')

# Use token for Redfish API calls
curl -H "Authorization: Bearer $TOKEN" \
  /redfish/v1/Systems/1
```

**API Client:**

```python
import requests

# Authenticate once
auth_response = requests.post('/api/v1/authorize', json={
    'username': 'admin',
    'password': 'your-secure-password'
})
token = auth_response.json()['token']

# Use for all API calls
headers = {'Authorization': f'Bearer {token}'}
devices = requests.get('/api/v1/devices', headers=headers)

```

```mermaid
sequenceDiagram
    API2->>Gateway: Login via JWT
    Gateway->>JWT: Generate token B

    Note over WebUI,Gateway: All clients use same JWT authentication

    par Concurrent requests with JWT tokens
        WebUI->>Gateway: JWT-based request (Bearer token)
        and API1->>Gateway: JWT-based request (Bearer token A)
        and API2->>Gateway: JWT-based request (Bearer token B)
    end

    Gateway-->>WebUI: Response with JWT context
    Gateway-->>API1: Response with JWT context A
    Gateway-->>API2: Response with JWT context B
```

#### 7. Simplified Authentication Gateway Components

```mermaid
graph TB
    subgraph "Client Layer"
        WEB[Web Browser]
        API[API Clients]
        REDFISH[Redfish Clients]
    end

    subgraph "Authentication Gateway"
        ROUTER[Router/Middleware]
        JWT_MW[JWT Middleware]

        subgraph "Authentication Services"
            JWT_SVC[JWT Service]
            CONFIG[Configuration Validator]
        end

        subgraph "Configuration"
            CONFIG_FILE[config.yml<br/>adminUsername<br/>adminPassword<br/>jwtKey]
        end
    end

    subgraph "Protected Services"
        REDFISH_API[Redfish APIs]
        DEVICE_API[Device Management APIs]
        WEB_UI[Web UI Endpoints]
    end

    %% All clients use Bearer tokens
    WEB -->|JWT Bearer Token| ROUTER
    API -->|JWT Bearer Token| ROUTER
    REDFISH -->|JWT Bearer Token| ROUTER

    %% Router to JWT middleware only
    ROUTER --> JWT_MW

    %% Middleware to services
    JWT_MW --> JWT_SVC
    JWT_SVC --> CONFIG
    CONFIG --> CONFIG_FILE

    %% Authentication flow to protected services
    JWT_MW --> WEB_UI
    JWT_MW --> REDFISH_API
    JWT_MW --> DEVICE_API
```

#### 8. Authentication with In-Memory Rate Limiting

```mermaid
sequenceDiagram
    participant Client as Client
    participant Gateway as Auth Gateway
    participant RateLimit as Rate Limiter (In-Memory)
    participant Config as Configuration
    participant JWT as JWT Service

    Client->>Gateway: 1. POST /api/v1/authorize {username, password}
    Gateway->>RateLimit: 2. Check rate limit for client IP

    alt Rate limit exceeded
        RateLimit-->>Gateway: 3a. Rate limit exceeded
        Gateway-->>Client: 4a. 429 Too Many Requests
    else Within rate limit
        RateLimit-->>Gateway: 3b. Request allowed
        Gateway->>Config: 4b. Validate credentials

        alt Invalid credentials
            Config-->>Gateway: 5a. Invalid credentials
            Gateway->>RateLimit: 6a. Record failed attempt
            Gateway-->>Client: 7a. 401 Unauthorized (DMTF compliant)
        else Valid credentials
            Config-->>Gateway: 5b. Credentials valid
            Gateway->>JWT: 6b. Generate JWT token
            JWT-->>Gateway: 7b. Signed JWT token
            Gateway->>RateLimit: 8b. Reset failure count
            Gateway-->>Client: 9b. 200 OK {token}
        end
    end
```

### Security Considerations

#### Simple Yet Secure

The unified JWT approach maintains strong security with minimal complexity:

##### Token Security

- **Secret Management**: Store JWT signing key in environment variables
- **Secure Transmission**: Always use HTTPS/TLS in production
- **Token Expiration**: Configurable expiration times (default: 24 hours)
- **Secure Storage**: Web UI stores tokens in secure, HTTP-only cookies

##### Configuration Security

- **Password Hashing**: Admin password properly hashed with bcrypt
- **Environment Variables**: Sensitive config never committed to code
- **Key Rotation**: JWT signing key can be rotated
- **Minimal Attack Surface**: No user management endpoints to secure

##### Rate Limiting Security

- **Brute Force Protection**: In-memory tracking prevents password attacks
- **No External Dependencies**: No Redis or database needed for rate limiting
- **Automatic Cleanup**: Expired rate limit entries automatically removed
- **IP-Based Blocking**: Prevents attacks from specific source addresses
- **Configurable Thresholds**: Adjustable limits for different security needs
- **DMTF Compliant Responses**: Returns proper HTTP 429 with Redfish error format
