# AGENTS.md - free5GC Workspace Rules and Guidelines

Welcome to the free5GC development workspace. This document serves as a guide for agentic developer interactions, directory layouts, and conventions for maximum token efficiency and performance.

---

## 1. Architectural Overview
free5GC is an open-source 5G Mobile Core Network implementing 3GPP Release 15 (R15). It uses a Service-Based Architecture (SBA) composed of modular Network Functions (NFs):

### Control Plane Network Functions (Go backend)
- **AMF** ([NFs/amf](file:///home/mummadisetty/dfki/free5gc/NFs/amf)): Access and Mobility Management Function. Handles connection and mobility management (NAS signaling, security, and registration).
- **SMF** ([NFs/smf](file:///home/mummadisetty/dfki/free5gc/NFs/smf)): Session Management Function. Establishes, modifies, and releases sessions; controls IP address allocation.
- **NRF** ([NFs/nrf](file:///home/mummadisetty/dfki/free5gc/NFs/nrf)): NF Repository Function. Supports service discovery and status monitoring.
- **UDM** ([NFs/udm](file:///home/mummadisetty/dfki/free5gc/NFs/udm)) & **UDR** ([NFs/udr](file:///home/mummadisetty/dfki/free5gc/NFs/udr)): Unified Data Management and Unified Data Repository. Manages user subscriptions and credential generation.
- **AUSF** ([NFs/ausf](file:///home/mummadisetty/dfki/free5gc/NFs/ausf)): Authentication Server Function.
- **PCF** ([NFs/pcf](file:///home/mummadisetty/dfki/free5gc/NFs/pcf)): Policy Control Function.
- **NSSF** ([NFs/nssf](file:///home/mummadisetty/dfki/free5gc/NFs/nssf)): Network Slice Selection Function.
- **BSF** ([NFs/bsf](file:///home/mummadisetty/dfki/free5gc/NFs/bsf)): Binding Support Function.
- **CHF** ([NFs/chf](file:///home/mummadisetty/dfki/free5gc/NFs/chf)): Charging Function.
- **NEF** ([NFs/nef](file:///home/mummadisetty/dfki/free5gc/NFs/nef)): Network Exposure Function.

### User Plane Network Functions (C/Go)
- **UPF** ([NFs/upf](file:///home/mummadisetty/dfki/free5gc/NFs/upf)): User Plane Function. Forwards user packets between the RAN and the Data Network (DN). Requires the `gtp5g` Linux kernel module.

### Non-3GPP Gateways
- **N3IWF** ([NFs/n3iwf](file:///home/mummadisetty/dfki/free5gc/NFs/n3iwf)): Non-3GPP Interworking Function.
- **TNGF** ([NFs/tngf](file:///home/mummadisetty/dfki/free5gc/NFs/tngf)): Trusted Non-3GPP Gateway Function.

### Frontend and Database
- **Webconsole** ([webconsole/](file:///home/mummadisetty/dfki/free5gc/webconsole/)): Built using React + TypeScript for UI, and a Go backend.
- **Database**: MongoDB is used across all NFs to persist subscriber data and configuration.

---

## 2. Core File Paths
- **[NFs/](file:///home/mummadisetty/dfki/free5gc/NFs/)**: Git submodules containing code for each Network Function.
- **[config/](file:///home/mummadisetty/dfki/free5gc/config/)**: Default YAML configurations for all NFs.
- **[test/](file:///home/mummadisetty/dfki/free5gc/test/)**: Integration test suite simulating NFs using network namespaces.
- **[webconsole/](file:///home/mummadisetty/dfki/free5gc/webconsole/)**: Directory containing the Webconsole panel interface.
- **[Makefile](file:///home/mummadisetty/dfki/free5gc/Makefile)**: Targets to build binaries (`make all`, `make nfs`, `make <nf>`, `make webconsole`).
- **[run.sh](file:///home/mummadisetty/dfki/free5gc/run.sh)**: Script to run all control plane NFs.
- **[test.sh](file:///home/mummadisetty/dfki/free5gc/test.sh)**: Entry-point for running specific or all integration tests.
- **[quick-setup.sh](file:///home/mummadisetty/dfki/free5gc/quick-setup.sh)**: Setup script for database initialization and configurations.

---

## 3. Coding Guidelines
- **Golang Conventions**: Use standard Go patterns (Go 1.26+), strict type checking, and default to `golangci-lint` clean code.
- **Error Handling**: Explicit and detailed error propagation. Never ignore errors.
- **Logging**: Use the standard `logrus` logger pattern configured for individual NFs.
- **Git Submodule Workflows**: Remember to run `git submodule update --init --recursive` when pulling changes. Never modify submodule code without committing inside the submodule.
- **Network Namespace Operations**: Upf, testing, and gateway components configure network namespaces. Command runs or tests containing network setup require sudo/root privileges.

---

## 4. Token & Context Efficiency Instructions for Gemini

To maximize reasoning quality and minimize token usage, the AI assistant must abide by the following:

- **Zero Conversational Filler**: Do not output greeting messages, standard introductions, conversational pleasantries, or summarizations of actions. Jump straight into the code or response.
- **Push Back on Scope**: If requested to read large directories or massive logs, prioritize target files first or ask the user to specify line ranges/specific directories (e.g., [amf/](file:///home/mummadisetty/dfki/free5gc/NFs/amf/)).
- **Bite-Sized Explanations**: Map telecom domain concepts (e.g., NGAP, PFCP, GTP) to systems/network programming primitives (e.g., sockets, state machines, packet parsers). Break explanations into clear numbered lists.
- **Clickable File Links**: Generate markdown links using standard format for all files and symbol definitions (e.g., [config/amfcfg.yaml](file:///home/mummadisetty/dfki/free5gc/config/amfcfg.yaml)).
