# Documentation

This directory contains reference documentation and specifications for the RGS (Remote Gaming Server) project.

## Contents

| Document | Description |
|----------|-------------|
| [TECHNICAL_SPECIFICATION.md](TECHNICAL_SPECIFICATION.md) | RGS implementation specification based on GLI-19 |
| [GLI-19-Interactive-Gaming-Systems-v3.0.pdf](GLI-19-Interactive-Gaming-Systems-v3.0.pdf) | Official GLI-19 standard (source document) |

---

## RGS Technical Specification

**File:** `TECHNICAL_SPECIFICATION.md`

The technical specification provides a complete implementation guide for building a GLI-19 compliant Remote Gaming Server. It covers:

- **System Architecture** - Component design, service breakdown, technology stack
- **Platform Requirements** - Clock sync, integrity verification, gaming management
- **Player Account Management** - Registration, authentication, sessions, limits
- **Random Number Generator** - Cryptographic RNG implementation, statistical testing
- **Game Engine** - Sessions, cycles, outcomes, recalls, interrupted games
- **Financial Transactions** - Wallet service, deposits, withdrawals, transaction logging
- **Data Management** - Required data fields, retention, audit logging
- **Security Controls** - Encryption, access control, password policies
- **Communications** - API protocols, connection security, error handling
- **Reporting** - Required reports, formats, scheduling
- **API Specification** - REST endpoints, WebSocket protocol

This specification maps directly to GLI-19 section references for traceability.

---

## GLI-19: Standards for Interactive Gaming Systems

**File:** `GLI-19-Interactive-Gaming-Systems-v3.0.pdf`

**Version:** 3.0  
**Release Date:** July 17, 2020  
**Publisher:** Gaming Laboratories International (GLI)

### Overview

GLI-19 provides regulators, suppliers, and operators with clarity and best practices for interactive gaming systems. This standard covers requirements for:

- **System Architecture** - Design and security requirements for interactive gaming platforms
- **Player Account Management** - Registration, authentication, and account security
- **Financial Transactions** - Payment processing, deposits, withdrawals, and reconciliation
- **Game Software** - RNG requirements, game fairness, and software integrity
- **Security Controls** - Access controls, encryption, and data protection
- **Communications** - Network security and protocol requirements
- **Logging and Reporting** - Audit trails, event logging, and regulatory reporting
- **Responsible Gaming** - Player protection features and self-exclusion mechanisms

### Source

Downloaded from [Gaming Laboratories International](https://gaminglabs.com/wp-content/uploads/2020/07/GLI-19-Interactive-Gaming-Systems-v3.0.pdf)

### Usage

This specification serves as a compliance reference for implementing the Remote Gaming Server. Key sections should be reviewed when implementing:

- Player authentication and session management
- Game outcome generation (RNG)
- Financial transaction processing
- Audit logging and reporting
- Security controls and encryption

