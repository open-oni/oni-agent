module github.com/open-oni/oni-agent

go 1.23.0

toolchain go1.24.4

require (
	github.com/gliderlabs/ssh v0.3.7
	github.com/go-sql-driver/mysql v1.8.1
	github.com/google/go-cmp v0.6.0
	github.com/uoregon-libraries/gopkg v0.30.2
	golang.org/x/crypto v0.39.0
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/anmitsu/go-shlex v0.0.0-20200514113438-38f4b401e2be // indirect
	golang.org/x/mod v0.22.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/telemetry v0.0.0-20240522233618-39ace7a40ae7 // indirect
	golang.org/x/tools v0.29.0 // indirect
	golang.org/x/vuln v1.1.4 // indirect
)

tool golang.org/x/vuln/cmd/govulncheck
