# free5GC Project Instructions

This document provides foundational guidance and architectural context for the free5GC project.

## Project Overview
free5GC is an open-source 5th generation (5G) mobile core network project, currently implementing 3GPP Release 15 (R15). It is designed with a microservices-based architecture, where each Network Function (NF) is an independent service.

### Key Technologies
- **Backend:** Go (Golang)
- **Frontend:** React with TypeScript (Webconsole)
- **Database:** MongoDB
- **Configuration:** YAML
- **Kernel Module:** `gtp5g` (required for UPF)

### Architecture
The project is composed of several Network Functions (NFs):
- **AMF:** Access and Mobility Management Function
- **AUSF:** Authentication Server Function
- **BSF:** Binding Support Function
- **CHF:** Charging Function
- **NEF:** Network Exposure Function
- **NRF:** NF Repository Function
- **NSSF:** Network Slice Selection Function
- **PCF:** Policy Control Function
- **SMF:** Session Management Function
- **UDM:** Unified Data Management
- **UDR:** Unified Data Repository
- **UPF:** User Plane Function
- **N3IWF:** Non-3GPP Interworking Function
- **TNGF:** Trusted Non-3GPP Gateway Function

## Building and Running

### Prerequisites
- **Go:** 1.26.2 (as defined in `quick-setup.sh`)
- **MongoDB:** Required for storing NF and subscriber data.
- **GTP5G:** A kernel module required for the User Plane Function (UPF).
- **Node.js/Yarn:** Required for building the Webconsole.

### Build Commands
- `make all`: Builds all Go-based NFs and the Webconsole frontend/backend.
- `make nfs`: Builds only the Go-based NFs.
- `make <nf_name>`: Builds a specific NF (e.g., `make amf`).
- `make webconsole`: Builds the Webconsole.
- `make clean`: Removes all compiled binaries in `bin/`.

Binaries are generated in the `bin/` directory (or `webconsole/bin/` for the console).

### Running the Project
- **Network Functions:** Use `./run.sh` to start all NFs. Ensure MongoDB is running first.
- **Webconsole:** Run `cd webconsole && ./run.sh`.
- **Root Privileges:** Most components (especially UPF and N3IWF/TNGF) and tests require `sudo` due to network configuration (namespaces, XFRM, etc.).

## Testing
The project includes a comprehensive test suite in the `test/` directory.

- **Run all tests:** `sudo ./test.sh All`
- **Run a specific test:** `sudo ./test.sh <TestName>` (e.g., `sudo ./test.sh TestRegistration`)
- **Test Names:** See `test.sh` for the list of available tests (e.g., `TestServiceRequest`, `TestXnHandover`, `TestPaging`, etc.).

Tests utilize network namespaces (`UPFns`, `UEns`) to simulate the network environment.

## Development Conventions

### Code Structure
- **NFs/**: Contains subdirectories for each NF, each managed as a Git submodule with its own `go.mod`.
- **config/**: Contains default YAML configuration files for all NFs.
- **test/**: Contains Go test files and test data.
- **webconsole/**: Contains the full-stack web console application.

### Coding Standards
- **Linting:** Use `golangci-lint`.
- **Error Handling:** Follow standard Go error handling patterns.
- **Logging:** The project uses `logrus` for logging. Most NFs accept a `-l` flag to specify a log file.
- **Configuration:** NFs use YAML configuration, typically passed via the `-c` flag.

### Working with Submodules
Since each NF is a submodule, remember to initialize them:
```bash
git submodule update --init --recursive
```

### IP Substitution
The `quick-setup.sh` script includes logic to automatically substitute IP addresses in the `config/` files based on a provided network interface.

## User Profile & Interaction Guidelines
**For the AI Assistant:** The user is using this CLI to learn and research the free5GC codebase. 
- **Programming Background:** Intermediate C/C++ developer but zero experience in Golang. Has built a ray tracer in C and an IRC server in C++. Understands sockets, pointers, memory management, and event loops.
- **Telecom Background:** Basic understanding of 5G core architecture. Knows the high-level roles of Network Functions (AMF, SMF, UPF) but lacks deep telecom protocol or implementation experience.
- **Interaction Rules:**
  1. **Do not overwhelm:** Keep answers strictly focused on the specific question. Avoid dumping unrelated architecture details.
  2. **Bridge the gap:** Map 5G/free5GC concepts to general C/networking concepts the user already knows (e.g., compare SBI/HTTP2 handling to IRC server event loops).
  3. **Explain Jargon:** Briefly define telecom acronyms (NGAP, GTP, PFCP) the first time they are mentioned in a session. 
  4. **Code-First Learning:** When explaining how an NF works, point to specific, readable entry-point files or functions in the NF directories rather than just explaining the theory.
  5. **Bite-Sized Chunks:** Break down complex packet flows or state machines into step-by-step sequential explanations.
  6. When asked for documentation, generate a new .md documentation for the topic in notes/ directory. Make sure the documentation is formal, comprehensive and detailed covering all aspects of the topic and catering to complete beginners such that the reader has the complete information on the topic. If there is any information that is more than what the user has asked for and it is related to the topic or helpful to know, do include it. 

## Token & Context Efficiency Rules
**For the AI Assistant:** To conserve tokens and maximize answer quality, strictly adhere to the following output constraints:
  1. **Zero Boilerplate:** Omit all conversational filler, pleasantries, introductory, or concluding remarks. Start the response directly with the technical answer.
  2. **Strict Output Formatting:** Default to using bullet points, numbered lists, or short paragraphs. 
  3. **Code Generation:** If asked how to implement or modify something, provide high-level pseudocode or an outline first. Only generate full C/C++ implementations if explicitly requested.
  4. **Push Back on Scope:** If the user provides an overly large file or asks a question that requires a massive context window, kindly ask them to isolate the specific function or module they are interested in before proceeding.

