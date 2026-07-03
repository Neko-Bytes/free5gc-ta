# Subscriber Information Stored by UDM/UDR

In a 5G Core network, the **UDM (Unified Data Management)** is the functional equivalent of the HSS in LTE. However, following the Service-Based Architecture (SBA) principles, the UDM itself is "stateless" regarding permanent storage. It handles the logic for managing subscriber data, while the actual persistent storage is delegated to the **UDR (Unified Data Repository)**.

When we talk about "subscriber information stored by UDM," we are referring to the data models that UDM manages via the `Nudr` interface and serves via the `Nudm` interface.

## 1. Identifiers (Identity Data)
These are the primary keys used to locate a subscriber in the database.
*   **SUPI (Subscription Permanent Identifier):** The globally unique identifier for a subscriber (e.g., IMSI-based `imsi-208930000000001`).
*   **GPSI (Generic Public Subscription Identifier):** The public identifier used outside the 5G system, such as an MSISDN (phone number) or an external ID. UDM handles the translation between GPSI and SUPI.

## 2. Authentication Subscription Data
This data is used by the AUSF (Authentication Server Function) via the UDM to verify the UE's identity during the primary authentication procedure.
*   **Authentication Method:** (e.g., 5G AKA, EAP-AKA').
*   **Security Credentials:**
    *   **K (Permanent Key):** The 128-bit or 256-bit root key shared between the UE and the network.
    *   **OP / OPc (Operator Code):** A parameter used to derive individual keys.
    *   **SQN (Sequence Number):** Used to prevent replay attacks.
*   **Authentication Management Field (AMF):** Provides configuration for the authentication process.

## 3. Access and Mobility Subscription Data (AM Data)
This data controls how and where a user can connect to the network. It is primarily used by the AMF.
*   **Subscribed UE-AMBR:** (Aggregate Maximum Bit Rate) The maximum allowed upload/download speed for all non-GBR (Guaranteed Bit Rate) sessions of the UE.
*   **Allowed NSSAI:** The list of **S-NSSAIs** (Single Network Slice Selection Assistance Information) that the user is permitted to use.
*   **RAT Restrictions:** Restrictions on the type of Radio Access Technology (e.g., 5G NR, LTE) the user can access.
*   **Forbidden Areas:** List of tracking areas where the user is not allowed to roam or access services.

## 4. Session Management Subscription Data (SM Data)
This data is used by the SMF (Session Management Function) to establish PDU (Protocol Data Unit) sessions. It is organized per Slice (S-NSSAI) and Data Network Name (DNN).
*   **DNN Configuration:**
    *   **PDU Session Types:** (IPv4, IPv6, IPv4v6, Ethernet, Unstructured).
    *   **Session-AMBR:** The maximum bit rate for a specific PDU session.
    *   **QoS Profile:** Default 5QI (5G QoS Identifier) and ARP (Allocation and Retention Priority) for the session.
    *   **Static IP Address:** (Optional) If the user is assigned a permanent IP.

## 5. SMF Selection Subscription Data
Determines which SMF should handle a user's session based on their location and requested service.
*   **Subscribed SMF list:** A mapping of (S-NSSAI, DNN) to a specific SMF instance or group.

## 6. UE Context in SMF Data
While the above are "subscription" data (static), UDM also tracks some "context" data (dynamic).
*   **Active SMF Info:** The ID of the SMF currently serving an active PDU session for the UE. This allows the UDM to inform other NFs where the user's sessions are anchored.

---

### Implementation Reference in free5gc
In the `free5gc` codebase, these structures are defined in the `openapi/models` and used within UDM's internal context:
- **File:** `NFs/udm/internal/context/context.go`
- **Struct:** `UdmUeContext` tracks these fields for active UEs.
- **Service Handler:** `NFs/udm/internal/sbi/processor/subscriber_data_management.go` contains the logic for fetching this data from UDR.

### Jargon Summary 
- **SBI (Service Based Interface):** The HTTP/2 based communication method between 5G NFs.
- **DNN (Data Network Name):** Equivalent to APN in LTE; it identifies the external network (e.g., "internet").
- **NSSAI / S-NSSAI:** The "Slice" ID. 5G allows virtualizing the network into slices for different use cases (e.g., IoT vs. Mobile Broadband).
- **PLMN (Public Land Mobile Network):** The network operator ID (MCC + MNC).
