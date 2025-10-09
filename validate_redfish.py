#!/usr/bin/env python3

"""
Simple Redfish validation script that tests our ComputerSystem.Reset implementation
"""

import requests
import json
import sys

# Configuration
BASE_URL = "http://localhost:8182/api/redfish/v1"
USERNAME = "standalone"
PASSWORD = "G@ppm0ym"

def get_jwt_token():
    """Get JWT token for authentication"""
    login_url = "http://localhost:8182/api/v1/authorize"
    login_data = {
        "username": USERNAME,
        "password": PASSWORD
    }
    
    response = requests.post(login_url, json=login_data)
    if response.status_code == 200:
        return response.json()["token"]
    else:
        print(f"Failed to get JWT token: {response.status_code}")
        sys.exit(1)

def make_request(path, method="GET", data=None):
    """Make authenticated request to Redfish endpoint"""
    token = get_jwt_token()
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    url = f"{BASE_URL}{path}"
    
    if method == "GET":
        response = requests.get(url, headers=headers)
    elif method == "POST":
        response = requests.post(url, headers=headers, json=data)
    else:
        raise ValueError(f"Unsupported method: {method}")
    
    return response

def validate_redfish_compliance():
    """Validate Redfish compliance for our implementation"""
    print("🔍 Redfish Service Validation for ComputerSystem.Reset")
    print("=" * 60)
    
    # Test 1: Service Root
    print("\n1. Testing Service Root...")
    response = make_request("/")
    if response.status_code == 200:
        data = response.json()
        print(f"   ✅ Service Root: {data.get('RedfishVersion', 'Unknown')}")
        print(f"   ✅ Systems URI: {data.get('Systems', {}).get('@odata.id', 'Not found')}")
    else:
        print(f"   ❌ Service Root failed: {response.status_code}")
        return
    
    # Test 2: Systems Collection
    print("\n2. Testing Systems Collection...")
    response = make_request("/Systems")
    if response.status_code == 200:
        data = response.json()
        systems = data.get("Members", [])
        print(f"   ✅ Systems Collection: {len(systems)} systems found")
        
        if systems:
            system_id = systems[0]["@odata.id"].split("/")[-1]
            print(f"   ✅ First System ID: {system_id}")
            
            # Test 3: Individual System
            print(f"\n3. Testing Individual System ({system_id})...")
            response = make_request(f"/Systems/{system_id}")
            if response.status_code == 200:
                data = response.json()
                actions = data.get("Actions", {})
                reset_action = actions.get("#ComputerSystem.Reset", {})
                
                if reset_action:
                    print("   ✅ ComputerSystem.Reset action found")
                    print(f"   ✅ Target URI: {reset_action.get('target', 'Not found')}")
                    
                    allowed_values = reset_action.get("ResetType@Redfish.AllowableValues", [])
                    print(f"   ✅ Allowed Reset Types: {', '.join(allowed_values)}")
                    
                    # Test 4: Reset Action
                    if "ForceRestart" in allowed_values:
                        print(f"\n4. Testing ComputerSystem.Reset Action...")
                        reset_data = {"ResetType": "ForceRestart"}
                        reset_response = make_request(f"/Systems/{system_id}/Actions/ComputerSystem.Reset", 
                                                    method="POST", data=reset_data)
                        
                        if reset_response.status_code == 200:
                            task_data = reset_response.json()
                            print("   ✅ Reset action successful")
                            print(f"   ✅ Task State: {task_data.get('TaskState', 'Unknown')}")
                            print(f"   ✅ Task Status: {task_data.get('TaskStatus', 'Unknown')}")
                            
                            messages = task_data.get("Messages", [])
                            for msg in messages:
                                print(f"   ✅ Message: {msg.get('Message', 'No message')}")
                        else:
                            print(f"   ❌ Reset action failed: {reset_response.status_code}")
                    
                    # Test 5: Error Handling
                    print(f"\n5. Testing Error Handling...")
                    
                    # Test invalid reset type
                    invalid_data = {"ResetType": "InvalidType"}
                    error_response = make_request(f"/Systems/{system_id}/Actions/ComputerSystem.Reset", 
                                                method="POST", data=invalid_data)
                    
                    if error_response.status_code == 400:
                        print("   ✅ Invalid reset type properly rejected (400)")
                        error_data = error_response.json()
                        if "error" in error_data:
                            print(f"   ✅ Error message: {error_data['error'].get('message', 'No message')}")
                    else:
                        print(f"   ❌ Invalid reset type not properly handled: {error_response.status_code}")
                    
                    # Test 405 Method Not Allowed
                    method_response = make_request(f"/Systems/{system_id}/Actions/ComputerSystem.Reset", method="GET")
                    if method_response.status_code == 405:
                        print("   ✅ GET method properly rejected (405)")
                    else:
                        print(f"   ❌ GET method not properly handled: {method_response.status_code}")
                    
                else:
                    print("   ❌ ComputerSystem.Reset action not found")
            else:
                print(f"   ❌ Individual system failed: {response.status_code}")
        else:
            print("   ❌ No systems found in collection")
    else:
        print(f"   ❌ Systems Collection failed: {response.status_code}")
    
    print(f"\n🎉 Redfish Validation Complete!")
    print("=" * 60)

if __name__ == "__main__":
    try:
        validate_redfish_compliance()
    except Exception as e:
        print(f"❌ Validation failed with error: {e}")
        sys.exit(1)
