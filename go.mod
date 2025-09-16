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
	codeberg.org/chavacava/garif v0.2.0 // indirect
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/BurntSushi/toml v1.5.0 // indirect
	github.com/anmitsu/go-shlex v0.0.0-20200514113438-38f4b401e2be // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/fatih/structtag v1.2.0 // indirect
	github.com/hashicorp/go-version v1.7.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mgechev/dots v1.0.0 // indirect
	github.com/mgechev/revive v1.12.0 // indirect
	github.com/spf13/afero v1.14.0 // indirect
	golang.org/x/mod v0.27.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/telemetry v0.0.0-20250807160809-1a19826ec488 // indirect
	golang.org/x/text v0.28.0 // indirect
	golang.org/x/tools v0.36.0 // indirect
	golang.org/x/vuln v1.1.4 // indirect
)

tool (
	github.com/mgechev/revive
	golang.org/x/vuln/cmd/govulncheck
)
